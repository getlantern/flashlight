// Command opp can be used to determine the traffic log's memory overhead per packet.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/google/gopacket/pcapgo"
	"github.com/montanaflynn/stats"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/diagnostics/trafficlog"
)

var (
	flagPacketSize = flag.Uint("psize", 0, "size (in bytes) of packets stored in the buffer")
	flagBufferSize = flag.Uint("bsize", 0, "size (in bytes) of both the capture and save buffers")
	flagVerbose    = flag.Bool("v", false, "prints more information in the output")
)

func init() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintln(out, "Calculates the overhead-per-packet of the traffic log.")
		fmt.Fprintln(out, "\nIf no options are specified, a generally-correct figure is calculated.")
		fmt.Fprintln(out, "\nOptions:")
		flag.PrintDefaults()
	}
}

// This allows us to manipulate the amount of data stored per packet.
type testMutatorFactory struct {
	packetData []byte
}

func newFactory(bytesPerPacket int) testMutatorFactory {
	return testMutatorFactory{make([]byte, bytesPerPacket)}
}

func (f testMutatorFactory) MutatorFor(_ trafficlog.LinkType) trafficlog.PacketMutator {
	return func(pkt []byte, w io.Writer) error { _, err := w.Write(f.packetData); return err }
}

func countSavedPackets(addr string, tl *trafficlog.TrafficLog) (int, error) {
	tl.SaveCaptures(addr, time.Duration(math.MaxInt64))
	buf := new(bytes.Buffer)
	if err := tl.WritePcapng(buf); err != nil {
		return 0, errors.New("failed to write packets to pcapng: %v", err)
	}
	pcapR, err := pcapgo.NewNgReader(buf, pcapgo.NgReaderOptions{})
	if err != nil {
		return 0, errors.New("failed to initialize pcap reader: %v", err)
	}
	count := 0
	for {
		if _, _, err := pcapR.ReadPacketData(); err == io.EOF {
			break
		}
		count++
	}
	return count, nil
}

// Assumes the capture and save buffers are the same size.
func fillBuffers(tl *trafficlog.TrafficLog) (packetCount int, err error) {
	// Part of an IANA block reserved for future use. Shouldn't have anything listening on it,
	// though it wouldn't matter much for our purposes if it did.
	const blackHoleAddr = "240.0.0.0:999"

	conn, err := net.Dial("udp4", blackHoleAddr)
	if err != nil {
		return 0, errors.New("failed to create test network connection: %v", err)
	}

	if err := tl.UpdateAddresses([]string{blackHoleAddr}); err != nil {
		return 0, errors.New("failed to log test traffic: %v", err)
	}

	// Doesn't matter what we write - we'll replace the contents anyway.
	payload := []byte{0}

	currentCount, lastCount := 0, -1
	for currentCount > lastCount {
		// Send a bunch of packets and see if we fill the buffer.
		for i := 0; i < 100; i++ {
			if _, err := conn.Write(payload); err != nil {
				return 0, errors.New("failed to write to listener: %v", err)
			}
		}

		select {
		case err := <-tl.Errors():
			return 0, errors.New("capture error: %v", err)
		default:
		}

		if currentCount != 0 {
			lastCount = currentCount
		}
		currentCount, err = countSavedPackets(blackHoleAddr, tl)
		if err != nil {
			return 0, errors.New("failed to count packets in save buffer: %v", err)
		}
	}

	// Buffers are full now. Total packet count is twice what we found in the save buffer (since the
	// capture buffer holds the same amount).
	return 2 * currentCount, nil
}

type memStats struct {
	PacketCount, Allocated int
}

