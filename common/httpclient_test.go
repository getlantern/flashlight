package common

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDNSResolvers(t *testing.T) {
	t.Skip("Skipping DNS resolver tests for github")

	t.Run("DNS over HTTPS", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		failed := []string{}
		for _, host := range dohResolvers {
			req, err := http.NewRequest("GET", host+"?dns=AAABAAABAAAAAAAAB2V4YW1wbGUDY29tAAABAAE", nil)
			req.Header.Set("Accept", "application/dns-message")
			resp, err := client.Do(req)
			if !assert.NoError(t, err) {
				failed = append(failed, host)
				continue
			}
			_, err = io.ReadAll(resp.Body)
			if !assert.NoError(t, err) {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()
		}
		assert.Empty(t, failed, "All DNS over TLS resolvers failed")
	})
	t.Run("DNS over TLS", func(t *testing.T) {
		t.Skip("")
		failed := []string{}
		for _, host := range dotResolvers {
			t.Logf("Testing DNS over TLS resolver: %s", host)
			resolver := net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
					cfg := &tls.Config{
						ServerName:         host,
						InsecureSkipVerify: true,
					}
					addr = net.JoinHostPort(host, "853")
					return tls.DialWithDialer(new(net.Dialer), "tcp", addr, cfg)
				},
			}
			ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			resp, err := resolver.LookupHost(ctx, "google.com")
			if !assert.NoError(t, err) {
				failed = append(failed, host)
				continue
			}
			assert.Greater(t, len(resp), 0, "Expected at least one answer from %s", host)
		}
		assert.Empty(t, failed, "All DNS over TLS resolvers failed")
	})
}
