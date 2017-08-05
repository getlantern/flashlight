package pro

import (
	"encoding/base64"
	"testing"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewUser(t *testing.T) {
	deviceID := base64.StdEncoding.EncodeToString(uuid.NodeID())
	u, err := newUserWithClient(deviceID, nil)

	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")

	u, err = getUserDataWithClient(u.ID, u.Token, deviceID, nil)
	assert.NoError(t, err, "Unexpected error")
	assert.NotNil(t, u, "Should have gotten a user")
}
