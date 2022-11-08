package instrument

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/getlantern/geo"
	"github.com/getlantern/multipath"
	"github.com/getlantern/proxy/v2/filters"
)

// Instrument is the common interface about what can be instrumented.
type Instrument interface {
	WrapFilter(prefix string, f filters.Filter) filters.Filter
	WrapConnErrorHandler(prefix string, f func(conn net.Conn, err error)) func(conn net.Conn, err error)
	Blacklist(b bool)
	Mimic(m bool)
	MultipathStats([]string) []multipath.StatsTracker
	Throttle(m bool, reason string)
	XBQHeaderSent()
	SuspectedProbing(fromIP net.IP, reason string)
	VersionCheck(redirect bool, method, reason string)
	ProxiedBytes(sent, recv int, platform, version, app, dataCapCohort string, clientIP net.IP)
	TCPPackets(clientAddr string, sentDataPackets, retransmissions, consecRetransmissions int)
	quicSentPacket()
	quicLostPacket()
}

// NoInstrument is an implementation of Instrument which does nothing
type NoInstrument struct {
}

func (i NoInstrument) WrapFilter(prefix string, f filters.Filter) filters.Filter { return f }
func (i NoInstrument) WrapConnErrorHandler(prefix string, f func(conn net.Conn, err error)) func(conn net.Conn, err error) {
	return f
}
func (i NoInstrument) Blacklist(b bool) {}
func (i NoInstrument) Mimic(m bool)     {}
func (i NoInstrument) MultipathStats(protocols []string) (trackers []multipath.StatsTracker) {
	for _, _ = range protocols {
		trackers = append(trackers, multipath.NullTracker{})
	}
	return
}
func (i NoInstrument) Throttle(m bool, reason string) {}

func (i NoInstrument) XBQHeaderSent()                                    {}
func (i NoInstrument) SuspectedProbing(fromIP net.IP, reason string)     {}
func (i NoInstrument) VersionCheck(redirect bool, method, reason string) {}
func (i NoInstrument) ProxiedBytes(sent, recv int, platform, version, app, dataCapCohort string, clientIP net.IP) {
}
func (i NoInstrument) TCPPackets(clientAddr string, sentDataPackets, retransmissions, consecRetransmissions int) {
}
func (i NoInstrument) quicSentPacket() {}
func (i NoInstrument) quicLostPacket() {}

// CommonLabels defines a set of common labels apply to all metrics instrumented.
type CommonLabels struct {
	Protocol              string
	BuildType             string
	SupportTLSResumption  bool
	RequireTLSResumption  bool
	MissingTicketReaction string
}

// PromLabels turns the common labels to Prometheus form.
func (c *CommonLabels) PromLabels() prometheus.Labels {
	return map[string]string{
		"protocol":                c.Protocol,
		"build_type":              c.BuildType,
		"support_tls_resumption":  strconv.FormatBool(c.SupportTLSResumption),
		"require_tls_resumption":  strconv.FormatBool(c.RequireTLSResumption),
		"missing_ticket_reaction": c.MissingTicketReaction,
	}
}

type instrumentedFilter struct {
	requests prometheus.Counter
	errors   prometheus.Counter
	duration prometheus.Observer
	filters.Filter
}

func (f *instrumentedFilter) Apply(cs *filters.ConnectionState, req *http.Request, next filters.Next) (*http.Response, *filters.ConnectionState, error) {
	start := time.Now()
	res, cs, err := f.Filter.Apply(cs, req, next)
	f.requests.Inc()
	if err != nil {
		f.errors.Inc()
	}
	f.duration.Observe(time.Since(start).Seconds())
	return res, cs, err
}

// PromInstrument is an implementation of Instrument which exports Prometheus
// metrics.
type PromInstrument struct {
	countryLookup    geo.CountryLookup
	ispLookup        geo.ISPLookup
	commonLabels     prometheus.Labels
	commonLabelNames []string
	filters          map[string]*instrumentedFilter
	errorHandlers    map[string]func(conn net.Conn, err error)

	blacklistChecked, blacklisted, mimicryChecked, mimicked, quicLostPackets, quicSentPackets, tcpConsecRetransmissions, tcpSentDataPackets, throttlingChecked, xbqSent prometheus.Counter

	bytesSent, bytesRecv, bytesSentByISP, bytesRecvByISP, throttled, notThrottled, suspectedProbing, versionCheck *prometheus.CounterVec

	mpFramesSent, mpBytesSent, mpFramesReceived, mpBytesReceived, mpFramesRetransmitted, mpBytesRetransmitted *prometheus.CounterVec

	tcpRetransmissionRate prometheus.Observer
}

