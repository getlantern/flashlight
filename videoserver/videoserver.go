package videoserver

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.videoserver")
)

func ServeVideo(resp http.ResponseWriter, req *http.Request) {
	video := fmt.Sprintf("videos/%v", req.URL.Query().Get("v"))
	log.Debugf("Serving video: %v", video)
	file, err := os.Open(video)
	if err != nil {
		log.Errorf("Unable to open video %v: %v", video, err)
		resp.WriteHeader(http.StatusNotFound)
		return
	}
	pr, pw := io.Pipe()
	go io.Copy(pw, file)
	resp.WriteHeader(http.StatusOK)
	io.Copy(resp, pr)
}
