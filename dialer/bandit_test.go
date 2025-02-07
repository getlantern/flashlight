package dialer

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
			got, index := o.(*banditDialer).chooseDialerForDomain(tt.args.network, tt.args.addr)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BanditDialer.chooseDialerForDomain() got = %v, want %v", got, tt.want)
			}
			assert.Equal(t, tt.want1, index, "BanditDialer.chooseDialerForDomain() got1 = %v, want %v", index, tt.want1)
		})
	}
}

func TestNewBandit(t *testing.T) {
	oldDialer := newTcpConnDialer()
	oldDialerMetric := banditMetrics{
		Reward:    0.7,
		Count:     10,
		UpdatedAt: time.Now().UTC().Unix(),
	}
	tests := []struct {
		name   string
		opts   *Options
		assert func(t *testing.T, got Dialer, err error, dir string)
		setup  func() string
	}{
		{
			name: "should fail if there are no dialers",
			opts: &Options{
				Dialers: nil,
			},
			assert: func(t *testing.T, got Dialer, err error, _ string) {
				assert.Nil(t, got)
				assert.Error(t, err)
			},
		},
		{
			name: "should return a BanditDialer if there's only one dialer",
			opts: &Options{
				Dialers: []ProxyDialer{newTcpConnDialer()},
			},
			assert: func(t *testing.T, got Dialer, err error, _ string) {
				assert.NotNil(t, got)
				assert.NoError(t, err)
				assert.IsType(t, &banditDialer{}, got)
			},
		},
		{
			name: "should load the last bandit rewards if they exist",
			opts: &Options{
				Dialers: []ProxyDialer{oldDialer, newTcpConnDialer()},
			},
			setup: func() string {
				tempDir, err := os.MkdirTemp("", "client_test")
				require.NoError(t, err)

				// create rewards.csv
				err = os.WriteFile(filepath.Join(tempDir, "rewards.csv"), []byte(fmt.Sprintf("dialer,reward,count,updated at\n%s,%f,%d,%d\n", oldDialer.Name(), oldDialerMetric.Reward, oldDialerMetric.Count, oldDialerMetric.UpdatedAt)), 0644)
				require.NoError(t, err)
				return tempDir
			},
			assert: func(t *testing.T, got Dialer, err error, dir string) {
				assert.NotNil(t, got)
				assert.NoError(t, err)
				assert.IsType(t, &banditDialer{}, got)
				rewards := got.(*banditDialer).bandit.GetRewards()
				counts := got.(*banditDialer).bandit.GetCounts()
				// checking if the rewards are loaded correctly
				assert.Equal(t, oldDialerMetric.Reward, rewards[0])
				assert.Equal(t, oldDialerMetric.Count, counts[0])
				// since there's no data for the second dialer, it should be 0
				assert.Equal(t, float64(0), rewards[1])
				assert.Equal(t, 0, counts[1])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := ""
			if tt.setup != nil {
				dir = tt.setup()
				defer os.RemoveAll(dir)
				tt.opts.BanditDir = dir
			}
			got, err := NewBandit(tt.opts)
			tt.assert(t, got, err, dir)
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
		{
			name: "should return an error if failed upstream",
			opts: &Options{
				Dialers: []ProxyDialer{newFailingTcpConnDialer()},
			},
			want:    nil,
			wantErr: true,
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
			if tt.wantErr {
				assert.Error(t, err)
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
		dataRecv           uint64
		elapsedTimeReading int64
	}
	tests := []struct {
		name string
		args args
		want func(float64) bool
	}{
		{
			name: "should return 0 if no data received",
			args: args{
				dataRecv:           0,
				elapsedTimeReading: secondsForSample * 1000,
			},
			want: func(got float64) bool {
				return got == 0
			},
		},
		{
			name: "should return 1 if pretty fast",
			args: args{
				dataRecv:           topExpectedBps * secondsForSample,
				elapsedTimeReading: secondsForSample * 1000,
			},
			want: func(got float64) bool {
				return got == 1
			},
		},
		{
			name: "should return 1 if super fast",
			args: args{
				dataRecv:           topExpectedBps * 50,
				elapsedTimeReading: secondsForSample * 1000,
			},
			want: func(got float64) bool {
				return got > 1
			},
		},
		{
			name: "should return <1 if sorta fast",
			args: args{
				dataRecv:           2000,
				elapsedTimeReading: secondsForSample * 1000,
			},
			want: func(got float64) bool {
				return got > 0 && got < 1
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeReceiveSpeed(tt.args.dataRecv, tt.args.elapsedTimeReading); !assert.True(t, tt.want(got)) {
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

func TestUpdateBanditRewards(t *testing.T) {
	var tests = []struct {
		name   string
		given  map[string]banditMetrics
		assert func(t *testing.T, dir string, err error)
	}{
		{
			name: "it should update rewards file",
			given: map[string]banditMetrics{
				"test-dialer": {
					Reward: 1.0,
					Count:  1,
				},
			},
			assert: func(t *testing.T, dir string, err error) {
				assert.NoError(t, err)
				f, err := os.Open(filepath.Join(dir, "rewards.csv"))
				require.NoError(t, err)
				defer f.Close()
				b, err := io.ReadAll(f)
				require.NoError(t, err)

				lines := strings.Split(string(b), "\n")
				// check if headers are there
				assert.Equal(t, lines[0], "dialer,reward,count,updated at")
				// check if the data is there
				cols := strings.Split(lines[1], ",")
				assert.Equal(t, cols[dialerNameCSVHeader], "test-dialer")
				assert.Equal(t, cols[rewardCSVHeader], "1.000000")
				assert.Equal(t, cols[countCSVHeader], "1")
				assert.NotEmpty(t, cols[updatedAtCSVHeader])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "client_test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			banditDialer := &banditDialer{
				opts: &Options{
					BanditDir: tempDir,
				},
				banditRewardsMutex: new(sync.Mutex),
			}
			err = banditDialer.updateBanditRewards(tt.given)
			tt.assert(t, tempDir, err)
		})
	}
}

func TestLoadLastBanditRewards(t *testing.T) {
	now := time.Now().UTC().Unix()
	var tests = []struct {
		name   string
		given  string
		assert func(t *testing.T, metrics map[string]banditMetrics, err error)
	}{
		{
			name:  "it should load the rewards",
			given: fmt.Sprintf("dialer,reward,count,updated at\ntest-dialer,1.000000,1,%d\n", now),
			assert: func(t *testing.T, metrics map[string]banditMetrics, err error) {
				assert.NoError(t, err)
				assert.Contains(t, metrics, "test-dialer")
				assert.Equal(t, 1.0, metrics["test-dialer"].Reward)
				assert.Equal(t, 1, metrics["test-dialer"].Count)
				assert.Equal(t, now, metrics["test-dialer"].UpdatedAt)
			},
		},
		{
			name:  "it should ignore dialers with updated at greater than 7 days",
			given: fmt.Sprintf("dialer,reward,count,updated at\ntest-dialer,1.000000,1,%d\n", now-60*60*24*8),
			assert: func(t *testing.T, metrics map[string]banditMetrics, err error) {
				assert.NoError(t, err)
				assert.Empty(t, metrics)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "bandit_test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			err = os.WriteFile(filepath.Join(tempDir, "rewards.csv"), []byte(tt.given), 0644)
			require.NoError(t, err)

			banditDialer := &banditDialer{
				opts: &Options{
					BanditDir: tempDir,
				},
				banditRewardsMutex: new(sync.Mutex),
			}
			metrics, err := banditDialer.loadLastBanditRewards()
			tt.assert(t, metrics, err)
		})
	}
}

func newTcpConnDialer() ProxyDialer {
	client, server := net.Pipe()
	return &tcpConnDialer{
		client: client,
		server: server,
		name:   uuid.New().String(),
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
	name       string
	dial       func() (net.Conn, bool, error)
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

	if t.dial != nil {
		return t.dial()
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
func (t *tcpConnDialer) Name() string {
	return t.name
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

//go:generate mockgen -package=dialer -destination=mocks_test.go net Conn

func TestBanditDialerIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	baseDialer := newTcpConnDialer()
	message := "hello"
	connSleepTime := 200 * time.Millisecond

	baseDialer.(*tcpConnDialer).dial = func() (net.Conn, bool, error) {
		conn := NewMockConn(ctrl)
		conn.EXPECT().Read(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
			time.Sleep(connSleepTime)
			return copy(b, []byte(message)), io.EOF
		}).AnyTimes()
		return conn, false, nil
	}

	banditDir, err := os.MkdirTemp("", "bandit_dial_test")
	require.NoError(t, err)
	defer os.RemoveAll(banditDir)

	opts := &Options{
		Dialers:   []ProxyDialer{baseDialer},
		BanditDir: banditDir,
	}
	bandit, err := NewBandit(opts)
	require.NoError(t, err)
	banditDialer := bandit.(*banditDialer)
	banditDialer.secondsUntilRewardSample = 1 * time.Second
	banditDialer.secondsUntilSaveBanditRewards = 1200 * time.Millisecond

	ctx := context.Background()
	banditConn, err := banditDialer.DialContext(ctx, "tcp", "localhost:8080")
	require.NoError(t, err)

	got, err := io.ReadAll(banditConn)
	assert.NoError(t, err)
	assert.Equal(t, message, string(got[:len(message)]))

	// waiting so reward is sampled and bandit rewards are stored
	time.Sleep(1400 * time.Millisecond)

	rewards := banditDialer.bandit.GetRewards()
	counts := banditDialer.bandit.GetCounts()

	// there's only one dialer
	assert.Len(t, counts, 1)
	assert.Len(t, rewards, 1)
	// since there's only one dialer and one Dial call, we're expecting one count
	assert.Equal(t, 1, counts[0])
	assert.InEpsilon(t, normalizeReceiveSpeed(uint64(len(got)), connSleepTime.Milliseconds()), rewards[0], 0.2)

	// check if rewards.csv was written
	assert.FileExists(t, filepath.Join(banditDir, "rewards.csv"))
}
