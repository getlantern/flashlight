package logging

import (
	"testing"
)

func TestLogging(t *testing.T) {
	log := LoggerFor("test-logging")
	log.Debug("test")
}
