package ios

import (
	"github.com/getlantern/flashlight/ios/logger"
	"github.com/getlantern/golog"
)

var (
	log      = golog.LoggerFor("ios")
	statsLog = golog.LoggerFor("ios.stats")
	swiftLog = golog.LoggerFor("ios.swift")
)

// ConfigureFileLogging configures logging to log to files at the given fullLogFilePath.
func ConfigureFileLogging(fullLogFilePath string) error {
	return logger.ConfigureFileLogging(fullLogFilePath)
}

// LogDebug logs the given msg to the swift logger at debug level
func LogDebug(msg string) {
	swiftLog.Debug(msg)
}

// LogError logs the given msg to the swift logger at error level
func LogError(msg string) {
	swiftLog.Error(msg)
}
