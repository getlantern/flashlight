package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoCache(t *testing.T) {
	var rw httptest.ResponseRecorder
	Start(":0", false, "")
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(&rw, req)
	assert.Equal(t, "no-cache, no-store, must-revalidate", rw.HeaderMap.Get("Cache-Control"))
}
