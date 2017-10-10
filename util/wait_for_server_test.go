package util

import (
	"net"
	"testing"
	"time"

	"github.com/getlantern/eventual"
	"github.com/stretchr/testify/assert"
)

func TestWaitForServer(t *testing.T) {
	cv := eventual.NewValue()
	go WaitForServer("127.0.0.1:8087")

	var ret interface{}
	var valid bool
	go func() {
		ret, valid = cv.Get(10 * time.Second)
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
	assert.True(t, valid)
	assert.NotNil(t, ret)
}
