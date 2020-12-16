package ops

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/getlantern/ops"
	"github.com/stretchr/testify/assert"
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
			assert.Equal(t, strings.Split(host, ":")[0], ctx["origin_host"])
			assert.Equal(t, port, ctx["origin_port"])
		}
	}
}

func TestProxyName(t *testing.T) {
	runTest := func(expectMatch bool, name string, expectedName, expectedDatacenter string) {
		op := Begin("testop").ProxyName(name)
		ctx := ops.AsMap(op, false)
		op.End()
		if !expectMatch {
			assert.Empty(t, ctx["proxy_name"])
			assert.Empty(t, ctx["dc"])
		} else {
			assert.Equal(t, expectedName, ctx["proxy_name"])
			assert.Equal(t, expectedDatacenter, ctx["dc"])
		}
	}

	// We don't really have these kinds of proxy names anymore
	// runTest(true, "fp-https-donyc3-20180101-006-kcp", "fp-https-donyc3-20180101-006", "donyc3")
	// runTest(true, "fp-obfs4-donyc3-20160715-005", "fp-obfs4-donyc3-20160715-005", "donyc3")
	runTest(true, "fp-donyc3-20180101-006-kcp", "fp-donyc3-20180101-006", "donyc3")
	runTest(true, "fp-donyc3-20180101-006", "fp-donyc3-20180101-006", "donyc3")
	// For performance reasons, we don't bother checking that the name is totally valid
	// runTest(false, "fp-14325-adsfds-006", "", "")
	runTest(false, "cloudcompile", "", "")
}
