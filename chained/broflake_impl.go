package chained

import (
	"context"
	"net"
	"time"

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

	// Override BroflakeOptions defaults as applicable
	bo := clientcore.NewDefaultBroflakeOptions()

	if cTableSize := ptSettingInt(pc, "broflake_ctablesize"); cTableSize != 0 {
		bo.CTableSize = cTableSize
	}

	if pTableSize := ptSettingInt(pc, "broflake_ptablesize"); pTableSize != 0 {
		bo.PTableSize = pTableSize
	}

	if busBufferSz := ptSettingInt(pc, "broflake_busbuffersz"); busBufferSz != 0 {
		bo.BusBufferSz = busBufferSz
	}

	if netstated := ptSetting(pc, "broflake_netstated"); netstated != "" {
		bo.Netstated = netstated
	}

	// Override WebRTCOptions defaults as applicable
	wo := clientcore.NewDefaultWebRTCOptions()

	if discoverySrv := ptSetting(pc, "broflake_discoverysrv"); discoverySrv != "" {
		wo.DiscoverySrv = discoverySrv
	}

	if endpoint := ptSetting(pc, "broflake_endpoint"); endpoint != "" {
		wo.Endpoint = endpoint
	}

	if genesisAddr := ptSetting(pc, "broflake_genesisaddr"); genesisAddr != "" {
		wo.GenesisAddr = genesisAddr
	}

	// XXX: config.ProxyConfig pluggabletransportsettings do not support serialization of rich types like
	// time.Duration. Consequently, we're somewhat riskily rehydrating our two timeout values here by
	// assuming that the coefficient is time.Second. Beware!

	if NATFailTimeout := ptSettingInt(pc, "broflake_natfailtimeout"); NATFailTimeout != 0 {
		wo.NATFailTimeout = time.Duration(NATFailTimeout) * time.Second
	}

	if ICEFailTimeout := ptSettingInt(pc, "broflake_icefailtimeout"); ICEFailTimeout != 0 {
		wo.ICEFailTimeout = time.Duration(ICEFailTimeout) * time.Second
	}

	if tag := ptSetting(pc, "broflake_tag"); tag != "" {
		wo.Tag = tag
	}

	// TODO: here we need to inject our custom STUNBatch function as applicable, where should that code live?

	qo := &clientcore.QUICLayerOptions{
		ServerName:         ptSetting(pc, "broflake_egress_server_name"),
		InsecureSkipVerify: ptSettingBool(pc, "broflake_egress_insecure_skip_verify"),
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

	return b.QUICLayer.DialContext(ctx)
}

func (b *broflakeImpl) close() {
	b.QUICLayer.Close()
	b.ui.Stop()
}
