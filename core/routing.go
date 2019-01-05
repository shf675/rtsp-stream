package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Roverr/rtsp-stream/core/config"
	"github.com/Roverr/rtsp-stream/core/streaming"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

// ErrNoStreamFn is used to create dynamic errors for unknown hosts requested as stream
var ErrNoStreamFn = func(path string) error { return fmt.Errorf("%s is not a known stream", path) }

// ErrStreamAlreadyActive is an error describing that we cannot restart the stream because it's already running
var ErrStreamAlreadyActive = errors.New("Stream is already active")

// streamDto describes an uri where the client can access the stream
type streamDto struct {
	URI string `json:"uri"`
}

// summariseDto describes each stream and their state of running
type summariseDto struct {
	Running bool   `json:"running"`
	URI     string `json:"uri"`
}

// validateURI is for validiting that the URI is in a valid format
func validateURI(dto *streamDto, body io.Reader) error {
	// Parse request
	uri, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(uri, dto); err != nil {
		return err
	}

	if _, err := url.Parse(dto.URI); err != nil {
		return errors.New("Invalid URI")
	}
	return nil
}

func handleAlreadyRunningStream(w http.ResponseWriter, s streaming.Stream, spec *config.Specification, dir string) {
	// If transcoding is not running, spin it back up
	if !s.Streak.IsActive() {
		err := s.Restart(spec, dir)
		if err != nil {
			logrus.Error(err)
			http.Error(w, "Unexpected error", 500)
			return
		}
	}
	// If the stream is already running return its path
	b, err := json.Marshal(streamDto{URI: s.Path})
	if err != nil {
		http.Error(w, "Unexpected error", 500)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	w.Write(b)
}

// determinesHost is for parsing out the host from the storage path
func determineHost(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 1 {
		return parts[1]
	}
	return ""
}

// GetRouter returns the return for the application
func GetRouter(config *config.Specification) (*httprouter.Router, *Controller) {
	fileServer := http.FileServer(http.Dir(config.StoreDir))
	router := httprouter.New()
	controllers := Controller{config, map[string]streaming.Stream{}, fileServer}
	if config.ListEndpoint {
		router.GET("/list", controllers.ListStreamHandler)
	}
	router.POST("/start", controllers.StartStreamHandler)
	router.GET("/stream/*filepath", controllers.FileHandler)

	// Start cleaning process in the background
	go func() {
		for {
			<-time.After(config.CleanupTime)
			controllers.cleanUnused()
		}
	}()

	return router, &controllers
}
