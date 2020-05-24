package borda

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeDimensions(t *testing.T) {
	dims := map[string]interface{}{
		"error":      "string 127.0.0.1:8006 and stuff",
		"error_text": "string fe80::14fa:9793:de38:8320 and stuff",
		"other":      "127.0.0.1:8006 fe80::14fa:9793:de38:8320",
	}
	sanitizeDimensions(dims)
	assert.Equal(t, "string <addr> and stuff", dims["error"])
	assert.Equal(t, "string <addr> and stuff", dims["error_text"])
	assert.EqualValues(t, "127.0.0.1:8006 fe80::14fa:9793:de38:8320", dims["other"])
}
