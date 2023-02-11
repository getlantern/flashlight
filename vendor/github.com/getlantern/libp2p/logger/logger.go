package logger

import "log"

var Log Logger = StdLogger{}

type Logger interface {
	Printf(format string, a ...any)
	Infof(format string, a ...any)
	Errorf(format string, a ...any)
}

type NoLogger struct{}

func (l NoLogger) Printf(format string, a ...any) {}
func (l NoLogger) Infof(format string, a ...any)  {}
func (l NoLogger) Errorf(format string, a ...any) {}

type StdLogger struct{}

func (l StdLogger) Printf(format string, a ...any) { log.Printf(format+"\n", a...) }
func (l StdLogger) Infof(format string, a ...any)  { log.Printf(format+"\n", a...) }
func (l StdLogger) Errorf(format string, a ...any) { log.Printf(format+"\n", a...) }
