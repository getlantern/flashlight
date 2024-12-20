package chained

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/refraction-networking/water"
	_ "github.com/refraction-networking/water/transport/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc struct {
	f func(req *http.Request) (*http.Response, error)
}

func (f *roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f.f(req)
}

func TestNewWaterImpl(t *testing.T) {
	type params struct {
		configDir      string
		raddr          string
		pc             *config.ProxyConfig
		reportDialCore reportDialCoreFn
	}
	f, err := testData.Open("testdata/reverse_tinygo_v1.wasm")
	require.NoError(t, err)

	wantWASM, err := io.ReadAll(f)
	require.NoError(t, err)

	b64WASM := base64.StdEncoding.EncodeToString(wantWASM)

	configDir, err := os.MkdirTemp("", "water")
	require.NoError(t, err)
	defer os.RemoveAll(configDir)

	var tests = []struct {
		name          string
		givenParams   params
		assert        func(t *testing.T, actual *waterImpl, err error)
		setHTTPClient func()
	}{
		{
			name: "create new waterImpl with success",
			givenParams: params{
				configDir: configDir,
				raddr:     "127.0.0.1",
				pc: &config.ProxyConfig{
					PluggableTransportSettings: map[string]string{
						"water_wasm": b64WASM,
					},
				},
				reportDialCore: func(op *ops.Op, dialCore func() (net.Conn, error)) (net.Conn, error) {
					return nil, nil
				},
			},
			assert: func(t *testing.T, actual *waterImpl, err error) {
				require.NoError(t, err)
				require.NotNil(t, actual)
				readyChan := actual.ready()
				assert.NotNil(t, readyChan)
				select {
				case err, ok := <-readyChan:
					assert.True(t, ok)
					require.NoError(t, err)
				}
				require.NotNil(t, actual.dialer)
				assert.NotNil(t, actual.reportDialCore)
			},
			setHTTPClient: func() {
				waterHTTPClient = &http.Client{
					Transport: &roundTripFunc{
						f: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(bytes.NewBuffer(wantWASM)),
							}, nil
						},
					},
				}
			},
		},
		{
			name: "create new waterImpl by downloading wasm",
			givenParams: params{
				configDir: configDir,
				raddr:     "127.0.0.1",
				pc: &config.ProxyConfig{
					PluggableTransportSettings: map[string]string{
						"wasm_available_at": "http://example.com/wasm.wasm,http://example2.com/wasm.wasm",
						"water_transport":   "plain.v1.tinygo.wasm",
					},
				},
				reportDialCore: func(op *ops.Op, dialCore func() (net.Conn, error)) (net.Conn, error) {
					return nil, nil
				},
			},
			assert: func(t *testing.T, actual *waterImpl, err error) {
				defer actual.close()
				require.NoError(t, err)
				require.NotNil(t, actual)
				readyChan := actual.ready()
				assert.NotNil(t, readyChan)
				select {
				case err, ok := <-readyChan:
					assert.True(t, ok)
					require.NoError(t, err)
				}
				assert.NotNil(t, actual.dialer)
				assert.NotNil(t, actual.reportDialCore)
			},
			setHTTPClient: func() {
				waterHTTPClient = &http.Client{
					Transport: &roundTripFunc{
						f: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(bytes.NewBuffer(wantWASM)),
							}, nil
						},
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setHTTPClient()
			waterImpl, err := newWaterImpl(tt.givenParams.configDir, tt.givenParams.raddr, tt.givenParams.pc, tt.givenParams.reportDialCore)
			tt.assert(t, waterImpl, err)
		})
	}
}

//go:embed testdata/*
var testData embed.FS

func TestWaterDialServer(t *testing.T) {
	ctx := context.Background()

	f, err := testData.Open("testdata/reverse_tinygo_v1.wasm")
	require.NoError(t, err)

	wasm, err := io.ReadAll(f)
	require.NoError(t, err)

	b64WASM := base64.StdEncoding.EncodeToString(wasm)

	pc := &config.ProxyConfig{PluggableTransportSettings: map[string]string{
		"water_wasm": b64WASM,
	}}

	testOp := ops.Begin("test")
	defer testOp.End()

	configDir, err := os.MkdirTemp("", "water")
	require.NoError(t, err)
	defer os.RemoveAll(configDir)

	var tests = []struct {
		name                string
		givenOp             *ops.Op
		givenCtx            context.Context
		givenConfigDir      string
		givenReportDialCore reportDialCoreFn
		givenDialer         water.Dialer
		givenAddr           string
		assert              func(t *testing.T, actual net.Conn, err error)
		setHTTPClient       func()
	}{
		{
			name:           "should fail to dial when endpoint is not available",
			givenOp:        testOp,
			givenCtx:       ctx,
			givenConfigDir: configDir,
			givenReportDialCore: func(op *ops.Op, dialCore func() (net.Conn, error)) (net.Conn, error) {
				assert.Equal(t, testOp, op)
				assert.NotNil(t, dialCore)
				return dialCore()
			},
			assert: func(t *testing.T, actual net.Conn, err error) {
				assert.ErrorContains(t, err, "transport endpoint is not connected")
				assert.Nil(t, actual)
			},
			givenAddr: "127.0.0.1:8080",
			setHTTPClient: func() {
				waterHTTPClient = &http.Client{
					Transport: &roundTripFunc{
						f: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: http.StatusOK,
								Body:       io.NopCloser(bytes.NewBuffer(wasm)),
							}, nil
						},
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setHTTPClient()
			waterImpl, err := newWaterImpl(tt.givenConfigDir, tt.givenAddr, pc, tt.givenReportDialCore)
			require.NoError(t, err)
			defer waterImpl.close()
			conn, err := waterImpl.dialServer(tt.givenOp, tt.givenCtx)
			tt.assert(t, conn, err)
		})
	}
}
