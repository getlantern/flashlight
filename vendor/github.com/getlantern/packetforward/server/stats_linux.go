package server

import (
	"sync/atomic"
	"time"
)

func (s *server) printStats() {
	defer close(s.closed)
	defer s.opts.Opts.StatsTracker.Close()

	ticker := time.NewTicker(s.opts.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.close:
			return
		case <-ticker.C:
			s.clientsMx.Lock()
			numClients := len(s.clients)
			s.clientsMx.Unlock()
			log.Debugf("Number of Clients: %d", numClients)
			log.Debugf("Reads Succeeded: %d   Failed: %d", atomic.LoadInt64(&s.successfulReads), atomic.LoadInt64(&s.failedReads))
			log.Debugf("Writes Succeeded: %d   Failed: %d", atomic.LoadInt64(&s.successfulWrites), atomic.LoadInt64(&s.failedWrites))
		}
	}
}
