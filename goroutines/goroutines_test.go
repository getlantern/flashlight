package goroutines

import (
	"sync"
	"testing"
	"time"
)

func TestMonitor(t *testing.T) {
	// Not a real test, just shows what the output looks like.
	stop := Monitor(30*time.Millisecond, 5, 2)
	defer stop()
	time.Sleep(100 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		time.Sleep(10 * time.Millisecond)
		wg.Add(1)
		go func() {
			time.Sleep(100 * time.Millisecond)
			wg.Done()
		}()
	}
	wg.Wait()
}
