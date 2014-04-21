package main

import (
	"log"
	"net"
	"time"
)

var (
	bytesGivenChan        = make(chan int, 1000)
	REPORT_STATS_INTERVAL = 20 * time.Second
)

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

func reportStats() {
	checkpoint := make(chan bool)
	checkpointResult := make(chan int)

	go func() {
		bytesSum := 0

		for {
			select {
			case bytesGiven := <-bytesGivenChan:
				bytesSum += bytesGiven
				log.Printf("bytesSum: %d", bytesSum)
			case <-checkpoint:
				log.Printf("checkpointing at: %d", bytesSum)
				checkpointResult <- bytesSum
				bytesSum = 0
			}
		}
	}()

	go func() {
		for {
			nextInterval := time.Now().Truncate(REPORT_STATS_INTERVAL).Add(REPORT_STATS_INTERVAL)
			waitTime := nextInterval.Sub(time.Now())
			time.Sleep(waitTime)
			checkpoint <- true
			bytesSum := <-checkpointResult
			// report
			log.Printf("reporting bytesSum = %d and zeroing", bytesSum)
		}
	}()
}
