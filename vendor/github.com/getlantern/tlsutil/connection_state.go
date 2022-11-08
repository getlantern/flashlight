package tlsutil

import (
	"crypto/cipher"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
)

// ConnectionState tracks the state of a TLS connection. This reflects a subset of the information
// usually held internally on the tls.Conn type.
type ConnectionState struct {
	version                 uint16
	readCipher, writeCipher *cipherState
	mac                     macFunction
	seq                     [8]byte  // 64-bit sequence number
	additionalData          [13]byte // to avoid allocs; interface method args escape
}

// NewConnectionState creates a connection state based on the input version and cipher suite. The
// suite should be represented in https://golang.org/pkg/crypto/tls/#pkg-constants.
//
// The secret, IV, and sequence number will be used as needed as parameters for the cipher suite.
// The sequence number is sometimes used as a nonce and should thus be unique per-connection. All
// parameters should be agreed upon by both client and server.
func NewConnectionState(version, cipherSuite uint16, secret [52]byte, iv [16]byte, seq [8]byte) (
	*ConnectionState, error) {

	cs, ok := cipherSuites[cipherSuite]
	if !ok {
		return nil, fmt.Errorf("unrecognized cipher suite identifier %d", cipherSuite)
	}
	return &ConnectionState{
		version:     version,
		readCipher:  cs.getCipherState(secret, iv, true, version),
		writeCipher: cs.getCipherState(secret, iv, false, version),
		seq:         seq,
	}, nil
}

func (cs *ConnectionState) getWriteCipher() interface{} {
	if cs.writeCipher == nil {
		return nil
	}

	return cs.writeCipher.cipher
}

func (cs *ConnectionState) getReadCipher() interface{} {
	if cs.readCipher == nil {
		return nil
	}

	return cs.readCipher.cipher
}

// Simplified version of tls.Conn.maxPayloadSizeForWrite.
// Subtract TLS overheads to get the maximum payload size.
func (cs ConnectionState) maxPayloadSizeForWrite() int {
	explicitNonceLen := 0
	if cs.writeCipher != nil {
		explicitNonceLen = cs.writeCipher.explicitNonceLen(cs.version)
	}
	payloadBytes := tcpMSSEstimate - recordHeaderLen - explicitNonceLen
	if cs.writeCipher != nil {
		mac := cs.writeCipher.mac
		switch ciph := cs.writeCipher.cipher.(type) {
		case cipher.Stream:
			payloadBytes -= mac.Size()
		case cipher.AEAD:
			payloadBytes -= ciph.Overhead()
		case cbcMode:
			blockSize := ciph.BlockSize()
			// The payload must fit in a multiple of blockSize, with
			// room for at least one padding byte.
			payloadBytes = (payloadBytes & ^(blockSize - 1)) - 1
			// The MAC is appended before padding so affects the
			// payload size directly.
			payloadBytes -= mac.Size()
		default:
			panic(fmt.Sprintf("unknown cipher type: %#x", ciph))
		}
	} else {
		return maxPlaintext
	}

	return payloadBytes
}

// encrypt encrypts payload, adding the appropriate nonce and/or MAC, and
// appends it to record, which contains the record header.
func (cs *ConnectionState) encrypt(record, payload []byte, rand io.Reader) ([]byte, error) {
	_cipher := cs.getWriteCipher()
	if _cipher == nil {
		return append(record, payload...), nil
	}

	var explicitNonce []byte
	if explicitNonceLen := cs.writeCipher.explicitNonceLen(cs.version); explicitNonceLen > 0 {
		record, explicitNonce = sliceForAppend(record, explicitNonceLen)
		if _, isCBC := _cipher.(cbcMode); !isCBC && explicitNonceLen < 16 {
			// The AES-GCM construction in TLS has an explicit nonce so that the
			// nonce can be random. However, the nonce is only 8 bytes which is
			// too small for a secure, random nonce. Therefore we use the
			// sequence number as the nonce. The 3DES-CBC construction also has
			// an 8 bytes nonce but its nonces must be unpredictable (see RFC
			// 5246, Appendix F.3), forcing us to use randomness. That's not
			// 3DES' biggest problem anyway because the birthday bound on block
			// collision is reached first due to its simlarly small block size
			// (see the Sweet32 attack).
			copy(explicitNonce, cs.seq[:])
		} else {
			if _, err := io.ReadFull(rand, explicitNonce); err != nil {
				return nil, err
			}
		}
	}

	var mac []byte
	if cs.writeCipher.mac != nil {
		mac = cs.writeCipher.mac.MAC(cs.seq[:], record[:recordHeaderLen], payload, nil)
	}

	var dst []byte
	switch c := _cipher.(type) {
	case cipher.Stream:
		record, dst = sliceForAppend(record, len(payload)+len(mac))
		c.XORKeyStream(dst[:len(payload)], payload)
		c.XORKeyStream(dst[len(payload):], mac)
	case aead:
		nonce := explicitNonce
		if len(nonce) == 0 {
			nonce = cs.seq[:]
		}

		if cs.version == tls.VersionTLS13 {
			record = append(record, payload...)

			// Encrypt the actual ContentType and replace the plaintext one.
			record = append(record, record[0])
			record[0] = byte(recordTypeApplicationData)

			n := len(payload) + 1 + c.Overhead()
			record[3] = byte(n >> 8)
			record[4] = byte(n)

			record = c.Seal(record[:recordHeaderLen],
				nonce, record[recordHeaderLen:], record[:recordHeaderLen])
		} else {
			copy(cs.additionalData[:], cs.seq[:])
			copy(cs.additionalData[8:], record)
			record = c.Seal(record, nonce, payload, cs.additionalData[:])
		}
	case cbcMode:
		blockSize := c.BlockSize()
		plaintextLen := len(payload) + len(mac)
		paddingLen := blockSize - plaintextLen%blockSize
		record, dst = sliceForAppend(record, plaintextLen+paddingLen)
		copy(dst, payload)
		copy(dst[len(payload):], mac)
		for i := plaintextLen; i < len(dst); i++ {
			dst[i] = byte(paddingLen - 1)
		}
		if len(explicitNonce) > 0 {
			c.SetIV(explicitNonce)
		}
		c.CryptBlocks(dst, dst)
	default:
		panic(fmt.Sprintf("unknown cipher type: %#x", c))
	}

	// Update length to include nonce, MAC and any block padding needed.
	n := len(record) - recordHeaderLen
	record[3] = byte(n >> 8)
	record[4] = byte(n)
	cs.incSeq()

	return record, nil
}

