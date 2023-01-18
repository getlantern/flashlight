package darkstar

import (
	"bytes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
)

// payloadSizeMask is the maximum size of payload in bytes.
const payloadSizeMask = 0x3FFF // 16*1024 - 1

type writer struct {
	io.Writer
	cipher.AEAD
	//	nonce   []byte
	counter uint64
	buf     []byte
}

// newWriter wraps an io.Writer with AEAD encryption.
func newWriter(w io.Writer, aead cipher.AEAD) *writer {
	return &writer{
		Writer:  w,
		AEAD:    aead,
		buf:     make([]byte, 2+aead.Overhead()+payloadSizeMask+aead.Overhead()),
		counter: 0,
	}
}

// Write encrypts b and writes to the embedded io.Writer.
func (w *writer) Write(b []byte) (int, error) {
	n, err := w.ReadFrom(bytes.NewBuffer(b))
	return int(n), err
}

// ReadFrom reads from the given io.Reader until EOF or error, encrypts and
// writes to the embedded io.Writer. Returns number of bytes read from r and
// any error encountered.
func (w *writer) ReadFrom(r io.Reader) (n int64, err error) {
	for {
		buf := w.buf
		payloadBuf := buf[2+w.Overhead() : 2+w.Overhead()+payloadSizeMask]
		nr, er := r.Read(payloadBuf)

		if nr > 0 {
			if nr > math.MaxUint16 {
				return 0, errors.New("message size too big")
			}
			n += int64(nr)
			buf = buf[:2+w.Overhead()+nr+w.Overhead()]
			payloadBuf = payloadBuf[:nr]
			buf[0], buf[1] = byte(nr>>8), byte(nr) // big-endian payload size

			if w.counter > math.MaxUint64-2 {
				return 0, errors.New("nonce counter overflow")
			}

			nonceBytes := nonce(w.counter)
			w.counter += 1
			w.Seal(buf[:0], nonceBytes, buf[:2], nil)

			nonceBytes = nonce(w.counter)
			w.counter += 1
			w.Seal(payloadBuf[:0], nonceBytes, payloadBuf, nil)

			_, ew := w.Writer.Write(buf)
			if ew != nil {
				err = ew
				break
			}
		}

		if er != nil {
			if er != io.EOF { // ignore EOF as per io.ReaderFrom contract
				err = er
			}
			break
		}
	}

	return n, err
}

type reader struct {
	io.Reader
	cipher.AEAD
	counter  uint64
	buf      []byte
	leftover []byte
}

// newReader wraps an io.Reader with AEAD decryption.
func newReader(r io.Reader, aead cipher.AEAD) *reader {
	return &reader{
		Reader:  r,
		AEAD:    aead,
		buf:     make([]byte, payloadSizeMask+aead.Overhead()),
		counter: 0,
	}
}

// read and decrypt a record into the internal buffer. Return decrypted payload length and any error encountered.
func (r *reader) read() (int, error) {
	// decrypt payload size
	buf := r.buf[:2+r.Overhead()]
	_, err := io.ReadFull(r.Reader, buf)
	if err != nil {
		return 0, err
	}

	if r.counter > math.MaxUint64-2 {
		return 0, errors.New("nonce counter overflow")
	}

	nonceBytes := nonce(r.counter)
	r.counter += 1

	_, err = r.Open(buf[:0], nonceBytes, buf, nil)
	if err != nil {
		return 0, err
	}

	size := (int(buf[0])<<8 + int(buf[1])) & payloadSizeMask

	// decrypt payload
	buf = r.buf[:size+r.Overhead()]
	_, err = io.ReadFull(r.Reader, buf)
	if err != nil {
		return 0, err
	}

	nonceBytes = nonce(r.counter)
	r.counter += 1

	_, err = r.Open(buf[:0], nonceBytes, buf, nil)
	if err != nil {
		return 0, err
	}

	return size, nil
}

// Read reads from the embedded io.Reader, decrypts and writes to b.
func (r *reader) Read(b []byte) (int, error) {
	// copy decrypted bytes (if any) from previous record first
	if len(r.leftover) > 0 {
		n := copy(b, r.leftover)
		r.leftover = r.leftover[n:]
		return n, nil
	}

	n, err := r.read()
	m := copy(b, r.buf[:n])
	if m < n { // insufficient len(b), keep leftover for next read
		r.leftover = r.buf[m:n]
	}
	return m, err
}

