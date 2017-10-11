package util

import (
	"net"
	"time"

	"github.com/getlantern/golog"
)

var wlog = golog.LoggerFor("flashlight.util")

//WaitForServer continually tries to hit a server at the specified, typically
//local, address.
func WaitForServer(addr string) {
	for {
		conn, err := net.DialTimeout("tcp", addr, 4*time.Second)
		if err != nil {
			wlog.Debugf("Nothing listening at %v", addr)
			time.Sleep(200 * time.Millisecond)
			continue
		}
		conn.Close()
		break
	}
}
