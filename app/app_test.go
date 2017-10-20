package app

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUIFilter(t *testing.T) {
	a := &App{
		Flags: make(map[string]interface{}),
	}
	a.Flags["ui-domain"] = "test.lantern.io"
	a.Init()

	uiFilter := a.uiFilter()

	u, _ := url.Parse("http://test.lantern.io")
	r := &http.Request{
		Host: "test",
		URL:  u,
	}

	_, err := uiFilter(r)
	assert.NoError(t, err)

	uiFilter = a.uiFilterWithAddr(func() string {
		return ""
	})
	rr, err := uiFilter(r)

	assert.NoError(t, err)
	assert.Equal(t, "", rr.Host)

	r.Method = http.MethodConnect
	rr, err = uiFilter(r)

	assert.NoError(t, err)
	assert.Equal(t, "", rr.Host)
	assert.Equal(t, "", rr.URL.Host)
}

func TestLocalHTTPToken(t *testing.T) {
	// Avoid polluting real settings.
	tmpfile, err := ioutil.TempFile("", "TestLocalHTTPToken")
	if err != nil {
		t.Errorf("Could not create temp file %v", err)
	}

	defer os.Remove(tmpfile.Name()) // clean up

	//ui.Start(":", "", "", "", func() bool { return true })
	set := loadSettingsFrom("1", "1/1/1", "1/1/1", tmpfile.Name())

	// Just make sure we correctly set the token.
	set.SetLocalHTTPToken("fdada")
	tok := localHTTPToken(set)
	assert.Equal(t, "fdada", tok, "Unexpected token")
	assert.True(t, len(tok) > 4, "Unexpected token length for tok: "+tok)
	assert.Equal(t, tok, set.GetLocalHTTPToken(), "Unexpected token")

	// If the token is blank, it should generate and save a new one.
	set.SetLocalHTTPToken("")
	tok = localHTTPToken(set)
	assert.NotEqual(t, "fdada", tok, "Unexpected token")
	assert.True(t, len(tok) > 4, "Unexpected token length for tok: "+tok)
	assert.Equal(t, tok, set.GetLocalHTTPToken(), "Unexpected token")
}
