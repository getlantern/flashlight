package chained

import (
	"encoding/hex"
	"testing"

	tls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/require"
)

var (
	firefoxSampleHello = "1603010200010001fc03033b6aa797e7119cb350c645bef5fee28a7a2c85b9d2027e0cc751d42642f000962095ffa63d2435d184d49adf9d929072fc76a227e0b2b4323e05562a50928282180022130113031302c02bc02fcca9cca8c02cc030c00ac009c013c014009c009d002f0035010001910000000e000c0000096c6f63616c686f737400170000ff01000100000a000e000c001d00170018001901000101000b00020100002300000010000e000c02683208687474702f312e310005000501000000000022000a000804030503060302030033006b0069001d0020d01ad620e95e47149d6c22f54c4904e5533a3f828ed16cb6122436dcdc5c3e1d0017004104235e144fbbc20753ccfcd2c4fec975a117b0a17ad721ea94fb943bcf2b71b0a7e7d63e2cfb648134686e07d3b894f0274cf5e0ac211f51f1ef269c8ddf6ba47d002b00050403040303000d0018001604030503060308040805080604010501060102030201002d00020101001c000240010015008d000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

	goodSample []byte
	badSample  []byte
)

func init() {
	var err error
	goodSample, err = hex.DecodeString(firefoxSampleHello)
	if err != nil {
		panic(err)
	}

	badSample = make([]byte, len(goodSample))
	copy(badSample, goodSample)
	badSample[0] = 0
}

func TestUTLSSpec(t *testing.T) {
	cases := []struct {
		name          string
		id            tls.ClientHelloID
		sample        []byte
		expectSpec    bool
		expectedError string
	}{
		{name: "successful custom hello", id: tls.HelloCustom, sample: goodSample, expectSpec: true},
		{name: "custom hello with empty sample", id: tls.HelloCustom, expectedError: "illegal combination of HelloCustom and nil sample hello"},
		{name: "custom hello with short sample", id: tls.HelloCustom, sample: []byte("hi"), expectedError: "sample hello is too small"},
		{name: "custom hello with bad sample", id: tls.HelloCustom, sample: badSample, expectedError: "failed to fingerprint"},
		{name: "non-custom hello", id: tls.HelloChrome_106_Shuffle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs := helloSpec{
				id:     tc.id,
				sample: tc.sample,
			}

			id, spec, err := hs.utlsSpec()
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.id, id)
				if tc.expectSpec {
					require.NotNil(t, spec)
				} else {
					require.Nil(t, spec)
				}
			}
		})
	}
}

func TestSupportsSessionTickets(t *testing.T) {
	cases := []struct {
		name                   string
		id                     tls.ClientHelloID
		sample                 []byte
		supportsSessionTickets bool
		expectedError          string
	}{
		{name: "successful custom hello", id: tls.HelloCustom, sample: goodSample, supportsSessionTickets: true},
		{name: "custom hello with empty sample", id: tls.HelloCustom, expectedError: "failed to get hello spec"},
		{name: "unknown hello", id: tls.ClientHelloID{Client: "unknown"}, expectedError: "failed to get spec for hello"},
		{name: "Chrome", id: tls.HelloChrome_106_Shuffle, supportsSessionTickets: true},
		{name: "Safari", id: tls.HelloSafari_16_0, supportsSessionTickets: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs := helloSpec{
				id:     tc.id,
				sample: tc.sample,
			}

			supportsSessionTickets, err := hs.supportsSessionTickets()
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.supportsSessionTickets, supportsSessionTickets)
			}
		})
	}
}
