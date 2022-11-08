package bbr

import (
	"io"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/getlantern/http-proxy-lantern/v2/common"
)

const (
	upstreamABEUnknown = 0
)

func doProbeUpstream(url string) float64 {
	resp, err := http.Get(url)
	if err != nil {
		log.Errorf("Unable to probe upstream at %v: %v", url, err)
		return 0
	}
	io.Copy(ioutil.Discard, resp.Body)
	_newUpstreamABE := resp.Trailer.Get(common.BBRAvailableBandwidthEstimateHeader)
	newUpstreamABE, err := strconv.ParseFloat(_newUpstreamABE, 64)
	if err != nil {
		log.Errorf("Unable to parse upstream ABE %v: %v", _newUpstreamABE, err)
		return upstreamABEUnknown
	}
	return newUpstreamABE
}
