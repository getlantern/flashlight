package darkstar

import (
	"errors"
	"github.com/OperatorFoundation/go-shadowsocks2/shadowaead"
	"io"
	"net"
	"sync"
)

// ErrShortPacket means that the packet is too short for a valid encrypted packet.
var ErrShortPacket = errors.New("short packet")

var _zerononce [128]byte // read-only. 128 bytes is more than enough.

// Pack encrypts plaintext using Cipher with a randomly generated salt and
// returns a slice of dst containing the encrypted packet and any error occurred.
// Ensure len(dst) >= ciph.SaltSize() + len(plaintext) + aead.Overhead().
func Pack(dst, plaintext []byte, ciph shadowaead.Cipher) ([]byte, error) {
	aead, err := ciph.Encrypter(nil)
	if err != nil {
		return nil, err
	}

	if len(dst) < len(plaintext)+aead.Overhead() {
		return nil, io.ErrShortBuffer
	}
	b := aead.Seal(dst, _zerononce[:aead.NonceSize()], plaintext, nil)
	return dst[:len(b)], nil
}

// Unpack decrypts pkt using Cipher and returns a slice of dst containing the decrypted payload and any error occurred.
// Ensure len(dst) >= len(pkt) - aead.SaltSize() - aead.Overhead().
func Unpack(dst, pkt []byte, ciph shadowaead.Cipher) ([]byte, error) {
	aead, err := ciph.Decrypter(nil)
	if err != nil {
		return nil, err
	}
	if len(pkt) < aead.Overhead() {
		return nil, ErrShortPacket
	}
	if len(dst)+aead.Overhead() < len(pkt) {
		return nil, io.ErrShortBuffer
	}
	b, err := aead.Open(dst[:0], _zerononce[:aead.NonceSize()], pkt, nil)
	return b, err
}

type packetConn struct {
	net.PacketConn
	shadowaead.Cipher
	sync.Mutex
	buf []byte // write lock
}

// NewPacketConn wraps a net.PacketConn with cipher
func NewPacketConn(c net.PacketConn, ciph shadowaead.Cipher) net.PacketConn {
	const maxPacketSize = 64 * 1024
	return &packetConn{PacketConn: c, Cipher: ciph, buf: make([]byte, maxPacketSize)}
}

// WriteTo encrypts b and write to addr using the embedded PacketConn.
func (c *packetConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.Lock()
	defer c.Unlock()
	buf, err := Pack(c.buf, b, c)
	if err != nil {
		return 0, err
	}
	_, err = c.PacketConn.WriteTo(buf, addr)
	return len(b), err
}

// ReadFrom reads from the embedded PacketConn and decrypts into b.
func (c *packetConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(b)
	if err != nil {
		return n, addr, err
	}
	bb, err := Unpack(b[c.Cipher.SaltSize():], b[:n], c)
	if err != nil {
		return n, addr, err
	}
	copy(b, bb)
	return len(bb), addr, err
}