func NewPrometheus(countryLookup geo.CountryLookup, ispLookup geo.ISPLookup, c CommonLabels) *PromInstrument {
	commonLabels := c.PromLabels()
	commonLabelNames := make([]string, len(commonLabels))
	i := 0
	for k := range commonLabels {
		commonLabelNames[i] = k
		i++
	}
	return &PromInstrument{
		countryLookup:    countryLookup,
		ispLookup:        ispLookup,
		commonLabels:     commonLabels,
		commonLabelNames: commonLabelNames,
		filters:          make(map[string]*instrumentedFilter),
		errorHandlers:    make(map[string]func(conn net.Conn, err error)),
		blacklistChecked: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_blacklist_checked_requests_total",
		}, commonLabelNames).With(commonLabels),
		blacklisted: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_blacklist_blacklisted_requests_total",
		}, commonLabelNames).With(commonLabels),
		bytesSent: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_downstream_sent_bytes_total",
			Help: "Bytes sent to the client connections. Pluggable transport overhead excluded",
		}, append(commonLabelNames, "app_platform", "app_version", "app", "datacap_cohort")).MustCurryWith(commonLabels),
		bytesRecv: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_downstream_received_bytes_total",
			Help: "Bytes received from the client connections. Pluggable transport overhead excluded",
		}, append(commonLabelNames, "app_platform", "app_version", "app", "datacap_cohort")).MustCurryWith(commonLabels),
		bytesSentByISP: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_downstream_by_isp_sent_bytes_total",
			Help: "Bytes sent to the client connections, by country and isp. Pluggable transport overhead excluded",
		}, append(commonLabelNames, "country", "isp")).MustCurryWith(commonLabels),
		bytesRecvByISP: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_downstream_by_isp_received_bytes_total",
			Help: "Bytes received from the client connections, by country and isp. Pluggable transport overhead excluded",
		}, append(commonLabelNames, "country", "isp")).MustCurryWith(commonLabels),

		quicLostPackets: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_downstream_quic_lost_packets_total",
			Help: "Number of QUIC packets lost and effectively resent to the client connections.",
		}, commonLabelNames).With(commonLabels),
		quicSentPackets: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_downstream_quic_sent_packets_total",
			Help: "Number of QUIC packets sent to the client connections.",
		}, commonLabelNames).With(commonLabels),

		mimicryChecked: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_apache_mimicry_checked_total",
		}, commonLabelNames).With(commonLabels),
		mimicked: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_apache_mimicry_mimicked_total",
		}, commonLabelNames).With(commonLabels),

		mpFramesSent: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_multipath_sent_frames_total",
		}, append(commonLabelNames, "path_protocol")).MustCurryWith(commonLabels),
		mpBytesSent: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_multipath_sent_bytes_total",
		}, append(commonLabelNames, "path_protocol")).MustCurryWith(commonLabels),
		mpFramesReceived: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_multipath_received_frames_total",
		}, append(commonLabelNames, "path_protocol")).MustCurryWith(commonLabels),
		mpBytesReceived: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_multipath_received_bytes_total",
		}, append(commonLabelNames, "path_protocol")).MustCurryWith(commonLabels),
		mpFramesRetransmitted: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_multipath_retransmissions_total",
		}, append(commonLabelNames, "path_protocol")).MustCurryWith(commonLabels),
		mpBytesRetransmitted: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_multipath_retransmission_bytes_total",
		}, append(commonLabelNames, "path_protocol")).MustCurryWith(commonLabels),

		tcpConsecRetransmissions: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_downstream_tcp_consec_retransmissions_before_terminates_total",
			Help: "Number of TCP retransmissions happen before the connection gets terminated, as a measure of blocking in the form of continuously dropped packets.",
		}, commonLabelNames).With(commonLabels),
		tcpRetransmissionRate: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "proxy_tcp_retransmission_rate",
			Buckets: []float64{0.01, 0.1, 0.5},
		}, commonLabelNames).With(commonLabels),
		tcpSentDataPackets: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_downstream_tcp_sent_data_packets_total",
			Help: "Number of TCP data packets (packets with non-zero data length) sent to the client connections.",
		}, commonLabelNames).With(commonLabels),

		xbqSent: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_xbq_header_sent_total",
		}, commonLabelNames).With(commonLabels),

		throttlingChecked: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_device_throttling_checked_total",
		}, commonLabelNames).With(commonLabels),
		throttled: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_device_throttling_throttled_total",
		}, append(commonLabelNames, "reason")).MustCurryWith(commonLabels),
		notThrottled: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_device_throttling_not_throttled_total",
		}, append(commonLabelNames, "reason")).MustCurryWith(commonLabels),

		suspectedProbing: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_suspected_probing_total",
		}, append(commonLabelNames, "country", "reason")).MustCurryWith(commonLabels),

		versionCheck: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "proxy_version_check_total",
		}, append(commonLabelNames, "method", "redirected", "reason")).MustCurryWith(commonLabels),
	}
}

