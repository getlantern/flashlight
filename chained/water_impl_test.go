package chained

import (
	"context"
	"embed"
	"encoding/base64"
	"io"
	"net"
	"testing"

	"github.com/getlantern/common/config"
	"github.com/getlantern/flashlight/v7/ops"
	"github.com/refraction-networking/water"
	_ "github.com/refraction-networking/water/transport/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWaterImpl(t *testing.T) {
	type params struct {
		raddr          string
		pc             *config.ProxyConfig
		reportDialCore reportDialCoreFn
	}
	f, err := testData.Open("testdata/reverse_tinygo_v1.wasm")
	require.NoError(t, err)

	wantWASM, err := io.ReadAll(f)
	require.NoError(t, err)

	b64WASM := base64.StdEncoding.EncodeToString(wantWASM)

	var tests = []struct {
		name        string
		givenParams params
		assert      func(t *testing.T, actual *waterImpl, err error)
	}{
		{
			name: "create new waterImpl with success",
			givenParams: params{
				raddr: "127.0.0.1",
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
				require.NotNil(t, actual.dialer)
				assert.NotNil(t, actual.reportDialCore)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			waterImpl, err := newWaterImpl(tt.givenParams.raddr, tt.givenParams.pc, tt.givenParams.reportDialCore)
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

	pc := &config.ProxyConfig{PluggableTransportSettings: map[string]string{"water_wasm": b64WASM}}
	testOp := ops.Begin("test")
	defer testOp.End()

	var tests = []struct {
		name                string
		givenOp             *ops.Op
		givenCtx            context.Context
		givenReportDialCore reportDialCoreFn
		givenDialer         water.Dialer
		givenAddr           string
		assert              func(t *testing.T, actual net.Conn, err error)
	}{
		{
			name:     "should fail to dial when endpoint is not available",
			givenOp:  testOp,
			givenCtx: ctx,
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			waterImpl, err := newWaterImpl(tt.givenAddr, pc, tt.givenReportDialCore)
			require.NoError(t, err)
			conn, err := waterImpl.dialServer(tt.givenOp, tt.givenCtx)
			tt.assert(t, conn, err)
		})
	}
}
