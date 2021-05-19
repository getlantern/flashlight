package chained

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"net"

	"golang.org/x/crypto/ssh"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/netx"
	"github.com/getlantern/ossh"
)

type osshImpl struct {
	nopCloser
	reportDialCore reportDialCoreFn
	addr           string
	cfg            ossh.DialerConfig
}

func newOSSHImpl(addr string, s *ChainedServerInfo, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	keyword := s.ptSetting("ossh_obfuscation_keyword")
	if keyword == "" {
		return nil, errors.New("obfuscation keyword must be configured")
	}
	keyPEM := s.ptSetting("ossh_server_public_key")
	if keyPEM == "" {
		return nil, errors.New("server public key must be configured")
	}
	keyBlock, rest := pem.Decode([]byte(keyPEM))
	if len(rest) > 0 {
		return nil, errors.New("failed to decode server public key as PEM block")
	}
	if keyBlock.Type != "RSA PUBLIC KEY" {
		return nil, errors.New("expected key block of type 'RSA PUBLIC KEY', got %v", keyBlock.Type)
	}
	rsaKey, err := x509.ParsePKCS1PublicKey(keyBlock.Bytes)
	if err != nil {
		return nil, errors.New("failed to parse server public key as PKCS1: %v", err)
	}
	sshKey, err := ssh.NewPublicKey(rsaKey)
	if err != nil {
		return nil, errors.New("failed to convert RSA key to SSH key: %v", err)
	}
	cfg := ossh.DialerConfig{ObfuscationKeyword: keyword, ServerPublicKey: sshKey}

	return &osshImpl{reportDialCore: reportDialCore, addr: addr, cfg: cfg}, nil
}

func (impl *osshImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	tcpConn, err := impl.reportDialCore(op, func() (net.Conn, error) {
		return netx.DialContext(ctx, "tcp", impl.addr)
	})
	if err != nil {
		return nil, errors.New("failed to dial TCP: %v", err)
	}
	return ossh.Client(tcpConn, impl.cfg), nil
}
