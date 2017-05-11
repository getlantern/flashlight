package app

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/getlantern/flashlight/ui"
	"github.com/stretchr/testify/assert"
)

func TestLocalHTTPToken(t *testing.T) {
	// Avoid polluting real settings.
	tmpfile, err := ioutil.TempFile("", "TestLocalHTTPToken")
	if err != nil {
		t.Errorf("Could not create temp file %v", err)
	}

	defer os.Remove(tmpfile.Name()) // clean up

	ui.Start(":", false, "", "")
	defer ui.Stop()
	set := loadSettingsFrom("1", "1/1/1", "1/1/1", tmpfile.Name())

	app := &App{
		settings: set,
	}
	// Just make sure we correctly set the token.
	set.SetLocalHTTPToken("fdada")
	tok := app.localHTTPToken()
	assert.Equal(t, "fdada", tok, "Unexpected token")
	assert.True(t, len(tok) > 4, "Unexpected token length for tok: "+tok)
	assert.Equal(t, tok, set.GetLocalHTTPToken(), "Unexpected token")

	// If the token is blank, it should generate and save a new one.
	set.SetLocalHTTPToken("")
	tok = app.localHTTPToken()
	assert.NotEqual(t, "fdada", tok, "Unexpected token")
	assert.True(t, len(tok) > 4, "Unexpected token length for tok: "+tok)
	assert.Equal(t, tok, set.GetLocalHTTPToken(), "Unexpected token")
}
