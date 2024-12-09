// Copied from lantern-cloud: e57d588aa976ca9d1a8531c92972e20a7d1640f9
//
// This file should be kept in sync with lantern-cloud
package apipb

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	commonconfig "github.com/getlantern/common/config"
)

// Values configured uniformly on all clients. Note that we have no mechanism for updating these
// values for existing client-proxy assignments.
const (
	tlsClientHelloSplitting = false
	tlsClientHelloID        = "HelloBrowser"
)

// For flashlight, since it doesn't accept an empty string.
var defaultTLSSuites string

func init() {
	suites := make([]string, 0)
	for _, suite := range tls.CipherSuites() {
		suites = append(suites, fmt.Sprintf("0x%04x", suite.ID))
	}

	defaultTLSSuites = strings.Join(suites, ",")
}

// ProxyToLegacyConfig converts a ProxyConnectConfig to the legacy format
func ProxyToLegacyConfig(cfg *ProxyConnectConfig) (*commonconfig.ProxyConfig, error) {
	legacy := new(commonconfig.ProxyConfig)

	legacy.Name = cfg.Name
	legacy.Addr = fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port)
	legacy.Cert = string(cfg.CertPem)
	legacy.AuthToken = cfg.AuthToken
	legacy.Trusted = cfg.Trusted
	legacy.Bias = 9 // bias clients towards using lantern-cloud proxies

	if cfg.Location != nil {
		legacy.Location = &commonconfig.ProxyConfig_ProxyLocation{
			City:        cfg.Location.City,
			Country:     cfg.Location.Country,
			CountryCode: cfg.Location.CountryCode,
			Latitude:    cfg.Location.Latitude,
			Longitude:   cfg.Location.Longitude,
		}
	}

	legacy.Track = cfg.Track
	legacy.Region = "platinum" // TODO: is region required?

	switch pCfg := cfg.ProtocolConfig.(type) {
	case *ProxyConnectConfig_ConnectCfgTls:
		legacy.PluggableTransport = "https"
		// currently, lantern-cloud uses multiplexing for all https connections so we set the
		// multiplexed addr to the same as the addr
		legacy.MultiplexedAddr = legacy.Addr

		// TLS-level config
		legacy.TLSClientHelloSplitting = tlsClientHelloSplitting
		legacy.TLSClientHelloID = tlsClientHelloID
		legacy.TLSServerNameIndicator = pCfg.ConnectCfgTls.ServerNameIndicator

		ss, err := serializeTLSSessionState(pCfg.ConnectCfgTls.SessionState)
		if err != nil {
			return nil, fmt.Errorf("serialize TLS session error: %w", err)
		}

		legacy.TLSClientSessionState = ss
		legacy.PluggableTransportSettings = map[string]string{}

		if pCfg.ConnectCfgTls.TlsFrag != "" {
			legacy.PluggableTransportSettings["tlsfrag"] = pCfg.ConnectCfgTls.TlsFrag
		}

	case *ProxyConnectConfig_ConnectCfgTlsmasq:
		legacy.PluggableTransport = "tlsmasq"

		// TLS-level config
		legacy.TLSClientHelloSplitting = tlsClientHelloSplitting
		legacy.TLSClientHelloID = tlsClientHelloID

		tlsmasqCfg := pCfg.ConnectCfgTlsmasq

		tlsmasqOrigin, _, err := net.SplitHostPort(tlsmasqCfg.OriginAddr)
		if err != nil {
			return nil, fmt.Errorf("bad tlsmasq origin addr: %w", err)
		}

		tlsmasqSuites := strings.Join(tlsmasqCfg.TlsSupportedCipherSuites, ",")

		// An empty list should indicate the default, but flashlight doesn't
		// like that.
		if len(tlsmasqCfg.TlsSupportedCipherSuites) == 0 {
			tlsmasqSuites = defaultTLSSuites
		}

		legacy.PluggableTransportSettings = map[string]string{
			"tlsmasq_secret":        hex.EncodeToString(tlsmasqCfg.Secret),
			"tlsmasq_sni":           tlsmasqOrigin,
			"tlsmasq_tlsminversion": tlsmasqCfg.TlsMinVersion,
			"tlsmasq_suites":        tlsmasqSuites,
		}

		if pCfg.ConnectCfgTlsmasq.TlsFrag != "" {
			legacy.PluggableTransportSettings["tlsfrag"] = pCfg.ConnectCfgTlsmasq.TlsFrag
		}

	case *ProxyConnectConfig_ConnectCfgShadowsocks:
		legacy.PluggableTransport = "shadowsocks"
		legacy.MultiplexedAddr = legacy.Addr

		ssCfg := pCfg.ConnectCfgShadowsocks

		legacy.PluggableTransportSettings = map[string]string{
			"shadowsocks_secret":           ssCfg.Secret,
			"shadowsocks_cipher":           ssCfg.Cipher,
			"shadowsocks_prefix_generator": ssCfg.PrefixGenerator,
			"shadowsocks_with_tls":         strconv.FormatBool(ssCfg.WithTls),
		}

	case *ProxyConnectConfig_ConnectCfgBroflake:
		legacy.PluggableTransport = "broflake"
		legacy.Addr = "broflake"

		bbCfg := pCfg.ConnectCfgBroflake
		legacy.StunServers = bbCfg.StunServers

		legacy.PluggableTransportSettings = map[string]string{
			"broflake_ctablesize":                  strconv.Itoa(int(bbCfg.CtableSize)),
			"broflake_ptablesize":                  strconv.Itoa(int(bbCfg.PtableSize)),
			"broflake_natfailtimeout":              strconv.Itoa(int(bbCfg.NatFailTimeout)),
			"broflake_icefailtimeout":              strconv.Itoa(int(bbCfg.IceFailTimeout)),
			"broflake_discoverysrv":                bbCfg.DiscoverySrv,
			"broflake_endpoint":                    bbCfg.Endpoint,
			"broflake_egress_server_name":          bbCfg.EgressServerName,
			"broflake_egress_insecure_skip_verify": strconv.FormatBool(bbCfg.EgressInsecureSkipVerify),
			"broflake_egress_ca":                   bbCfg.EgressCa,
			"broflake_stunbatchsize":               strconv.Itoa(int(bbCfg.StunBatchSize)),
		}

		legacy.Bias = 10                  // bias clients toward broflake
		legacy.AllowedDomains = []string{ // currently, we only send ad traffic through Broflake
			"doubleclick.net",
			"adservice.google.com",
			"adservice.google.com.hk",
			"adservice.google.co.jp",
			"adservice.google.nl",
			"googlesyndication.com",
			"googletagservices.com",
			"googleadservices.com",
		}

	case *ProxyConnectConfig_ConnectCfgStarbridge:
		legacy.PluggableTransport = "starbridge"

		legacy.PluggableTransportSettings = map[string]string{
			"starbridge_public_key": pCfg.ConnectCfgStarbridge.PublicKey,
		}

	case *ProxyConnectConfig_ConnectCfgAlgeneva:
		legacy.PluggableTransport = "algeneva"
		legacy.PluggableTransportSettings = map[string]string{
			"algeneva_strategy": pCfg.ConnectCfgAlgeneva.Strategy,
		}
	case *ProxyConnectConfig_ConnectCfgWater:
		legacy.PluggableTransport = "water"
		duration, err := time.ParseDuration(pCfg.ConnectCfgWater.DownloadTimeout.String())
		if err != nil {
			duration = 5 * time.Minute
		}
		legacy.PluggableTransportSettings = map[string]string{
			"water_wasm":        base64.StdEncoding.EncodeToString(pCfg.ConnectCfgWater.Wasm),
			"water_transport":   pCfg.ConnectCfgWater.Transport,
			"wasm_available_at": strings.Join(pCfg.ConnectCfgWater.WasmAvailableAt, ","),
			"download_timeout":  duration.String(),
		}

	default:
		return nil, fmt.Errorf("unsupported protocol config: %T", cfg.ProtocolConfig)
	}

	return legacy, nil
}

// serializeTLSSessionState serializes the input TLS session state. This format is the one expected
// by clients in the legacy config. When we move away from the legacy config, clients can just read
// the session state out of the protcol buffers message.
func serializeTLSSessionState(ss *ProxyConnectConfig_TLSConfig_SessionState) (string, error) {
	type sessionState struct {
		SessionTicket []uint8
		Vers          uint16
		CipherSuite   uint16
		MasterSecret  []byte
		SessionState  []byte
	}

	if ss.Version > math.MaxUint16 {
		return "", errors.New("invalid version")
	} else if ss.CipherSuite > math.MaxUint16 {
		return "", errors.New("invalid cipher suite")
	}

	b, err := json.Marshal(sessionState{
		SessionTicket: ss.SessionTicket,
		Vers:          uint16(ss.Version),
		CipherSuite:   uint16(ss.CipherSuite),
		MasterSecret:  ss.MasterSecret,
		SessionState:  ss.SessionState,
	})
	if err != nil {
		return "", fmt.Errorf("marshal error: %w", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}
