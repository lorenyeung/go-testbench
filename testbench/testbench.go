package main

import (
	"encoding/json"
	"fmt"
	"go-testbench/auth"
	"go-testbench/dockerapi"
	"go-testbench/helpers"
	"io/ioutil"

	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	log "github.com/Sirupsen/logrus"
)

type socket struct {
	Details        []metadataJSON `json:"data"`
	ContainersList []map[string]interface{}
	Messages       []string
}

type metadata struct {
	Details    []metadataJSON `json:"data"`
	LastUpdate string         `json:"lastUpdate"`
}
type metadataJSON struct {
	ID              string        `json:"id"`
	Backend         []backendJSON `json:"backend"`
	VersionCall     string        `json:"versionCall"`
	VersionSpec     string        `json:"versionSpec"`
	VersionPing     string        `json:"versionPing"`
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

	userVar, err := user.Current()
	auth.CheckErr(err)

	flags := helpers.SetFlags(userVar)
	helpers.SetLogger(flags.LogLevelVar)

	if strings.Contains(flags.DataFileVar, flags.ConfigFolder) {
		log.Println("Checking existence of config folder")
		if _, err := os.Stat(userVar.HomeDir + flags.ConfigFolder); os.IsNotExist(err) {
			log.Println("No config folder found")
			err = os.MkdirAll(userVar.HomeDir+flags.ConfigFolder, 0700)
			helpers.Check(err, true, "Generating "+userVar.HomeDir+flags.ConfigFolder+" directory", helpers.Trace())
		}
	}

	file, err := os.Open(flags.DataFileVar)
	helpers.Check(err, true, "data JSON read", helpers.Trace())
	msg, _ := ioutil.ReadAll(file)
	var mp, initMp metadata

	// Decode JSON into our map
	json.Unmarshal([]byte(msg), &mp)
	json.Unmarshal([]byte(msg), &initMp)

	var mpPtr *metadata = &mp
	//var wg sync.WaitGroup
	go func() {
		for {
			log.Info("Triggering health check update:", mp.Details[0].VersionPing)
			mp2 := read(mp, msg, flags.HealthcheckHostVar+":")
			mp = mp2
			time.Sleep(3 * time.Second)
			//max time of failures is ~ 4 seconds + ^sleep, min is probably around 0.5 + ^sleep
		}
	}()
	initMp = read(initMp, msg, flags.HealthcheckHostVar+":")
	var checkPtr *metadata = &initMp
	//wg.Wait()

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
		var err error

		for i := range action.First {
			fmt.Println("stop or start", action.First[i])
			_, err = bashCommandWrapper(action.First[i])
		}
		if len(action.Second) > 0 {
			for i := range action.Second {
				fmt.Println("restart", action.Second[i])
				_, err = bashCommandWrapper(action.Second[i])
			}
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"response": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"response": "OK"})
		}
	})

	// backend listing for docker containers
	var containersList []map[string]interface{}
	//containerRaw, _ := json.Marshal(dockerapi.ListRunningContainers())
	// err2 := json.Unmarshal([]byte(containerRaw), &containersList)
	// if err2 != nil {
	// 	panic(err)
	// }
	var containersListPtr *[]map[string]interface{} = &containersList

	router.GET("/data", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": mp,
		})

	})
	//websocket
	//var clients = make(map[*websocket.Conn]bool) // connected clients
	//var broadcast = make(chan check.Details)     //broadcast channel

	router.GET("/", func(c *gin.Context) {
		//check := read(mp, msg, healthcheckHost)
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"title": "lorenTestbench", "art_data": mp.Details, "containers": containersList, "websocket": flags.WebsocketHostVar + ":" + flags.PortVar})
	})

	//testing hubs
	hub := newHub()
	go hub.run()

	router.GET("/ws", func(c *gin.Context) {
		wshandler(c.Writer, c.Request, msg, flags.WebsocketHostVar+":", checkPtr, containersListPtr, mpPtr)
		serveWs(hub, c.Writer, c.Request)
	})
	router.Run("0.0.0.0:" + flags.PortVar) // listen and serve on 0.0.0.0:8080
}

