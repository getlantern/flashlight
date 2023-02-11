// Package tlsutil is used for lower-level TLS operations. The code in this package is largely
// adapted from crypto/tls in the standard library.
package tlsutil

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
)

// A LargePayloadError is returned by WriteRecord if the payload is too large for a single record.
type LargePayloadError struct {
	MaxSizeForCipherSuite int
	payloadSize           int
}

func (e LargePayloadError) Error() string {
	return fmt.Sprintf("payload of %d bytes is too large for a single record", e.payloadSize)
}

// A DecryptError is returned by ReadRecord if the payload could not be successfully decrypted.
type DecryptError struct {
	cause error
}

func (e DecryptError) Error() string { return e.cause.Error() }
func (e DecryptError) Unwrap() error { return e.cause }

// TLS record types.
type recordType uint8

// Constants copied from crypto/tls.
const (
	// tcpMSSEstimate is a conservative estimate of the TCP maximum segment
	// size (MSS). A constant is used, rather than querying the kernel for
	// the actual MSS, to avoid complexity. The value here is the IPv6
	// minimum MTU (1280 bytes) minus the overhead of an IPv6 header (40
	// bytes) and a TCP header with timestamps (32 bytes).
	tcpMSSEstimate = 1208

	maxPlaintext       = 16384        // maximum plaintext payload length
	maxCiphertext      = 16384 + 2048 // maximum ciphertext payload length
	maxCiphertextTLS13 = 16384 + 256  // maximum ciphertext length in TLS 1.3
	recordHeaderLen    = 5            // record header length
	maxHandshake       = 65536        // maximum handshake we support (protocol max is 16 MB)

	recordTypeChangeCipherSpec recordType = 20
	recordTypeAlert            recordType = 21
	recordTypeHandshake        recordType = 22
	recordTypeApplicationData  recordType = 23
)

// WriteRecord writes the data to the input writer. Unlike WriteRecords, this function returns a
// LargePayloadError if the data cannot fit into a single record. In general, WriteRecords should be
// used unless containing the write to a single record is a requirement.
func WriteRecord(w io.Writer, data []byte, cs *ConnectionState) (int, error) {
	if len(data) > cs.maxPayloadSizeForWrite() {
		return 0, LargePayloadError{cs.maxPayloadSizeForWrite(), len(data)}
	}
	return WriteRecords(w, data, cs)
}

// WriteRecords writes the data to the input writer. The payload will be broken up into multiple
// records as needed (the cipher suite has a maximum payload size).
//
// This function is adapted from tls.Conn.writeRecordLocked.
func WriteRecords(w io.Writer, data []byte, cs *ConnectionState) (int, error) {
	var n int
	for len(data) > 0 {
		m := len(data)
		if maxPayload := cs.maxPayloadSizeForWrite(); m > maxPayload {
			m = maxPayload
		}

		outBuf := make([]byte, recordHeaderLen)
		outBuf[0] = byte(recordTypeApplicationData)
		vers := cs.version
		if vers == 0 {
			// Some TLS servers fail if the record version is
			// greater than TLS 1.0 for the initial ClientHello.
			vers = tls.VersionTLS10
		} else if vers == tls.VersionTLS13 {
			// TLS 1.3 froze the record layer version to 1.2.
			// See RFC 8446, Section 5.1.
			vers = tls.VersionTLS12
		}
		outBuf[1] = byte(vers >> 8)
		outBuf[2] = byte(vers)
		outBuf[3] = byte(m >> 8)
		outBuf[4] = byte(m)

		var err error
		outBuf, err = cs.encrypt(outBuf, data[:m], rand.Reader)
		if err != nil {
			return n, err
		}
		if _, err := w.Write(outBuf); err != nil {
			return n, err
		}
		n += m
		data = data[m:]
	}

	return n, nil
}

// ReadResult is the result of an attempt to read a TLS record.
type ReadResult struct {
	Data []byte

	// N is the total number of bytes read off the reader up to and including this record.
	N int
}

// ReadRecord reads a TLS record from the input reader. The input secret is broken up into a session
// key and MAC key as needed for the connection's cipher suite.
//
// ReadRecord may "over-read" from r. In this case, unprocessed data is returned along with the
// record data or error.
func ReadRecord(r io.Reader, cs *ConnectionState) (data []byte, unprocessed []byte, err error) {
	buf := new(bytes.Buffer)
	record, _, err := readRecord(r, buf, cs, recordTypeApplicationData)
	return record, buf.Bytes(), err
}

