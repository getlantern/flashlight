package ui

import (
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/getlantern/replica"
)

type replicaHttpServer struct {
	// This is the S3 key prefix used to group uploads for listing.
	instancePrefix string
}

func (me replicaHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("replica server request path: %q", r.URL.Path)
	switch r.URL.Path {
	case "/upload":
		u, err := uuid.NewRandom()
		if err != nil {
			panic(err)
		}
		s3Key := fmt.Sprintf("/%s/%s/%s", me.instancePrefix, u.String(), r.FormValue("name"))
		log.Debugf("uploading replica key %q", s3Key)
		err = replica.Upload(r.Body, s3Key)
		if err != nil {
			panic(err)
		}
		t, err := replica.GetObjectTorrent(s3Key)
		if err != nil {
			panic(err)
		}
		defer t.Close()
		w.Header().Set("Content-Type", "application/x-bittorrent")
		io.Copy(w, t)
	default:
		http.NotFound(w, r)
	}
}
