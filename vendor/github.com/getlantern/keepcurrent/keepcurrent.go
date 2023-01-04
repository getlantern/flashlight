// Package keepcurrent periodically polls from the source and if it's changed,
// syncs with a set of destinations

package keepcurrent

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"time"
)

// ErrUnmodified is the error to signal that the source has not been modified
// since the last sync.
var ErrUnmodified = errors.New("unmodified")

// Source represents somewhere any data can be fetched from
type Source interface {
	// Fetch fetches the data from the source if modified since the designated time.
	Fetch(ifNewerThan time.Time) (io.ReadCloser, error)
}

// Sink represents somewhere the data can be written to
type Sink interface {
	UpdateFrom(io.Reader) error
	String() string
}

// Runner runs the logic to synchronizes data from the source to the sinks
type Runner struct {
	// If given, OnSourceError is called if there is any error fetching from
	// the source. tries is how many times has been tried and failed. It should
	// return the wait time before trying again, or zero to stop retrying.
	OnSourceError func(err error, tries int) time.Duration
	// If given, OnSinkError is called if there is any error writing to any of
	// the sinks. There is no retry logic as sinks are local and considered to
	// be more reliable than the source.
	OnSinkError func(sink Sink, err error)

	Validate func(data []byte) error

	source      Source
	sinks       []Sink
	lastUpdated time.Time
}

// New construct a runner which synchronizes data from one source to one or more sinks
func New(from Source, to ...Sink) *Runner {
	return NewWithValidator(func(data []byte) error { return nil }, from, to...)
}

// Like New but with a function that validates data before sending it to the sinks
func NewWithValidator(validate func(data []byte) error, from Source, to ...Sink) *Runner {
	return &Runner{
		OnSourceError: func(error, int) time.Duration { return 0 },
		OnSinkError:   func(Sink, error) {},
		Validate:      validate,
		source:        from,
		sinks:         to,
		lastUpdated:   time.Time{},
	}
}

// InitFrom synchronizes data from the given source to configured sinks.
func (runner *Runner) InitFrom(s Source) {
	if len(runner.sinks) == 0 {
		return
	}
	runner.syncOnce(s, nil)
}

// Start starts the loop to actually synchronizes data with given interval. It
// returns a function to stop the loop.
func (runner *Runner) Start(interval time.Duration) func() {
	if len(runner.sinks) == 0 {
		return func() {}
	}
	tk := time.NewTicker(interval)
	chStop := make(chan struct{})
	chStopped := make(chan struct{})
	go func() {
		for {
			runner.syncOnce(runner.source, chStop)
			select {
			case <-chStop:
				tk.Stop()
				close(chStopped)
				return
			case <-tk.C:
			}
		}
	}()
	return func() { close(chStop); <-chStopped }
}

func (runner *Runner) syncOnce(from Source, chStop chan struct{}) {
	var data []byte
	for tries := 1; ; tries++ {
		start := time.Now()
		rc, err := from.Fetch(runner.lastUpdated)
		if err == ErrUnmodified {
			return
		}
		if err == nil {
			// Read ahead to surface any error reading from the source
			data, err = ioutil.ReadAll(rc)
			rc.Close()
		}
		if err == nil {
			err = runner.Validate(data)
		}
		if err == nil {
			runner.lastUpdated = start
			break
		}
		d := runner.OnSourceError(err, tries)
		if d == 0 {
			return
		}
		select {
		case <-chStop:
			return
		case <-time.After(d):
		}
	}
	for _, s := range runner.sinks {
		if err := s.UpdateFrom(bytes.NewReader(data)); err != nil {
			runner.OnSinkError(s, err)
		}
	}
}

// ExpBackoff returns an OnSourceError handler which does exponential backoff
// starting with base, doubles for every retry, and stops retrying after 'stop'
// attempts.
func ExpBackoff(base time.Duration, stop int) func(err error, tries int) time.Duration {
	return ExpBackoffThenFail(base, stop, func(err error) {})
}

// ExpBackoffThenFail does the same as ExpBackoff but also calls the onFail
// callback when it stops retrying.
func ExpBackoffThenFail(base time.Duration, stop int, onFail func(err error)) func(err error, tries int) time.Duration {
	return func(err error, tries int) time.Duration {
		if tries >= stop {
			onFail(err)
			return 0
		}
		return base * (1 << (tries - 1))
	}
}
