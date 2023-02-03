package publicip

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

var (
	httpClient = &http.Client{
		Transport: httpTransport,
	}
	httpTransport = &http.Transport{
		DialContext: dialContext,
		// I guess we want to establish a new connection each time, since we want the remote to
		// reflect our latest IP.
		DisableKeepAlives: true,
	}
)

func fromHttp(ctx context.Context) (ip net.IP, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://icanhazip.com", nil)
	if err != nil {
		return
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	const max = 45 // The longest string representation for IPv4/IPv6.
	b := bytes.NewBuffer(make([]byte, 0, max))
	_, err = io.CopyN(b, resp.Body, max)
	if err == io.EOF {
		err = nil
	}
	// Probably should only trim trailing space, or our max char thing won't work.
	ip = net.ParseIP(strings.TrimSpace(b.String()))
	if ip != nil {
		err = nil
		return
	}
	if err == nil {
		err = fmt.Errorf("error parsing ip from %q", b.String())
	}
	return
}