func (cs *ConnectionState) decrypt(record []byte) ([]byte, recordType, error) {
	var plaintext []byte
	typ := recordType(record[0])
	payload := record[recordHeaderLen:]

	// In TLS 1.3, change_cipher_spec messages are to be ignored without being
	// decrypted. See RFC 8446, Appendix D.4.
	if cs.version == tls.VersionTLS13 && typ == recordTypeChangeCipherSpec {
		return payload, typ, nil
	}

	paddingGood := byte(255)
	paddingLen := 0

	explicitNonceLen := 0
	if cs.readCipher != nil {
		explicitNonceLen = cs.readCipher.explicitNonceLen(cs.version)
	}

	_cipher := cs.getReadCipher()
	if _cipher != nil {
		switch c := _cipher.(type) {
		case cipher.Stream:
			c.XORKeyStream(payload, payload)
		case aead:
			if len(payload) < explicitNonceLen {
				return nil, 0, errors.New("bad record MAC")
			}
			nonce := payload[:explicitNonceLen]
			if len(nonce) == 0 {
				nonce = cs.seq[:]
			}
			payload = payload[explicitNonceLen:]

			additionalData := cs.additionalData[:]
			if cs.version == tls.VersionTLS13 {
				additionalData = record[:recordHeaderLen]
			} else {
				copy(additionalData, cs.seq[:])
				copy(additionalData[8:], record[:3])
				n := len(payload) - c.Overhead()
				additionalData[11] = byte(n >> 8)
				additionalData[12] = byte(n)
			}

			var err error
			plaintext, err = c.Open(payload[:0], nonce, payload, additionalData)
			if err != nil {
				return nil, 0, errors.New("bad record MAC")
			}
		case cbcMode:
			blockSize := c.BlockSize()
			mac := cs.readCipher.mac
			minPayload := explicitNonceLen + roundUp(mac.Size()+1, blockSize)
			if len(payload)%blockSize != 0 || len(payload) < minPayload {
				return nil, 0, errors.New("bad record MAC")
			}

			if explicitNonceLen > 0 {
				c.SetIV(payload[:explicitNonceLen])
				payload = payload[explicitNonceLen:]
			}
			c.CryptBlocks(payload, payload)

			// In a limited attempt to protect against CBC padding oracles like
			// Lucky13, the data past paddingLen (which is secret) is passed to
			// the MAC function as extra data, to be fed into the HMAC after
			// computing the digest. This makes the MAC roughly constant time as
			// long as the digest computation is constant time and does not
			// affect the subsequent write, modulo cache effects.
			if cs.version == tls.VersionSSL30 {
				paddingLen, paddingGood = extractPaddingSSL30(payload)
			} else {
				paddingLen, paddingGood = extractPadding(payload)
			}
		default:
			panic(fmt.Sprintf("unknown cipher type: %#x", c))
		}

		if cs.version == tls.VersionTLS13 {
			if typ != recordTypeApplicationData {
				return nil, 0, errors.New("unexpected message")
			}
			if len(plaintext) > maxPlaintext+1 {
				return nil, 0, errors.New("record overflow")
			}
			// Remove padding and find the ContentType scanning from the end.
			for i := len(plaintext) - 1; i >= 0; i-- {
				if plaintext[i] != 0 {
					typ = recordType(plaintext[i])
					plaintext = plaintext[:i]
					break
				}
				if i == 0 {
					return nil, 0, errors.New("unexpected message")
				}
			}
		}
	} else {
		plaintext = payload
	}

	if cs.readCipher != nil && cs.readCipher.mac != nil {
		mac := cs.readCipher.mac
		macSize := mac.Size()
		if len(payload) < macSize {
			return nil, 0, errors.New("bad record MAC")
		}

		n := len(payload) - macSize - paddingLen
		n = subtle.ConstantTimeSelect(int(uint32(n)>>31), 0, n) // if n < 0 { n = 0 }
		record[3] = byte(n >> 8)
		record[4] = byte(n)
		remoteMAC := payload[n : n+macSize]
		localMAC := mac.MAC(cs.seq[0:], record[:recordHeaderLen], payload[:n], payload[n+macSize:])

		// This is equivalent to checking the MACs and paddingGood
		// separately, but in constant-time to prevent distinguishing
		// padding failures from MAC failures. Depending on what value
		// of paddingLen was returned on bad padding, distinguishing
		// bad MAC from bad padding can lead to an attack.
		//
		// See also the logic at the end of extractPadding.
		macAndPaddingGood := subtle.ConstantTimeCompare(localMAC, remoteMAC) & int(paddingGood)
		if macAndPaddingGood != 1 {
			return nil, 0, errors.New("bad record MAC")
		}

		plaintext = payload[:n]
	}

	cs.incSeq()
	return plaintext, typ, nil
}

