package main

import (
	"encoding/json"
	"fmt"
	"go-testbench/auth"
	"go-testbench/dockerapi"
	"go-testbench/helpers"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
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
	healthcheckHost := "loren.jfrog.team:"

	user, err := user.Current()
	auth.CheckErr(err)
	configFolder := "/.lorenygo/testBench/"
	configPath := user.HomeDir + configFolder

	log.Println("Checking existence of home folder")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Println("No config folder found")
		err = os.MkdirAll(configPath, 0700)
		helpers.Check(err, true, "Generating "+configPath+" directory")
	}

	file, err := os.Open(configPath + "data.json")
	helpers.Check(err, true, "data JSON read")
	msg, _ := ioutil.ReadAll(file)
	var mp metadata
	// Decode JSON into our map
	json.Unmarshal([]byte(msg), &mp)

	router := gin.Default()
	router.LoadHTMLGlob(user.HomeDir + "/go/src/go-testbench/templates/*")
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

	// backend listing for docker containers
	var containersList []map[string]interface{}
	containerRaw, _ := json.Marshal(dockerapi.ListRunningContainers())
	err2 := json.Unmarshal([]byte(containerRaw), &containersList)
	if err2 != nil {
		panic(err)
	}

	check := read(mp, msg, healthcheckHost)
	var checkPtr *metadata = &check

	//websocket
	//var clients = make(map[*websocket.Conn]bool) // connected clients
	//var broadcast = make(chan check.Details)     //broadcast channel

	router.GET("/", func(c *gin.Context) {
		//check := read(mp, msg, healthcheckHost)
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"title": "lorenTestbench", "art_data": check.Details, "containers": containersList})
	})

	router.GET("/ws", func(c *gin.Context) {
		wshandler(c.Writer, c.Request, msg, healthcheckHost, checkPtr)
	})
	router.Run("0.0.0.0:8080") // listen and serve on 0.0.0.0:8080
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
	conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%v", initCheck.Details)))
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
			err := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%v", check.Details)))
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
			result, code := auth.GetRestAPI("GET", false, "http://"+healthcheckHost+partsPort[2]+mp.Details[i].HealthcheckCall, "", "", "")
			if string(result) == mp.Details[i].HealthExpResp && code != 0 {
				mp.Details[i].HealthPing = "OK"
			}
			ping(mp.Details[i].Backend, healthcheckHost)
			//fmt.Println("testing status:", status)
		}
		//platform healthcheck
		if mp.Details[i].Platform {
			result, _ := auth.GetRestAPI("GET", false, "http://"+healthcheckHost+partsPort[2]+mp.Details[i].PlatformHcCall, "", "", "")
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
