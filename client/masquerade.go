package client

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/flashlight/log"
)

const (
	NumWorkers = 25
)

// verifiedMasqueradeSet represents a set of Masquerade configurations.
// verifiedMasqueradeSet verifies each configured Masquerade by attempting to
// proxy using it.
type verifiedMasqueradeSet struct {
	testServer   *ServerInfo
	masquerades  []*Masquerade
	candidatesCh chan *Masquerade
	stopCh       chan interface{}
	verifiedCh   chan *Masquerade
	wg           sync.WaitGroup
}

// nextVerified returns the next available verified *Masquerade, blocking until
// such is available.  The masquerade is immediately written back onto
// verifiedCh, turning verifiedCh into a sort of cyclic queue.
func (vms *verifiedMasqueradeSet) nextVerified() *Masquerade {
	masquerade := <-vms.verifiedCh
	go func() {
		vms.verifiedCh <- masquerade
	}()
	return masquerade
}

// newVerifiedMasqueradeSet sets up a new verifiedMasqueradeSet that verifies
// each of the given masquerades against the given testServer.
func newVerifiedMasqueradeSet(testServer *ServerInfo, masquerades []*Masquerade) *verifiedMasqueradeSet {
	vms := &verifiedMasqueradeSet{
		testServer:   testServer,
		masquerades:  masquerades,
		candidatesCh: make(chan *Masquerade),
		stopCh:       make(chan interface{}, 1),
		verifiedCh:   make(chan *Masquerade),
	}

	vms.wg.Add(NumWorkers)
	// Spawn some worker goroutines to verify masquerades
	for i := 0; i < NumWorkers; i++ {
		go vms.verify()
	}

	// Feed candidates for verification
	go vms.feedCandidates()

	return vms
}

// feedCandidates feeds the candidate masquerades to our worker routines in
// random order
func (vms *verifiedMasqueradeSet) feedCandidates() {
	for _, i := range rand.Perm(len(vms.masquerades)) {
		if !vms.feedCandidate(vms.masquerades[i]) {
			break
		}
	}
	close(vms.candidatesCh)
}

func (vms *verifiedMasqueradeSet) feedCandidate(candidate *Masquerade) bool {
	select {
	case <-vms.stopCh:
		log.Debug("Received stop, not feeding any further")
		return false
	case vms.candidatesCh <- candidate:
		log.Debug("Fed candidate")
		return true
	}
}

// stop stops the verification process
func (vms *verifiedMasqueradeSet) stop() {
	log.Debug("Stop called")
	vms.stopCh <- nil
	go func() {
		log.Debug("Draining verified channel")
		for {
			_, ok := <-vms.verifiedCh
			if !ok {
				log.Debug("Done draining")
				break
			}
		}
	}()
	log.Debug("Waiting for workers to finish")
	vms.wg.Wait()
	log.Debug("Closing vms.verifiedCh")
	close(vms.verifiedCh)

}

// verify checks masquerades obtained from candidatesCh to see if they work on
// the test server.
func (vms *verifiedMasqueradeSet) verify() {
	for {
		candidate, ok := <-vms.candidatesCh
		if !ok {
			log.Debug("Verification worker stopped")
			vms.wg.Done()
			return
		}
		vms.doVerify(candidate)
	}
}

func (vms *verifiedMasqueradeSet) doVerify(masquerade *Masquerade) {
	errCh := make(chan error, 2)
	go func() {
		// Limit amount of time we'll wait for a response
		time.Sleep(30 * time.Second)
		errCh <- fmt.Errorf("Timed out making HEAD request via %s", masquerade.Domain)
	}()
	go func() {
		start := time.Now()
		httpClient := HttpClient(vms.testServer, masquerade)
		req, _ := http.NewRequest("HEAD", "http://www.google.com/humans.txt", nil)
		resp, err := httpClient.Do(req)
		if err != nil {
			errCh <- fmt.Errorf("HTTP ERROR FOR MASQUERADE %v: %v", masquerade.Domain, err)
			return
		} else {
			body, err := ioutil.ReadAll(resp.Body)
			defer resp.Body.Close()
			if err != nil {
				errCh <- fmt.Errorf("HTTP Body Error: %s", body)
			} else {
				delta := time.Now().Sub(start)
				log.Debugf("SUCCESSFUL CHECK FOR: %s IN %s, %s", masquerade.Domain, delta, body)
				errCh <- nil
			}
		}
	}()
	err := <-errCh
	if err != nil {
		log.Error(err)
		return
	}
	vms.verifiedCh <- masquerade
}
