//go:build cronet
// +build cronet

package chained

import (
	"context"
	"net"

	"github.com/getlantern/common/config"
	"github.com/getlantern/cromagnon"
	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/ops"
	"github.com/getlantern/keyman"
)

type cronetImpl struct {
	reportDialCore reportDialCoreFn
	dialer         *cromagnon.Client
}

func newCronetImpl(name, addr string, pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {

	path := ptSetting(pc, "path")
	noVerify := ptSettingBool(pc, "skipVerify")
	useH3 := ptSettingBool(pc, "http3")

	cert, err := keyman.LoadCertificateFromPEMBytes([]byte(pc.Cert))
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}
	pinnedCert := cert.X509()

	options := &cromagnon.ClientOptions{
		Addr:               addr,
		Path:               path,
		PinnedCert:         pinnedCert,
		InsecureSkipVerify: noVerify,
		UseH3:              useH3,
	}

	client, err := cromagnon.NewClient(options)
	if err != nil {
		return nil, log.Error(errors.Wrap(err).With("addr", addr))
	}

	return &cronetImpl{
		reportDialCore: reportDialCore,
		dialer:         client,
	}, nil
}

func (impl *cronetImpl) close() {
	log.Debug("Closing cronet session: Proxy closed.")
	impl.dialer.Close()
}

func (impl *cronetImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	return impl.reportDialCore(op, func() (net.Conn, error) {
		conn, err := impl.dialer.Dial()
		if err != nil {
			log.Debugf("Failed to establish cronet connection: %s", err)
		} else {
			log.Debug("established new cronet connection.")
		}
		return conn, err
	})
}
