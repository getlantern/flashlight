package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestURLParam(t *testing.T) {
	// Test known error case.
	str := ":"
	val := SetURLParam(str, "key", "value")
	assert.Equal(t, str, val)

	str = "http://test.com/?why=now"

	val = SetURLParam(str, "key", "value")
	assert.Equal(t, "http://test.com/?key=value&why=now", val)
}
