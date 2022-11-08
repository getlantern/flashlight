package enhttp

import (
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// NewServerHandler creates an http.Handler that performs the server-side
// processing of enhttp. reapIdleTime specifies the amount of time a connection
// is allowed to remain idle before being forcibly closed. serverURL optionally
// specifies the unique URL at which this server can be reached (used for sticky
// routing).
func NewServerHandler(reapIdleTime time.Duration, serverURL string) http.Handler {
	s := &server{
		reapIdleTime: reapIdleTime,
		serverURL:    serverURL,
		conns:        make(map[string]*tsconn, 1000),
	}
	go s.reapExpiredConns()
	return s
}

type tsconn struct {
	net.Conn
	ts int64
}

type server struct {
	reapIdleTime time.Duration
	serverURL    string
	conns        map[string]*tsconn
	mx           sync.RWMutex
}

func (s *server) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	connID := req.Header.Get(ConnectionIDHeader)
	s.mx.RLock()
	conn := s.conns[connID]
	s.mx.RUnlock()
	first := conn == nil
	isClose := req.Header.Get(Close) != ""

	if first {
		if isClose {
			log.Debug("Attempt to close already closed connection")
			resp.WriteHeader(http.StatusOK)
			return
		}

		// Connect to the origin
		origin := req.Header.Get(OriginHeader)
		_conn, err := net.Dial("tcp", origin)
		if err != nil {
			log.Errorf("Unable to dial %v: %v", origin, err)
			resp.WriteHeader(http.StatusBadGateway)
			return
		}
		conn := &tsconn{_conn, intFromTime(time.Now())}

		// Remember the origin connection
		s.mx.Lock()
		s.conns[connID] = conn
		s.mx.Unlock()

		// Write the request body to origin
		_, err = io.Copy(conn, req.Body)
		if err != nil {
			log.Errorf("Error reading request body: %v", err)
			resp.WriteHeader(http.StatusBadRequest)
			return
		}
		req.Body.Close()

		// Set up the response for streaming
		resp.Header().Set("Connection", "Keep-Alive")
		resp.Header().Set("Transfer-Encoding", "chunked")
		if s.serverURL != "" {
			resp.Header().Set(ServerURL, s.serverURL)
		}
		resp.WriteHeader(http.StatusOK)

		// Force writing the HTTP response header to client
		resp.(http.Flusher).Flush()

		// Read from the origin and write data to client
		buf := make([]byte, 8192)
		for {
			n, err := conn.Read(buf)
			atomic.StoreInt64(&conn.ts, intFromTime(time.Now()))
			if n > 0 {
				resp.Write(buf[:n])
				resp.(http.Flusher).Flush()
			}
			if err != nil {
				return
			}
		}
	}

	if isClose {
		// Close the connection
		conn.Close()
		s.mx.Lock()
		delete(s.conns, connID)
		s.mx.Unlock()
		resp.WriteHeader(http.StatusOK)
		return
	}

	// Not first request, simply write request data to origin
	_, err := io.Copy(conn, req.Body)
	atomic.StoreInt64(&conn.ts, intFromTime(time.Now()))
	if err != nil {
		log.Errorf("Error reading request body: %v", err)
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	resp.WriteHeader(http.StatusOK)
}

func (s *server) reapExpiredConns() {
	for {
		time.Sleep(s.reapIdleTime)

		s.mx.Lock()
		allConns := make(map[string]*tsconn, len(s.conns))
		for connId, conn := range s.conns {
			allConns[connId] = conn
		}
		s.mx.Unlock()

		log.Debugf("Reaper examining %d connections", len(allConns))
		reapedConns := make([]string, 0)
		now := intFromTime(time.Now())
		for connId, conn := range allConns {
			if time.Duration(now-atomic.LoadInt64(&conn.ts)) >= s.reapIdleTime {
				log.Debugf("Reaping idle connection to: %v", conn.RemoteAddr())
				conn.Close()
				reapedConns = append(reapedConns, connId)
			}
		}

		s.mx.Lock()
		for _, connId := range reapedConns {
			delete(s.conns, connId)
		}
		s.mx.Unlock()
	}
}
