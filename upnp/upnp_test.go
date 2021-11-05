package upnp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortMapping(t *testing.T) {

	err := GetIPAndForwardPort(context.Background())
	assert.NoError(t, err)
}
