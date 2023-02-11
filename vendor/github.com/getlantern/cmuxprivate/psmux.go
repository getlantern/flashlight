package cmuxprivate

import (
	"net"

	"github.com/getlantern/cmux/v2"
	"github.com/getlantern/psmux"
)

type psmuxSession struct{ session *psmux.Session }

func (s *psmuxSession) AcceptStream() (net.Conn, error) { return s.session.AcceptStream() }
func (s *psmuxSession) OpenStream() (net.Conn, error)   { return s.session.OpenStream() }
func (s *psmuxSession) Close() error                    { return s.session.Close() }
func (s *psmuxSession) NumStreams() int                 { return s.session.NumStreams() }

type psmuxProtocol struct {
	config *psmux.Config
}

// NewPsmuxProtocol creates a new psmux based Protocol using
// the psmux configuration givien.  If config is nil, the
// default psmux configuration is used.
func NewPsmuxProtocol(config *psmux.Config) *psmuxProtocol {
	if config == nil {
		config = psmux.DefaultConfig()
	}
	return &psmuxProtocol{config: config}
}

func (s *psmuxProtocol) Client(conn net.Conn) (cmux.Session, error) {
	session, err := psmux.Client(conn, s.config)
	return &psmuxSession{session}, err
}

func (s *psmuxProtocol) Server(conn net.Conn) (cmux.Session, error) {
	session, err := psmux.Server(conn, s.config)
	return &psmuxSession{session}, err
}

func (s *psmuxProtocol) TranslateError(err error) error {
	return err
}
