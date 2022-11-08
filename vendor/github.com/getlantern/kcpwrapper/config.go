// Package kcpwrapper provides a wrapper around kcp that allows the use of
// regular net interfaces like dialing and listening. It is built to be
// compatible with servers and clients running github.com/xtaci/kcptun and
// duplicates some of the code from that project.
package kcpwrapper

import (
	"crypto/sha1"
	"math/rand"
	"time"

	"github.com/getlantern/golog"
	kcp "github.com/getlantern/kcp-go/v5"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// SALT is use for pbkdf2 key expansion
	SALT = "kcp-go"
)

var (
	log = golog.LoggerFor("kcpwrapper")
)

// CommonConfig contains common configuration parameters across dialers and
// listeners.
type CommonConfig struct {
	Key          string `json:"key"`
	Crypt        string `json:"crypt"`
	Mode         string `json:"mode"`
	MTU          int    `json:"mtu"`
	SndWnd       int    `json:"sndwnd"`
	RcvWnd       int    `json:"rcvwnd"`
	DataShard    int    `json:"datashard"`
	ParityShard  int    `json:"parityshard"`
	DSCP         int    `json:"dscp"`
	NoComp       bool   `json:"nocomp"`
	AckNodelay   bool   `json:"acknodelay"`
	NoDelay      int    `json:"nodelay"`
	Interval     int    `json:"interval"`
	Resend       int    `json:"resend"`
	NoCongestion int    `json:"nc"`
	SockBuf      int    `json:"sockbuf"`
	KeepAlive    int    `json:"keepalive"`
	block        kcp.BlockCrypt
}

func (cfg *CommonConfig) applyDefaults() {
	rand.Seed(int64(time.Now().Nanosecond()))
	switch cfg.Mode {
	case "normal":
		cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion = 0, 40, 2, 1
	case "fast":
		cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion = 0, 30, 2, 1
	case "fast2":
		cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion = 1, 20, 2, 1
	case "fast3":
		cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion = 1, 10, 2, 1
	}

	pass := pbkdf2.Key([]byte(cfg.Key), []byte(SALT), 4096, 32, sha1.New)
	switch cfg.Crypt {
	case "sm4":
		cfg.block, _ = kcp.NewSM4BlockCrypt(pass[:16])
	case "tea":
		cfg.block, _ = kcp.NewTEABlockCrypt(pass[:16])
	case "xor":
		cfg.block, _ = kcp.NewSimpleXORBlockCrypt(pass)
	case "none":
		cfg.block, _ = kcp.NewNoneBlockCrypt(pass)
	case "aes-128":
		cfg.block, _ = kcp.NewAESBlockCrypt(pass[:16])
	case "aes-192":
		cfg.block, _ = kcp.NewAESBlockCrypt(pass[:24])
	case "blowfish":
		cfg.block, _ = kcp.NewBlowfishBlockCrypt(pass)
	case "twofish":
		cfg.block, _ = kcp.NewTwofishBlockCrypt(pass)
	case "cast5":
		cfg.block, _ = kcp.NewCast5BlockCrypt(pass[:16])
	case "3des":
		cfg.block, _ = kcp.NewTripleDESBlockCrypt(pass[:24])
	case "xtea":
		cfg.block, _ = kcp.NewXTEABlockCrypt(pass[:16])
	case "salsa20":
		cfg.block, _ = kcp.NewSalsa20BlockCrypt(pass)
	default:
		cfg.Crypt = "aes"
		cfg.block, _ = kcp.NewAESBlockCrypt(pass)
	}

	log.Debugf("encryption: %v", cfg.Crypt)
	log.Debugf("nodelay parameters: %v,%v,%v,%v", cfg.NoDelay, cfg.Interval, cfg.Resend, cfg.NoCongestion)
	log.Debugf("sndwnd: %v    recvwnd: %v", cfg.SndWnd, cfg.RcvWnd)
	log.Debugf("compression: %v", !cfg.NoComp)
	log.Debugf("mtu: %v", cfg.MTU)
	log.Debugf("datashard: %v   parityshard: %v", cfg.DataShard, cfg.ParityShard)
	log.Debugf("acknodelay: %v", cfg.AckNodelay)
	log.Debugf("dscp: %v", cfg.DSCP)
	log.Debugf("sockbuf: %v", cfg.SockBuf)
	log.Debugf("keepalive: %v", cfg.KeepAlive)
}
