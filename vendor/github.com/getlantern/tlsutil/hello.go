package tlsutil

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/cryptobyte"
)

const (
	// TLS handshake message types.
	typeClientHello uint8 = 1
	typeServerHello uint8 = 2
)

// ValidateClientHello can be used to identify the input bytes as a TLS ClientHello message. Returns
// an error iff the input message cannot be parsed as a ClientHello. Returns io.UnexpectedEOF if the
// message is well-formed, but incomplete. When b contains a complete ClientHello, the return value
// n indicates that the first n bytes of b contain the record. Returns ErrorUnexpectedAlert if b
// begins with an alert record.
func ValidateClientHello(b []byte) (n int, err error) {
	msgType, b, err := parseHandshakeHeader(b)
	if err != nil {
		return 0, err
	}
	if msgType != typeClientHello {
		return 0, fmt.Errorf("expected ClientHello message, got TLS handshake type %d", b[0])
	}

	m := new(clientHelloMsg)
	if !m.unmarshal(b) {
		return 0, fmt.Errorf("malformed message")
	}
	return len(b) + recordHeaderLen, nil
}

// A ServerHello message. Only includes a subset of fields on the message.
type ServerHello struct {
	Version, Suite uint16
	Random         []byte
}

// ParseServerHello parses a ServerHello message. Returns ErrorUnexpectedAlert if b starts with an
// alert record.
func ParseServerHello(b []byte) (*ServerHello, error) {
	msgType, b, err := parseHandshakeHeader(b)
	if err != nil {
		return nil, err
	}
	if msgType != typeServerHello {
		return nil, fmt.Errorf("expected ServerHello message, got TLS handshake type %d", b[0])
	}

	m := new(serverHelloMsg)
	if !m.unmarshal(b) {
		return nil, fmt.Errorf("malformed message")
	}
	if m.supportedVersion != 0 {
		return &ServerHello{m.supportedVersion, m.cipherSuite, m.random}, nil
	}
	return &ServerHello{m.vers, m.cipherSuite, m.random}, nil
}

func parseHandshakeHeader(b []byte) (msgType uint8, rest []byte, err error) {
	const (
		minLen   = recordHeaderLen + 4 // 4 bytes is the minimum hello message size
		alertLen = recordHeaderLen + 2
	)
	if len(b) >= alertLen && recordType(b[0]) == recordTypeAlert {
		return 0, nil, UnexpectedAlertError{Alert: Alert(b[alertLen-1])}
	}
	if len(b) < minLen {
		return 0, nil, fmt.Errorf("message of length %d bytes is less than minimum of %d bytes", len(b), minLen)
	}

	version := uint16(b[1])<<8 | uint16(b[2])
	cs := &ConnectionState{version: version}
	b, _, err = readRecord(bytes.NewReader(b), new(bytes.Buffer), cs, recordTypeHandshake)
	if err != nil {
		return 0, nil, err
	}
	if len(b) < 4 {
		return 0, nil, errors.New("message too small")
	}

	msgType = b[0]
	n := int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n > maxHandshake {
		return 0, nil, fmt.Errorf("message of length %d bytes exceeds maximum of %d bytes", n, maxHandshake)
	}
	if len(b) < 4+n {
		return 0, nil, fmt.Errorf("message too small: length specified as %d, but is actually %d", 4+n, len(b))
	}
	return msgType, b[:n+4], nil
}

type clientHelloMsg struct {
	raw                              []byte
	vers                             uint16
	random                           []byte
	sessionID                        []byte
	cipherSuites                     []uint16
	compressionMethods               []uint8
	nextProtoNeg                     bool
	serverName                       string
	ocspStapling                     bool
	supportedCurves                  []curveID
	supportedPoints                  []uint8
	ticketSupported                  bool
	sessionTicket                    []uint8
	supportedSignatureAlgorithms     []signatureScheme
	supportedSignatureAlgorithmsCert []signatureScheme
	secureRenegotiationSupported     bool
	secureRenegotiation              []byte
	alpnProtocols                    []string
	scts                             bool
	supportedVersions                []uint16
	cookie                           []byte
	keyShares                        []keyShare
	earlyData                        bool
	pskModes                         []uint8
	pskIdentities                    []pskIdentity
	pskBinders                       [][]byte
}

