package chained

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/v7/common"
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
	libraryVersionString   = "LibraryVersion: "

	initOnce sync.Once
)

func persistedSessionStateFor(server string) (*tls.ClientSessionState, time.Time) {
	currentSessionStatesMx.RLock()
	state, ok := currentSessionStates[server]
	currentSessionStatesMx.RUnlock()
	if !ok {
		return nil, time.Time{}
	}
	return state.state, state.timestamp
}

func saveSessionState(server string, state *tls.ClientSessionState, updatedTime time.Time) {
	if state == nil {
		return
	}
	select {
	case saveSessionStateCh <- sessionStateForServer{server, state, updatedTime}:
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

func isSameLibraryVersion(first_row string) bool {
	if !strings.HasPrefix(first_row, libraryVersionString) {
		log.Debugf("%v string not found in tls_session_states", libraryVersionString)
		return false
	}
	fileLibraryVersion := strings.TrimPrefix(first_row, libraryVersionString)
	log.Debugf("tls_session_states file LibraryVersion %v, current LibraryVersion %v", fileLibraryVersion, common.LibraryVersion)
	return fileLibraryVersion >= common.LibraryVersion
}

func persistSessionStates(configDir string, saveInterval time.Duration) {
	filename := filepath.Join(configDir, "tls_session_states")

	existing, err := os.ReadFile(filename)
	if err == nil {
		log.Debugf("Initializing current session states from %v", filename)
		rows := strings.Split(string(existing), "\n")
		if len(rows) > 0 && isSameLibraryVersion(rows[0]) { // the first row contains the LibraryVersion
			rows = rows[1:] // Remove the first item containing the LibraryVersion
			for _, row := range rows {
				state, err := parsePersistedState(row)
				if err != nil {
					log.Errorf("unable to parse persisted session state from %v: %v", filename, err)
					continue
				}
				currentSessionStates[state.server] = *state
			}
		} else {
			log.Errorf("Different LibraryVersion found, deleting %v", filename)
			if err := os.Remove(filename); err != nil {
				log.Errorf("Failed to delete %v: %v", filename, err)
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
			states := make(map[string]sessionStateForServer, len(currentSessionStates))
			for server, state := range currentSessionStates {
				states[server] = state
			}
			currentSessionStatesMx.RUnlock()

			// Add LibraryVersion to the first line of the file
			serialized := fmt.Sprintf("%s%s\n", libraryVersionString, common.LibraryVersion)
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
			err := os.WriteFile(filename, []byte(serialized), 0644)
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
