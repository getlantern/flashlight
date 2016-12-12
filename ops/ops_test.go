package ops

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func TestRequest(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodHead, http.MethodConnect, http.MethodConnect}
	protos := []string{"http", "http", "https", "http", "http"}
	hosts := []string{"www.duckduckgo.com", "www.duckduckgo.com:8080", "www.duckduckgo.com:443", "www.duckduckgo.com:443", "www.duckduckgo.com"}
	expectedPorts := []string{"80", "8080", "443", "443", ""}

	var ctxs []map[string]interface{}
	RegisterReporter(func(failure error, ctx map[string]interface{}) {
		ctxs = append(ctxs, ctx)
	})

	for i, method := range methods {
		proto := protos[i]
		host := hosts[i]
		req, _ := http.NewRequest(method, fmt.Sprintf("%v://%v", proto, host), nil)
		op := Begin("test").Request(req)
		op.End()
	}

	if assert.Len(t, ctxs, len(methods)) {
		for i, ctx := range ctxs {
			method := methods[i]
			host := hosts[i]
			port := expectedPorts[i]
			assert.Equal(t, method, ctx["http_method"])
			assert.Equal(t, "HTTP/1.1", ctx["http_proto"])
			assert.Equal(t, host, ctx["origin"])
			assert.Equal(t, port, ctx["origin_port"])
		}
	}
}
