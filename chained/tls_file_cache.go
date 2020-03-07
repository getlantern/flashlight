package chained

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/tlsresumption"
	tls "github.com/refraction-networking/utls"
)

type sessionStateForServer struct {
	server    string
	state     *tls.ClientSessionState
	timestamp time.Time
}

var (
	saveSessionStateCh     = make(chan sessionStateForServer, 100)
	currentSessionStates   = make(map[string]sessionStateForServer)
	currentSessionStatesMx sync.RWMutex

	initOnce sync.Once
)

func persistedSessionStateFor(server string, defaultState *tls.ClientSessionState,
	stateTTL time.Duration) *tls.ClientSessionState {

	currentSessionStatesMx.RLock()
	state, ok := currentSessionStates[server]
	currentSessionStatesMx.RUnlock()
	if ok && time.Now().Sub(state.timestamp) < stateTTL {
		return state.state
	}
	return defaultState
}

func saveSessionState(server string, state *tls.ClientSessionState) {
	if state == nil {
		return
	}
	select {
	case saveSessionStateCh <- sessionStateForServer{server, state, time.Now()}:
		// okay
	default:
		// channel full, drop update
	}
}

// PersistSessionStates makes sure that session states are stored on disk in the given configDir
func PersistSessionStates(configDir string) {
	initOnce.Do(func() {
		// We need a write lock here because other go routines can try to access session states
		// before this completes.
		currentSessionStatesMx.Lock()
		persistSessionStates(configDir, 15*time.Second)
		currentSessionStatesMx.Unlock()
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
			state, err := parsePersistedState(row)
			if err != nil {
				log.Errorf("unable to parse persisted session state from %v: %v", filename, err)
				continue
			}
			currentSessionStates[state.server] = *state
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
			states := make(map[string]sessionStateForServer, len(currentSessionStates))
			for server, state := range currentSessionStates {
				states[server] = state
			}
			currentSessionStatesMx.RUnlock()
			serialized := ""
			rowDelim := "" // for first row, don't include a delimiter
			for server, state := range states {
				serializedState, err := tlsresumption.SerializeClientSessionState(state.state)
				if err != nil {
					log.Errorf("unable to serialize session state for %v: %v", server, err)
					continue
				}
				serialized = fmt.Sprintf(
					"%v%v%v,%v,%v",
					serialized, rowDelim, server, state.timestamp.Unix(), serializedState)
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
		case newState, open := <-saveSessionStateCh:
			if !open {
				return
			}
			currentSessionStatesMx.Lock()
			// Check to ensure we don't overwrite the same ticket with a new timestamp.
			current, ok := currentSessionStates[newState.server]
			if !ok || !bytes.Equal(current.state.SessionTicket(), newState.state.SessionTicket()) {
				currentSessionStates[newState.server] = newState
			}
			currentSessionStatesMx.Unlock()
			dirty = true
		case <-ticker.C:
			saveIfNecessary()
		}
	}
}

func parsePersistedState(row string) (*sessionStateForServer, error) {
	// This function decodes both old-form (pre 5.8.5) and new-form (post 5.8.5) rows.
	// Old-form rows have the following structure:
	//		serverName,serializedState
	// where serializedState may include commas. New-form rows introduce a timestamp field:
	//		serverName,timestamp,serializedState
	// where timestamp is a Unix epoch second.

	state := sessionStateForServer{}
	splits := strings.SplitN(row, ",", 2)
	if len(splits) != 2 {
		return nil, errors.New("expected at least two comma-separated fields")
	}
	state.server = splits[0]
	remainder := splits[1]

	// Assume old-form, but try new-form.
	serializedState := remainder
	splits = strings.SplitN(remainder, ",", 2)
	if len(splits) == 2 {
		tsUnix, err := strconv.ParseInt(splits[0], 10, 64)
		if err == nil {
			state.timestamp = time.Unix(tsUnix, 0)
			serializedState = splits[1]
		}
	}

	var err error
	state.state, err = tlsresumption.ParseClientSessionState(serializedState)
	if err != nil {
		return nil, errors.New("failed to parse serialized state: %v", err)
	}
	return &state, nil
}
