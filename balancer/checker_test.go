package balancer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddCheckTarget(t *testing.T) {
	assert.Equal(t, 4, len(checkTargets.top(10)))

	AddCheckTarget("newsite.org:80")

	assert.Equal(t, 5, len(checkTargets.top(10)))

	// Test no port
	AddCheckTarget("newsite.org")
	assert.Equal(t, 5, len(checkTargets.top(10)))

	// Test bad port
	AddCheckTarget("newsite.org:443")
	assert.Equal(t, 5, len(checkTargets.top(10)))

	// Test internal
	AddCheckTarget("getiantem.org:443")
	assert.Equal(t, 5, len(checkTargets.top(10)))

	// Test failed check.
	checkTargets.checkFailed("http://newsite.org:80/index.html")

	assert.Equal(t, 4, len(checkTargets.top(10)))

}
