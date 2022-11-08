package Starbridge

import (
	"crypto"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"strconv"
	"time"

	replicant "github.com/OperatorFoundation/Replicant-go/Replicant/v3"
	"github.com/OperatorFoundation/Replicant-go/Replicant/v3/polish"
	"github.com/OperatorFoundation/Replicant-go/Replicant/v3/toneburst"
	"github.com/OperatorFoundation/go-shadowsocks2/darkstar"
	pt "github.com/OperatorFoundation/shapeshifter-ipc/v3"
	"github.com/aead/ecdh"
	"golang.org/x/net/proxy"
)

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

type ClientConfig struct {
	Address                   string `json:"serverAddress"`
	ServerPersistentPublicKey string `json:"serverPersistentPublicKey"`
}

type ServerConfig struct {
	ServerPersistentPrivateKey string `json:"serverPersistentPrivateKey"`
}

type starbridgeTransportListener struct {
	address  string
	listener *net.TCPListener
	config   ServerConfig
}

func newStarbridgeTransportListener(address string, listener *net.TCPListener, config ServerConfig) *starbridgeTransportListener {
	return &starbridgeTransportListener{address: address, listener: listener, config: config}
}

func (listener *starbridgeTransportListener) Addr() net.Addr {
	interfaces, _ := net.Interfaces()
	addrs, _ := interfaces[0].Addrs()
	return addrs[0]
}

// Accept waits for and returns the next connection to the listener.
func (listener *starbridgeTransportListener) Accept() (net.Conn, error) {
	host, portString, splitError := net.SplitHostPort(listener.address)
	if splitError != nil {
		return nil, splitError
	}

	port, intError := strconv.Atoi(portString)
	if intError != nil {
		return nil, intError
	}

	if len(listener.config.ServerPersistentPrivateKey) != 64 {
		return nil, errors.New("incorrect key size")
	}

	keyBytes, keyError := hex.DecodeString(listener.config.ServerPersistentPrivateKey)
	if keyError != nil {
		return nil, keyError
	}

	keyCheckSuccess := CheckPrivateKey(keyBytes)
	if !keyCheckSuccess {
		return nil, errors.New("bad private key")
	}

	replicantConfig := getServerConfig(host, port, keyBytes)

	conn, err := listener.listener.Accept()
	if err != nil {
		return nil, err
	}

	serverConn, serverError := NewServerConnection(replicantConfig, conn)
	if serverError != nil {
		conn.Close()
		return nil, serverError
	}

	return serverConn, nil
}

// Close closes the transport listener.
// Any blocked Accept operations will be unblocked and return errors.
func (listener *starbridgeTransportListener) Close() error {
	return listener.listener.Close()
}

// Listen checks for a working connection
func (config ServerConfig) Listen(address string) (net.Listener, error) {
	addr, resolveErr := pt.ResolveAddr(address)
	if resolveErr != nil {
		return nil, resolveErr
	}

	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return newStarbridgeTransportListener(address, ln, config), nil
}