func (m *clientHelloMsg) unmarshal(data []byte) bool {
	*m = clientHelloMsg{raw: data}
	s := cryptobyte.String(data)

	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint16(&m.vers) || !s.ReadBytes(&m.random, 32) ||
		!readUint8LengthPrefixed(&s, &m.sessionID) {
		return false
	}

	var cipherSuites cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&cipherSuites) {
		return false
	}
	m.cipherSuites = []uint16{}
	m.secureRenegotiationSupported = false
	for !cipherSuites.Empty() {
		var suite uint16
		if !cipherSuites.ReadUint16(&suite) {
			return false
		}
		if suite == scsvRenegotiation {
			m.secureRenegotiationSupported = true
		}
		m.cipherSuites = append(m.cipherSuites, suite)
	}

	if !readUint8LengthPrefixed(&s, &m.compressionMethods) {
		return false
	}

	if s.Empty() {
		// ClientHello is optionally followed by extension data
		return true
	}

	var extensions cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&extensions) || !s.Empty() {
		return false
	}

	for !extensions.Empty() {
		var extension uint16
		var extData cryptobyte.String
		if !extensions.ReadUint16(&extension) ||
			!extensions.ReadUint16LengthPrefixed(&extData) {
			return false
		}

		switch extension {
		case extensionServerName:
			// RFC 6066, Section 3
			var nameList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&nameList) || nameList.Empty() {
				return false
			}
			for !nameList.Empty() {
				var nameType uint8
				var serverName cryptobyte.String
				if !nameList.ReadUint8(&nameType) ||
					!nameList.ReadUint16LengthPrefixed(&serverName) ||
					serverName.Empty() {
					return false
				}
				if nameType != 0 {
					continue
				}
				if len(m.serverName) != 0 {
					// Multiple names of the same name_type are prohibited.
					return false
				}
				m.serverName = string(serverName)
				// An SNI value may not include a trailing dot.
				if strings.HasSuffix(m.serverName, ".") {
					return false
				}
			}
		case extensionNextProtoNeg:
			// draft-agl-tls-nextprotoneg-04
			m.nextProtoNeg = true
		case extensionStatusRequest:
			// RFC 4366, Section 3.6
			var statusType uint8
			var ignored cryptobyte.String
			if !extData.ReadUint8(&statusType) ||
				!extData.ReadUint16LengthPrefixed(&ignored) ||
				!extData.ReadUint16LengthPrefixed(&ignored) {
				return false
			}
			m.ocspStapling = statusType == statusTypeOCSP
		case extensionSupportedCurves:
			// RFC 4492, sections 5.1.1 and RFC 8446, Section 4.2.7
			var curves cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&curves) || curves.Empty() {
				return false
			}
			for !curves.Empty() {
				var curve uint16
				if !curves.ReadUint16(&curve) {
					return false
				}
				m.supportedCurves = append(m.supportedCurves, curveID(curve))
			}
		case extensionSupportedPoints:
			// RFC 4492, Section 5.1.2
			if !readUint8LengthPrefixed(&extData, &m.supportedPoints) ||
				len(m.supportedPoints) == 0 {
				return false
			}
		case extensionSessionTicket:
			// RFC 5077, Section 3.2
			m.ticketSupported = true
			extData.ReadBytes(&m.sessionTicket, len(extData))
		case extensionSignatureAlgorithms:
			// RFC 5246, Section 7.4.1.4.1
			var sigAndAlgs cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&sigAndAlgs) || sigAndAlgs.Empty() {
				return false
			}
			for !sigAndAlgs.Empty() {
				var sigAndAlg uint16
				if !sigAndAlgs.ReadUint16(&sigAndAlg) {
					return false
				}
				m.supportedSignatureAlgorithms = append(
					m.supportedSignatureAlgorithms, signatureScheme(sigAndAlg))
			}
		case extensionSignatureAlgorithmsCert:
			// RFC 8446, Section 4.2.3
			var sigAndAlgs cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&sigAndAlgs) || sigAndAlgs.Empty() {
				return false
			}
			for !sigAndAlgs.Empty() {
				var sigAndAlg uint16
				if !sigAndAlgs.ReadUint16(&sigAndAlg) {
					return false
				}
				m.supportedSignatureAlgorithmsCert = append(
					m.supportedSignatureAlgorithmsCert, signatureScheme(sigAndAlg))
			}
		case extensionRenegotiationInfo:
			// RFC 5746, Section 3.2
			if !readUint8LengthPrefixed(&extData, &m.secureRenegotiation) {
				return false
			}
			m.secureRenegotiationSupported = true
		case extensionALPN:
			// RFC 7301, Section 3.1
			var protoList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&protoList) || protoList.Empty() {
				return false
			}
			for !protoList.Empty() {
				var proto cryptobyte.String
				if !protoList.ReadUint8LengthPrefixed(&proto) || proto.Empty() {
					return false
				}
				m.alpnProtocols = append(m.alpnProtocols, string(proto))
			}
		case extensionSCT:
			// RFC 6962, Section 3.3.1
			m.scts = true
		case extensionSupportedVersions:
			// RFC 8446, Section 4.2.1
			var versList cryptobyte.String
			if !extData.ReadUint8LengthPrefixed(&versList) || versList.Empty() {
				return false
			}
			for !versList.Empty() {
				var vers uint16
				if !versList.ReadUint16(&vers) {
					return false
				}
				m.supportedVersions = append(m.supportedVersions, vers)
			}
		case extensionCookie:
			// RFC 8446, Section 4.2.2
			if !readUint16LengthPrefixed(&extData, &m.cookie) ||
				len(m.cookie) == 0 {
				return false
			}
		case extensionKeyShare:
			// RFC 8446, Section 4.2.8
			var clientShares cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&clientShares) {
				return false
			}
			for !clientShares.Empty() {
				var ks keyShare
				if !clientShares.ReadUint16((*uint16)(&ks.group)) ||
					!readUint16LengthPrefixed(&clientShares, &ks.data) ||
					len(ks.data) == 0 {
					return false
				}
				m.keyShares = append(m.keyShares, ks)
			}
		case extensionEarlyData:
			// RFC 8446, Section 4.2.10
			m.earlyData = true
		case extensionPSKModes:
			// RFC 8446, Section 4.2.9
			if !readUint8LengthPrefixed(&extData, &m.pskModes) {
				return false
			}
		case extensionPreSharedKey:
			// RFC 8446, Section 4.2.11
			if !extensions.Empty() {
				return false // pre_shared_key must be the last extension
			}
			var identities cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&identities) || identities.Empty() {
				return false
			}
			for !identities.Empty() {
				var psk pskIdentity
				if !readUint16LengthPrefixed(&identities, &psk.label) ||
					!identities.ReadUint32(&psk.obfuscatedTicketAge) ||
					len(psk.label) == 0 {
					return false
				}
				m.pskIdentities = append(m.pskIdentities, psk)
			}
			var binders cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&binders) || binders.Empty() {
				return false
			}
			for !binders.Empty() {
				var binder []byte
				if !readUint8LengthPrefixed(&binders, &binder) ||
					len(binder) == 0 {
					return false
				}
				m.pskBinders = append(m.pskBinders, binder)
			}
		default:
			// Ignore unknown extensions.
			continue
		}

		if !extData.Empty() {
			return false
		}
	}

	return true
}

