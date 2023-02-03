package polish

import (
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"

	"github.com/OperatorFoundation/go-shadowsocks2/darkstar"
	"github.com/aead/ecdh"
)

type DarkStarPolishServerConfig struct {
	ServerPrivateKey []byte
	Host             string
	Port             int
}

type DarkStarPolishClientConfig struct {
	ServerPublicKey []byte
	Host            string
	Port            int
}

type DarkStarPolishServer struct {
	darkStarServer darkstar.DarkStarServer
}

type DarkStarPolishServerConnection struct {
	darkStarServer darkstar.DarkStarServer
	conn           net.Conn
}

type DarkStarPolishClientConnection struct {
	darkStarClient darkstar.DarkStarClient
}

func (serverConfig DarkStarPolishServerConfig) Construct() (Server, error) {
	return NewDarkStarServer(serverConfig), nil
}

func (clientConfig DarkStarPolishClientConfig) Construct() (Connection, error) {
	return NewDarkStarClient(clientConfig), nil
}

func (server DarkStarPolishServer) NewConnection(conn net.Conn) Connection {
	// this does the DarkStar handshake
	// serverStreamConn, connError := server.darkStarServer.StreamConn(conn)
	// fmt.Printf("streamConn type: %T\n", serverStreamConn)
	// if connError != nil {
	// 	return nil
	// }

	return &DarkStarPolishServerConnection{
		darkStarServer: server.darkStarServer,
		conn:           conn,
	}
}

// TODO: handshake behavior is already performed in DarkStarPolishServer.darkStarServer.StreamConn()
func (serverConn *DarkStarPolishServerConnection) Handshake(conn net.Conn) (net.Conn, error) {
	streamConn, connError := serverConn.darkStarServer.StreamConn(conn)
	if connError != nil {
		return nil, connError
	}
	if streamConn == nil {
		return nil, errors.New("streamConn in server handshake returned nil")
	}
	return streamConn, nil
}

func (clientConn *DarkStarPolishClientConnection) Handshake(conn net.Conn) (net.Conn, error) {
	streamConn, connError := clientConn.darkStarClient.StreamConn(conn)
	if connError != nil {
		return nil, connError
	}
	if streamConn == nil {
		return nil, errors.New("streamConn in client handshake returned nil")
	}
	return streamConn, nil
}

func NewDarkStarPolishClientConfigFromPrivate(serverPrivateKey crypto.PrivateKey, host string, port int) (*DarkStarPolishClientConfig, error) {
	keyExchange := ecdh.Generic(elliptic.P256())
	serverPublicKey := keyExchange.PublicKey(serverPrivateKey)
	fmt.Print("server publicKey: ")
	fmt.Println(serverPublicKey)
	publicKeyBytes, keyError := darkstar.PublicKeyToBytes(serverPublicKey)
	if keyError != nil {
		return nil, keyError
	}

	return &DarkStarPolishClientConfig{
		ServerPublicKey: publicKeyBytes,
		Host:            host,
		Port:            port,
	}, nil
}

func NewDarkStarPolishClientConfig(serverPublicKey []byte, host string, port int) (*DarkStarPolishClientConfig, error) {
	return &DarkStarPolishClientConfig{
		ServerPublicKey: serverPublicKey,
		Host:            host,
		Port:            port,
	}, nil
}

func NewDarkStarPolishServerConfig(host string, port int) (*DarkStarPolishServerConfig, error) {
	keyExchange := ecdh.Generic(elliptic.P256())
	serverEphemeralPrivateKey, _, keyError := keyExchange.GenerateKey(rand.Reader)
	if keyError != nil {
		return nil, keyError
	}

	privateKeyBytes, ok := serverEphemeralPrivateKey.([]byte)
	if !ok {
		return nil, errors.New("could not convert private key to bytes")
	}

	return &DarkStarPolishServerConfig{
		ServerPrivateKey: privateKeyBytes,
		Host:             host,
		Port:             port,
	}, nil
}

func NewDarkStarClient(config DarkStarPolishClientConfig) Connection {
	publicKeyString := hex.EncodeToString(config.ServerPublicKey)
	darkStarClient := darkstar.NewDarkStarClient(publicKeyString, config.Host, config.Port)
	darkStarClientConnection := DarkStarPolishClientConnection{
		darkStarClient: *darkStarClient,
	}
	return &darkStarClientConnection
}

func NewDarkStarServer(config DarkStarPolishServerConfig) DarkStarPolishServer {
	privateKeyString := hex.EncodeToString(config.ServerPrivateKey)
	darkStarServer := darkstar.NewDarkStarServer(privateKeyString, config.Host, config.Port)
	darkStarPolishServer := DarkStarPolishServer{
		darkStarServer: *darkStarServer,
	}
	return darkStarPolishServer
}
