package chained

import (
	"context"
	"net"

	"github.com/getlantern/broflake/clientcore"
	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/ops"
)

type broflakeImpl struct {
	reportDialCore reportDialCoreFn // TODO: I don't know what this is for yet
	QUICLayer      *clientcore.QUICLayer
	ui             *clientcore.UIImpl
}

func newBroflakeImpl(pc *config.ProxyConfig, reportDialCore reportDialCoreFn) (proxyImpl, error) {
	// TODO: I don't know what the reportDialCoreFn is, and I'm not sure if I need to know. I'm
	// just imitating the function signature and approach of other impls...

	// TODO: extract config settings from ProxyConfig and override bo fields as applicable
	bo := clientcore.NewDefaultBroflakeOptions()

	// TODO: extract config settings from ProxyConfig and override wo fields as applicable
	wo := clientcore.NewDefaultWebRTCOptions()
	wo.DiscoverySrv = "https://bf-freddie.herokuapp.com"
	wo.Endpoint = "/v1/signal"

	// TODO: here we need to inject our custom STUNBatch function as applicable, where should that code live?

	// TODO: extract config settings from ProxyConfig and override these fields as applicable
	qo := &clientcore.QUICLayerOptions{
		ServerName:         "",
		InsecureSkipVerify: true,
	}

	// Construct, init, and start a Broflake client!
	bfconn, ui, err := clientcore.NewBroflake(bo, wo, nil)
	if err != nil {
		return nil, err
	}

	ql, err := clientcore.NewQUICLayer(bfconn, qo)
	if err != nil {
		return nil, err
	}

	ql.DialAndMaintainQUICConnection()

	return &broflakeImpl{
		reportDialCore: reportDialCore,
		QUICLayer:      ql,
		ui:             ui,
	}, nil
}

func (b *broflakeImpl) dialServer(op *ops.Op, ctx context.Context) (net.Conn, error) {
	// TODO: I don't know what to do with 'op'

	// TODO: it may or may not be necessary to wrap this dial in a CONNECT, a la the "Integrate
	// Broflake Redux" PR...
	return b.QUICLayer.DialContext(ctx)
}

func (b *broflakeImpl) close() {
	b.QUICLayer.Close()
	b.ui.Stop()
}
