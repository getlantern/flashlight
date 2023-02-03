/*
	MIT License

	Copyright (c) 2020 Operator Foundation

	Permission is hereby granted, free of charge, to any person obtaining a copy
	of this software and associated documentation files (the "Software"), to deal
	in the Software without restriction, including without limitation the rights
	to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
	copies of the Software, and to permit persons to whom the Software is
	furnished to do so, subject to the following conditions:

	The above copyright notice and this permission notice shall be included in all
	copies or substantial portions of the Software.

	THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
	IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
	FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
	AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
	LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
	OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
	SOFTWARE.
*/

package replicant

import (
	"net"
	"time"

	pt "github.com/OperatorFoundation/shapeshifter-ipc/v3"
	"golang.org/x/net/proxy"
)

// This makes Replicant compliant with Optimizer
type TransportClient struct {
	Config  ClientConfig
	Address string
	// TODO: Dialer can be removed later (both here and in dispatcher)
	Dialer  proxy.Dialer
}

type TransportServer struct {
	Config  ServerConfig
	Address string
	// TODO: Dialer can be removed later (both here and in dispatcher)
	Dialer  proxy.Dialer
}

func NewClient(config ClientConfig, dialer proxy.Dialer) TransportClient {
	return TransportClient{
		Config:  config,
		Address: config.Address,
		Dialer:  dialer,
	}
}

func NewServer(config ServerConfig, address string, dialer proxy.Dialer) TransportServer {
	return TransportServer{
		Config:  config,
		Address: address,
		Dialer:  dialer,
	}
}

func (transport TransportClient) Dial() (net.Conn, error) {
	dialTimeout := time.Minute * 5
	conn, dialErr := net.DialTimeout("tcp", transport.Address, dialTimeout)
	if dialErr != nil {
		return nil, dialErr
	}

	dialConn := conn
	transportConn, err := NewClientConnection(conn, transport.Config)
	if err != nil {
		_ = dialConn.Close()
		return nil, err
	}

	return transportConn, nil
}

func (transport TransportServer) Listen() (net.Listener, error) {
	addr, resolveErr := pt.ResolveAddr(transport.Address)
	if resolveErr != nil {
		return nil, resolveErr
	}

	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return newReplicantTransportListener(ln, transport.Config), nil
}
