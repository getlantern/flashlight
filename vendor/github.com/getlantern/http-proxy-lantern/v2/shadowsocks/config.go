// Package shadowsocks is a wrapper around shadowsocks, specifically our fork
// of "Jigsaw-Code/outline-ss-server" which lives in
// github.com/getlantern/lantern-shadowsocks
package shadowsocks

import (
	"container/list"
	"fmt"

	"github.com/Jigsaw-Code/outline-ss-server/service"
	outlineShadowsocks "github.com/Jigsaw-Code/outline-ss-server/shadowsocks"
)

const (
	DefaultCipher        = "chacha20-ietf-poly1305"
	DefaultReplayHistory = 10000
	DefaultMaxPending    = 1000
)

type CipherConfig struct {
	ID     string
	Cipher string
	Secret string
}

// NewCipherListWithConfigs creates a CipherList with the given
// configuration
func NewCipherListWithConfigs(configs []CipherConfig) (service.CipherList, error) {
	cipherList := service.NewCipherList()
	err := UpdateCipherList(cipherList, configs)
	if err != nil {
		return nil, err
	}
	return cipherList, nil
}

// UpdateCipherList replaces the contents of the given cipherList with the
// configuration given.
func UpdateCipherList(cipherList service.CipherList, configs []CipherConfig) error {
	list := list.New()
	for _, config := range configs {
		cipher := config.Cipher
		if cipher == "" {
			cipher = DefaultCipher
		}
		if config.Secret == "" {
			return fmt.Errorf("Secret was not specified for cipher %s", config.ID)
		}
		ci, err := outlineShadowsocks.NewCipher(cipher, config.Secret)
		if err != nil {
			return fmt.Errorf("Failed to create cipher entry (%v, %v, %v) : %w", config.ID, config.Cipher, config.Secret, err)
		}
		entry := service.MakeCipherEntry(config.ID, ci, config.Secret)
		list.PushBack(&entry)
	}
	cipherList.Update(list)
	return nil
}