// Run runs the PromInstrument exporter on the given address. The
// path is /metrics.
func (p *PromInstrument) Run(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return server.ListenAndServe()
}

// WrapFilter wraps a filter to instrument the requests/errors/duration
// (so-called RED) of processed requests.
func (p *PromInstrument) WrapFilter(prefix string, f filters.Filter) filters.Filter {
	wrapped := p.filters[prefix]
	if wrapped == nil {
		wrapped = &instrumentedFilter{
			promauto.NewCounterVec(prometheus.CounterOpts{
				Name: prefix + "_requests_total",
			}, p.commonLabelNames).With(p.commonLabels),
			promauto.NewCounterVec(prometheus.CounterOpts{
				Name: prefix + "_request_errors_total",
			}, p.commonLabelNames).With(p.commonLabels),
			promauto.NewHistogramVec(prometheus.HistogramOpts{
				Name:    prefix + "_request_duration_seconds",
				Buckets: []float64{0.001, 0.01, 0.1, 1},
			}, p.commonLabelNames).With(p.commonLabels),
			f}
		p.filters[prefix] = wrapped
	}
	return wrapped
}

// WrapConnErrorHandler wraps an error handler to instrument the error count.
func (p *PromInstrument) WrapConnErrorHandler(prefix string, f func(conn net.Conn, err error)) func(conn net.Conn, err error) {
	h := p.errorHandlers[prefix]
	if h == nil {
		errors := promauto.NewCounterVec(prometheus.CounterOpts{
			Name: prefix + "_errors_total",
		}, p.commonLabelNames).With(p.commonLabels)
		consec_errors := promauto.NewCounterVec(prometheus.CounterOpts{
			Name: prefix + "_consec_per_client_ip_errors_total",
		}, p.commonLabelNames).With(p.commonLabels)
		if f == nil {
			f = func(conn net.Conn, err error) {}
		}
		var mu sync.Mutex
		var lastRemoteIP string
		h = func(conn net.Conn, err error) {
			errors.Inc()
			addr := conn.RemoteAddr()
			if addr == nil {
				return
			}
			host, _, err := net.SplitHostPort(addr.String())
			if err != nil {
				return
			}
			mu.Lock()
			if lastRemoteIP != host {
				lastRemoteIP = host
				mu.Unlock()
				consec_errors.Inc()
			} else {
				mu.Unlock()
			}
			f(conn, err)
		}
		p.errorHandlers[prefix] = h
	}
	return h
}

// Blacklist instruments the blacklist checking.
func (p *PromInstrument) Blacklist(b bool) {
	p.blacklistChecked.Inc()
	if b {
		p.blacklisted.Inc()
	}
}

// Mimic instruments the Apache mimicry.
func (p *PromInstrument) Mimic(m bool) {
	p.mimicryChecked.Inc()
	if m {
		p.mimicked.Inc()
	}
}

// Throttle instruments the device based throttling.
func (p *PromInstrument) Throttle(m bool, reason string) {
	p.throttlingChecked.Inc()
	if m {
		p.throttled.With(prometheus.Labels{"reason": reason}).Inc()
	} else {
		p.notThrottled.With(prometheus.Labels{"reason": reason}).Inc()
	}
}