func memstatsFor(packetSize, bufferSize int, separateProcess bool) (*memStats, error) {
	if separateProcess {
		cmd := exec.Command(
			os.Args[0],
			"-psize", strconv.Itoa(packetSize),
			"-bsize", strconv.Itoa(bufferSize),
		)
		out, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return nil, errors.New(string(exitErr.Stderr))
			}
			return nil, err
		}
		memstats := new(memStats)
		if err := json.Unmarshal(out, memstats); err != nil {
			return nil, errors.New("failed to unmarshal stats: %v", err)
		}
		return memstats, nil
	}

	opts := trafficlog.Options{
		MTULimit:       trafficlog.MTULimitNone,
		MutatorFactory: newFactory(int(*flagPacketSize)),
	}
	tl := trafficlog.New(bufferSize, bufferSize, &opts)

	packetCount, err := fillBuffers(tl)
	if err != nil {
		return nil, errors.New("failed to fill buffers: %v", err)
	}

	var memstats runtime.MemStats
	time.Sleep(150 * time.Millisecond)
	runtime.GC()
	runtime.ReadMemStats(&memstats)

	return &memStats{packetCount, int(memstats.Alloc)}, nil
}

type testCase struct {
	PacketSize, BufferSize int
	memStats
}

// Format of the output in the general case (no options specified).
type outputStats struct {
	BaseAllocated             int `json:",omitempty"`
	MeanPacketOverhead        float64
	OverheadStandardDeviation float64
	TestCases                 []testCase `json:",omitempty"`
}

func fail(a ...interface{}) {
	fmt.Fprintln(os.Stderr, a...)
	os.Exit(1)
}

func main() {
	flag.Parse()
	trafficlog.SetMeasurementMode(true)

	if *flagPacketSize+*flagBufferSize > 0 && *flagPacketSize**flagBufferSize == 0 {
		fail("both packet size and buffer size must be specified")
	}
	if *flagPacketSize != 0 {
		packetSize, bufferSize := int(*flagPacketSize), int(*flagBufferSize)
		memstats, err := memstatsFor(packetSize, bufferSize, false)
		if err != nil {
			fail(err)
		}
		if err := json.NewEncoder(os.Stdout).Encode(memstats); err != nil {
			fail("failed to encode stats as JSON:", err)
		}
		os.Exit(0)
	}

	memstats, err := memstatsFor(1, 1, true)
	if err != nil {
		fail(err)
	}
	output := outputStats{}
	output.BaseAllocated = memstats.Allocated
	output.TestCases = []testCase{}

	packetOverheadData := []int{}
	for _, input := range []struct{ packetSize, bufferSize int }{
		// Gather statistics for a range of packet and buffer sizes. We use packet sizes which are
		// factors of the buffer sizes to make the math a bit more straight-forward.
		//
		// Note that these calculations don't work well for low packet counts (n < ~200). Low
		// packet counts shouldn't be seen much in practice anyway. For very large packet sizes or
		// counts, the overhead per packet begins to decrease. We don't calculcate for these cases
		// as we prefer to overestimate.
		{4, 2000},
		{4, 4000},
		{4, 8000},
		{4, 16000},
		{4, 32000},
		{2, 8000},
		{8, 8000},
		{16, 8000},
		{32, 8000},
	} {
		memstats, err := memstatsFor(input.packetSize, input.bufferSize, true)
		if err != nil {
			fail(err)
		}
		additionalMemoryAlloc := int(memstats.Allocated - output.BaseAllocated)
		packetData := memstats.PacketCount * input.packetSize
		overheadPerPacket := (additionalMemoryAlloc - packetData) / memstats.PacketCount
		packetOverheadData = append(packetOverheadData, overheadPerPacket)

		output.TestCases = append(output.TestCases, testCase{
			input.packetSize, input.bufferSize, *memstats,
		})
	}

	packetOverheadStats := stats.LoadRawData(packetOverheadData)
	output.MeanPacketOverhead, err = packetOverheadStats.Mean()
	if err != nil {
		fail("failed to calculate packet overhead mean:", err)
	}
	output.OverheadStandardDeviation, err = packetOverheadStats.StandardDeviationPopulation()
	if err != nil {
		fail("failed to calculate packet overhead standard deviation:", err)
	}

	enc := json.NewEncoder(os.Stdout)
	if *flagVerbose {
		enc.SetIndent("", "\t")
	} else {
		output.TestCases = nil
		output.BaseAllocated = 0
	}

	if enc.Encode(output); err != nil {
		fail("failed to encode output as JSON:", err)
	}
}
