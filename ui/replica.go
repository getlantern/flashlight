package ui

import (
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/getlantern/replica"
)

type ReplicaHttpServer struct {
	// This is the S3 key prefix used to group uploads for listing.
	InstancePrefix string
}

type countWriter struct {
	bytesWritten int64
}

func (me *countWriter) Write(b []byte) (int, error) {
	me.bytesWritten += int64(len(b))
	return len(b), nil
}

func (me ReplicaHttpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("replica server request path: %q", r.URL.Path)
	switch r.URL.Path {
	case "/upload":
		u, err := uuid.NewRandom()
		if err != nil {
			panic(err)
		}
		s3Key := fmt.Sprintf("/%s/%s/%s", me.InstancePrefix, u.String(), r.URL.Query().Get("name"))
		log.Debugf("uploading replica key %q", s3Key)
		var cw countWriter
		err = replica.Upload(io.TeeReader(r.Body, &cw), s3Key)
		log.Debugf("upload read %d bytes", cw.bytesWritten)
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
