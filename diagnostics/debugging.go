package diagnostics

import (
	"fmt"
	"time"

	"github.com/google/gopacket/pcap"
)

func println(a ...interface{}) {
	fmt.Print("TRAFFICLOG: ", fmt.Sprintln(a...))
}

func printf(s string, a ...interface{}) {
	fmt.Printf("TRAFFICLOG: %s\n", fmt.Sprintf(s, a...))
}

type handleStatsTracker struct {
	received, dropped int
	lastCall          time.Time
}

func newHandleStatsTracker() *handleStatsTracker {
	return &handleStatsTracker{0, 0, time.Now()}
}

// Should not be called concurrently with other methods on h.
func (t *handleStatsTracker) printStats(h *pcap.Handle) {
	const meanPacketSize = 875

	handleStats, err := h.Stats()
	if err != nil {
		println("failed to obtain handle stats:", err)
		return
	}

	var (
		newlyReceived        = handleStats.PacketsReceived - t.received
		newlyDropped         = handleStats.PacketsDropped - t.dropped
		currentDropRate      = float64(newlyDropped) / float64(newlyDropped+newlyReceived)
		overallDropRate      = float64(handleStats.PacketsDropped) / float64(handleStats.PacketsReceived+handleStats.PacketsDropped)
		currentTime          = time.Now()
		timePeriod           = currentTime.Sub(t.lastCall)
		ingressRate          = (float64(newlyReceived) / float64(timePeriod)) * float64(1e9)
		estimatedConsumption = ingressRate * float64(meanPacketSize) / float64(125000)
	)

	printf(
		"capture stats:\n\ttotal received packets: %d\n\ttotal dropped packets: %d\n\tcurrent drop rate: %.2f %%\n\ttotal drop rate: %.2f %%\n\tingress rate: %.f pkt/s\n\testimated consumption: %.1f Mbps\n\ttime period: %v\n",
		handleStats.PacketsReceived,
		handleStats.PacketsDropped,
		100*currentDropRate,
		100*overallDropRate,
		ingressRate,
		estimatedConsumption,
		timePeriod,
	)
	t.received = handleStats.PacketsReceived
	t.dropped = handleStats.PacketsDropped
	t.lastCall = currentTime
}
