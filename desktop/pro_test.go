package desktop

import (
	"fmt"
	"testing"
	"time"
)

func TestTimer(t *testing.T) {

	start := time.Now()
	retry := time.NewTimer(0)
	retryLater := func() {
		if !retry.Stop() {
			<-retry.C
		}
		retry.Reset(1 * time.Second)
	}

	go func() {
		for {
			select {
			case <-retry.C:
				fmt.Printf("Timer called after %v\n", time.Now().Sub(start))
				go retryLater()
			}
		}
	}()

	time.Sleep(10 * time.Second)
}
