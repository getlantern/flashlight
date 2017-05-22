package shortcut

import (
	"testing"

	"github.com/getlantern/shortcut"
	"github.com/stretchr/testify/assert"
)

var (
	cnIP       = "59.50.0.1"
	loadedType = shortcut.New([]string{}, []string{})
)

func TestConfigureThenEnable(t *testing.T) {
	enable := false
	sc := New(func() bool { return enable })
	assert.IsType(t, nullShortcut{}, sc.sc, "should be disabled by default")
	assert.False(t, sc.Allow(cnIP), "should not allow any address when disabled")
	sc.Configure("cn")
	assert.IsType(t, loadedType, sc.sc, "should load the shortcut list")
	assert.False(t, sc.Allow(cnIP), "should not allow any address when disabled")
	enable = true
	assert.IsType(t, loadedType, sc.sc, "should enable")
	assert.True(t, sc.Allow(cnIP), "should allow address in the list")
}
func TestEnableThenConfigure(t *testing.T) {
	enable := false
	sc := New(func() bool { return enable })
	assert.IsType(t, nullShortcut{}, sc.sc, "should be disabled by default")
	assert.False(t, sc.Allow(cnIP), "should not allow any address when disabled")
	enable = true
	assert.IsType(t, nullShortcut{}, sc.sc, "should still be disabled if not configured")
	assert.False(t, sc.Allow(cnIP), "should not allow any address when disabled")
	sc.Configure("cn")
	assert.IsType(t, loadedType, sc.sc, "should enable")
	assert.True(t, sc.Allow(cnIP), "should allow address in the list")
}
