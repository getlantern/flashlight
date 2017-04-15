package util

import (
	"net/http"
	"net/http/httputil"
)

func DumpRequest(resp *http.Response) {
	dump, err := httputil.DumpRequest(resp, true)
	if err != nil {
		log.Error(err)
		return
	}

	log.Debugf("%q", dump)
}

func DumpResponse(resp *http.Response, body bool) {
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		log.Error(err)
		return
	}

	log.Debugf("%q", dump)
}
