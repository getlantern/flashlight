package trafficlog

import (
	"net"
	"time"

	"github.com/getlantern/errors"
)

const routeCheckInterval = time.Second

type routeUpdate struct {
	ip    net.IP
	iface networkInterface
}

// routeWatcher watches for changes to routes used to connect to a host.
type routeWatcher struct {
	updatesChan chan routeUpdate
	errorChan   chan error
	stopChan    chan struct{}
}

func newRouteWatcher(host string) (*routeWatcher, error) {
	// There are more sophisticated ways to watch for route changes, but this will do for our needs.
	getRouteUpdates := func() ([]routeUpdate, error) {
		remoteIPs, err := net.LookupIP(host)
		if err != nil {
			return nil, errors.New("failed to find IP for host: %v", err)
		}
		if len(remoteIPs) == 0 {
			return nil, errors.New("failed to resolve host")
		}
		updates := make([]routeUpdate, len(remoteIPs))
		for i, rip := range remoteIPs {
			iface, err := networkInterfaceFor(rip)
			if err != nil {
				return nil, errors.New("failed to find interface: %v", err)
			}
			updates[i] = routeUpdate{rip, *iface}
		}
		return updates, nil
	}

	updates, err := getRouteUpdates()
	if err != nil {
		return nil, err
	}

	w := routeWatcher{
		make(chan routeUpdate),
		make(chan error),
		make(chan struct{}),
	}
	go func() {
		timer := time.NewTimer(routeCheckInterval)
		for {
			if err != nil {
				w.sendError(errors.New("failed to update route for %s: %v", host, err))
			}
			for _, u := range updates {
				w.sendUpdate(u)
			}

			select {
			case <-timer.C:
				updates, err = getRouteUpdates()
				timer.Reset(routeCheckInterval)
			case <-w.stopChan:
				timer.Stop()
				close(w.updatesChan)
				close(w.errorChan)
				return
			}
		}
	}()
	return &w, nil
}

func (rw *routeWatcher) sendUpdate(u routeUpdate) {
	select {
	case rw.updatesChan <- u:
	case <-rw.stopChan:
	}
}

func (rw *routeWatcher) sendError(err error) {
	select {
	case rw.errorChan <- err:
	case <-rw.stopChan:
	}
}

func (rw *routeWatcher) updates() <-chan routeUpdate {
	return rw.updatesChan
}

func (rw *routeWatcher) errors() <-chan error {
	return rw.errorChan
}

func (rw *routeWatcher) close() {
	close(rw.stopChan)
}
