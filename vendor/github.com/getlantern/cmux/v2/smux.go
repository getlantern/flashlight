package cmux

import (
	"net"

	"github.com/xtaci/smux"
)

// NewSmuxProtocol creates a new smux based Protocol using
// the smux configuration givien.  If config is nil, the
// default smux configuration is used.
func NewSmuxProtocol(config *smux.Config) *smuxProtocol {
	if config == nil {
		config = smux.DefaultConfig()
	}
	return &smuxProtocol{config: config}
}

type smuxProtocol struct {
	config *smux.Config
}

func (s *smuxProtocol) Client(conn net.Conn) (Session, error) {
	session, err := smux.Client(conn, s.config)
	return &smuxSession{session}, err
}

func (s *smuxProtocol) Server(conn net.Conn) (Session, error) {
	session, err := smux.Server(conn, s.config)
	return &smuxSession{session}, err
}

func (s *smuxProtocol) TranslateError(err error) error {
	if err == smux.ErrTimeout {
		return ErrTimeout
	} else {
		return err
	}
}

type smuxSession struct{ session *smux.Session }

func (s *smuxSession) AcceptStream() (net.Conn, error) { return s.session.AcceptStream() }
func (s *smuxSession) OpenStream() (net.Conn, error)   { return s.session.OpenStream() }
func (s *smuxSession) Close() error                    { return s.session.Close() }
func (s *smuxSession) NumStreams() int                 { return s.session.NumStreams() }
