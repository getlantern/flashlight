// package psmux is a multiplexing library for Golang.
//
// It relies on an underlying connection to provide reliability and ordering, such as TCP or KCP,
// and provides stream-oriented multiplexing over a single channel.
package psmux

import (
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

// Config is used to tune the Smux session
type Config struct {
	// SMUX Protocol version, support 1,2
	Version int

	// Disabled keepalive
	KeepAliveDisabled bool

	// KeepAliveInterval is how often to send a NOP command to the remote
	KeepAliveInterval time.Duration

	// KeepAliveTimeout is how long the session
	// will be closed if no data has arrived
	KeepAliveTimeout time.Duration

	// MaxFrameSize is used to control the maximum
	// frame size to sent to the remote
	MaxFrameSize int

	// MaxReceiveBuffer is used to control the maximum
	// number of data in the buffer pool
	MaxReceiveBuffer int

	// MaxStreamBuffer is used to control the maximum
	// number of data per stream
	MaxStreamBuffer int

	// MaxPaddingRatio limits padding bytes to a
	// percentage of the write size and influences
	// the overall overhead introduced by padding.
	//
	// A MaxPaddingRatio of 0.5 allows a write of 100
	// bytes to be padded by 50 bytes. It also limits
	// the smallest write that will receive any padding
	// to 16 (as there is a minimum of 8 padding bytes).
	// In general, the minimum padded write size
	// will be ceil(8/MaxPaddingRatio).
	MaxPaddingRatio float64

	// MaxPaddedSize determines the maximum sized write
	// that will be padded.  Writes larger than this are
	// not padded and small writes are not padded beyond this
	// size.
	MaxPaddedSize int

	// AggressivePadding enables aggressive (large) padding for this
	// number of initial writes in a session as determined by
	// AggressivePaddingRatio.
	//
	// The overhead bytes contributed by aggressive padding are bounded above by:
	// min(1.0, AgressivePaddingRatio)*(MaxPaddedSize*AggressivePadding).
	// Unless AgressivePaddingRatio >> 1.0 and writes are
	// very small, it is generally far less in practice due to
	// write distribution and random selection of padding amounts.
	AggressivePadding int

	// AggressivePaddingRatio sets the max padding ratio for aggressive
	// initial padding.  The total padding is still bounded
	// by MaxPaddedSize for any given write.
	AggressivePaddingRatio float64
}

// DefaultConfig is used to return a default configuration
func DefaultConfig() *Config {
	return &Config{
		Version:                1,
		KeepAliveInterval:      10 * time.Second,
		KeepAliveTimeout:       30 * time.Second,
		MaxFrameSize:           32768,
		MaxReceiveBuffer:       4194304,
		MaxStreamBuffer:        65536,
		MaxPaddingRatio:        0.10,
		MaxPaddedSize:          1200,
		AggressivePadding:      16,
		AggressivePaddingRatio: 0.3,
	}
}

// VerifyConfig is used to verify the sanity of configuration
func VerifyConfig(config *Config) error {
	if !(config.Version == 1 || config.Version == 2) {
		return errors.New("unsupported protocol version")
	}
	if !config.KeepAliveDisabled {
		if config.KeepAliveInterval == 0 {
			return errors.New("keep-alive interval must be positive")
		}
		if config.KeepAliveTimeout < config.KeepAliveInterval {
			return fmt.Errorf("keep-alive timeout must be larger than keep-alive interval")
		}
	}
	if config.MaxFrameSize <= 0 {
		return errors.New("max frame size must be positive")
	}
	if config.MaxFrameSize > 65535 {
		return errors.New("max frame size must not be larger than 65535")
	}
	if config.MaxReceiveBuffer <= 0 {
		return errors.New("max receive buffer must be positive")
	}
	if config.MaxStreamBuffer <= 0 {
		return errors.New("max stream buffer must be positive")
	}
	if config.MaxStreamBuffer > config.MaxReceiveBuffer {
		return errors.New("max stream buffer must not be larger than max receive buffer")
	}
	if config.MaxStreamBuffer > math.MaxInt32 {
		return errors.New("max stream buffer cannot be larger than 2147483647")
	}
	if config.MaxPaddingRatio < 0 {
		return errors.New("max padding ratio cannot be negative.")
	}
	if config.MaxPaddedSize < 0 {
		return errors.New("max padded size cannot be negative.")
	}
	if config.MaxPaddedSize > config.MaxFrameSize {
		return errors.New("max padded size cannot exceed maximum frame size.")
	}
	if config.AggressivePadding < 0 {
		return errors.New("aggressive padding count cannot be negative.")
	}
	if config.AggressivePaddingRatio < 0 {
		return errors.New("aggressive padding ratio cannot be negative.")
	}
	return nil
}

// Server is used to initialize a new server-side connection.
func Server(conn io.ReadWriteCloser, config *Config) (*Session, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := VerifyConfig(config); err != nil {
		return nil, err
	}
	return newSession(config, conn, false), nil
}

// Client is used to initialize a new client-side connection.
func Client(conn io.ReadWriteCloser, config *Config) (*Session, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := VerifyConfig(config); err != nil {
		return nil, err
	}
	return newSession(config, conn, true), nil
}
