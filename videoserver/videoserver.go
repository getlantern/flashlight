package videoserver

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.videoserver")
)

func ServeVideo(resp http.ResponseWriter, req *http.Request) {
	videoHash := strings.Split(path.Base(req.URL.Query().Get("v")), ".")[0]
	log.Debugf("Serving video: %v", videoHash)
	cmd := exec.Command("ipfs", "cat", videoHash)
	data, _ := cmd.StdoutPipe()
	stdErr, _ := cmd.StderrPipe()
	go io.Copy(os.Stderr, stdErr)
	resp.WriteHeader(http.StatusOK)
	go io.Copy(resp, data)
	err := cmd.Run()
	if err != nil {
		log.Errorf("Error reading %v from ipfs: %v", videoHash, err)
	}
}
