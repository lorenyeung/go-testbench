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
	"strings"

	"github.com/gin-gonic/gin"
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

	var containersList []map[string]interface{}
	containerRaw, _ := json.Marshal(dockerapi.ListRunningContainers())
	err2 := json.Unmarshal([]byte(containerRaw), &containersList)
	if err2 != nil {
		panic(err)
	}

	check := read(mp, msg)
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"title": "lorenTestbench", "art_data": check.Details, "containers": containersList})
	})

	router.GET("/login", auth.LoginHandler)

	router.Run("0.0.0.0:8080") // listen and serve on 0.0.0.0:8080
}

func read(mp metadata, msg []byte) metadata {
	// Decode JSON into our map
	json.Unmarshal([]byte(msg), &mp)

	for i := range mp.Details {
		fmt.Println(mp.Details[i].Title)
		//non jfrog platform, dumb tcp ping to backend + healthcheck if applicable
		if !mp.Details[i].Platform {
			result, code := auth.GetRestAPI("GET", false, mp.Details[i].URL+mp.Details[i].HealthcheckCall, "", "", "")
			fmt.Println("health result:", string(result), code)
			if string(result) == mp.Details[i].HealthExpResp && code != 0 {
				mp.Details[i].HealthPing = "OK"
			}
			status := ping(mp.Details[i].Backend)
			fmt.Println("testing status:", status)
		}
		//platform healthcheck
		if mp.Details[i].Platform {
			result, _ := auth.GetRestAPI("GET", false, mp.Details[i].URL+mp.Details[i].PlatformHcCall, "", "", "")
			var platform platformStruct
			json.Unmarshal(result, &platform)

			// overall health check via router
			if platform.Router.Message == "OK" {
				mp.Details[i].HealthPing = "OK"
			}
			//Backend[0] is always router
			mp.Details[i].Backend[0].Health = platform.Router.Message

			for j := range platform.Services {

				parts := strings.Split(platform.Services[j].ServiceID, "@")
				//+1 is extremely hacky, should be index matching
				fmt.Println(parts[0], platform.Services[j], mp.Details[i].Backend[j+1].Jfid)
				if parts[0] == mp.Details[i].Backend[j+1].Jfid {
					mp.Details[i].Backend[j+1].Health = platform.Services[j].Message
				}
			}

			//err response:Get <url>: dial tcp <host>:<port>: connect: connection refused
		}
	}
	return mp
}

func ping(backend []backendJSON) []backendJSON {
	for key, value := range backend {
		//fmt.Println(backend[key], key, value.Port)
		conn, err := net.Dial("tcp", "localhost:"+value.Port)
		if err != nil {
			//err response:dial tcp <host>:<port>: connect: connection refused
			//fmt.Println(err)
			backend[key].Health = "DOWN"

		} else {
			fmt.Println(conn, "OK")
			backend[key].Health = "OK"
		}
		//fmt.Println("heatlh:", backend[key].Health)
	}
	return backend
}
