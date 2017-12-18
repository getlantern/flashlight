package videoserver

import (
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
	"github.com/getlantern/ipfswrapper"

	"github.com/getlantern/flashlight/loconf"
)

var (
	log       = golog.LoggerFor("flashlight.videoserver")
	videoList = eventual.NewValue()
	ipfsNode  *ipfswrapper.Node
)

func FetchLoop() (func(), error) {
	repoDir := filepath.Join(appdir.General("Lantern"), "ipfs-repo")
	node, err := ipfswrapper.Start(repoDir, "")
	if err != nil {
		return func() {}, err
	}
	ipfsNode = node
	tk := time.NewTicker(1 * time.Hour)
	ch := make(chan bool)
	stop := func() {
		ipfsNode.Stop()
		ch <- true
	}
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
	return stop, nil
}

func ServeVideo(resp http.ResponseWriter, req *http.Request) {
	origin := req.Header.Get("Origin")
	resp.Header().Set("Access-Control-Allow-Origin", origin)
	if req.Method == "OPTIONS" {
		resp.Header().Set("Connection", "keep-alive")
		resp.Header().Set("Access-Control-Allow-Origin", origin)
		resp.Header().Set("Access-Control-Allow-Methods", "GET")
		resp.Header().Set("Access-Control-Allow-Headers", req.Header.Get("Access-Control-Request-Headers"))
		resp.Header().Set("Via", "Lantern Client")
		resp.Write([]byte("preflight complete"))
		return
	}

	videoHash := strings.Split(path.Base(req.URL.Query().Get("v")), ".")[0]
	if videoHash == "" {
		serveList(resp, req)
		return
	}

	log.Debugf("Serving video: %v", videoHash)
	reader, err := ipfsNode.GetFile("/ipfs/" + videoHash)
	if err != nil {
		log.Errorf("Error reading %v from ipfs: %v", videoHash, err)
		resp.WriteHeader(http.StatusInternalServerError)
	} else {
		n, err := io.Copy(resp, reader)
		if err != nil {
			log.Errorf("Error reading %v from ipfs: %v", videoHash, err)
			resp.WriteHeader(http.StatusInternalServerError)
		} else {
			log.Debugf("Served %d bytes for video %v", n, videoHash)
		}
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
