package tlsutil

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/rc4"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"hash"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/cryptobyte"
	"golang.org/x/crypto/hkdf"
)

// A cipher suite currently in use.
type cipherState struct {
	cipher interface{}
	mac    macFunction
}

// explicitNonceLen returns the number of bytes of explicit nonce or IV included
// in each record. Explicit nonces are present only in CBC modes after TLS 1.0
// and in certain AEAD modes in TLS 1.2.
func (cs cipherState) explicitNonceLen(version uint16) int {
	switch c := cs.cipher.(type) {
	case cipher.Stream:
		return 0
	case aead:
		return c.explicitNonceLen()
	case cbcMode:
		// TLS 1.1 introduced a per-record explicit IV to fix the BEAST attack.
		if version >= tls.VersionTLS11 {
			return c.BlockSize()
		}
		return 0
	default:
		panic(fmt.Sprintf("unknown cipher type: %#x", c))
	}
}

// cipherSuiteI is an interface unifying the cipherSuite and cipherSuiteTLS13 types.
type cipherSuiteI interface {
	getCipherState(secret [52]byte, iv [16]byte, reading bool, version uint16) *cipherState
}

var cipherSuites = map[uint16]cipherSuiteI{
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305:    cipherSuite{32, 0, 12, nil, nil, aeadChaCha20Poly1305},
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305:  cipherSuite{32, 0, 12, nil, nil, aeadChaCha20Poly1305},
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:   cipherSuite{16, 0, 4, nil, nil, aeadAESGCM},
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256: cipherSuite{16, 0, 4, nil, nil, aeadAESGCM},
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:   cipherSuite{32, 0, 4, nil, nil, aeadAESGCM},
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384: cipherSuite{32, 0, 4, nil, nil, aeadAESGCM},
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256:   cipherSuite{16, 32, 16, cipherAES, macSHA256, nil},
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA:      cipherSuite{16, 20, 16, cipherAES, macSHA1, nil},
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256: cipherSuite{16, 32, 16, cipherAES, macSHA256, nil},
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA:    cipherSuite{16, 20, 16, cipherAES, macSHA1, nil},
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:      cipherSuite{32, 20, 16, cipherAES, macSHA1, nil},
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA:    cipherSuite{32, 20, 16, cipherAES, macSHA1, nil},
	tls.TLS_RSA_WITH_AES_128_GCM_SHA256:         cipherSuite{16, 0, 4, nil, nil, aeadAESGCM},
	tls.TLS_RSA_WITH_AES_256_GCM_SHA384:         cipherSuite{32, 0, 4, nil, nil, aeadAESGCM},
	tls.TLS_RSA_WITH_AES_128_CBC_SHA256:         cipherSuite{16, 32, 16, cipherAES, macSHA256, nil},
	tls.TLS_RSA_WITH_AES_128_CBC_SHA:            cipherSuite{16, 20, 16, cipherAES, macSHA1, nil},
	tls.TLS_RSA_WITH_AES_256_CBC_SHA:            cipherSuite{32, 20, 16, cipherAES, macSHA1, nil},
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:     cipherSuite{24, 20, 8, cipher3DES, macSHA1, nil},
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:           cipherSuite{24, 20, 8, cipher3DES, macSHA1, nil},

	// RC4-based cipher suites are disabled by default.
	tls.TLS_RSA_WITH_RC4_128_SHA:         cipherSuite{16, 20, 0, cipherRC4, macSHA1, nil},
	tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA:   cipherSuite{16, 20, 0, cipherRC4, macSHA1, nil},
	tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA: cipherSuite{16, 20, 0, cipherRC4, macSHA1, nil},

	tls.TLS_AES_128_GCM_SHA256:       cipherSuiteTLS13{16, aeadAESGCMTLS13, crypto.SHA256},
	tls.TLS_CHACHA20_POLY1305_SHA256: cipherSuiteTLS13{32, aeadChaCha20Poly1305, crypto.SHA256},
	tls.TLS_AES_256_GCM_SHA384:       cipherSuiteTLS13{32, aeadAESGCMTLS13, crypto.SHA384},
}

// A cipherSuite is a stripped-down version of the construct of the same name in crypto/tls.
type cipherSuite struct {
	// the lengths, in bytes, of the key material needed for each component.
	keyLen int
	macLen int
	ivLen  int

	cipher func(key, iv []byte, isRead bool) interface{}
	mac    func(version uint16, macKey []byte) macFunction
	aead   func(key, fixedNonce []byte) aead
}

func (cs cipherSuite) getCipherState(secret [52]byte, iv [16]byte, reading bool, version uint16) *cipherState {
	c := cipherState{}
	key, trimmedIV := secret[:cs.keyLen], iv[:cs.ivLen]
	if cs.cipher != nil {
		c.cipher = cs.cipher(key, trimmedIV, reading)
	} else {
		c.cipher = cs.aead(key, trimmedIV)
	}
	if cs.mac != nil {
		macKey := secret[cs.keyLen : cs.keyLen+cs.macLen]
		c.mac = cs.mac(version, macKey)
	}
	return &c
}