// XBQHeaderSent counts the number of times XBQ header is sent along with the
// response.
func (p *PromInstrument) XBQHeaderSent() {
	p.xbqSent.Inc()
}

// SuspectedProbing records the number of visits which looks like active
// probing.
func (p *PromInstrument) SuspectedProbing(fromIP net.IP, reason string) {
	fromCountry := p.countryLookup.CountryCode(fromIP)
	p.suspectedProbing.With(prometheus.Labels{"country": fromCountry, "reason": reason}).Inc()
}

// VersionCheck records the number of times the Lantern version header is
// checked and if redirecting to the upgrade page is required.
func (p *PromInstrument) VersionCheck(redirect bool, method, reason string) {
	labels := prometheus.Labels{"method": method, "redirected": strconv.FormatBool(redirect), "reason": reason}
	p.versionCheck.With(labels).Inc()
}

// ProxiedBytes records the volume of application data clients sent and
// received via the proxy.
func (p *PromInstrument) ProxiedBytes(sent, recv int, platform, version, app, dataCapCohort string, clientIP net.IP) {
	labels := prometheus.Labels{"app_platform": platform, "app_version": version, "app": app, "datacap_cohort": dataCapCohort}
	p.bytesSent.With(labels).Add(float64(sent))
	p.bytesRecv.With(labels).Add(float64(recv))
	country := p.countryLookup.CountryCode(clientIP)
	by_isp := prometheus.Labels{"country": country, "isp": "omitted"}
	// We care about ISPs within these countries only, to reduce cardinality of the metrics
	if country == "CN" || country == "IR" || country == "AE" || country == "TK" {
		by_isp["isp"] = p.ispLookup.ISP(clientIP)
	}
	p.bytesSentByISP.With(by_isp).Add(float64(sent))
	p.bytesRecvByISP.With(by_isp).Add(float64(recv))
}

// TCPPackets records the number/rate of TCP data packets and retransmissions
// mainly for block detection.
func (p *PromInstrument) TCPPackets(clientAddr string, sentDataPackets, retransmissions, consecRetransmissions int) {
	p.tcpRetransmissionRate.Observe(float64(retransmissions) / float64(sentDataPackets))
	p.tcpSentDataPackets.Add(float64(sentDataPackets))
	p.tcpConsecRetransmissions.Add(float64(consecRetransmissions))
}

// quicPackets is used by QuicTracer to update QUIC retransmissions mainly for block detection.
func (p *PromInstrument) quicSentPacket() {
	p.quicSentPackets.Inc()
}
func (p *PromInstrument) quicLostPacket() {
	p.quicLostPackets.Inc()
}

type stats struct {
	framesSent          prometheus.Counter
	bytesSent           prometheus.Counter
	framesRetransmitted prometheus.Counter
	bytesRetransmitted  prometheus.Counter
	framesReceived      prometheus.Counter
	bytesReceived       prometheus.Counter
}

func (s *stats) OnRecv(n uint64) {
	s.framesReceived.Inc()
	s.bytesReceived.Add(float64(n))
}
func (s *stats) OnSent(n uint64) {
	s.framesSent.Inc()
	s.bytesSent.Add(float64(n))
}
func (s *stats) OnRetransmit(n uint64) {
	s.framesRetransmitted.Inc()
	s.bytesRetransmitted.Add(float64(n))
}
func (s *stats) UpdateRTT(time.Duration) {
	// do nothing as the RTT from different clients can vary significantly
}

func (prom *PromInstrument) MultipathStats(protocols []string) (trackers []multipath.StatsTracker) {
	for _, p := range protocols {
		path_protocol := prometheus.Labels{"path_protocol": p}
		trackers = append(trackers, &stats{
			framesSent:          prom.mpFramesSent.With(path_protocol),
			bytesSent:           prom.mpBytesSent.With(path_protocol),
			framesReceived:      prom.mpFramesReceived.With(path_protocol),
			bytesReceived:       prom.mpBytesReceived.With(path_protocol),
			framesRetransmitted: prom.mpFramesRetransmitted.With(path_protocol),
			bytesRetransmitted:  prom.mpBytesRetransmitted.With(path_protocol),
		})
	}
	return
}
