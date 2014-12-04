package client

import (
	"github.com/getlantern/balancer"
)

func (client *Client) getBalancer() *balancer.Balancer {
	bal := <-client.balCh
	client.balCh <- bal
	return bal
}

func (client *Client) initBalancer(cfg *ClientConfig) *balancer.Balancer {
	dialers := make([]*balancer.Dialer, 0, len(cfg.FrontedServers))

	for _, s := range cfg.FrontedServers {
		dialer := s.Dialer(cfg.MasqueradeSets)
		dialers = append(dialers, dialer)
	}

	bal := balancer.New(dialers...)

	if client.balInitialized {
		log.Trace("Draining balancer channel")
		<-client.balCh
	} else {
		log.Trace("Creating balancer channel")
		client.balCh = make(chan *balancer.Balancer, 1)
	}
	log.Trace("Publishing balancer")
	client.balCh <- bal

	return bal
}
