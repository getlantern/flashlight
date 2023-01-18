package tlslistener

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"time"
)

func maintainSessionTicketKey(cfg *tls.Config, sessionTicketKeyFile string, keyListener func(keys [][32]byte)) {
	// read cached session ticket keys
	keyBytes, err := ioutil.ReadFile(sessionTicketKeyFile)
	if err != nil {
		if !os.IsNotExist(err) {
			panic(fmt.Errorf("Unable to read session ticket key file %v: %v", sessionTicketKeyFile, err))
		}
		keyBytes = make([]byte, 0)
	}

	// Create a new key right away
	keyBytes = prependToSessionTicketKeys(cfg, sessionTicketKeyFile, keyBytes, keyListener)

	// Then rotate key every 24 hours
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			keyBytes = prependToSessionTicketKeys(cfg, sessionTicketKeyFile, keyBytes, keyListener)
		}
	}()
}

func prependToSessionTicketKeys(cfg *tls.Config, sessionTicketKeyFile string, keyBytes []byte, keyListener func(keys [][32]byte)) []byte {
	newKey := makeSessionTicketKey()
	keyBytes = append(newKey, keyBytes...)
	saveSessionTicketKeys(sessionTicketKeyFile, keyBytes)

	numKeys := len(keyBytes) / 32
	keys := make([][32]byte, 0, numKeys)
	for i := 0; i < numKeys; i++ {
		currentKeyBytes := keyBytes[i*32:]
		var key [32]byte
		copy(key[:], currentKeyBytes)
		keys = append(keys, key)
	}
	cfg.SetSessionTicketKeys(keys)
	keyListener(keys)
	return keyBytes
}

func saveSessionTicketKeys(sessionTicketKeyFile string, keyBytes []byte) {
	err := ioutil.WriteFile(sessionTicketKeyFile, keyBytes, 0644)
	if err != nil {
		panic(fmt.Errorf("Unable to save session ticket key bytes to %v: %v", sessionTicketKeyFile, err))
	}
}

func makeSessionTicketKey() []byte {
	b := make([]byte, 32)
	rand.Read(b)
	return b
}
