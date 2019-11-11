package trafficlog

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"os"
	"testing"

	"github.com/getlantern/errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestAppStripper(t *testing.T) {
	// 100 ethernet-level packets (frames).
	const packetsFile = "testdata/100.pkts"

	pkts, err := readPacketsFile(packetsFile)
	require.NoError(t, err)

	stripped := make([][]byte, len(pkts))
	stripper := new(AppStripperFactory).MutatorFor(LinkTypeEthernet)
	for i, pkt := range pkts {
		buf := new(bytes.Buffer)
		require.NoError(t, stripper(pkt, buf))
		stripped[i] = buf.Bytes()
	}

	for i := range pkts {
		decoded := gopacket.NewPacket(pkts[i], layers.LayerTypeEthernet, gopacket.Default)
		if decoded.ErrorLayer() != nil {
			t.Fatal(decoded.ErrorLayer().Error())
		}
		strippedDecoded := gopacket.NewPacket(stripped[i], layers.LayerTypeEthernet, gopacket.Default)
		if strippedDecoded.ErrorLayer() != nil {
			t.Fatal(strippedDecoded.ErrorLayer().Error())
		}

		require.Equal(t, decoded.NetworkLayer().LayerContents(), strippedDecoded.NetworkLayer().LayerContents())
		require.Equal(t, decoded.TransportLayer().LayerContents(), strippedDecoded.TransportLayer().LayerContents())
		require.Empty(t, strippedDecoded.TransportLayer().LayerPayload())
	}
}

func BenchmarkAppStripper(b *testing.B) {
	// This file has 100 packets with a mean packet size close to what we see empirically (~750
	// bytes). The packets are from an actual capture and reflect variance we see in practice.
	const filename = "testdata/100.pkts"

	pkts, err := readPacketsFile(filename)
	if err != nil {
		b.Fatal(err)
	}
	stripper := new(AppStripperFactory).MutatorFor(LinkTypeEthernet)
	b.ResetTimer()

	for i := 0; i < b.N/100; i++ {
		for j := 0; j < len(pkts); j++ {
			if err := stripper(pkts[j], ioutil.Discard); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func readPacketsFile(filename string) (packets [][]byte, err error) {
	// packetsFile is the format of .pkts files in the testdata directory.
	type packetsFile struct {
		Packets [][]byte
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, errors.New("failed to open file: %v", err)
	}
	defer f.Close()

	pf := new(packetsFile)
	if err := gob.NewDecoder(f).Decode(pf); err != nil {
		return nil, errors.New("failed to decode file: %v", err)
	}
	return pf.Packets, nil
}
