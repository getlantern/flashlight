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
	checkpointCh          = make(chan bool)
	checkpointResultCh    = make(chan int)
)

// startReportingStats reports statistics for this proxy to statshub under the
// given instanceId
func startReportingStats(instanceId string) {
	go collectStats()
	go reportStats(instanceId)
}

// collectStats collects bytesGiven from the countingConn
func collectStats() {
	bytesSum := 0

	for {
		select {
		case bytesGiven := <-bytesGivenChan:
			bytesSum += bytesGiven
		case <-checkpointCh:
			checkpointResultCh <- bytesSum
			bytesSum = 0
		}
	}
}

// reportStats periodically checkpoints the total bytes given and reports them
// to statshub via HTTP post
func reportStats(instanceId string) {
	for {
		nextInterval := time.Now().Truncate(REPORT_STATS_INTERVAL).Add(REPORT_STATS_INTERVAL)
		waitTime := nextInterval.Sub(time.Now())
		time.Sleep(waitTime)
		checkpointCh <- true
		bytesSum := <-checkpointResultCh
		err := postStats(instanceId, bytesSum)
		if err != nil {
			log.Printf("Error on posting stats: %s", err)
		} else {
			log.Printf("Reported %d bytesGiven to statshub", bytesSum)
		}
	}
}

func postStats(instanceId string, bytesSum int) error {
	report := map[string]interface{}{
		"dims": map[string]string{},
		"increments": map[string]int{
			"bytesGivenFallback": bytesSum,
		},
	}

	jsonBytes, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("Unable to marshal json for stats: %s", err)
	}

	url := fmt.Sprintf(STATSHUB_URL_TEMPLATE, instanceId)
	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBytes))
	if err != nil {
		return fmt.Errorf("Unable to post stats to statshub: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Unexpected response status posting stats to statshub: %d", resp.StatusCode)
	}
	return nil
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
