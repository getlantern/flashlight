// Package tlsresumption provides utilities for implementing out of band sharing of client session
// states for tls session resumption.
package tlsresumption

import (
	"encoding/base64"
	"encoding/json"
	"net"

	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	utls "github.com/refraction-networking/utls"
)

var (
	log = golog.LoggerFor("tlsresumption")
)

// MakeClientSessionStates makes num client session states for connecting to the TLS server at the given
// address. It connections to the server and handshakes num times and returns as many client session states
// as it can successfully build, returning the latest error.
//
// Note, this does not verify the server's certificate so is potentially susceptible to MITM attacks.
func MakeClientSessionStates(addr string, num int) ([]string, error) {
	result := make([]string, 0, num)

	var finalErr error
	for i := 0; i < num; i++ {
		conn, err := dialUTLS(nil, addr)
		if err != nil {
			finalErr = err
			continue
		}
		ss := conn.HandshakeState.Session
		conn.Close()
		if ss == nil {
			finalErr = errors.New("no client session state found")
			continue
		}

		ssString, err := SerializeClientSessionState(ss)
		if err != nil {
			finalErr = err
			continue
		}

		result = append(result, ssString)
	}

	return result, finalErr
}

// ParseClientSessionState parses the serialized client session state into a utls.ClientSessionState
func ParseClientSessionState(serialized string) (*utls.ClientSessionState, error) {
	b, err := base64.StdEncoding.DecodeString(serialized)
	if err != nil {
		return nil, errors.New("unable to base64 decode serialized client session state: %v", err)
	}
	sss := &serializedClientSessionState{}
	err = json.Unmarshal(b, sss)
	if err != nil {
		return nil, err
	}
	ss := &utls.ClientSessionState{}
	ss.SetSessionTicket(sss.SessionTicket)
	ss.SetVers(sss.Vers)
	ss.SetCipherSuite(sss.CipherSuite)
	ss.SetMasterSecret(sss.MasterSecret)
	return ss, nil
}

func dialUTLS(ss *utls.ClientSessionState, addr string) (*utls.UConn, error) {
	cfg := &utls.Config{
		InsecureSkipVerify: true,
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, errors.New("unable to dial %v: %v", addr, err)
	}
	uconn := utls.UClient(conn, cfg, utls.HelloChrome_Auto)
	uconn.SetSessionState(ss)
	if handshakeErr := uconn.Handshake(); handshakeErr != nil {
		conn.Close()
		return nil, errors.New("error handshaking with %v: %v", addr, handshakeErr)
	}
	return uconn, nil
}

type serializedClientSessionState struct {
	SessionTicket []uint8
	Vers          uint16
	CipherSuite   uint16
	MasterSecret  []byte
}

// SerializeClientSessionState serializes a ClientSessionState into a string representation of the same.
func SerializeClientSessionState(ss *utls.ClientSessionState) (string, error) {
	sss := &serializedClientSessionState{
		SessionTicket: ss.SessionTicket(),
		Vers:          ss.Vers(),
		CipherSuite:   ss.CipherSuite(),
		MasterSecret:  ss.MasterSecret(),
	}

	b, err := json.Marshal(sss)
	if err != nil {
		return "", errors.New("unable to marshal client session state to JSON: %v", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}
