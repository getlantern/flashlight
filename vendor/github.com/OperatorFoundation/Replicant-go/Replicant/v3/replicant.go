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

// Package replicant provides a PT 2.1 Go API implementation of the Replicant adversary-tunable transport
package replicant

import (
	"errors"
	"fmt"
	"net"

	"github.com/OperatorFoundation/Replicant-go/Replicant/v3/polish"
	"github.com/OperatorFoundation/Replicant-go/Replicant/v3/toneburst"
)

type ConnectionState struct {
	toneburst toneburst.ToneBurst
	polish    polish.Connection
}

type replicantTransportListener struct {
	listener *net.TCPListener
	config   ServerConfig
}

func newReplicantTransportListener(listener *net.TCPListener, config ServerConfig) *replicantTransportListener {
	return &replicantTransportListener{listener: listener, config: config}
}

func NewClientConnection(conn net.Conn, config ClientConfig) (net.Conn, error) {
	// Initialize a client connection.

	state, clientError := NewReplicantClientConnectionState(config)
	if clientError != nil {
		return nil, clientError
	}

	if state.toneburst != nil {
		err := state.toneburst.Perform(conn)
		if err != nil {
			return nil, err
		}
	}
	if state.polish != nil {
		rconn, err := state.polish.Handshake(conn)
		if err != nil {
			return nil, err
		}
		return rconn, nil
	}

	return conn, nil
}

func NewServerConnection(conn net.Conn, config ServerConfig) (net.Conn, error) {
	// Initialize a client connection.
	var polishServer polish.Server
	var serverError error

	if config.Polish != nil {
		polishServer, serverError = config.Polish.Construct()
		if serverError != nil {
			fmt.Printf("error: %s", serverError)
			return nil, serverError
		}
	}

	state, connError := NewReplicantServerConnectionState(config, polishServer, conn)
	if connError != nil {
		fmt.Printf("error: %s", connError)
		return nil, connError
	}

	if state.toneburst != nil {
		err := state.toneburst.Perform(conn)
		if err != nil {
			fmt.Println("> Toneburst error: ", err.Error())
			return nil, err
		}
	}

	if state.polish != nil {
		rconn, err := state.polish.Handshake(conn)
		if err != nil {
			fmt.Println("> Polish handshake failed", err.Error())
			return nil, err
		}
		if rconn == nil {
			return nil, errors.New("handshake returned nil")
		}
		return rconn, nil
	}

	return conn, nil
}

func NewReplicantClientConnectionState(config ClientConfig) (*ConnectionState, error) {
	var tb toneburst.ToneBurst
	var toneburstError error
	var p polish.Connection
	var polishError error

	if config.Toneburst != nil {
		tb, toneburstError = config.Toneburst.Construct()
		if toneburstError != nil {
			return nil, toneburstError
		}
	}

	if config.Polish != nil {
		p, polishError = config.Polish.Construct()
		if polishError != nil {
			return nil, polishError
		}
	}

	return &ConnectionState{tb, p}, nil
}

func NewReplicantServerConnectionState(config ServerConfig, polishServer polish.Server, conn net.Conn) (*ConnectionState, error) {
	var tb toneburst.ToneBurst
	var toneburstError error
	var p polish.Connection

	if config.Toneburst != nil {
		tb, toneburstError = config.Toneburst.Construct()
		if toneburstError != nil {
			return nil, toneburstError
		}
	}

	if polishServer != nil {
		p = polishServer.NewConnection(conn)
	}

	return &ConnectionState{tb, p}, nil
}
