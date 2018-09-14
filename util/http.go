package util

import (
	"net/http"
	"net/http/httputil"

	log "github.com/sirupsen/logrus"
)

func DumpRequest(req *http.Request) {
	dump, err := httputil.DumpRequest(req, true)
	if err != nil {
		log.Error(err)
		return
	}

	log.Infof("%q", dump)
}

func DumpResponse(resp *http.Response) {
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		log.Error(err)
		return
	}

	log.Infof("%q", dump)
}
