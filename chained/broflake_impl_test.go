package chained

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/getlantern/broflake/clientcore"
	"github.com/getlantern/common/config"
	"github.com/stretchr/testify/assert"
)

func TestMakeBroflakeOptions(t *testing.T) {
	pc := &config.ProxyConfig{
		PluggableTransportSettings: map[string]string{
			"broflake_client_type":                 "foo",
			"broflake_ctablesize":                  "69",
			"broflake_ptablesize":                  "69",
			"broflake_busbuffersz":                 "31337",
			"broflake_netstated":                   "bar",
			"broflake_discoverysrv":                "baz",
			"broflake_endpoint":                    "qux",
			"broflake_genesisaddr":                 "quux",
			"broflake_natfailtimeout":              "420",
			"broflake_icefailtimeout":              "666",
			"broflake_tag":                         "garply",
			"broflake_stunbatchsize":               "911",
			"broflake_egress_server_name":          "waldo",
			"broflake_egress_insecure_skip_verify": "true",
		},
		StunServers: []string{
			"stun:123.456.789",
			"stun:127.0.0.1",
		},
	}

	// Ensure that supplied values make their way into the correct options structs
	bo, wo, qo := makeBroflakeOptions(pc)

	assert.Equal(t, pc.PluggableTransportSettings["broflake_client_type"], bo.ClientType)
	ctablesize, err := strconv.Atoi(pc.PluggableTransportSettings["broflake_ctablesize"])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, ctablesize, bo.CTableSize)
	ptablesize, err := strconv.Atoi(pc.PluggableTransportSettings["broflake_ptablesize"])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, ptablesize, bo.PTableSize)
	busbuffersz, err := strconv.Atoi(pc.PluggableTransportSettings["broflake_busbuffersz"])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, busbuffersz, bo.BusBufferSz)
	assert.Equal(t, pc.PluggableTransportSettings["broflake_netstated"], bo.Netstated)
	assert.Equal(t, pc.PluggableTransportSettings["broflake_discoverysrv"], wo.DiscoverySrv)
	assert.Equal(t, pc.PluggableTransportSettings["broflake_endpoint"], wo.Endpoint)
	assert.Equal(t, pc.PluggableTransportSettings["broflake_genesisaddr"], wo.GenesisAddr)
	natfailtimeout, err := strconv.Atoi(pc.PluggableTransportSettings["broflake_natfailtimeout"])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, time.Duration(natfailtimeout)*time.Second, wo.NATFailTimeout)
	icefailtimeout, err := strconv.Atoi(pc.PluggableTransportSettings["broflake_icefailtimeout"])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, time.Duration(icefailtimeout)*time.Second, wo.ICEFailTimeout)
	assert.Equal(t, pc.PluggableTransportSettings["broflake_tag"], wo.Tag)
	stunbatchsize, err := strconv.Atoi(pc.PluggableTransportSettings["broflake_stunbatchsize"])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, uint32(stunbatchsize), wo.STUNBatchSize)
	assert.Equal(t, pc.PluggableTransportSettings["broflake_egress_server_name"], qo.ServerName)
	insecureskipverify, err := strconv.ParseBool(pc.PluggableTransportSettings["broflake_egress_insecure_skip_verify"])
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, insecureskipverify, qo.InsecureSkipVerify)

	// Assert against the default options structs to be sure our test values != default values
	dbo := clientcore.NewDefaultBroflakeOptions()
	dwo := clientcore.NewDefaultWebRTCOptions()
	dqo := clientcore.QUICLayerOptions{}

	assert.NotEqual(t, pc.PluggableTransportSettings["broflake_client_type"], dbo.ClientType)
	ctablesize, err = strconv.Atoi(pc.PluggableTransportSettings["broflake_ctablesize"])
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, ctablesize, dbo.CTableSize)
	ptablesize, err = strconv.Atoi(pc.PluggableTransportSettings["broflake_ptablesize"])
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, ptablesize, dbo.PTableSize)
	busbuffersz, err = strconv.Atoi(pc.PluggableTransportSettings["broflake_busbuffersz"])
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, busbuffersz, dbo.BusBufferSz)
	assert.NotEqual(t, pc.PluggableTransportSettings["broflake_netstated"], dbo.Netstated)
	assert.NotEqual(t, pc.PluggableTransportSettings["broflake_discoverysrv"], dwo.DiscoverySrv)
	assert.NotEqual(t, pc.PluggableTransportSettings["broflake_endpoint"], dwo.Endpoint)
	assert.NotEqual(t, pc.PluggableTransportSettings["broflake_genesisaddr"], dwo.GenesisAddr)
	natfailtimeout, err = strconv.Atoi(pc.PluggableTransportSettings["broflake_natfailtimeout"])
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, time.Duration(natfailtimeout)*time.Second, dwo.NATFailTimeout)
	icefailtimeout, err = strconv.Atoi(pc.PluggableTransportSettings["broflake_icefailtimeout"])
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, time.Duration(icefailtimeout)*time.Second, dwo.ICEFailTimeout)
	assert.NotEqual(t, pc.PluggableTransportSettings["broflake_tag"], dwo.Tag)
	stunbatchsize, err = strconv.Atoi(pc.PluggableTransportSettings["broflake_stunbatchsize"])
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, uint32(stunbatchsize), dwo.STUNBatchSize)
	assert.NotEqual(t, pc.PluggableTransportSettings["broflake_egress_server_name"], dqo.ServerName)
	insecureskipverify, err = strconv.ParseBool(pc.PluggableTransportSettings["broflake_egress_insecure_skip_verify"])
	if !assert.NoError(t, err) {
		return
	}
	assert.NotEqual(t, insecureskipverify, dqo.InsecureSkipVerify)

	// Including > 0 STUN servers should cause the default STUNBatch function to get overridden
	// TODO: this is a bit of a funky "test by contradiction," it'd be more rigorous to refactor
	// things such that we could compare the function pointer of the function that IS present
	assert.NotEqual(t, fmt.Sprintf("%p", wo.STUNBatch), fmt.Sprintf("%p", clientcore.DefaultSTUNBatchFunc))

	// Ensure that unsupplied values result in options structs with default values
	dpc := &config.ProxyConfig{}
	bo, wo, qo = makeBroflakeOptions(dpc)

	assert.Equal(t, bo.ClientType, dbo.ClientType)
	assert.Equal(t, bo.CTableSize, dbo.CTableSize)
	assert.Equal(t, bo.PTableSize, dbo.PTableSize)
	assert.Equal(t, bo.BusBufferSz, dbo.BusBufferSz)
	assert.Equal(t, bo.Netstated, dbo.Netstated)
	assert.Equal(t, wo.DiscoverySrv, dwo.DiscoverySrv)
	assert.Equal(t, wo.Endpoint, dwo.Endpoint)
	assert.Equal(t, wo.GenesisAddr, dwo.GenesisAddr)
	assert.Equal(t, wo.NATFailTimeout, dwo.NATFailTimeout)
	assert.Equal(t, wo.ICEFailTimeout, dwo.ICEFailTimeout)
	assert.Equal(t, wo.Tag, dwo.Tag)
	assert.Equal(t, wo.STUNBatchSize, dwo.STUNBatchSize)
	assert.Equal(t, qo.ServerName, dqo.ServerName)
	assert.Equal(t, qo.InsecureSkipVerify, dqo.InsecureSkipVerify)

	// Supports our test by contradiction, establishing the function pointer for the default STUNBatch function
	assert.Equal(t, fmt.Sprintf("%p", wo.STUNBatch), fmt.Sprintf("%p", clientcore.DefaultSTUNBatchFunc))
}

func TestGetRandomSubset(t *testing.T) {
	listSize := 100
	uniqueStrings := make([]string, 0, listSize)
	for i := 0; i < listSize; i++ {
		uniqueStrings = append(uniqueStrings, fmt.Sprintf("foo%v", i))
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for subsetSize := 0; subsetSize < listSize+1; subsetSize++ {
		seen := make([]string, 0, subsetSize)
		subset := getRandomSubset(uint32(subsetSize), rng, uniqueStrings)

		for _, s := range subset {
			assert.Contains(t, uniqueStrings, s)
			assert.NotContains(t, seen, s)
			seen = append(seen, s)
		}
		assert.Equal(t, len(seen), subsetSize)
	}

	subset := getRandomSubset(uint32(listSize*10), rng, uniqueStrings)
	assert.Equal(t, len(subset), listSize)
	assert.ElementsMatch(t, subset, uniqueStrings)

	nullSet := []string{}
	subset = getRandomSubset(uint32(100), rng, nullSet)
	assert.Equal(t, len(subset), 0)
}
