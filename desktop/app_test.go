package desktop

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"testing"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/trafficlog-flashlight/tlproc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: delete me
func TestTLInstall(t *testing.T) {
	path := trafficlogPathToExecutable[common.Platform]
	fmt.Println("path:", path)
	require.NotEmpty(t, path)
	u, err := user.Current()
	require.NoError(t, err)
	fmt.Println("user:", u.Username)
	require.NoError(t, tlproc.Install(path, u.Username, trafficlogInstallPrompt, trafficlogInstallIcon, false))
}

func TestLocalHTTPToken(t *testing.T) {
	// Avoid polluting real settings.
	tmpfile, err := ioutil.TempFile("", "TestLocalHTTPToken")
	if err != nil {
		t.Errorf("Could not create temp file %v", err)
	}

	defer os.Remove(tmpfile.Name()) // clean up

	//ui.Start(":", "", "", "", func() bool { return true })
	set := loadSettingsFrom("1", "1/1/1", "1/1/1", tmpfile.Name(), newChromeExtension())

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
