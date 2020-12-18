package ios

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	mtu = 1500

	userConfig = `
deviceID: 12345678
userID: 239316353
token: WnVUQIhV9UJx0rVs-zTWs6sXkwEgce4Rp5kFiYos5ble_H2R60Qpdw 
language: en-US
country: us
allowProbes: false`
)

const (
	msg = "hello there"
)

func TestClientUDP(t *testing.T) {
	serverAddr, stopServer := startUDPEchoServer(t)
	defer stopServer()

	serverHost, _serverPort, _ := net.SplitHostPort(serverAddr)
	serverPort, _ := strconv.Atoi(_serverPort)

	tmpDir, err := ioutil.TempDir("", "client_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	err = ioutil.WriteFile(filepath.Join(tmpDir, "userconfig.yaml"), []byte(userConfig), 0644)
	require.NoError(t, err)

	dialer := &realUDPDialer{receivedMessages: make(chan []byte, 100)}
	w, err := Client(&noopResponseWriter{}, dialer, &noopMemChecker{}, tmpDir, mtu, "8.8.8.8", "8.8.4.4")
	require.NoError(t, err)

	w.Write(udpPacket(t, serverHost, serverPort, msg))

	select {
	case echoed := <-dialer.receivedMessages:
		assert.Equal(t, msg, string(echoed))
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for echo response")
	}
}

func startUDPEchoServer(t *testing.T) (string, func()) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)
	conn, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)
	go func() {
		b := make([]byte, mtu)
		for {
			n, raddr, err := conn.ReadFromUDP(b)
			if err != nil {
				return
			}
			log.Debugf("Echoing: %v", string(b[:n]))
			conn.WriteToUDP(b[:n], raddr)
		}
	}()
	return conn.LocalAddr().String(), func() {
		conn.Close()
	}
}

func udpPacket(t *testing.T, serverHost string, serverPort int, payload string) []byte {
	ip := &layers.IPv4{
		Version:  4,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.ParseIP("127.0.0.1"),
		DstIP:    net.ParseIP(serverHost),
	}
	udp := &layers.UDP{
		SrcPort: 7890,
		DstPort: layers.UDPPort(serverPort),
	}
	udp.SetNetworkLayerForChecksum(ip)
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	err := gopacket.SerializeLayers(buf, opts, ip, udp, gopacket.Payload(payload))
	require.NoError(t, err)
	return buf.Bytes()
}

type noopResponseWriter struct {
}

func (rw *noopResponseWriter) Write(b []byte) bool {
	return true
}

type realUDPDialer struct {
	receivedMessages chan []byte
}

func (d *realUDPDialer) Dial(host string, port int) UDPConn {
	return &realUDPConn{receivedMessages: d.receivedMessages, host: host, port: port}
}

type realUDPConn struct {
	receivedMessages chan []byte
	host             string
	port             int
	*net.UDPConn
	cb *UDPCallbacks
}

func (conn *realUDPConn) RegisterCallbacks(cb *UDPCallbacks) {
	conn.cb = cb
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%v:%d", conn.host, conn.port))
	if err != nil {
		conn.closeOnError(err)
		return
	}

	uconn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		conn.closeOnError(err)
		return
	}

	conn.UDPConn = uconn
	conn.cb.OnDialSucceeded()
}

func (conn *realUDPConn) WriteDatagram(b []byte) {
	_, err := conn.UDPConn.Write(b)
	if err != nil {
		conn.cb.OnError(err)
	}
}

func (conn *realUDPConn) ReceiveDatagram() {
	go func() {
		b := make([]byte, mtu)
		n, err := conn.UDPConn.Read(b)
		if err != nil {
			conn.cb.OnError(err)
			return
		}
		conn.cb.OnReceive(b[:n])
		conn.receivedMessages <- b[:n]
	}()
}

func (conn *realUDPConn) Close() {
	err := conn.UDPConn.Close()
	if err != nil {
		conn.cb.OnError(err)
	}
	conn.cb.OnClose()
}

func (conn *realUDPConn) closeOnError(err error) {
	conn.cb.OnError(err)
	if conn.UDPConn != nil {
		conn.UDPConn.Close()
	} else {
		conn.cb.OnClose()
	}
}

type noopMemChecker struct{}

func (c *noopMemChecker) BytesBeforeCritical() int {
	return 1000000
}