// Dial connects to the address on the named network
func (config ClientConfig) Dial(address string) (net.Conn, error) {
	host, portString, splitError := net.SplitHostPort(config.Address)
	if splitError != nil {
		return nil, splitError
	}

	port, intError := strconv.Atoi(portString)
	if intError != nil {
		return nil, intError
	}

	if len(config.ServerPersistentPublicKey) != 64 {
		return nil, errors.New("incorrect key size")
	}

	keyBytes, keyError := hex.DecodeString(config.ServerPersistentPublicKey)
	if keyError != nil {
		return nil, keyError
	}

	publicKey := darkstar.BytesToPublicKey(keyBytes)

	keyCheckError := CheckPublicKey(publicKey)
	if keyCheckError != nil {
		return nil, keyCheckError
	}

	dialTimeout := time.Minute * 5
	conn, dialErr := net.DialTimeout("tcp", address, dialTimeout)
	if dialErr != nil {
		return nil, dialErr
	}

	replicantConfig := getClientConfig(host, port, keyBytes)
	transportConn, err := NewClientConnection(replicantConfig, conn)

	if err != nil {
		if conn != nil {
			_ = conn.Close()
		}
		return nil, err
	}

	return transportConn, nil
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

// Dial creates outgoing transport connection
func (transport *TransportClient) Dial() (net.Conn, error) {
	host, portString, splitError := net.SplitHostPort(transport.Address)
	if splitError != nil {
		return nil, splitError
	}

	port, intError := strconv.Atoi(portString)
	if intError != nil {
		return nil, intError
	}

	if len(transport.Config.ServerPersistentPublicKey) != 64 {
		return nil, errors.New("incorrect key size")
	}

	keyBytes, keyError := hex.DecodeString(transport.Config.ServerPersistentPublicKey)
	if keyError != nil {
		return nil, keyError
	}

	publicKey := darkstar.BytesToPublicKey(keyBytes)

	keyCheckError := CheckPublicKey(publicKey)
	if keyCheckError != nil {
		return nil, keyCheckError
	}

	replicantConfig := getClientConfig(host, port, keyBytes)

	dialTimeout := time.Minute * 5
	conn, dialErr := net.DialTimeout("tcp", transport.Address, dialTimeout)
	if dialErr != nil {
		return nil, dialErr
	}

	dialConn := conn

	transportConn, err := NewClientConnection(replicantConfig, conn)

	if err != nil {
		_ = dialConn.Close()
		return nil, err
	}

	return transportConn, nil
}

func (transport *TransportServer) Listen() (net.Listener, error) {
	addr, resolveErr := pt.ResolveAddr(transport.Address)
	if resolveErr != nil {
		return nil, resolveErr
	}

	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return newStarbridgeTransportListener(transport.Address, ln, transport.Config), nil
}

func NewClientConnection(config replicant.ClientConfig, conn net.Conn) (net.Conn, error) {
	return replicant.NewClientConnection(conn, config)
}

func NewServerConnection(config replicant.ServerConfig, conn net.Conn) (net.Conn, error) {
	return replicant.NewServerConnection(conn, config)
}

func NewReplicantClientConnectionState(config replicant.ClientConfig) (*replicant.ConnectionState, error) {
	return replicant.NewReplicantClientConnectionState(config)
}

func NewReplicantServerConnectionState(config replicant.ServerConfig, polishServer polish.Server, conn net.Conn) (*replicant.ConnectionState, error) {
	return replicant.NewReplicantServerConnectionState(config, polishServer, conn)
}

func getClientConfig(host string, port int, serverPublicKey []byte) replicant.ClientConfig {
	polishClientConfig := polish.DarkStarPolishClientConfig{
		Host:            host,
		Port:            port,
		ServerPublicKey: serverPublicKey,
	}

	toneburstClientConfig := toneburst.StarburstConfig{
		FunctionName: "SMTPClient",
	}

	clientConfig := replicant.ClientConfig{
		Toneburst: toneburstClientConfig,
		Polish:    polishClientConfig,
	}

	return clientConfig
}

func getServerConfig(host string, port int, serverPrivateKey []byte) replicant.ServerConfig {
	polishServerConfig := polish.DarkStarPolishServerConfig{
		Host:             host,
		Port:             port,
		ServerPrivateKey: serverPrivateKey,
	}

	toneburstServerConfig := toneburst.StarburstConfig{
		FunctionName: "SMTPServer",
	}

	serverConfig := replicant.ServerConfig{
		Toneburst: toneburstServerConfig,
		Polish:    polishServerConfig,
	}

	return serverConfig
}

func CheckPrivateKey(privKey crypto.PrivateKey) (success bool) {
	defer func() {
		if panicError := recover(); panicError != nil {
			success = false
		} else {
			success = true
		}
	}()
	
	keyExchange := ecdh.Generic(elliptic.P256())
	_, pubKey, keyError := keyExchange.GenerateKey(rand.Reader)
	if keyError != nil {
		success = false
		return
	}

	// verify that the given key bytes are on the chosen elliptic curve
	success = keyExchange.ComputeSecret(privKey, pubKey) != nil
	return 
}

func CheckPublicKey(pubkey crypto.PublicKey) (keyError error) {
	defer func() {
		if panicError := recover(); panicError != nil {
			keyError = errors.New("panicked on public key check")
		} 
	}()

	// verify that the given key bytes are on the chosen elliptic curve
	keyExchange := ecdh.Generic(elliptic.P256())
	result := keyExchange.Check(pubkey)
	keyError = result
	return 
}

func GenerateKeys() (publicKeyHex, privateKeyHex *string, keyError error) {
	keyExchange := ecdh.Generic(elliptic.P256())
	clientEphemeralPrivateKey, clientEphemeralPublicKeyPoint, keyError := keyExchange.GenerateKey(rand.Reader)
	if keyError != nil {
		return nil, nil, keyError
	}

	privateKeyBytes, ok := clientEphemeralPrivateKey.([]byte)
	if !ok {
		return nil, nil, errors.New("failed to convert privateKey to bytes")
	}

	publicKeyBytes, keyByteError := darkstar.PublicKeyToBytes(clientEphemeralPublicKeyPoint)
	if keyByteError != nil {
		return nil, nil, keyByteError
	}

	privateKey := hex.EncodeToString(privateKeyBytes)
	publicKey := hex.EncodeToString(publicKeyBytes)
	return &publicKey, &privateKey, nil
}

func GenerateNewConfigPair(serverHost string, serverPort int) (*ServerConfig, *ClientConfig, error) {
	portString := strconv.Itoa(serverPort)
	address := serverHost + ":" + portString
	publicKey, privateKey, keyError := GenerateKeys()
	if keyError != nil {
		return nil, nil, keyError
	}

	serverConfig := ServerConfig {
		ServerPersistentPrivateKey: *privateKey,
	}

	clientConfig := ClientConfig{
		Address: address,
		ServerPersistentPublicKey: *publicKey,
	}

	return &serverConfig, &clientConfig, nil
}

func GenerateConfigFiles(serverHost string, serverPort int) error {
	serverConfig, clientConfig, configError := GenerateNewConfigPair(serverHost, serverPort)
	if configError != nil {
		return configError
	}

	clientConfigBytes, clientMarshalError := json.Marshal(clientConfig)
	if clientMarshalError != nil {
		return clientMarshalError
	}

	serverConfigBytes, serverMarshalError := json.Marshal(serverConfig)
	if serverMarshalError != nil {
		return serverMarshalError
	}

	serverConfigWriteError := ioutil.WriteFile("StarbridgeServerConfig.json", serverConfigBytes, 0644)
	if serverConfigWriteError != nil {
		return serverConfigWriteError
	}

	clientConfigWriteError := ioutil.WriteFile("StarbridgeClientConfig.json", clientConfigBytes, 0644)
	if clientConfigWriteError != nil {
		return clientConfigWriteError
	}

	return nil
}