// A cipherSuiteTLS13 defines only the pair of the AEAD algorithm and hash
// algorithm to be used with HKDF. See RFC 8446, Appendix B.4.
type cipherSuiteTLS13 struct {
	keyLen int
	aead   func(key, fixedNonce []byte) aead
	hash   crypto.Hash
}

func (cs cipherSuiteTLS13) getCipherState(secret [52]byte, iv [16]byte, reading bool, version uint16) *cipherState {
	trafficSecret := make([]byte, len(secret)+len(iv))
	copy(trafficSecret, secret[:])
	copy(trafficSecret[len(secret):], iv[:])
	derivedKey, derivedIV := cs.trafficKey(trafficSecret)
	return &cipherState{cs.aead(derivedKey, derivedIV), nil}
}

// expandLabel implements HKDF-Expand-Label from RFC 8446, Section 7.1.
func (cs *cipherSuiteTLS13) expandLabel(secret []byte, label string, context []byte, length int) []byte {
	var hkdfLabel cryptobyte.Builder
	hkdfLabel.AddUint16(uint16(length))
	hkdfLabel.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddBytes([]byte("tls13 "))
		b.AddBytes([]byte(label))
	})
	hkdfLabel.AddUint8LengthPrefixed(func(b *cryptobyte.Builder) {
		b.AddBytes(context)
	})
	out := make([]byte, length)
	n, err := hkdf.Expand(cs.hash.New, secret, hkdfLabel.BytesOrPanic()).Read(out)
	if err != nil || n != length {
		panic("tls: HKDF-Expand-Label invocation failed unexpectedly")
	}
	return out
}

// trafficKey generates traffic keys according to RFC 8446, Section 7.3.
func (cs *cipherSuiteTLS13) trafficKey(trafficSecret []byte) (key, iv []byte) {
	key = cs.expandLabel(trafficSecret, "key", nil, cs.keyLen)
	iv = cs.expandLabel(trafficSecret, "iv", nil, aeadNonceLength)
	return
}

func cipherAES(key, iv []byte, isRead bool) interface{} {
	block, _ := aes.NewCipher(key)
	if isRead {
		return cipher.NewCBCDecrypter(block, iv)
	}
	return cipher.NewCBCEncrypter(block, iv)
}

func cipherRC4(key, iv []byte, isRead bool) interface{} {
	cipher, _ := rc4.NewCipher(key)
	return cipher
}

func cipher3DES(key, iv []byte, isRead bool) interface{} {
	block, _ := des.NewTripleDESCipher(key)
	if isRead {
		return cipher.NewCBCDecrypter(block, iv)
	}
	return cipher.NewCBCEncrypter(block, iv)
}

// macSHA1 returns a macFunction for the given protocol version.
func macSHA1(version uint16, key []byte) macFunction {
	if version == tls.VersionSSL30 {
		mac := ssl30MAC{
			h:   sha1.New(),
			key: make([]byte, len(key)),
		}
		copy(mac.key, key)
		return mac
	}
	return tls10MAC{h: hmac.New(newConstantTimeHash(sha1.New), key)}
}

// macSHA256 returns a SHA-256 based MAC. These are only supported in TLS 1.2
// so the given version is ignored.
func macSHA256(version uint16, key []byte) macFunction {
	return tls10MAC{h: hmac.New(sha256.New, key)}
}

type macFunction interface {
	// Size returns the length of the MAC.
	Size() int
	// MAC appends the MAC of (seq, header, data) to out. The extra data is fed
	// into the MAC after obtaining the result to normalize timing. The result
	// is only valid until the next invocation of MAC as the buffer is reused.
	MAC(seq, header, data, extra []byte) []byte
}

type aead interface {
	cipher.AEAD

	// explicitNonceLen returns the number of bytes of explicit nonce
	// included in each record. This is eight for older AEADs and
	// zero for modern ones.
	explicitNonceLen() int
}

const (
	aeadNonceLength   = 12
	noncePrefixLength = 4
)

// prefixNonceAEAD wraps an AEAD and prefixes a fixed portion of the nonce to
// each call.
type prefixNonceAEAD struct {
	// nonce contains the fixed part of the nonce in the first four bytes.
	nonce [aeadNonceLength]byte
	aead  cipher.AEAD
}

func (f *prefixNonceAEAD) NonceSize() int        { return aeadNonceLength - noncePrefixLength }
func (f *prefixNonceAEAD) Overhead() int         { return f.aead.Overhead() }
func (f *prefixNonceAEAD) explicitNonceLen() int { return f.NonceSize() }

func (f *prefixNonceAEAD) Seal(out, nonce, plaintext, additionalData []byte) []byte {
	copy(f.nonce[4:], nonce)
	return f.aead.Seal(out, f.nonce[:], plaintext, additionalData)
}

func (f *prefixNonceAEAD) Open(out, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	copy(f.nonce[4:], nonce)
	return f.aead.Open(out, f.nonce[:], ciphertext, additionalData)
}

// xoredNonceAEAD wraps an AEAD by XORing in a fixed pattern to the nonce
// before each call.
type xorNonceAEAD struct {
	nonceMask [aeadNonceLength]byte
	aead      cipher.AEAD
}

