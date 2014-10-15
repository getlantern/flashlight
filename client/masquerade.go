package client

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

// verifiedMasqueradeSet represents a set of Masquerade configurations.
// verifiedMasqueradeSet verifies each configured Masquerade by attempting to
// proxy using it.
type verifiedMasqueradeSet struct {
	testServer  *ServerInfo
	masquerades []*Masquerade
	verifiedCh  chan *Masquerade
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
		testServer:  testServer,
		masquerades: masquerades,
		verifiedCh:  make(chan *Masquerade),
	}

	// Verify all configured Masquerades on their own goroutine
	for _, masquerade := range vms.masquerades {
		go vms.verify(masquerade)
	}

	return vms
}

// verify checks a single masquerade domain to see if it works on the test
// server.
func (vms *verifiedMasqueradeSet) verify(masquerade *Masquerade) {
	httpClient := HttpClient(vms.testServer, masquerade)
	req, _ := http.NewRequest("HEAD", "http://www.google.com/humans.txt", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Errorf("HTTP ERROR FOR MASQUERADE %v: %v", masquerade.Domain, err)
		return
	} else {
		body, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			fmt.Errorf("HTTP Body Error: %s", body)
		} else {
			log.Debugf("SUCCESSFUL CHECK FOR: %s, %s", masquerade.Domain, body)
			vms.verifiedCh <- masquerade
		}
	}
}
