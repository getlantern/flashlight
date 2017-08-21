package ws

import (
	"io"
	"net/http"
	"sync"

	"github.com/getlantern/golog"
	"github.com/gorilla/websocket"
)

const (
	// MaxMessageSize determines the chunking size of messages used by gorilla
	MaxMessageSize = 1024
)

var (
	log      = golog.LoggerFor("flashlight.ws")
	upgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: MaxMessageSize,
		CheckOrigin:     func(r *http.Request) bool { return true }, // I need this to test Lantern UI from a different host.
	}
)

type ConnectFunc func(out chan<- []byte)

// clientChannels represents a data channel to/from the UI. UIChannel will have one
// underlying websocket connection for each connected browser window. All
// messages from any browser window are available via In and all messages sent
// to Out will be published to all browser windows.
type clientChannels struct {
	In  <-chan []byte
	Out chan<- []byte

	in  chan []byte
	out chan []byte

	muConns sync.Mutex
	nextId  int
	conns   map[int]*wsconn

	onConnect ConnectFunc
}

// newClients establishes a new channel acts as an http.Handler. When the UI
// connects to the handler, we will establish a websocket to the UI to carry
// messages for this UIChannel. The given onConnect function is called anytime
// that the UI connects.
func newClients(onConnect ConnectFunc) *clientChannels {
	in := make(chan []byte, 100)
	out := make(chan []byte)
	c := &clientChannels{
		In:        in,
		in:        in,
		Out:       out,
		out:       out,
		nextId:    0,
		conns:     make(map[int]*wsconn),
		onConnect: onConnect,
	}

	go c.writeAll()
	return c
}

func (c *clientChannels) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	log.Debugf("Got connection to the UI channel")
	var err error

	if req.Method != "GET" {
		http.Error(resp, "Method not allowed", 405)
		return
	}
	// Upgrade with a HTTP request returns a websocket connection
	ws, err := upgrader.Upgrade(resp, req, nil)
	if err != nil {
		log.Errorf("Unable to upgrade to websocket: %v", err)
		return
	}

	log.Debugf("Upgraded to websocket")
	defer func() {
		if err := ws.Close(); err != nil {
			log.Debugf("Error closing WebSockets connection: %s", err)
		}
	}()

	c.muConns.Lock()
	c.nextId++
	conn := &wsconn{
		id:  c.nextId,
		c:   c,
		ws:  ws,
		out: make(chan []byte, 100),
	}
	c.conns[conn.id] = conn
	c.muConns.Unlock()

	if c.onConnect != nil {
		c.onConnect(conn.out)
	}
	go conn.write()

	log.Tracef("About to read from websocket connection")
	conn.read()
}

func (c *clientChannels) writeAll() {
	defer func() {
		log.Debugf("Closing all websockets")
		c.muConns.Lock()
		for _, conn := range c.conns {
			c.doRemoveConn(conn)
		}
		c.muConns.Unlock()
	}()

	for msg := range c.out {
		for _, conn := range c.clonedConns() {
			select {
			case conn.out <- msg:
			default:
				log.Errorf("Failed to send message %v to websocket connection", msg)
			}
		}
	}
}

func (c *clientChannels) clonedConns() map[int]*wsconn {
	c.muConns.Lock()
	defer c.muConns.Unlock()
	clone := make(map[int]*wsconn)
	for k, v := range c.conns {
		clone[k] = v
	}
	return clone
}

func (c *clientChannels) lockedRemoveConn(conn *wsconn) {
	c.muConns.Lock()
	defer c.muConns.Unlock()
	c.doRemoveConn(conn)
}

func (c *clientChannels) doRemoveConn(conn *wsconn) {
	if err := conn.ws.Close(); err != nil {
		log.Debugf("Error closing WebSockets connection: %v", err)
	}
	close(conn.out)
	delete(c.conns, conn.id)
}

func (c *clientChannels) Close() {
	log.Debugf("Closing channel")
	close(c.out)
}

// wsconn ties a websocket.Conn to a clientChannels
type wsconn struct {
	id  int
	c   *clientChannels
	ws  *websocket.Conn
	out chan []byte
}

func (c *wsconn) read() {
	for {
		_, b, err := c.ws.ReadMessage()
		log.Tracef("Read message: %q", b)
		if err != nil {
			if err != io.EOF {
				log.Debugf("Error reading from UI: %v", err)
			}
			return
		}
		log.Tracef("Sending to channel...")
		c.c.in <- b
	}
}

func (c *wsconn) write() {
	for msg := range c.out {
		err := c.ws.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			log.Debugf("Error writing to WebSocket, closing: %v", err)
			c.c.lockedRemoveConn(c)
		}
	}
}
