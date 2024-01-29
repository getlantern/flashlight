package orchestrator

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/xtaci/lossyconn"
)

type mockDialer struct {
	loss  float64
	delay int
}

func (m *mockDialer) String() string {
	return "mock"
}

// SupportsAddr indicates whether this Dialer supports the given addr. If it does not, the
// balancer will not attempt to dial that addr with this Dialer.
func (m *mockDialer) SupportsAddr(network string, addr string) bool {
	panic("not implemented") // TODO: Implement
}

// DialContext dials out to the given origin. failedUpstream indicates whether
// this was an upstream error (as opposed to errors connecting to the proxy).
func (m *mockDialer) DialContext(ctx context.Context, network string, addr string) (net.Conn, bool, error) {
	conn, err := lossyconn.NewLossyConn(m.loss, m.delay)
	return conn, false, err
}

// Name returns the name for this Dialer
func (m *mockDialer) Name() string {
	panic("not implemented") // TODO: Implement
}

// Label returns a label for this Dialer (includes Name plus more).
func (m *mockDialer) Label() string {
	panic("not implemented") // TODO: Implement
}

// JustifiedLabel is like Label() but with elements justified for line-by
// -line display.
func (m *mockDialer) JustifiedLabel() string {
	panic("not implemented") // TODO: Implement
}

// Location returns the country code, country name and city name of the
// dialer, in this order.
func (m *mockDialer) Location() (string, string, string) {
	panic("not implemented") // TODO: Implement
}

// Protocol returns a string representation of the protocol used by this
// Dialer.
func (m *mockDialer) Protocol() string {
	panic("not implemented") // TODO: Implement
}

// Addr returns the address for this Dialer
func (m *mockDialer) Addr() string {
	panic("not implemented") // TODO: Implement
}

// Trusted indicates whether or not this dialer is trusted
func (m *mockDialer) Trusted() bool {
	panic("not implemented") // TODO: Implement
}

// NumPreconnecting returns the number of pending preconnect requests.
func (m *mockDialer) NumPreconnecting() int {
	panic("not implemented") // TODO: Implement
}

// NumPreconnected returns the number of preconnected connections.
func (m *mockDialer) NumPreconnected() int {
	panic("not implemented") // TODO: Implement
}

// MarkFailure marks a dial failure on this dialer.
func (m *mockDialer) MarkFailure() {
	panic("not implemented") // TODO: Implement
}

// EstRTT provides a round trip delay time estimate, similar to how RTT is
// estimated in TCP (https://tools.ietf.org/html/rfc6298)
func (m *mockDialer) EstRTT() time.Duration {
	panic("not implemented") // TODO: Implement
}

// EstBandwidth provides the estimated bandwidth in Mbps
func (m *mockDialer) EstBandwidth() float64 {
	panic("not implemented") // TODO: Implement
}

// EstSuccessRate returns the estimated success rate dialing this dialer.
func (m *mockDialer) EstSuccessRate() float64 {
	panic("not implemented") // TODO: Implement
}

// Attempts returns the total number of dial attempts
func (m *mockDialer) Attempts() int64 {
	panic("not implemented") // TODO: Implement
}

// Successes returns the total number of dial successes
func (m *mockDialer) Successes() int64 {
	panic("not implemented") // TODO: Implement
}

// ConsecSuccesses returns the number of consecutive dial successes
func (m *mockDialer) ConsecSuccesses() int64 {
	panic("not implemented") // TODO: Implement
}

// Failures returns the total number of dial failures
func (m *mockDialer) Failures() int64 {
	panic("not implemented") // TODO: Implement
}

// ConsecFailures returns the number of consecutive dial failures
func (m *mockDialer) ConsecFailures() int64 {
	panic("not implemented") // TODO: Implement
}

// Succeeding indicates whether or not this dialer is currently good to use
func (m *mockDialer) Succeeding() bool {
	panic("not implemented") // TODO: Implement
}