// incSeq increments the sequence number.
func (cs *ConnectionState) incSeq() {
	for i := 7; i >= 0; i-- {
		cs.seq[i]++
		if cs.seq[i] != 0 {
			return
		}
	}

	// Not allowed to let sequence number wrap.
	// Instead, must renegotiate before it does.
	// Not likely enough to bother.
	panic("TLS: sequence number wraparound")
}

// sliceForAppend extends the input slice by n bytes. head is the full extended
// slice, while tail is the appended part. If the original slice has sufficient
// capacity no allocation is performed.
func sliceForAppend(in []byte, n int) (head, tail []byte) {
	if total := len(in) + n; cap(in) >= total {
		head = in[:total]
	} else {
		head = make([]byte, total)
		copy(head, in)
	}
	tail = head[len(in):]
	return
}

// extractPadding returns, in constant time, the length of the padding to remove
// from the end of payload. It also returns a byte which is equal to 255 if the
// padding was valid and 0 otherwise. See RFC 2246, Section 6.2.3.2.
func extractPadding(payload []byte) (toRemove int, good byte) {
	if len(payload) < 1 {
		return 0, 0
	}

	paddingLen := payload[len(payload)-1]
	t := uint(len(payload)-1) - uint(paddingLen)
	// if len(payload) >= (paddingLen - 1) then the MSB of t is zero
	good = byte(int32(^t) >> 31)

	// The maximum possible padding length plus the actual length field
	toCheck := 256
	// The length of the padded data is public, so we can use an if here
	if toCheck > len(payload) {
		toCheck = len(payload)
	}

	for i := 0; i < toCheck; i++ {
		t := uint(paddingLen) - uint(i)
		// if i <= paddingLen then the MSB of t is zero
		mask := byte(int32(^t) >> 31)
		b := payload[len(payload)-1-i]
		good &^= mask&paddingLen ^ mask&b
	}

	// We AND together the bits of good and replicate the result across
	// all the bits.
	good &= good << 4
	good &= good << 2
	good &= good << 1
	good = uint8(int8(good) >> 7)

	// Zero the padding length on error. This ensures any unchecked bytes
	// are included in the MAC. Otherwise, an attacker that could
	// distinguish MAC failures from padding failures could mount an attack
	// similar to POODLE in SSL 3.0: given a good ciphertext that uses a
	// full block's worth of padding, replace the final block with another
	// block. If the MAC check passed but the padding check failed, the
	// last byte of that block decrypted to the block size.
	//
	// See also macAndPaddingGood logic below.
	paddingLen &= good

	toRemove = int(paddingLen) + 1
	return
}

// extractPaddingSSL30 is a replacement for extractPadding in the case that the
// protocol version is SSLv3. In this version, the contents of the padding
// are random and cannot be checked.
func extractPaddingSSL30(payload []byte) (toRemove int, good byte) {
	if len(payload) < 1 {
		return 0, 0
	}

	paddingLen := int(payload[len(payload)-1]) + 1
	if paddingLen > len(payload) {
		return 0, 0
	}

	return paddingLen, 255
}

// cbcMode is an interface for block ciphers using cipher block chaining.
type cbcMode interface {
	cipher.BlockMode
	SetIV([]byte)
}

func roundUp(a, b int) int {
	return a + (b-a%b)%b
}
