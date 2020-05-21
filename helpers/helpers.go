package helpers

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/Sirupsen/logrus"
)

//FileStorageJSON file details call
type FileStorageJSON struct {
	LastModified  string `json:"lastModified"`
	ConvertedTime time.Time
	Size          string `json:"size"`
	DownloadURI   string `json:"downloadUri"`
	Checksums     struct {
		Sha256 string `json:"sha256"`
	} `json:"checksums"`
}

// StorageJSON file list call
type StorageJSON struct {
	Children []struct {
		URI    string `json:"uri"`
		Folder string `json:"folder"`
	} `json:"children"`
}

//TimeSlice sorted data structure
type TimeSlice []FileStorageJSON

func (p TimeSlice) Len() int {
	return len(p)
}

func (p TimeSlice) Less(i, j int) bool {
	return p[i].ConvertedTime.Before(p[j].ConvertedTime)
}

func (p TimeSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

//ByteCountDecimal convert bytes to human readable data size
func ByteCountDecimal(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "kMGTPE"[exp])
}

//StringToInt64 self explanatory
func StringToInt64(data string) int64 {
	convert, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		log.Error("%d is not of type %T", convert, convert)
		os.Exit(127)
	}
	return convert
}

//PrintSorted print data in human readable format
func PrintSorted(sorted TimeSlice, url, repo, download string) {
	for key, value := range sorted {
		fmt.Printf("%d\t%s\t%s\t%s\n", key+1, value.ConvertedTime.Format("2006-01-02 15:04:05"), ByteCountDecimal(StringToInt64(value.Size)), strings.TrimPrefix(value.DownloadURI, url+"/"+repo+"/"+download+"/"))
	}
}

//PrintDownloadPercent self explanatory
func PrintDownloadPercent(done chan int64, path string, total int64) {
	var stop = false
	for {
		select {
		case <-done:
			stop = true
		default:
			file, err := os.Open(path)
			Check(err, true, "Opening file path", Trace())
			fi, err := file.Stat()
			Check(err, true, "Getting file statistics", Trace())
			size := fi.Size()
			if size == 0 {
				size = 1
			}
			var percent = float64(size) / float64(total) * 100
			if percent != 100 {
				fmt.Printf("\r%.0f%% %s", percent, path)
			}
		}
		if stop {
			break
		}
		time.Sleep(time.Second)
	}
}

//ComputeSha256 self explanatory
func ComputeSha256(path string) string {
	f, err := os.Open(path)
	Check(err, true, "Opening file path", Trace())
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Fatal(err)
	}
	return (hex.EncodeToString(h.Sum(nil)[:]))
}

//Check logger for errors
func Check(e error, panicCheck bool, logs string, trace TraceData) {
	if e != nil && panicCheck {
		log.Error(logs, " failed with error:", e, " ", trace.Fn, " on line:", trace.Line)
		panic(e)
	}
	if e != nil && !panicCheck {
		log.Warn(logs, " failed with error:", e, " ", trace.Fn, " on line:", trace.Line)
	}
}

//TraceData trace data struct
type TraceData struct {
	File string
	Line int
	Fn   string
}

//Trace get function data
func Trace() TraceData {
	var trace TraceData
	pc, file, line, ok := runtime.Caller(1)
	if !ok {
		log.Warn("Failed to get function data")
		return trace
	}

	fn := runtime.FuncForPC(pc)
	trace.File = file
	trace.Line = line
	trace.Fn = fn.Name()
	return trace
}

//SetLogger sets logger settings
func SetLogger(logLevelVar string) {
	level, err := log.ParseLevel(logLevelVar)
	if err != nil {
		level = log.InfoLevel
	}
	log.SetLevel(level)

	log.SetReportCaller(true)
	customFormatter := new(logrus.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05"
	customFormatter.QuoteEmptyFields = true
	customFormatter.FullTimestamp = true
	customFormatter.CallerPrettyfier = func(f *runtime.Frame) (string, string) {
		repopath := strings.Split(f.File, "/")
		function := strings.Replace(f.Function, "go-pkgdl/", "", -1)
		return fmt.Sprintf("%s\t", function), fmt.Sprintf(" %s:%d\t", repopath[len(repopath)-1], f.Line)
	}

	logrus.SetFormatter(customFormatter)
	log.Info("Log level set at ", level)
}

//Flags struct
type Flags struct {
	HealthcheckHostVar, PortVar, DataFileVar, WebsocketHostVar, LogLevelVar, ConfigFolder string
}

//SetFlags function
func SetFlags(userVar *user.User) Flags {
	var flags Flags
	configFile := "data.json"
	flags.ConfigFolder = "/.lorenygo/testBench/"
	flag.StringVar(&flags.LogLevelVar, "log", "INFO", "Order of Severity: TRACE, DEBUG, INFO, WARN, ERROR, FATAL, PANIC")
	flag.StringVar(&flags.PortVar, "port", "8080", "Port")
	flag.StringVar(&flags.HealthcheckHostVar, "healthcheckhost", "loren.jfrog.team", "healthcheck Host")
	flag.StringVar(&flags.WebsocketHostVar, "websockethost", "loren.jfrog.team", "websocket Host")
	flag.StringVar(&flags.DataFileVar, "data", userVar.HomeDir+flags.ConfigFolder+configFile, "Path to JSON file")
	flag.Parse()
	return flags
}
