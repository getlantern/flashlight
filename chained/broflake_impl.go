package chained

import (
	"context"
	"math/rand"
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

	if STUNBatchSize := ptSettingInt(pc, "broflake_stunbatchsize"); STUNBatchSize != 0 {
		wo.STUNBatchSize = uint32(STUNBatchSize)
	}

	// XXX: STUN servers are handled in a subtly different way than the rest of our settings overrides,
	// because they're a nonscalar quantity passed via a different top level field in the ProxyConfig.
	// If (and only if) a list of STUN servers has been supplied in the ProxyConfig, we'll override
	// Broflake's default STUNBatch function.
	if srvs := pc.GetStunServers(); srvs != nil {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))

		wo.STUNBatch = func(size uint32) (batch []string, err error) {
			return getRandomSubset(size, rng, srvs)
		}
	}

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

// getRandomSubset is a helper for our custom STUNBatch function. It returns a 'size'-sized
// random subset of the strings in 'set'.
func getRandomSubset(size uint32, rng *rand.Rand, set []string) (batch []string, err error) {
	if size > uint32(len(set)) {
		size = uint32(len(set))
	}

	indices := rng.Perm(len(set))[:size]
	batch = make([]string, 0, len(indices))
	for _, i := range indices {
		batch = append(batch, set[i])
	}

	return batch, nil
}