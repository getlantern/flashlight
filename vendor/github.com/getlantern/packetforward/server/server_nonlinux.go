// +build !linux

package server

import (
	"github.com/getlantern/errors"
)

// NewServer constructs a new unstarted packetforward Server. The server can be started by
// calling Serve(). On non-linux platforms, this server does nothing.
func NewServer(opts *Opts) (Server, error) {
	return nil, errors.New("unsupported platform (currently packetforward servers are only supported on linux)")
}
