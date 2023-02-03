package tlsutil

import "strconv"

// Alert represents a TLS alert, sent by the peer.
// See https://datatracker.ietf.org/doc/html/rfc5246#section-7.2
type Alert uint8

// Possible Alerts. See https://datatracker.ietf.org/doc/html/rfc5246#section-7.2
const (
	AlertCloseNotify                  Alert = 0
	AlertUnexpectedMessage            Alert = 10
	AlertBadRecordMAC                 Alert = 20
	AlertDecryptionFailed             Alert = 21
	AlertRecordOverflow               Alert = 22
	AlertDecompressionFailure         Alert = 30
	AlertHandshakeFailure             Alert = 40
	AlertBadCertificate               Alert = 42
	AlertUnsupportedCertificate       Alert = 43
	AlertCertificateRevoked           Alert = 44
	AlertCertificateExpired           Alert = 45
	AlertCertificateUnknown           Alert = 46
	AlertIllegalParameter             Alert = 47
	AlertUnknownCA                    Alert = 48
	AlertAccessDenied                 Alert = 49
	AlertDecodeError                  Alert = 50
	AlertDecryptError                 Alert = 51
	AlertExportRestriction            Alert = 60
	AlertProtocolVersion              Alert = 70
	AlertInsufficientSecurity         Alert = 71
	AlertInternalError                Alert = 80
	AlertInappropriateFallback        Alert = 86
	AlertUserCanceled                 Alert = 90
	AlertNoRenegotiation              Alert = 100
	AlertMissingExtension             Alert = 109
	AlertUnsupportedExtension         Alert = 110
	AlertCertificateUnobtainable      Alert = 111
	AlertUnrecognizedName             Alert = 112
	AlertBadCertificateStatusResponse Alert = 113
	AlertBadCertificateHashValue      Alert = 114
	AlertUnknownPSKIdentity           Alert = 115
	AlertCertificateRequired          Alert = 116
	AlertNoApplicationProtocol        Alert = 120
)

var alertText = map[Alert]string{
	AlertCloseNotify:                  "close notify",
	AlertUnexpectedMessage:            "unexpected message",
	AlertBadRecordMAC:                 "bad record MAC",
	AlertDecryptionFailed:             "decryption failed",
	AlertRecordOverflow:               "record overflow",
	AlertDecompressionFailure:         "decompression failure",
	AlertHandshakeFailure:             "handshake failure",
	AlertBadCertificate:               "bad certificate",
	AlertUnsupportedCertificate:       "unsupported certificate",
	AlertCertificateRevoked:           "revoked certificate",
	AlertCertificateExpired:           "expired certificate",
	AlertCertificateUnknown:           "unknown certificate",
	AlertIllegalParameter:             "illegal parameter",
	AlertUnknownCA:                    "unknown certificate authority",
	AlertAccessDenied:                 "access denied",
	AlertDecodeError:                  "error decoding message",
	AlertDecryptError:                 "error decrypting message",
	AlertExportRestriction:            "export restriction",
	AlertProtocolVersion:              "protocol version not supported",
	AlertInsufficientSecurity:         "insufficient security level",
	AlertInternalError:                "internal error",
	AlertInappropriateFallback:        "inappropriate fallback",
	AlertUserCanceled:                 "user canceled",
	AlertNoRenegotiation:              "no renegotiation",
	AlertMissingExtension:             "missing extension",
	AlertUnsupportedExtension:         "unsupported extension",
	AlertCertificateUnobtainable:      "certificate unobtainable",
	AlertUnrecognizedName:             "unrecognized name",
	AlertBadCertificateStatusResponse: "bad certificate status response",
	AlertBadCertificateHashValue:      "bad certificate hash value",
	AlertUnknownPSKIdentity:           "unknown PSK identity",
	AlertCertificateRequired:          "certificate required",
	AlertNoApplicationProtocol:        "no application protocol",
}

func (e Alert) String() string {
	s, ok := alertText[e]
	if ok {
		return s
	}
	return "alert(" + strconv.Itoa(int(e)) + ")"
}

func (e Alert) Error() string {
	return e.String()
}

// UnexpectedAlertError is returned in some cases when an alert record is encountered unexpectedly.
type UnexpectedAlertError struct {
	Alert Alert
}

func (e UnexpectedAlertError) Error() string {
	return "received unexpected alert record"
}
