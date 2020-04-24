package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go-testbench/auth"
	"go-testbench/dockerapi"
	"go-testbench/helpers"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type metadata struct {
	Details []metadataJSON `json:"data"`
}
type metadataJSON struct {
	ID              string        `json:"id"`
	Backend         []backendJSON `json:"backend"`
	VersionCall     string        `json:"versionCall"`
	VersionSpec     string        `json:"versionSpec"`
	URL             string        `json:"url"`
	HealthcheckCall string        `json:"healthcheckCall"`
	HealthExpResp   string        `json:"healthExpResp"`
	ImageSrc        string        `json:"imageSrc"`
	Title           string        `json:"title"`
	Content         string        `json:"content"`
	Platform        bool          `json:"platform"`
	PlatformHcCall  string        `json:"platformHcCall"`
	HealthPing      string        `json:"healthPing"`
	StartCmd        []string      `json:"startCmd"`
	StopCmd         []string      `json:"stopCmd"`
}

type actionJSON struct {
	First  []string `json:"first"`
	Second []string `json:"second"`
}

type backendJSON struct {
	Service string `json:"service"`
	Port    string `json:"port"`
	Jfid    string `json:"jfid"`
	Health  string `json:"health"`
}

type platformStruct struct {
	Router struct {
		NodeID  string `json:"node_id"`
		State   string `json:"state"`
		Message string `json:"message"`
	}
	Services []platformServices `json:"services"`
}

type platformServices struct {
	ServiceID string `json:"service_id"`
	NodeID    string `json:"node_id"`
	State     string `json:"state"`
	Message   string `json:"message"`
}

func main() {

	user, err := user.Current()
	auth.CheckErr(err)

	configFolder := "/.lorenygo/testBench/"
	configFile := "data.json"

	var healthcheckHostVar, portVar, dataFileVar, websocketHostVar string
	flag.StringVar(&portVar, "port", "8080", "Port")
	flag.StringVar(&healthcheckHostVar, "healthcheckhost", "loren.jfrog.team", "healthcheck Host")
	flag.StringVar(&websocketHostVar, "websockethost", "loren.jfrog.team", "websocket Host")
	flag.StringVar(&dataFileVar, "data", user.HomeDir+configFolder+configFile, "Path to JSON file")
	flag.Parse()

	if strings.Contains(dataFileVar, configFolder) {
		log.Println("Checking existence of config folder")
		if _, err := os.Stat(user.HomeDir + configFolder); os.IsNotExist(err) {
			log.Println("No config folder found")
			err = os.MkdirAll(user.HomeDir+configFolder, 0700)
			helpers.Check(err, true, "Generating "+user.HomeDir+configFolder+" directory")
		}
	}

	file, err := os.Open(dataFileVar)
	helpers.Check(err, true, "data JSON read")
	msg, _ := ioutil.ReadAll(file)
	var mp metadata
	// Decode JSON into our map
	json.Unmarshal([]byte(msg), &mp)

	router := gin.Default()
	router.LoadHTMLGlob(os.Getenv("GOPATH") + "/src/go-testbench/templates/*")
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})
	router.GET("/containers", func(c *gin.Context) {
		containers := dockerapi.ListRunningContainers()
		c.JSON(200, gin.H{
			"containers": containers,
		})
	})

	router.POST("/actionable", func(c *gin.Context) {

		var action actionJSON
		c.BindJSON(&action)

		for i := range action.First {
			fmt.Println("stop or start", action.First[i])
			bashCommandWrapper(action.First[i])
		}
		if len(action.Second) > 0 {
			for i := range action.Second {
				fmt.Println("restart", action.Second[i])
				bashCommandWrapper(action.Second[i])
			}
		}

		c.JSON(http.StatusOK, gin.H{"response": c.PostForm("stuff")})
	})

	// backend listing for docker containers
	var containersList []map[string]interface{}
	containerRaw, _ := json.Marshal(dockerapi.ListRunningContainers())
	err2 := json.Unmarshal([]byte(containerRaw), &containersList)
	if err2 != nil {
		panic(err)
	}

	check := read(mp, msg, healthcheckHostVar+":")
	var checkPtr *metadata = &check

	//websocket
	//var clients = make(map[*websocket.Conn]bool) // connected clients
	//var broadcast = make(chan check.Details)     //broadcast channel

	router.GET("/", func(c *gin.Context) {
		//check := read(mp, msg, healthcheckHost)
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"title": "lorenTestbench", "art_data": check.Details, "containers": containersList, "websocket": websocketHostVar + ":" + portVar})
	})

	router.GET("/ws", func(c *gin.Context) {
		wshandler(c.Writer, c.Request, msg, websocketHostVar+":", checkPtr)
	})
	router.Run("0.0.0.0:" + portVar) // listen and serve on 0.0.0.0:8080
}