func bashCommandWrapper(cmdString string) (string, error) {
	command := strings.Split(cmdString, " ")
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		fmt.Println(err)
		return "ERROR", err
	}
	err = cmd.Wait()
	if err != nil {
		fmt.Println(err)
		return "ERROR", err
	}
	return "OK", nil
}

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func wshandler(w http.ResponseWriter, r *http.Request, msg []byte, healthcheckHost string, initCheck *metadata, containersListInit *[]map[string]interface{}, mp *metadata) {
	conn, err := wsupgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Warn("Failed to upgrade ws: %+v", err)
		return
	}
	defer conn.Close()

	//return init state on first call
	initCheckJSON, _ := json.Marshal(*mp) //sending current as a test

	err2 := conn.WriteMessage(websocket.TextMessage, initCheckJSON)
	if err2 != nil {
		return
	}

	//init connect
	_, initmsgRec, errRead := conn.ReadMessage()
	fmt.Println("init:", string(initmsgRec))

	//receive UI current status

	for {

		//keepalive
		err3 := conn.WriteMessage(websocket.PingMessage, []byte("keepalive"))
		if err3 != nil {
			log.Warn("err3 error")
			return
		}

		if errRead != nil {
			break
		}
		blah, _ := json.Marshal(mp)
		err := conn.WriteMessage(websocket.TextMessage, blah)
		if err != nil {
			log.Info("closing socket ", initmsgRec)
			break
		}

		// if reflect.DeepEqual([]byte(fmt.Sprintf("%v", initCheck.Details)), []byte(fmt.Sprintf("%v", mp.Details))) {
		// 	fmt.Println(mp.Details[11].ID, "it matches ---- check:", initCheck.Details[11].Backend[0].Health, " init check:", mp.Details[11].Backend[0].Health, mp.LastUpdate)
		// 	time.Sleep(3 * time.Second)
		// 	if errRead != nil {
		// 		log.Warn("closing socket")
		// 		break
		// 	}
		// } else {
		// 	fmt.Println("it doesnt ----------------------------------------------------------------------------------------- it doesnt")
		// 	checkJSON, _ := json.Marshal(mp)
		// 	err := conn.WriteMessage(websocket.TextMessage, checkJSON)

		// 	initCheck = mp
		// 	if err != nil {
		// 		log.Info("closing socket")
		// 		break
		// 	}

		// }

		// var containersList []map[string]interface{}
		// containerRaw, _ := json.Marshal(dockerapi.ListRunningContainers())
		// err2 := json.Unmarshal([]byte(containerRaw), &containersList)
		// if err2 != nil {
		// 	panic(err2)
		// }

		// if !reflect.DeepEqual(*containersListInit, containersList) {
		// 	//fmt.Println("container list doesn't match ------------------------------------------------", *containersListInit, "-----------------", containersList)
		// }
		//		second := time.Now().Second()
		// for second%3 != 0 {
		// 	time.Sleep(1 * time.Second)
		// 	if time.Now().Second()%3 == 0 {
		// 		break
		// 	}
		// }

		// t, msgRec, errRead := conn.ReadMessage()
		// log.Info(t, string(msgRec))
		// if errRead != nil {
		// 	log.Info("closing socket")
		// 	conn.Close()
		// 	return
		// }
		second := time.Now().Second()
		for second%3 != 0 {
			time.Sleep(1 * time.Second)
			if time.Now().Second()%3 == 0 {
				log.Debug("time to update websocket ", initmsgRec)
				break
			}
		}
		//sleep 1 second to avoid duping the for loop if the whole iteration takes less than 1 second
		time.Sleep(1 * time.Second)
	}
}

