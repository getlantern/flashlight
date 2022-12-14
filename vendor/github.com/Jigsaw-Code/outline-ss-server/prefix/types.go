package prefix

import (
	"strings"
)

type Prefix interface {
	Make() ([]byte, error)
}

func FromString(s string) Prefix {
	switch strings.ToLower(s) {
	case "none":
		return NewNonePrefix()
	case "dnsovertcp":
		return NewDNSOverTCPPrefix()
	default:
		return NewNonePrefix()
	}
}