// WriteTo reads from the embedded io.Reader, decrypts and writes to w until
// there's no more data to write or when an error occurs. Return number of
// bytes written to w and any error encountered.
func (r *reader) WriteTo(w io.Writer) (n int64, err error) {
	// write decrypted bytes left over from previous record
	for len(r.leftover) > 0 {
		nw, ew := w.Write(r.leftover)
		r.leftover = r.leftover[nw:]
		n += int64(nw)
		if ew != nil {
			return n, ew
		}
	}

	for {
		nr, er := r.read()
		if nr > 0 {
			nw, ew := w.Write(r.buf[:nr])
			n += int64(nw)

			if ew != nil {
				err = ew
				break
			}
		}

		if er != nil {
			if er != io.EOF { // ignore EOF as per io.Copy contract (using src.WriteTo shortcut)
				err = er
			}
			break
		}
	}

	return n, err
}

func nonce(counter uint64) []byte {
	// NIST Special Publication 800-38D - Recommendation for Block Cipher Modes of Operation: Galois/Counter Mode (GCM) and GMAC
	// https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf
	// Section 8.2.1 - Deterministic Construction
	// Applicable to nonces of 96 bytes or less.
	/*
	   In the deterministic construction, the IV is the concatenation of two
	   fields, called the fixed field and the invocation field. The fixed field
	   shall identify the device, or, more generally, the context for the
	   instance of the authenticated encryption function. The invocation field
	   shall identify the sets of inputs to the authenticated encryption
	   function in that particular device.
	   For any given key, no two distinct devices shall share the same fixed
	   field, and no two distinct sets of inputs to any single device shall
	   share the same invocation field. Compliance with these two requirements
	   implies compliance with the uniqueness requirement on IVs in Sec. 8.
	   If desired, the fixed field itself may be constructed from two or more
	   smaller fields. Moreover, one of those smaller fields could consist of
	   bits that are arbitrary (i.e., not necessarily deterministic nor unique
	   to the device), as long as the remaining bits ensure that the fixed
	   field is not repeated in its entirety for some other device with the
	   same key.
	   Similarly, the entire fixed field may consist of arbitrary bits when
	   there is only one context to identify, such as when a fresh key is
	   limited to a single session of a communications protocol. In this case,
	   if different participants in the session share a common fixed field,
	   then the protocol shall ensure that the invocation fields are distinct
	   for distinct data inputs.
	*/

	fixedField := []byte{0x1a, 0x1a, 0x1a, 0x1a} // 4 bytes = 32 bits
	/*
	   The invocation field typically is either 1) an integer counter or 2) a
	   linear feedback shift register that is driven by a primitive polynomial
	   to ensure a maximal cycle length. In either case, the invocation field
	   increments upon each invocation of the authenticated encryption
	   function.
	   The lengths and positions of the fixed field and the invocation field
	   shall be fixed for each supported IV length for the life of the key. In
	   order to promote interoperability for the default IV length of 96 bits,
	   this Recommendation suggests, but does not require, that the leading
	   (i.e., leftmost) 32 bits of the IV hold the fixed field; and that the
	   trailing (i.e., rightmost) 64 bits hold the invocation field.
	*/

	invocationField := make([]byte, 8)
	binary.BigEndian.PutUint64(invocationField, counter)

	nonceData := append(fixedField[:], invocationField[:]...)

	return nonceData
}

type darkStarStreamConn struct {
	net.Conn
	encryptCipher cipher.AEAD
	decryptCipher cipher.AEAD
	r             *reader
	w             *writer
}

func (c *darkStarStreamConn) initReader() error {
	c.r = newReader(c.Conn, c.decryptCipher)
	return nil
}

func (c *darkStarStreamConn) Read(b []byte) (int, error) {
	if c.r == nil {
		if err := c.initReader(); err != nil {
			return 0, err
		}
	}
	return c.r.Read(b)
}

func (c *darkStarStreamConn) WriteTo(w io.Writer) (int64, error) {
	if c.r == nil {
		if err := c.initReader(); err != nil {
			return 0, err
		}
	}
	return c.r.WriteTo(w)
}

func (c *darkStarStreamConn) initWriter() error {
	c.w = newWriter(c.Conn, c.encryptCipher)
	return nil
}

func (c *darkStarStreamConn) Write(b []byte) (int, error) {
	if c.w == nil {
		if err := c.initWriter(); err != nil {
			return 0, err
		}
	}
	return c.w.Write(b)
}

func (c *darkStarStreamConn) ReadFrom(r io.Reader) (int64, error) {
	if c.w == nil {
		if err := c.initWriter(); err != nil {
			return 0, err
		}
	}
	return c.w.ReadFrom(r)
}

// NewConn wraps a stream-oriented net.Conn with cipher.
func NewDarkStarConn(c net.Conn, encryptCipher cipher.AEAD, decryptCipher cipher.AEAD) net.Conn {
	return &darkStarStreamConn{Conn: c, encryptCipher: encryptCipher, decryptCipher: decryptCipher}
}