type serverHelloMsg struct {
	raw                          []byte
	vers                         uint16
	random                       []byte
	sessionID                    []byte
	cipherSuite                  uint16
	compressionMethod            uint8
	nextProtoNeg                 bool
	nextProtos                   []string
	ocspStapling                 bool
	ticketSupported              bool
	secureRenegotiationSupported bool
	secureRenegotiation          []byte
	alpnProtocol                 string
	scts                         [][]byte
	supportedVersion             uint16
	serverShare                  keyShare
	selectedIdentityPresent      bool
	selectedIdentity             uint16

	// HelloRetryRequest extensions
	cookie        []byte
	selectedGroup curveID
}

func (m *serverHelloMsg) unmarshal(data []byte) bool {
	*m = serverHelloMsg{raw: data}
	s := cryptobyte.String(data)

	if !s.Skip(4) || // message type and uint24 length field
		!s.ReadUint16(&m.vers) || !s.ReadBytes(&m.random, 32) ||
		!readUint8LengthPrefixed(&s, &m.sessionID) ||
		!s.ReadUint16(&m.cipherSuite) ||
		!s.ReadUint8(&m.compressionMethod) {
		return false
	}

	if s.Empty() {
		// ServerHello is optionally followed by extension data
		return true
	}

	var extensions cryptobyte.String
	if !s.ReadUint16LengthPrefixed(&extensions) || !s.Empty() {
		return false
	}

	for !extensions.Empty() {
		var extension uint16
		var extData cryptobyte.String
		if !extensions.ReadUint16(&extension) ||
			!extensions.ReadUint16LengthPrefixed(&extData) {
			return false
		}

		switch extension {
		case extensionNextProtoNeg:
			m.nextProtoNeg = true
			for !extData.Empty() {
				var proto cryptobyte.String
				if !extData.ReadUint8LengthPrefixed(&proto) ||
					proto.Empty() {
					return false
				}
				m.nextProtos = append(m.nextProtos, string(proto))
			}
		case extensionStatusRequest:
			m.ocspStapling = true
		case extensionSessionTicket:
			m.ticketSupported = true
		case extensionRenegotiationInfo:
			if !readUint8LengthPrefixed(&extData, &m.secureRenegotiation) {
				return false
			}
			m.secureRenegotiationSupported = true
		case extensionALPN:
			var protoList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&protoList) || protoList.Empty() {
				return false
			}
			var proto cryptobyte.String
			if !protoList.ReadUint8LengthPrefixed(&proto) ||
				proto.Empty() || !protoList.Empty() {
				return false
			}
			m.alpnProtocol = string(proto)
		case extensionSCT:
			var sctList cryptobyte.String
			if !extData.ReadUint16LengthPrefixed(&sctList) || sctList.Empty() {
				return false
			}
			for !sctList.Empty() {
				var sct []byte
				if !readUint16LengthPrefixed(&sctList, &sct) ||
					len(sct) == 0 {
					return false
				}
				m.scts = append(m.scts, sct)
			}
		case extensionSupportedVersions:
			if !extData.ReadUint16(&m.supportedVersion) {
				return false
			}
		case extensionCookie:
			if !readUint16LengthPrefixed(&extData, &m.cookie) ||
				len(m.cookie) == 0 {
				return false
			}
		case extensionKeyShare:
			// This extension has different formats in SH and HRR, accept either
			// and let the handshake logic decide. See RFC 8446, Section 4.2.8.
			if len(extData) == 2 {
				if !extData.ReadUint16((*uint16)(&m.selectedGroup)) {
					return false
				}
			} else {
				if !extData.ReadUint16((*uint16)(&m.serverShare.group)) ||
					!readUint16LengthPrefixed(&extData, &m.serverShare.data) {
					return false
				}
			}
		case extensionPreSharedKey:
			m.selectedIdentityPresent = true
			if !extData.ReadUint16(&m.selectedIdentity) {
				return false
			}
		default:
			// Ignore unknown extensions.
			continue
		}

		if !extData.Empty() {
			return false
		}
	}

	return true
}

