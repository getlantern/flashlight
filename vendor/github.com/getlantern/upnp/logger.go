package upnp

import "log"

var (
	Log = StdLogger{}
)

type Logger interface {
	Infof(format string, a ...any)
	Errorf(format string, a ...any)
}

type NoLogger struct{}

func (l NoLogger) Infof(format string, a ...any)  {}
func (l NoLogger) Errorf(format string, a ...any) {}

type StdLogger struct{}

func (l StdLogger) Infof(format string, a ...any)  { log.Printf(format, a...) }
func (l StdLogger) Errorf(format string, a ...any) { log.Printf(format, a...) }
