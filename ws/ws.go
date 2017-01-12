package ws

import (
	"io"
	"net/http"
	"sync"

	"github.com/getlantern/golog"
	"github.com/gorilla/websocket"
)

const (
	// Determines the chunking size of messages used by gorilla
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

type ConnectFunc func(write func([]byte) error) error

// UIChannel represents a data channel to/from the UI. UIChannel will have one
// underlying websocket connection for each connected browser window. All
// messages from any browser window are available via In and all messages sent
// to Out will be published to all browser windows.
type UIChannel struct {
	In  <-chan []byte
	Out chan<- []byte

	in     chan []byte
	out    chan []byte
	nextId int
	conns  map[int]*wsconn
	m      sync.Mutex

	onConnect ConnectFunc
}

// NewChannel establishes a new channel acts as an http.Handler. When the UI
// connects to the handler, we will establish a websocket to the UI to carry
// messages for this UIChannel. The given onConnect function is called anytime
// that the UI connects.
func NewChannel(onConnect ConnectFunc) *UIChannel {
	in := make(chan []byte, 100)
	out := make(chan []byte)

	c := &UIChannel{
		In:        in,
		in:        in,
		Out:       out,
		out:       out,
		nextId:    0,
		conns:     make(map[int]*wsconn),
		onConnect: onConnect,
	}

	go c.write()
	return c
}

func (c *UIChannel) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
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
	c.m.Lock()
	if c.onConnect != nil {
		err = c.onConnect(func(b []byte) error {
			log.Tracef("Writing initial message: %q", b)
			return ws.WriteMessage(websocket.TextMessage, b)
		})
		if err != nil {
			log.Errorf("Error processing onConnect, disconnecting websocket: %v", err)
			if err := ws.Close(); err != nil {
				log.Debugf("Error closing WebSockets connection: %s", err)
			}
			c.m.Unlock()
			return
		}
	}
	c.nextId += 1
	conn := &wsconn{
		id: c.nextId,
		c:  c,
		ws: ws,
	}
	c.conns[conn.id] = conn
	c.m.Unlock()
	log.Tracef("About to read from websocket connection")
	conn.read()
}

func (c *UIChannel) write() {
	defer func() {
		log.Tracef("Closing all websockets")
		c.m.Lock()
		for _, conn := range c.conns {
			if err := conn.ws.Close(); err != nil {
				log.Debugf("Error closing WebSockets connection", err)
			}
			delete(c.conns, conn.id)
		}
		c.m.Unlock()
	}()

	for msg := range c.out {
		c.m.Lock()
		for _, conn := range c.conns {
			err := conn.ws.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Debugf("Error writing to WebSocket: %v", err)
				delete(c.conns, conn.id)
			}
		}
		c.m.Unlock()
	}
}

func (c *UIChannel) Close() {
	log.Tracef("Closing channel")
	close(c.out)
}

// wsconn ties a websocket.Conn to a UIChannel
type wsconn struct {
	id int
	c  *UIChannel
	ws *websocket.Conn
}

func (c *wsconn) read() {
	for {
		_, b, err := c.ws.ReadMessage()
		log.Tracef("Read message: %q", b)
		if err != nil {
			if err != io.EOF {
				log.Debugf("Error reading from UI: %v", err)
			}
			if err := c.ws.Close(); err != nil {
				log.Debugf("Error closing WebSockets connection", err)
			}
			return
		}
		log.Tracef("Sending to channel...")
		c.c.in <- b
	}
}
