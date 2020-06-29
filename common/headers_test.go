package common

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessCORS(t *testing.T) {

	// CORS headers in response to localhost origin
	resp := &http.Response{Header: http.Header{}}
	r, err := http.NewRequest(http.MethodGet, "http://test.com", nil)
	assert.NoError(t, err)
	origin := "http://localhost:47899"
	r.Header.Set("Origin", origin)
	ProcessCORS(resp.Header, r)
	assert.Equal(t, origin, resp.Header.Get("Access-Control-Allow-Origin"))

	first := resp.Header.Values("Access-Control-Allow-Methods")[0]
	second := resp.Header.Values("Access-Control-Allow-Methods")[1]
	assert.True(t, first == "GET" || first == "POST")
	assert.True(t, second == "GET" || second == "POST")

	// CORS headers in response to 127.0.0.1 origin
	resp = &http.Response{Header: http.Header{}}
	r, err = http.NewRequest(http.MethodGet, "http://test.com", nil)
	assert.NoError(t, err)
	origin = "http://127.0.0.1:47899"
	r.Header.Set("Origin", origin)
	ProcessCORS(resp.Header, r)

	fmt.Printf("methods: %v\n", resp.Header.Values("Access-Control-Allow-Methods"))
	fmt.Printf("methods: %v\n", resp.Header.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, origin, resp.Header.Get("Access-Control-Allow-Origin"))
	first = resp.Header.Values("Access-Control-Allow-Methods")[0]
	second = resp.Header.Values("Access-Control-Allow-Methods")[1]
	assert.True(t, first == "GET" || first == "POST")
	assert.True(t, second == "GET" || second == "POST")

	// No CORS headers in response to public origin
	resp = &http.Response{Header: http.Header{}}
	r, err = http.NewRequest(http.MethodGet, "http://test.com", nil)
	assert.NoError(t, err)
	origin = "http://publicdomain:47899"
	r.Header.Set("Origin", origin)
	ProcessCORS(resp.Header, r)
	assert.Equal(t, "", resp.Header.Get("Access-Control-Allow-Origin"))

	// No CORS headers in response to no origin
	resp = &http.Response{Header: http.Header{}}
	r, err = http.NewRequest(http.MethodGet, "http://test.com", nil)
	assert.NoError(t, err)
	ProcessCORS(resp.Header, r)
	assert.Equal(t, "", resp.Header.Get("Access-Control-Allow-Origin"))
}