// read data.json into useable information, update healthchecks
func read(mp metadata, msg []byte, healthcheckHost string) metadata {

	//maybe concurrent this stuff
	var wg sync.WaitGroup
	wg.Add(len(mp.Details))
	start := time.Now()

	for i := range mp.Details {
		go func(i int) {
			defer wg.Done()

			//non jfrog platform, dumb tcp ping to backend + healthcheck if applicable
			partsPort := strings.Split(mp.Details[i].URL, ":")
			if !mp.Details[i].Platform {
				result, code, errCode := auth.GetRestAPI("GET", false, "http://"+healthcheckHost+partsPort[2]+mp.Details[i].HealthcheckCall, "", "", "")
				log.Debug("non Platform healthcheck for ", healthcheckHost+partsPort[2]+mp.Details[i].HealthcheckCall, " error code:", errCode, " status code:", code)
				if string(result) == mp.Details[i].HealthExpResp && code == 200 {
					mp.Details[i].HealthPing = "OK"
				} else if (strings.Contains(errCode, "timeout") || strings.Contains(errCode, "Timeout") || strings.Contains(errCode, "Connection refused") || strings.Contains(errCode, "Empty reply from server") || strings.Contains(errCode, "EOF")) && code == 0 {
					//fmt.Println(string(result), mp.Details[i].URL)
					mp.Details[i].HealthPing = "DOWN"
				} else {
					//fmt.Println(string(result), mp.Details[i].URL)
					mp.Details[i].HealthPing = "LIMBO"
				}

				ping(mp.Details[i].Backend, healthcheckHost)
				//fmt.Println("testing status:", status)
			}
			//platform healthcheck
			if mp.Details[i].Platform {
				result, code, errCode := auth.GetRestAPI("GET", false, "http://"+healthcheckHost+partsPort[2]+mp.Details[i].PlatformHcCall, "", "", "")
				log.Debug("Platform healthcheck for ", healthcheckHost+partsPort[2]+mp.Details[i].PlatformHcCall, " result:", string(result), " error code:", errCode, " status code:", code)
				var platform platformStruct
				err := json.Unmarshal(result, &platform)
				if err != nil {
					log.Warn("JSON unmarshall:", err)
				}

				// overall health check via router

				if code == 503 {
					mp.Details[i].HealthPing = "LIMBO"
				} else if platform.Router.State == "HEALTHY" {
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
				var wgBkend sync.WaitGroup
				wgBkend.Add(len(mp.Details[i].Backend))
				for k := range mp.Details[i].Backend {
					go func(k int) {
						defer wgBkend.Done()
						if mp.Details[i].Backend[k].Jfid == "" {
							start := time.Now()
							log.Trace("Dial start NPSC:", mp.Details[i].Backend[k].Service, healthcheckHost+mp.Details[i].Backend[k].Port)
							_, err := net.DialTimeout("tcp", healthcheckHost+mp.Details[i].Backend[k].Port, 2*time.Second)
							log.Trace("Dial end NPSC:", mp.Details[i].Backend[k].Service, time.Since(start))
							if err != nil {
								//err response:dial tcp <host>:<port>: connect: connection refused
								mp.Details[i].Backend[k].Health = "DOWN"

							} else {
								mp.Details[i].Backend[k].Health = "OK"
							}
							log.Debug("Dial NPSC health:", mp.Details[i].Backend[k].Health)
						}
					}(k)
				}
				wgBkend.Wait()
				//err response:Get <url>: dial tcp <host>:<port>: connect: connection refused
			}
			//version check, only if HealthPing is Healthy
			if mp.Details[i].VersionPing == "" && mp.Details[i].HealthPing == "OK" {
				//	fmt.Println("if check:", mp.Details[i].VersionPing, mp.Details[i].ID)
				result, _, _ := auth.GetRestAPI("GET", false, mp.Details[i].URL+mp.Details[i].VersionCall, "", "", "")

				if mp.Details[i].VersionSpec != "" && result != nil {
					log.Info(string(result))
					var versionResults map[string]interface{}
					err := json.Unmarshal(result, &versionResults)
					if err != nil || versionResults[mp.Details[i].VersionSpec] == nil {
						log.Warn(err, helpers.Trace())
						mp.Details[i].VersionPing = "N/A"
					} else {
						fmt.Println("spec:", versionResults[mp.Details[i].VersionSpec], mp.Details[i].ID)
						mp.Details[i].VersionPing = versionResults[mp.Details[i].VersionSpec].(string)
					}
				}
			}
		}(i)
	}
	wg.Wait()
	log.Debug("start:", start, " finish:", time.Now(), "read diff", time.Since(start))
	mp.LastUpdate = time.Now().String()
	return mp
}

//ping all backend services via tcp
func ping(backend []backendJSON, healthcheckHost string) []backendJSON {
	for key, value := range backend {
		go func(key int, value backendJSON) {
			start := time.Now()
			log.Trace("Dial start ping recursive:", healthcheckHost+value.Port)
			_, err := net.DialTimeout("tcp", healthcheckHost+value.Port, 2*time.Second)
			log.Trace("Dial end ping recursive:", healthcheckHost+value.Port, " ", time.Since(start))
			if err != nil {
				//err response:dial tcp <host>:<port>: connect: connection refused
				backend[key].Health = "DOWN"
			} else {
				backend[key].Health = "OK"
			}
			log.Debug("ping recursive health:", backend[key].Health)
		}(key, value)
	}
	return backend
}
