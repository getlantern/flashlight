package chained

import (
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/tlsresumption"
	tls "github.com/refraction-networking/utls"
)

type sessionStateForServer struct {
	server string
	state  *tls.ClientSessionState
}

var (
	saveSessionStateCh     = make(chan *sessionStateForServer, 100)
	currentSessionStates   = make(map[string]*tls.ClientSessionState)
	currentSessionStatesMx sync.RWMutex

	initOnce sync.Once
)

func persistedSessionStateFor(server string, defaultState *tls.ClientSessionState) *tls.ClientSessionState {
	currentSessionStatesMx.RLock()
	state := currentSessionStates[server]
	currentSessionStatesMx.RUnlock()
	if state != nil {
		return state
	}
	return defaultState
}

func saveSessionState(server string, state *tls.ClientSessionState) {
	if state == nil {
		return
	}
	select {
	case saveSessionStateCh <- &sessionStateForServer{server: server, state: state}:
		// okay
	default:
		// channel full, drop update
	}
}

// PersistSessionStates makes sure that session states are stored on disk in the given configDir
func PersistSessionStates(configDir string) {
	initOnce.Do(func() {
		persistSessionStates(configDir, 15*time.Second)
	})
}

func persistSessionStates(configDir string, saveInterval time.Duration) {
	filename, err := common.InConfigDir(configDir, "tls_session_states")
	if err != nil {
		log.Errorf("unable to get filename for persisting session states: %v", err)
		return
	}

	existing, err := ioutil.ReadFile(filename)
	if err == nil {
		log.Debugf("Initializing current session states from %v", filename)
		rows := strings.Split(string(existing), "\n")
		for _, row := range rows {
			cells := strings.SplitN(row, ",", 2)
			if len(cells) == 2 {
				server := cells[0]
				sessionState, err := tlsresumption.ParseClientSessionState(cells[1])
				if err != nil {
					log.Errorf("unable to parse persisted client session state for %v from %v: %v", server, filename, err)
					continue
				}
				currentSessionStates[server] = sessionState
				log.Debugf("Loaded persisted session state for %v", server)
			}
		}
	}

	log.Debugf("Will persist client session states at %v", filename)
	go maintainSessionStates(filename, saveInterval)
}

func maintainSessionStates(filename string, saveInterval time.Duration) {
	dirty := false

	saveIfNecessary := func() {
		if dirty {
			currentSessionStatesMx.RLock()
			states := make(map[string]*tls.ClientSessionState, len(currentSessionStates))
			for server, state := range currentSessionStates {
				states[server] = state
			}
			currentSessionStatesMx.RUnlock()
			serialized := ""
			rowDelim := "" // for first row, don't include a delimiter
			for server, state := range states {
				serializedState, err := tlsresumption.SerializeClientSessionState(state)
				if err != nil {
					log.Errorf("unable to serialize session state for %v: %v", server, err)
					continue
				}
				serialized = fmt.Sprintf("%v%v%v,%v", serialized, rowDelim, server, serializedState)
				rowDelim = "\n" // after first row, include a delimiter
			}
			err := ioutil.WriteFile(filename, []byte(serialized), 0644)
			if err != nil {
				log.Errorf("unable to update session states in %v: %v", filename, err)
				return
			}
			dirty = false
			log.Debugf("updated session states in %v", filename)
		}
	}
	defer saveIfNecessary()

	ticker := time.NewTicker(15 * time.Second)

	for {
		select {
		case state, open := <-saveSessionStateCh:
			if !open {
				return
			}
			currentSessionStatesMx.Lock()
			currentSessionStates[state.server] = state.state
			currentSessionStatesMx.Unlock()
			dirty = true
		case <-ticker.C:
			saveIfNecessary()
		}
	}
}
