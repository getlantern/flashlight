package bbr

import (
	"net/http"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("flashlight.bbr")
)

// OnResponse extracts bbr-related information from a response
func OnResponse(resp *http.Response) *http.Response {
	_abe := resp.Header.Get("X-Bbr-Abe")
	if _abe != "" {
		resp.Header.Del("X-Bbr-Abe")
		abe, err := strconv.ParseFloat(_abe, 64)
		if err == nil {
			log.Debugf("X-BBR-ABE: %.2f Mbps", abe)
		}
	}
	_sent := resp.Header.Get("X-Bbr-Sent")
	if _sent != "" {
		resp.Header.Del("X-Bbr-Sent")
		sent, err := strconv.Atoi(_sent)
		if err == nil {
			log.Debugf("X-BBR-Sent: %v", humanize.Bytes(uint64(sent)))
		}
	}
	return resp
}
