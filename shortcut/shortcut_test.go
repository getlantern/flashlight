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
	sc := New()
	assert.IsType(t, nullShortcut{}, sc.sc, "should be disabled by default")
	assert.False(t, sc.Allow(cnIP), "should not allow any address when disabled")
	sc.Configure("cn")
	assert.IsType(t, loadedType, sc.configured, "should load the shortcut list")
	assert.IsType(t, nullShortcut{}, sc.sc, "should still be disabled")
	assert.False(t, sc.Allow(cnIP), "should not allow any address when disabled")
	sc.Enable(true)
	assert.IsType(t, loadedType, sc.sc, "should enable")
	assert.True(t, sc.Allow(cnIP), "should allow address in the list")
}
func TestEnableThenConfigure(t *testing.T) {
	sc := New()
	assert.IsType(t, nullShortcut{}, sc.sc, "should be disabled by default")
	assert.False(t, sc.Allow(cnIP), "should not allow any address when disabled")
	sc.Enable(true)
	assert.IsType(t, nullShortcut{}, sc.sc, "should still be disabled if not configured")
	assert.False(t, sc.Allow(cnIP), "should not allow any address when disabled")
	sc.Configure("cn")
	assert.IsType(t, loadedType, sc.configured, "should load the shortcut list")
	assert.IsType(t, loadedType, sc.sc, "should enable")
	assert.True(t, sc.Allow(cnIP), "should allow address in the list")
}