// ReadRecords is like ReadRecord, but attempts to read all records in r. Results will be returned
// in a slice.
//
// This function is adapted from tls.Conn.readRecordOrCCS.
func ReadRecords(r io.Reader, cs *ConnectionState) ([]ReadResult, error) {
	var (
		buf            = new(bytes.Buffer)
		record, n, err = readRecord(r, buf, cs, recordTypeApplicationData)
		currentN       int
	)
	if err != nil {
		return nil, err
	}

	copyBytes := func(b []byte) []byte {
		copied := make([]byte, len(b))
		copy(copied, b)
		return copied
	}
	results := []ReadResult{{copyBytes(record), n - buf.Len()}}

	// It is possible for the loop below to exit "early" - while there are still unread records in
	// r. This would happen if r.Read returned a slice ending at a record boundary. In practice,
	// this likely means that records arrived while this function was executing. It is reasonable
	// that we cannot guarantee such records will be read. Additionally, the consequences are small
	// as the caller can simply call this function again if they are expecting more data.

	for buf.Len() > 0 {
		record, currentN, err = readRecord(r, buf, cs, recordTypeApplicationData)
		if err != nil {
			return nil, err
		}
		n += currentN
		results = append(results, ReadResult{copyBytes(record), n - buf.Len()})
	}
	return results, nil
}

// The returned byte slice is owned by buf. If consecutive calls are made with the same buffer, the
// contents of the returned slice should be copied between calls.
func readRecord(r io.Reader, buf *bytes.Buffer, cs *ConnectionState, expectedType recordType) ([]byte, int, error) {
	n64, err := readFromUntil(r, buf, recordHeaderLen)
	n := int(n64)
	if err != nil {
		// RFC 8446, Section 6.1 suggests that EOF without an alertCloseNotify
		// is an error, but popular web sites seem to do this, so we accept it
		// if and only if at the record boundary.
		if err == io.ErrUnexpectedEOF && buf.Len() == 0 {
			err = io.EOF
		}
		return nil, n, err
	}

	hdr := buf.Bytes()[:recordHeaderLen]
	vers := uint16(hdr[1])<<8 | uint16(hdr[2])
	payloadLen := int(hdr[3])<<8 | int(hdr[4])
	if cs.version != tls.VersionTLS13 && vers != cs.version {
		return nil, n, fmt.Errorf("received record with version %x when expecting version %x", vers, cs.version)
	}
	if cs.version == tls.VersionTLS13 && payloadLen > maxCiphertextTLS13 || payloadLen > maxCiphertext {
		return nil, n, fmt.Errorf("oversized record received with length %d", payloadLen)
	}
	n64, err = readFromUntil(r, buf, recordHeaderLen+payloadLen)
	n += int(n64)
	if err != nil {
		return nil, n, err
	}

	// Process message.
	data, typ, err := cs.decrypt(buf.Next(recordHeaderLen + payloadLen))
	if err != nil {
		return nil, n, DecryptError{&net.OpError{Op: "local error", Err: err}}
	}
	if len(data) > maxPlaintext {
		return nil, n, DecryptError{&net.OpError{Op: "local error", Err: errors.New("record overflow")}}
	}

	if typ != expectedType {
		if typ == recordTypeAlert && len(data) >= 2 {
			return nil, n, DecryptError{UnexpectedAlertError{Alert(data[1])}}
		}
		return nil, n, DecryptError{fmt.Errorf("unexpected record type: %d (expected %d)", typ, expectedType)}
	}
	// Application Data messages are always protected.
	if cs.getReadCipher() == nil && typ == recordTypeApplicationData {
		return nil, n, DecryptError{&net.OpError{Op: "local error", Err: errors.New("unexpected message")}}
	}

	return data, n, nil
}

// readFromUntil reads from r into c.rawInput until c.rawInput contains
// at least n bytes or else returns an error.
func readFromUntil(r io.Reader, buf *bytes.Buffer, n int) (int64, error) {
	if buf.Len() >= n {
		return 0, nil
	}
	needs := n - buf.Len()
	// There might be extra input waiting on the wire. Make a best effort
	// attempt to fetch it so that it can be used in (*Conn).Read to
	// "predict" closeNotify alerts.
	buf.Grow(needs + bytes.MinRead)
	return buf.ReadFrom(&atLeastReader{r, int64(needs)})
}

// atLeastReader reads from R, stopping with EOF once at least N bytes have been
// read. It is different from an io.LimitedReader in that it doesn't cut short
// the last Read call, and in that it considers an early EOF an error.
type atLeastReader struct {
	R io.Reader
	N int64
}

func (r *atLeastReader) Read(p []byte) (int, error) {
	if r.N <= 0 {
		return 0, io.EOF
	}
	n, err := r.R.Read(p)
	r.N -= int64(n) // won't underflow unless len(p) >= n > 9223372036854775809
	if r.N > 0 && err == io.EOF {
		return n, io.ErrUnexpectedEOF
	}
	if r.N <= 0 && err == nil {
		return n, io.EOF
	}
	return n, err
}