// readUint8LengthPrefixed acts like s.ReadUint8LengthPrefixed, but targets a
// []byte instead of a cryptobyte.String.
func readUint8LengthPrefixed(s *cryptobyte.String, out *[]byte) bool {
	return s.ReadUint8LengthPrefixed((*cryptobyte.String)(out))
}

// readUint16LengthPrefixed acts like s.ReadUint16LengthPrefixed, but targets a
// []byte instead of a cryptobyte.String.
func readUint16LengthPrefixed(s *cryptobyte.String, out *[]byte) bool {
	return s.ReadUint16LengthPrefixed((*cryptobyte.String)(out))
}

// TLS extension numbers
const (
	extensionServerName              uint16 = 0
	extensionStatusRequest           uint16 = 5
	extensionSupportedCurves         uint16 = 10 // supported_groups in TLS 1.3, see RFC 8446, Section 4.2.7
	extensionSupportedPoints         uint16 = 11
	extensionSignatureAlgorithms     uint16 = 13
	extensionALPN                    uint16 = 16
	extensionSCT                     uint16 = 18
	extensionSessionTicket           uint16 = 35
	extensionPreSharedKey            uint16 = 41
	extensionEarlyData               uint16 = 42
	extensionSupportedVersions       uint16 = 43
	extensionCookie                  uint16 = 44
	extensionPSKModes                uint16 = 45
	extensionCertificateAuthorities  uint16 = 47
	extensionSignatureAlgorithmsCert uint16 = 50
	extensionKeyShare                uint16 = 51

	extensionNextProtoNeg      uint16 = 13172 // not IANA assigned
	extensionRenegotiationInfo uint16 = 0xff01
)

// TLS signaling cipher suite values
const (
	scsvRenegotiation uint16 = 0x00ff
)

// TLS CertificateStatusType (RFC 3546)
const (
	statusTypeOCSP uint8 = 1
)

// signatureScheme identifies a signature algorithm supported by TLS. See
// RFC 8446, Section 4.2.3.
type signatureScheme uint16

// TLS 1.3 PSK Identity. Can be a Session Ticket, or a reference to a saved
// session. See RFC 8446, Section 4.2.11.
type pskIdentity struct {
	label               []byte
	obfuscatedTicketAge uint32
}

// curveID is the type of a TLS identifier for an elliptic curve. See
// https://www.iana.org/assignments/tls-parameters/tls-parameters.xml#tls-parameters-8.
//
// In TLS 1.3, this type is called NamedGroup, but at this time this library
// only supports Elliptic Curve based groups. See RFC 8446, Section 4.2.7.
type curveID uint16

// TLS 1.3 Key Share. See RFC 8446, Section 4.2.8.
type keyShare struct {
	group curveID
	data  []byte
}