func bashCommandWrapper(cmdString string) string {
	command := strings.Split(cmdString, " ")
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		fmt.Println(err)
	}
	err = cmd.Wait()
	if err != nil {
		fmt.Println(err)
		return "error"
	}
	return "OK"
}

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func wshandler(w http.ResponseWriter, r *http.Request, msg []byte, healthcheckHost string, initCheck *metadata) {
	conn, err := wsupgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Failed to upgrade ws: %+v", err)
		return
	}
	defer conn.Close()
	//return init state on first call
	initCheckJSON, _ := json.Marshal(*initCheck)
	conn.WriteMessage(websocket.TextMessage, initCheckJSON)
	t, msg2, errRead := conn.ReadMessage()
	for {
		//keepalive
		err := conn.WriteMessage(websocket.PingMessage, []byte("keepalive"))
		if err != nil {
			return
		}
		if errRead != nil {
			break
		}
		fmt.Println(t, string(msg2))

		//need to store previous send, and if there is a change, then write to socket
		var mp2 metadata
		// Decode JSON into our map
		json.Unmarshal([]byte(msg), &mp2)
		check := read(mp2, msg, healthcheckHost)
		if reflect.DeepEqual([]byte(fmt.Sprintf("%v", initCheck.Details)), []byte(fmt.Sprintf("%v", check.Details))) {
			fmt.Println("it matches ---- check:", check.Details[3].Backend[4].Health, " init check:", initCheck.Details[3].Backend[4].Health)
		} else {
			fmt.Println("it doesn't ---- check:", check.Details[3].Backend[4].Health, " init check:", initCheck.Details[3].Backend[4].Health)
		}

		if !reflect.DeepEqual(*initCheck, check) {
			fmt.Println("it doesnt ----------------------------------------------------------------------------------------- it doesnt")
			checkJSON, _ := json.Marshal(check)
			err := conn.WriteMessage(websocket.TextMessage, checkJSON)

			initCheck = &check
			if err != nil {
				break
			}
		}
		time.Sleep(3 * time.Second)
	}
}

// read data.json into useable information, update healthchecks
func read(mp metadata, msg []byte, healthcheckHost string) metadata {
	// Decode JSON into our map
	json.Unmarshal([]byte(msg), &mp)

	for i := range mp.Details {
		//fmt.Println(mp.Details[i].Title)
		//non jfrog platform, dumb tcp ping to backend + healthcheck if applicable
		partsPort := strings.Split(mp.Details[i].URL, ":")
		if !mp.Details[i].Platform {
			result, code, errCode := auth.GetRestAPI("GET", false, "http://"+healthcheckHost+partsPort[2]+mp.Details[i].HealthcheckCall, "", "", "")
			if string(result) == mp.Details[i].HealthExpResp && code != 0 {
				mp.Details[i].HealthPing = "OK"
			} else if strings.Contains(errCode, "connection refused") {
				//fmt.Println(string(result), mp.Details[i].URL)
				mp.Details[i].HealthPing = "DOWN"
			} else {
				fmt.Println(string(result), mp.Details[i].URL)
				mp.Details[i].HealthPing = "LIMBO"
			}

			ping(mp.Details[i].Backend, healthcheckHost)
			//fmt.Println("testing status:", status)
		}
		//platform healthcheck
		if mp.Details[i].Platform {
			result, _, _ := auth.GetRestAPI("GET", false, "http://"+healthcheckHost+partsPort[2]+mp.Details[i].PlatformHcCall, "", "", "")
			var platform platformStruct
			json.Unmarshal(result, &platform)

			// overall health check via router
			if platform.Router.State == "HEALTHY" {
				mp.Details[i].HealthPing = "OK"
			}
			//Backend[0] is always router
			mp.Details[i].Backend[0].Health = platform.Router.State

			for j := range platform.Services {

				parts := strings.Split(platform.Services[j].ServiceID, "@")
				//+1 is extremely hacky, should be index matching
				//fmt.Println(parts[0], platform.Services[j], mp.Details[i].Backend[j+1].Jfid)
				if parts[0] == mp.Details[i].Backend[j+1].Jfid && mp.Details[i].Backend[j+1].Jfid != "" {
					mp.Details[i].Backend[j+1].Health = platform.Services[j].Message
				}
			}

			// non platform services check
			//fmt.Println("non platform check")
			for k := range mp.Details[i].Backend {
				if mp.Details[i].Backend[k].Jfid == "" {
					_, err := net.Dial("tcp", healthcheckHost+mp.Details[i].Backend[k].Port)
					//fmt.Println(mp.Details[i].Backend[k].Service)
					if err != nil {
						//err response:dial tcp <host>:<port>: connect: connection refused
						//fmt.Println(err)
						mp.Details[i].Backend[k].Health = "DOWN"

					} else {
						//fmt.Println(conn, "OK")
						mp.Details[i].Backend[k].Health = "OK"
					}
				}
			}

			//err response:Get <url>: dial tcp <host>:<port>: connect: connection refused
		}
	}
	return mp
}

//ping all backend services via tcp
func ping(backend []backendJSON, healthcheckHost string) []backendJSON {
	for key, value := range backend {
		//fmt.Println(backend[key], key, value.Port)
		_, err := net.Dial("tcp", healthcheckHost+value.Port)
		if err != nil {
			//err response:dial tcp <host>:<port>: connect: connection refused
			//fmt.Println(err)
			backend[key].Health = "DOWN"

		} else {
			//fmt.Println(conn, "OK")
			backend[key].Health = "OK"
		}
		//fmt.Println("heatlh:", backend[key].Health)
	}
	return backend
}
