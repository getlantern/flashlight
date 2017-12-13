package videoserver

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"

	"github.com/getlantern/flashlight/loconf"
)

var (
	log       = golog.LoggerFor("flashlight.videoserver")
	videoList = eventual.NewValue()
)

func FetchLoop() func() {
	tk := time.NewTicker(1 * time.Hour)
	ch := make(chan bool)
	stop := func() { ch <- true }
	go func() {
		for {
			b, err := loconf.GetPopVideos(http.DefaultClient)
			if err != nil {
				log.Error(err)
			} else {
				videoList.Set(b)
			}

			select {
			case <-tk.C:
			case <-ch:
				tk.Stop()
				return
			}
		}
	}()
	return stop
}

func ServeVideo(resp http.ResponseWriter, req *http.Request) {
	if req.URL.RawQuery == "" {
		serveList(resp, req)
		return
	}
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

func serveList(resp http.ResponseWriter, req *http.Request) {
	b, valid := videoList.Get(0)
	if valid {
		resp.Write(b.([]byte))
	} else {
		resp.WriteHeader(http.StatusServiceUnavailable)
	}
}
