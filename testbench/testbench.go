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

	"github.com/gin-gonic/gin"
)

type metadata struct {
	Details []metadataJSON `json:"data"`
}
type metadataJSON struct {
	ID            string        `json:"id"`
	Backend       []backendJSON `json:"backend"`
	VersionCall   string        `json:"versionCall"`
	VersionSpec   string        `json:"versionSpec"`
	URL           string        `json:"url"`
	HealthCheck   string        `json:"healthcheck"`
	HealthExpResp string        `json:"healthExpResp"`
	ImageSrc      string        `json:"imageSrc"`
	FrontTitle    string        `json:"frontTitle"`
	FrontLink     string        `json:"frontLink"`
	BackTitle     string        `json:"backTitle"`
	BackContent   string        `json:"backContent"`
	Platform      bool          `json:"platform"`
	PlatformHc    string        `json:"platformHc"`
}

type backendJSON struct {
	Service string `json:"service"`
	Port    string `json:"port"`
	Jfid    string `json:"jfid"`
}

func main() {
	user, err := user.Current()
	auth.CheckErr(err)

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
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"title": "lorenTestbench",
		})
	})

	router.GET("/login", auth.LoginHandler)
	read()
	router.Run("0.0.0.0:8080") // listen and serve on 0.0.0.0:8080
}

func read() {

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	configFolder := "/.lorenygo/testBench/"
	configPath := usr.HomeDir + configFolder

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
	fmt.Println(len(mp.Details))
	for i := range mp.Details {
		fmt.Println(mp.Details[i].FrontTitle)
		//non jfrog platform, dumb tcp ping to backend + healthcheck if applicable
		if !mp.Details[i].Platform {
			resp, err := http.Get(mp.Details[i].URL + mp.Details[i].HealthCheck)
			fmt.Println(resp, err)
			ping(mp.Details[i].Backend)
		}
		//platform healthcheck
		if mp.Details[i].Platform {
			resp, err := http.Get(mp.Details[i].URL + mp.Details[i].PlatformHc)
			//err response:Get <url>: dial tcp <host>:<port>: connect: connection refused
			fmt.Println(resp, err)
		}

	}
}

func ping(backend []backendJSON) {
	for key, value := range backend {
		fmt.Println(backend[key], key, value.Port)
		conn, err := net.Dial("tcp", "localhost:"+value.Port)
		if err != nil {
			//err response:dial tcp <host>:<port>: connect: connection refused
			fmt.Println(err)
		} else {
			fmt.Println(conn, "OK")
		}
	}
}
