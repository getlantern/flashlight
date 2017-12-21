package videoserver

import (
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/eventual"
	"github.com/getlantern/golog"
	"github.com/getlantern/ipfswrapper"

	"github.com/getlantern/flashlight/loconf"
)

var (
	log         = golog.LoggerFor("flashlight.videoserver")
	videoList   = eventual.NewValue()
	ipfsNode    *ipfswrapper.Node
	rangeRegexp = regexp.MustCompile("bytes=(\\d+)-(\\d*)")
)

func FetchLoop() (func(), error) {
	repoDir := filepath.Join(appdir.General("Lantern"), "ipfs-repo")
	node, err := ipfswrapper.Start(repoDir, "")
	if err != nil {
		return func() {}, err
	}
	ipfsNode = node
	tk := time.NewTimer(10 * time.Second)
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
				tk.Reset(1 * time.Second)
			} else {
				videoList.Set(b)
				tk.Reset(1 * time.Hour)
			}

			select {
			case <-tk.C:
				runtime.GC()
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

	var offset, end int
	partialContent := false
	if v := req.Header.Get("Range"); v != "" {
		r := rangeRegexp.FindStringSubmatch(v)
		if len(r) != 3 {
			resp.WriteHeader(http.StatusBadRequest)
			return
		}
		var econv error
		offset, econv = strconv.Atoi(r[1])
		end, _ = strconv.Atoi(r[2])
		if econv != nil || offset < 0 || (end > 0 && end <= offset) {
			resp.WriteHeader(http.StatusBadRequest)
			return
		}
		partialContent = true
	}

	dag, err := ipfsNode.Get(videoHash)
	if err != nil {
		log.Errorf("Error reading %v from ipfs: %v", videoHash, err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer dag.Close()
	var reader io.Reader = dag
	resp.Header().Set("Accept-Ranges", "bytes")
	if partialContent {
		if uint64(end) >= dag.Size() {
			resp.WriteHeader(http.StatusBadRequest)
			return
		}
		_, eseek := dag.Seek(int64(offset), io.SeekStart)
		if eseek != nil {
			log.Error(eseek)
			resp.WriteHeader(http.StatusInternalServerError)
			return
		}
		if end == 0 {
			end = offset + 1048576 - 1
		}
		if uint64(end) >= dag.Size() {
			end = int(dag.Size() - 1)
		}
		crange := fmt.Sprintf("bytes %d-%d/%d", offset, end, dag.Size())
		resp.Header().Set("Content-Range", crange)
		length := end - offset + 1
		resp.Header().Set("Content-Length", strconv.Itoa(length))
		reader = io.LimitReader(dag, int64(length))
		resp.WriteHeader(http.StatusPartialContent)
	}
	n, ecopy := io.Copy(resp, reader)
	if ecopy != nil {
		log.Errorf("Error reading %v from ipfs: %v", videoHash, ecopy)
		// at this point it can do nothing but silently return
	} else {
		log.Debugf("Served %d bytes (%d-%d/%d) for video %v",
			n, offset, end, dag.Size(), videoHash)
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
