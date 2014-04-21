package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

const (
	STATSHUB_URL_TEMPLATE = "https://pure-journey-3547.herokuapp.com/stats/%s"
)

var (
	bytesGivenChan        = make(chan int, 1000)
	REPORT_STATS_INTERVAL = 20 * time.Second
)

// reportStats reports statistics for this proxy to statshub under the given
// instanceId
func reportStats(instanceId string) {
	checkpoint := make(chan bool)
	checkpointResult := make(chan int)

	// Collect bytesGiven from the countingConn
	go func() {
		bytesSum := 0

		for {
			select {
			case bytesGiven := <-bytesGivenChan:
				bytesSum += bytesGiven
			case <-checkpoint:
				checkpointResult <- bytesSum
				bytesSum = 0
			}
		}
	}()

	// Periodically checkpoint the total bytes given and report them to statshub
	// via HTTP post
	go func() {
		for {
			nextInterval := time.Now().Truncate(REPORT_STATS_INTERVAL).Add(REPORT_STATS_INTERVAL)
			waitTime := nextInterval.Sub(time.Now())
			time.Sleep(waitTime)
			checkpoint <- true
			bytesSum := <-checkpointResult
			report := map[string]interface{}{
				"dims": map[string]string{},
				"increments": map[string]int{
					"bytesGivenFallback": bytesSum,
				},
			}

			jsonBytes, err := json.Marshal(report)
			if err != nil {
				log.Printf("Unable to marshal json for stats: %s", err)
				continue
			}

			url := fmt.Sprintf(STATSHUB_URL_TEMPLATE, instanceId)
			resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBytes))
			if err != nil {
				log.Printf("Unable to post stats to statshub: %s", err)
				continue
			}
			if resp.StatusCode != 200 {
				log.Printf("Unexpected response status posting stats to statshub: %d", resp.StatusCode)
				continue
			}
			log.Printf("Reported %d bytes given to statshub", bytesSum)
		}
	}()
}

// countingConn is a wrapper for net.Conn that counts bytes
type countingConn struct {
	orig net.Conn
}

func (c *countingConn) Read(b []byte) (n int, err error) {
	n, err = c.orig.Read(b)
	bytesGivenChan <- n
	return
}

func (c *countingConn) Write(b []byte) (n int, err error) {
	n, err = c.orig.Write(b)
	bytesGivenChan <- n
	return
}

func (c *countingConn) Close() error {
	return c.orig.Close()
}

func (c *countingConn) LocalAddr() net.Addr {
	return c.orig.LocalAddr()
}

func (c *countingConn) RemoteAddr() net.Addr {
	return c.orig.RemoteAddr()
}

func (c *countingConn) SetDeadline(t time.Time) error {
	return c.orig.SetDeadline(t)
}

func (c *countingConn) SetReadDeadline(t time.Time) error {
	return c.orig.SetReadDeadline(t)
}

func (c *countingConn) SetWriteDeadline(t time.Time) error {
	return c.orig.SetWriteDeadline(t)
}
