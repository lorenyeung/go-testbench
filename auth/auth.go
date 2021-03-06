package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"go-testbench/dockerapi"
	"go-testbench/helpers"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/gin-gonic/contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

//https://skarlso.github.io/2016/06/12/google-signin-with-go/ great guide

// Credentials which stores google ids.
type Credentials struct {
	Cid     string `json:"cid"`
	Csecret string `json:"csecret"`
	HTTPS   bool   `json:"https"`
	Host    string `json:"host"`
	Oauth   string `json:"oauth"`
	Email   string `json:"email"`
}

// User is a retrieved and authenticated user.
type User struct {
	Sub           string `json:"sub"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Profile       string `json:"profile"`
	Picture       string `json:"picture"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Gender        string `json:"gender"`
}

var cred Credentials
var conf *oauth2.Config
var state string

func randToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// TODO: remove hardcoded directory

func indexHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index.tmpl", gin.H{})
}

func getLoginURL(state string) string {
	return conf.AuthCodeURL(state)
}

//CheckErr checks what the error is
func CheckErr(err error) {
	if err != nil {
		panic(err)
	}
}

//LoginHandler sets session state on login
func LoginHandler(c *gin.Context) {
	state = randToken()
	session := sessions.Default(c)
	session.Set("state", state)
	session.Save()
	c.Writer.Write([]byte("<html><title>Golang Google</title> <body> <a href='" + getLoginURL(state) + "'><button>Login with Google!</button> </a> </body></html>"))
}

//AuthorizeRequest is used to authorize a request for a certain end-point group.
func AuthorizeRequest() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		v := session.Get("user-id")
		if v == nil {
			c.HTML(http.StatusUnauthorized, "error.tmpl", gin.H{"message": "Please login."})
			c.Abort()
		}
		c.Next()
	}
}

// FieldHandler is a rudementary handler for logged in users. seems to load the main page, maybe use this to return to?
func FieldHandler(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user-id")
	picture := session.Get("user-pic")
	var containers []map[string]interface{}
	containerRaw, _ := json.Marshal(dockerapi.ListRunningContainers())
	err := json.Unmarshal([]byte(containerRaw), &containers)
	if err != nil {
		panic(err)
	}
	// use             <td>{{.}}</td> to get all data
	c.HTML(http.StatusOK, "field.tmpl", gin.H{"email": userID, "picture": picture, "art_data": containers})
}

// CreateHandler is a rudementary handler for logged in users.
func CreateHandler(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user-id")
	picture := session.Get("user-pic")
	var containers []map[string]interface{}
	containerRaw, _ := json.Marshal(dockerapi.ListRunningContainers())
	err := json.Unmarshal([]byte(containerRaw), &containers)
	if err != nil {
		panic(err)
	}
	artVersions := GetVersions()
	// use             <td>{{.}}</td> to get all data
	c.HTML(http.StatusOK, "create.tmpl", gin.H{"email": userID, "picture": picture, "art_data": containers, "art_versions": artVersions})
}

type version struct {
	Version []string `json:"versions"`
}

//GetVersions function
func GetVersions() []string {
	//resp, err := http.Get("https://api.bintray.com/packages/jfrog/artifactory-pro/jfrog-artifactory-pro-zip")
	resp, err := http.Get("https://api.bintray.com/packages/jfrog/reg2/jfrog:artifactory-pro")
	CheckErr(err)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	versions := version{}
	json.Unmarshal(body, &versions)
	return versions.Version
}

//GetRestAPI GET rest APIs response with error handling
func GetRestAPI(method string, auth bool, urlInput, userName, apiKey, filepath string) ([]byte, int, string) {
	timeStart := time.Now()
	log.Debug("Starting request ", method, urlInput)

	//c := make(chan struct{})
	// timer := time.AfterFunc(2*time.Second, func() {
	// 	close(c)
	// })

	var netTransport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 2 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 2 * time.Second,
	}

	client := http.Client{
		Timeout:   2 * time.Second,
		Transport: netTransport,
	}
	req, err := http.NewRequest(method, urlInput, nil)
	if auth {
		req.SetBasicAuth(userName, apiKey)
	}
	if err != nil {
		//fmt.Printf("The HTTP request failed with error %s\n", err)
	} else {
		resp, err := client.Do(req)
		helpers.Check(err, false, "The HTTP response", helpers.Trace())
		if err != nil {
			return nil, 0, err.Error()
		}
		defer resp.Body.Close()
		// for {
		// 	//timer.Reset(2 * time.Second)
		// 	timer.Reset(50 * time.Millisecond)
		// 	_, err = io.CopyN(ioutil.Discard, resp.Body, 256)
		// 	if err == io.EOF {
		// 		log.Warn("io eof")
		// 		break
		// 	} else if err != nil {
		// 		log.Fatal(err)
		// 	}
		// }

		if resp.StatusCode != 200 {
			//log.Printf("Got status code %d for %s, continuing\n", resp.StatusCode, urlInput)
		}
		//Mostly for HEAD requests
		statusCode := resp.StatusCode

		if filepath != "" && method == "GET" {
			// Create the file
			out, err := os.Create(filepath)
			helpers.Check(err, false, "File create", helpers.Trace())
			defer out.Close()

			//done := make(chan int64)
			//go helpers.PrintDownloadPercent(done, filepath, int64(resp.ContentLength))
			_, err = io.Copy(out, resp.Body)
			helpers.Check(err, false, "The file copy", helpers.Trace())
		} else {
			data, err := ioutil.ReadAll(resp.Body)
			helpers.Check(err, false, "Data read", helpers.Trace())
			log.Debug("End request for ", method, " url:", urlInput, time.Now(), " duration:", time.Since(timeStart))
			return data, statusCode, ""
		}
	}
	log.Debug("End request for ", method, urlInput, time.Now(), " duration:", time.Since(timeStart))
	return nil, 0, err.Error()
}
