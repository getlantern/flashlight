package diagnostics

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket/pcap"
)

var (
	numPackets  = int64(0)
	metricsLock = new(sync.Mutex)
)

func println(a ...interface{}) {
	fmt.Print("TRAFFICLOG: ", fmt.Sprintln(a...))
}

func printf(s string, a ...interface{}) {
	fmt.Printf("TRAFFICLOG: %s\n", fmt.Sprintf(s, a...))
}

func incrementNumPackets() {
	newNumber := atomic.AddInt64(&numPackets, 1)
	if newNumber%10000 == 0 {
		printf("captured %d packets", newNumber)
	}
}

func watchHandle(handle *pcap.Handle) {
	packetsLastMinute, droppedLastMinute := int64(0), int64(0)
	for {
		func() {
			time.Sleep(time.Minute)
			metricsLock.Lock()
			defer metricsLock.Unlock()

			handleStats, err := handle.Stats()
			if err != nil {
				println("failed to obtain handle stats:", err)
				return
			}

			pktsPastMinute := numPackets - packetsLastMinute
			droppedPastMinute := int64(handleStats.PacketsDropped) - droppedLastMinute
			printf(
				"TRAFFICLOG:\n\tdrop rate past minute: %.2f %%\n\tingress past minute: %d\n\t; droppedPastMinute: %d\n",
				100*float64(droppedPastMinute)/float64(pktsPastMinute+droppedPastMinute),
				pktsPastMinute,
				droppedPastMinute,
			)
			packetsLastMinute = numPackets
			droppedLastMinute = int64(handleStats.PacketsDropped)
		}()
	}
}
