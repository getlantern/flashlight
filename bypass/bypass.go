package client

import (
	"math/rand"
	"net/http"
	"time"
)

var done = make(chan bool, 1)

// Starts client access to the bypass server. The client periodically sends traffic to the server both via
// domain fronting and proxying to determine if proxies are blocked.
func Start() error {
	return StartWith(http.DefaultClient)
}

func StartWith(client *http.Client) error {
	// TODO: We need to pass the server here and somehow know all the servers we're proxying to.
	callRandomly(func() {
		client.Get("https://bypass.iantem.io/v1/")
	})
	return nil
}

func Close() {
	done <- true
}

func callRandomly(f func()) {
	for {
		select {
		case <-done:
			return
		case <-time.After(90 + time.Duration(rand.Intn(60))*time.Second):
			f()
		}
	}
}