// Probe performs active probing of the proxy to better understand
// connectivity and performance. If forPerformance is true, the dialer will
// probe more and with bigger data in order for bandwidth estimation to
// collect enough data to make a decent estimate. Probe returns true if it was
// successfully able to communicate with the Proxy.
func (m *mockDialer) Probe(forPerformance bool) bool {
	panic("not implemented") // TODO: Implement
}

// ProbeStats returns probe related stats for the dialer which can be used
// to estimate the overhead of active probling.
func (m *mockDialer) ProbeStats() (successes uint64, successKBs uint64, failures uint64, failedKBs uint64) {
	panic("not implemented") // TODO: Implement
}

// DataSent returns total bytes of application data sent to connections
// created via this dialer.
func (m *mockDialer) DataSent() uint64 {
	panic("not implemented") // TODO: Implement
}

// DataRecv returns total bytes of application data received from
// connections created via this dialer.
func (m *mockDialer) DataRecv() uint64 {
	panic("not implemented") // TODO: Implement
}

// Stop stops background processing for this Dialer.
func (m *mockDialer) Stop() {
	panic("not implemented") // TODO: Implement
}

func (m *mockDialer) WriteStats(w io.Writer) {
	panic("not implemented") // TODO: Implement
}

var noLossDialer = &mockDialer{
	loss:  0,
	delay: 0,
}

var highLossDialer = &mockDialer{
	loss:  1.0,
	delay: 0,
}

var highDelayDialer = &mockDialer{
	loss:  0,
	delay: 2,
}

var highLossAndDelayDialer = &mockDialer{
	loss:  1.0,
	delay: 2,
}

func TestOrchestrator_DialContext(t *testing.T) {
	type fields struct {
		dialers []Dialer
		//bandit  *bandit.EpsilonGreedy
	}
	type args struct {
		ctx     context.Context
		network string
		addr    string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    net.Conn
		wantErr bool
	}{
		{
			name: "should return error if no dialers",
			fields: fields{
				dialers: []Dialer{
					noLossDialer,
					highLossDialer,
					highDelayDialer,
					highLossAndDelayDialer,
				},
			},
			args: args{
				ctx:     context.Background(),
				network: "tcp",
				addr:    "localhost:8080",
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, _ := New(tt.fields.dialers)
			for i := 1; i < 20; i++ {
				_, err := o.DialContext(tt.args.ctx, tt.args.network, tt.args.addr)
				if err != nil {
					t.Errorf("Orchestrator.DialContext() error = %v", err)
					return
				}
				//got.Write([]byte("hello"))
				//got.Read(make([]byte, 5))
			}
			/*
				if (err != nil) != tt.wantErr {
					t.Errorf("Orchestrator.DialContext() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("Orchestrator.DialContext() = %v, want %v", got, tt.want)
				}
			*/
		})
	}
}

func Test_normalizeReceiveSpeed(t *testing.T) {
	type args struct {
		dataRecv uint64
	}
	tests := []struct {
		name string
		args args
		want func(float64) bool
	}{
		{
			name: "should return 0 if no data received",
			args: args{
				dataRecv: 0,
			},
			want: func(got float64) bool {
				return got == 0
			},
		},
		{
			name: "should return 1 if pretty fast",
			args: args{
				dataRecv: topExpectedSpeed * 5,
			},
			want: func(got float64) bool {
				return got == 1
			},
		},
		{
			name: "should return 1 if super fast",
			args: args{
				dataRecv: topExpectedSpeed * 50,
			},
			want: func(got float64) bool {
				return got == 1
			},
		},

		{
			name: "should return <1 if sorta fast",
			args: args{
				dataRecv: 20000,
			},
			want: func(got float64) bool {
				return got > 0 && got < 1
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeReceiveSpeed(tt.args.dataRecv); !tt.want(got) {
				t.Errorf("unexpected normalizeReceiveSpeed() = %v", got)
			}
		})
	}
}
