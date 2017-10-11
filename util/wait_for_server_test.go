package util

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlantern/eventual"
	"github.com/stretchr/testify/assert"
)

func TestWaitForServer(t *testing.T) {
	cv := eventual.NewValue()
	go WaitForServer("127.0.0.1:8087")

	var ret atomic.Value
	var valid atomic.Value
	go func() {
		ret1, valid1 := cv.Get(10 * time.Second)
		ret.Store(ret1)
		valid.Store(valid1)
	}()

	time.Sleep(2 * time.Second)

	go func() {
		// listen on all interfaces
		ln, _ := net.Listen("tcp", ":8087")

		// accept connection on port
		conn, _ := ln.Accept()
		cv.Set(conn)
		conn.Close()
	}()

	time.Sleep(2 * time.Second)
	assert.True(t, valid.Load().(bool))
	assert.NotNil(t, ret.Load())
}
