package ptlshs

import "bytes"

type recordType uint8

const (
	recordTypeChangeCipherSpec recordType = 20
	recordTypeAlert            recordType = 21
	recordTypeHandshake        recordType = 22
	recordTypeApplicationData  recordType = 23

	recordHeaderLen = 5
)

type tlsRecord []byte

func (r tlsRecord) recordType() recordType {
	if len(r) == 0 {
		panic("invalid zero-length TLS record")
	}
	return recordType(r[0])
}

// recordReader is used to read a stream of TLS records and identify record boundaries.
type recordReader struct {
	// current is either nil or incomplete
	current tlsRecord
}

// read from the stream. The input should reflect the next part of the stream, successive to the
// input from the last call to read. Any records completed in b are returned.
func (rr *recordReader) read(b []byte) []tlsRecord {
	records := []tlsRecord{}
	buf := bytes.NewBuffer(b)
	for buf.Len() > 0 {
		if len(rr.current) < recordHeaderLen {
			remainingInHeader := recordHeaderLen - len(rr.current)
			toCopy := min(buf.Len(), remainingInHeader)
			rr.current = append(rr.current, buf.Next(toCopy)...)
			if len(rr.current) < recordHeaderLen {
				// b ended in the middle of a header.
				return records
			}
		} else {
			toCopy := min(buf.Len(), rr.currentRemaining())
			rr.current = append(rr.current, buf.Next(toCopy)...)
			if rr.currentRemaining() == 0 {
				records = append(records, tlsRecord(rr.current))
				rr.current = nil
			}
		}
	}
	return records
}

func (rr *recordReader) currentRemaining() int {
	if len(rr.current) < recordHeaderLen {
		panic("current header is incomplete")
	}
	payloadLen := int(rr.current[3])<<8 | int(rr.current[4])
	return payloadLen - len(rr.current) + recordHeaderLen
}

func (rr *recordReader) currentlyBuffered() int {
	return len(rr.current)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