func (f *xorNonceAEAD) NonceSize() int        { return 8 } // 64-bit sequence number
func (f *xorNonceAEAD) Overhead() int         { return f.aead.Overhead() }
func (f *xorNonceAEAD) explicitNonceLen() int { return 0 }

func (f *xorNonceAEAD) Seal(out, nonce, plaintext, additionalData []byte) []byte {
	for i, b := range nonce {
		f.nonceMask[4+i] ^= b
	}
	result := f.aead.Seal(out, f.nonceMask[:], plaintext, additionalData)
	for i, b := range nonce {
		f.nonceMask[4+i] ^= b
	}

	return result
}

func (f *xorNonceAEAD) Open(out, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	for i, b := range nonce {
		f.nonceMask[4+i] ^= b
	}
	result, err := f.aead.Open(out, f.nonceMask[:], ciphertext, additionalData)
	for i, b := range nonce {
		f.nonceMask[4+i] ^= b
	}

	return result, err
}

func aeadAESGCM(key, noncePrefix []byte) aead {
	if len(noncePrefix) != noncePrefixLength {
		panic("tls: internal error: wrong nonce length")
	}
	aes, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCM(aes)
	if err != nil {
		panic(err)
	}

	ret := &prefixNonceAEAD{aead: aead}
	copy(ret.nonce[:], noncePrefix)
	return ret
}

func aeadAESGCMTLS13(key, nonceMask []byte) aead {
	if len(nonceMask) != aeadNonceLength {
		panic("tls: internal error: wrong nonce length")
	}
	aes, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCM(aes)
	if err != nil {
		panic(err)
	}

	ret := &xorNonceAEAD{aead: aead}
	copy(ret.nonceMask[:], nonceMask)
	return ret
}

func aeadChaCha20Poly1305(key, nonceMask []byte) aead {
	if len(nonceMask) != aeadNonceLength {
		panic("tls: internal error: wrong nonce length")
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		panic(err)
	}

	ret := &xorNonceAEAD{aead: aead}
	copy(ret.nonceMask[:], nonceMask)
	return ret
}

// ssl30MAC implements the SSLv3 MAC function, as defined in
// www.mozilla.org/projects/security/pki/nss/ssl/draft302.txt section 5.2.3.1
type ssl30MAC struct {
	h   hash.Hash
	key []byte
	buf []byte
}

func (s ssl30MAC) Size() int {
	return s.h.Size()
}

var ssl30Pad1 = [48]byte{0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36, 0x36}

var ssl30Pad2 = [48]byte{0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c, 0x5c}

// MAC does not offer constant timing guarantees for SSL v3.0, since it's deemed
// useless considering the similar, protocol-level POODLE vulnerability.
func (s ssl30MAC) MAC(seq, header, data, extra []byte) []byte {
	padLength := 48
	if s.h.Size() == 20 {
		padLength = 40
	}

	s.h.Reset()
	s.h.Write(s.key)
	s.h.Write(ssl30Pad1[:padLength])
	s.h.Write(seq)
	s.h.Write(header[:1])
	s.h.Write(header[3:5])
	s.h.Write(data)
	s.buf = s.h.Sum(s.buf[:0])

	s.h.Reset()
	s.h.Write(s.key)
	s.h.Write(ssl30Pad2[:padLength])
	s.h.Write(s.buf)
	return s.h.Sum(s.buf[:0])
}

type constantTimeHash interface {
	hash.Hash
	ConstantTimeSum(b []byte) []byte
}

// cthWrapper wraps any hash.Hash that implements ConstantTimeSum, and replaces
// with that all calls to Sum. It's used to obtain a ConstantTimeSum-based HMAC.
type cthWrapper struct {
	h constantTimeHash
}

func (c *cthWrapper) Size() int                   { return c.h.Size() }
func (c *cthWrapper) BlockSize() int              { return c.h.BlockSize() }
func (c *cthWrapper) Reset()                      { c.h.Reset() }
func (c *cthWrapper) Write(p []byte) (int, error) { return c.h.Write(p) }
func (c *cthWrapper) Sum(b []byte) []byte         { return c.h.ConstantTimeSum(b) }

func newConstantTimeHash(h func() hash.Hash) func() hash.Hash {
	return func() hash.Hash {
		return &cthWrapper{h().(constantTimeHash)}
	}
}

// tls10MAC implements the TLS 1.0 MAC function. RFC 2246, Section 6.2.3.
type tls10MAC struct {
	h   hash.Hash
	buf []byte
}

func (s tls10MAC) Size() int {
	return s.h.Size()
}

// MAC is guaranteed to take constant time, as long as
// len(seq)+len(header)+len(data)+len(extra) is constant. extra is not fed into
// the MAC, but is only provided to make the timing profile constant.
func (s tls10MAC) MAC(seq, header, data, extra []byte) []byte {
	s.h.Reset()
	s.h.Write(seq)
	s.h.Write(header)
	s.h.Write(data)
	res := s.h.Sum(s.buf[:0])
	if extra != nil {
		s.h.Write(extra)
	}
	return res
}
