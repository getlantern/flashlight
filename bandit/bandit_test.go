package bandit

import (
	"context"
	"io"
	"math/rand"
	"net"
	"reflect"
	"testing"
	"time"

	bandit "github.com/alextanhongpin/go-bandit"
	"github.com/getlantern/flashlight/v7/stats"
	"github.com/stretchr/testify/require"
)

func TestParallelDial(t *testing.T) {
	dialers := []Dialer{
		&tcpConnDialer{shouldFail: true},
		&tcpConnDialer{shouldFail: true},
		&tcpConnDialer{shouldFail: true},
		&tcpConnDialer{},
	}

	b, err := bandit.NewEpsilonGreedy(0.001, nil, nil)
	require.NoError(t, err)

	err = b.Init(len(dialers))
	require.NoError(t, err)

	parallelDial(dialers, b)
	require.Eventually(t, func() bool {
		for _, count := range b.Counts {
			if count < 1 {
				return false
			}
		}
		return true
	}, 5*time.Second, 100*time.Millisecond)

	// Select the arm with with a probability above epsilon
	// to ensure getting the best performing arm for testing
	// purposes.
	arm := b.SelectArm(0.5)
	require.Equal(t, 3, arm)
}

func TestNewWithStats(t *testing.T) {
	type args struct {
		dialers      []Dialer
		statsTracker stats.Tracker
	}
	tests := []struct {
		name    string
		args    args
		want    *BanditDialer
		wantErr bool
	}{
		{
			name: "should still succeed even if there are no dialers",
			args: args{
				dialers:      nil,
				statsTracker: stats.NewNoop(),
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "should return a BanditDialer if there's only one dialer",
			args: args{
				dialers:      []Dialer{newTcpConnDialer()},
				statsTracker: stats.NewNoop(),
			},
			want:    &BanditDialer{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewWithStats(tt.args.dialers, tt.args.statsTracker)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewWithStats() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && !reflect.TypeOf(got).AssignableTo(reflect.TypeOf(tt.want)) {
				t.Errorf("BanditDialer.DialContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBanditDialer_DialContext(t *testing.T) {
	type fields struct {
		dialers []Dialer
	}
	expectedConn := &dataTrackingConn{}
	tests := []struct {
		name    string
		fields  fields
		want    net.Conn
		wantErr bool
	}{
		{
			name: "should return an error if there are no dialers",
			fields: fields{
				dialers: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "should return a connection if there's only one dialer",
			fields: fields{
				dialers: []Dialer{newTcpConnDialer()},
			},
			want:    expectedConn,
			wantErr: false,
		},
		{
			name: "should return a connection if there are lots of dialers",
			fields: fields{
				dialers: []Dialer{newTcpConnDialer(), newTcpConnDialer(), newTcpConnDialer()},
			},
			want:    expectedConn,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := New(tt.fields.dialers)
			if err != nil {
				t.Fatal(err)
			}

			got, err := o.DialContext(context.Background(), "tcp", "localhost:8080")
			if (err != nil) != tt.wantErr {
				t.Errorf("BanditDialer.DialContext() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want == nil && got != nil {
				t.Errorf("BanditDialer.DialContext() = %v, want %v", got, tt.want)
				return
			}
			if tt.want != nil && !reflect.TypeOf(got).AssignableTo(reflect.TypeOf(tt.want)) {
				t.Errorf("BanditDialer.DialContext() = %v, want %v", got, tt.want)
			}
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
				dataRecv: topExpectedBytes * secondsForSample,
			},
			want: func(got float64) bool {
				return got == 1
			},
		},
		{
			name: "should return 1 if super fast",
			args: args{
				dataRecv: topExpectedBytes * 50,
			},
			want: func(got float64) bool {
				return got > 1
			},
		},

		{
			name: "should return <1 if sorta fast",
			args: args{
				dataRecv: 2000,
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

func Test_differentArm(t *testing.T) {
	type args struct {
		numDialers int
	}
	tests := []struct {
		name    string
		args    args
		isError func(int, int) bool
	}{
		{
			name: "should return the existing arm if there's only one dialer",
			args: args{
				numDialers: 1,
			},
			isError: func(existingArm int, got int) bool {
				return existingArm != got || got != 0
			},
		},

		{
			name: "should return a different arm if there's more than one dialer",
			args: args{
				numDialers: 2,
			},
			isError: func(existingArm int, got int) bool {
				return existingArm == got
			},
		},

		{
			name: "should return a different arm if there are lots of dialers",
			args: args{
				numDialers: 20,
			},
			isError: func(existingArm int, got int) bool {
				return existingArm == got
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			eg, err := bandit.NewEpsilonGreedy(0.1, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			eg.Init(tt.args.numDialers)
			existingArm := eg.SelectArm(rand.Float64())
			if got := differentArm(existingArm, tt.args.numDialers, eg); tt.isError(existingArm, got) {
				t.Errorf("differentArm() returned %v and existing is %v", got, existingArm)
			}
		})
	}
}

type tcpConnDialer struct {
	shouldFail bool
}

// Addr implements Dialer.
func (*tcpConnDialer) Addr() string {
	return "localhost:8080"
}

// Attempts implements Dialer.
func (*tcpConnDialer) Attempts() int64 {
	return 0
}

// ConsecFailures implements Dialer.
func (*tcpConnDialer) ConsecFailures() int64 {
	return 0
}

// ConsecSuccesses implements Dialer.
func (*tcpConnDialer) ConsecSuccesses() int64 {
	return 0
}

// DataRecv implements Dialer.
func (*tcpConnDialer) DataRecv() uint64 {
	return 1000
}

// DataSent implements Dialer.
func (*tcpConnDialer) DataSent() uint64 {
	return 1000
}

// DialContext implements Dialer.
func (t *tcpConnDialer) DialContext(ctx context.Context, network string, addr string) (conn net.Conn, failedUpstream bool, err error) {
	if t.shouldFail {
		return nil, true, io.EOF
	}
	return &net.TCPConn{}, false, nil
}

// EstBandwidth implements Dialer.
func (*tcpConnDialer) EstBandwidth() float64 {
	return 0
}

// EstRTT implements Dialer.
func (*tcpConnDialer) EstRTT() time.Duration {
	return time.Second
}

// EstSuccessRate implements Dialer.
func (*tcpConnDialer) EstSuccessRate() float64 {
	return 0
}

// Failures implements Dialer.
func (*tcpConnDialer) Failures() int64 {
	return 0
}

// JustifiedLabel implements Dialer.
func (*tcpConnDialer) JustifiedLabel() string {
	return "tcpConnDialer"
}

// Label implements Dialer.
func (*tcpConnDialer) Label() string {
	return "tcpConnDialer"
}

// Location implements Dialer.
func (*tcpConnDialer) Location() (string, string, string) {
	return "US", "United States", "San Francisco"
}

// MarkFailure implements Dialer.
func (*tcpConnDialer) MarkFailure() {
}

// Name implements Dialer.
func (*tcpConnDialer) Name() string {
	return "tcpConnDialer"
}

// NumPreconnected implements Dialer.
func (*tcpConnDialer) NumPreconnected() int {
	return 0
}

// NumPreconnecting implements Dialer.
func (*tcpConnDialer) NumPreconnecting() int {
	return 0
}

// Protocol implements Dialer.
func (*tcpConnDialer) Protocol() string {
	return "tcp"
}

// Stop implements Dialer.
func (*tcpConnDialer) Stop() {
}

// Succeeding implements Dialer.
func (*tcpConnDialer) Succeeding() bool {
	return true
}

// Successes implements Dialer.
func (*tcpConnDialer) Successes() int64 {
	return 0
}

// SupportsAddr implements Dialer.
func (*tcpConnDialer) SupportsAddr(network string, addr string) bool {
	return true
}

// Trusted implements Dialer.
func (*tcpConnDialer) Trusted() bool {
	return true
}

// WriteStats implements Dialer.
func (*tcpConnDialer) WriteStats(w io.Writer) {
}

func newTcpConnDialer() Dialer {
	return &tcpConnDialer{}
}
