package dialer

import (
	"context"
	"io"
	"math/rand"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBanditDialer_chooseDialerForDomain(t *testing.T) {
	baseDialer := newTcpConnDialer()
	type fields struct {
		dialers []ProxyDialer
	}
	type args struct {
		network string
		addr    string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   ProxyDialer
		want1  int
	}{
		{
			name: "should return the first dialer if there's only one dialer",
			fields: fields{
				dialers: []ProxyDialer{baseDialer},
			},
			args: args{
				network: "tcp",
				addr:    "localhost:8080",
			},
			want:  baseDialer,
			want1: 0,
		},
		{
			name: "choose the non-failing dialer if there are multiple dialers",
			fields: fields{
				dialers: []ProxyDialer{
					newFailingTcpConnDialer(),
					newFailingTcpConnDialer(),
					newFailingTcpConnDialer(),
					newFailingTcpConnDialer(),
					baseDialer,
					newFailingTcpConnDialer(),
					newFailingTcpConnDialer(),
				},
			},
			args: args{
				network: "tcp",
				addr:    "localhost:8080",
			},
			want:  baseDialer,
			want1: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &Options{
				Dialers: tt.fields.dialers,
			}
			o, err := NewBandit(opts)
			require.NoError(t, err)
			got, got1 := o.(*BanditDialer).chooseDialerForDomain(tt.args.network, tt.args.addr)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BanditDialer.chooseDialerForDomain() got = %v, want %v", got, tt.want)
			}
			assert.Equal(t, tt.want1, got1, "BanditDialer.chooseDialerForDomain() got1 = %v, want %v", got1, tt.want1)
		})
	}
}

func TestNewBandit(t *testing.T) {
	tests := []struct {
		name    string
		opts    *Options
		want    *BanditDialer
		wantErr bool
	}{
		{
			name: "should fail if there are no dialers",
			opts: &Options{
				Dialers: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "should return a BanditDialer if there's only one dialer",
			opts: &Options{
				Dialers: []ProxyDialer{newTcpConnDialer()},
			},
			want:    &BanditDialer{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewBandit(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBandit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil && !reflect.TypeOf(got).AssignableTo(reflect.TypeOf(tt.want)) {
				t.Errorf("BanditDialer.DialContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBanditDialer_DialContext(t *testing.T) {
	expectedConn := &dataTrackingConn{}
	tests := []struct {
		name    string
		opts    *Options
		want    net.Conn
		wantErr bool
	}{
		{
			name: "should return an error if there are no dialers",
			opts: &Options{
				Dialers: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "should return a connection if there's only one dialer",
			opts: &Options{
				Dialers: []ProxyDialer{newTcpConnDialer()},
			},
			want:    expectedConn,
			wantErr: false,
		},
		{
			name: "should return a connection if there are lots of dialers",
			opts: &Options{
				Dialers: []ProxyDialer{newTcpConnDialer(), newTcpConnDialer(), newTcpConnDialer()},
			},
			want:    expectedConn,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := NewBandit(tt.opts)
			if err != nil {
				if tt.wantErr {
					return
				}
				t.Fatal(err)
				return
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
				dataRecv: topExpectedBps * secondsForSample,
			},
			want: func(got float64) bool {
				return got == 1
			},
		},
		{
			name: "should return 1 if super fast",
			args: args{
				dataRecv: topExpectedBps * 50,
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
			existingArm := rand.Intn(tt.args.numDialers)
			if got := differentArm(existingArm, tt.args.numDialers); tt.isError(existingArm, got) {
				t.Errorf("differentArm() returned %v and existing is %v", got, existingArm)
			}
		})
	}
}

func newTcpConnDialer() ProxyDialer {
	client, server := net.Pipe()
	return &tcpConnDialer{
		client: client,
		server: server,
	}
}

func newFailingTcpConnDialer() ProxyDialer {
	return &tcpConnDialer{
		shouldFail: true,
	}
}

type tcpConnDialer struct {
	shouldFail bool
	client     net.Conn
	server     net.Conn
}

func (*tcpConnDialer) Ready() <-chan error {
	return nil
}

// DialProxy implements Dialer.
func (t *tcpConnDialer) DialProxy(ctx context.Context) (net.Conn, error) {
	if t.shouldFail {
		return nil, io.EOF
	}
	return t.client, nil
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
func (t *tcpConnDialer) ConsecFailures() int64 {
	if t.shouldFail {
		return 1
	}
	return 0
}

// ConsecSuccesses implements Dialer.
func (t *tcpConnDialer) ConsecSuccesses() int64 {
	if !t.shouldFail {
		return 1
	}
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
func (t *tcpConnDialer) Successes() int64 {
	if !t.shouldFail {
		return 1
	}
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
