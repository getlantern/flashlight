package common

import (
	"encoding/hex"
	"encoding/json"
	"net"
	"strconv"

	"github.com/pkg/errors"
)

// GenericFreePeer is a representation of a FreePeer that can be used in any
// project requiring it. Usually, specific projects (e.g.,
// p2pregistrar) would use this struct to share data
// between them, but each would have their own concrete representation.
//
// TODO <09-06-2022, soltzen> This is an ideal usecase for an interface, but
// making it a struct now is simpler. Consider refactoring this later.
type GenericFreePeer struct {
	*innerGenericFreePeer
	IP                 net.IP `json:"-"`
	Port               int    `json:"-"`
	PubCertFingerprint []byte `json:"-"`
}

type innerGenericFreePeer struct {
	// We're intentionally using very short JSON field names since this payload
	// (and the ones from other FreePeers) will be pushed to the DHT where
	// there's a size limitation (about 1KiBs per message)
	IP                 string `json:"i,omitempty"`
	Port               int    `json:"p,omitempty"`
	PubCertFingerprint string `json:"c,omitempty"`
}

func (p *GenericFreePeer) MarshalJSON() ([]byte, error) {
	return json.Marshal(&innerGenericFreePeer{
		IP:                 p.IP.String(),
		Port:               p.Port,
		PubCertFingerprint: hex.EncodeToString(p.PubCertFingerprint),
	})
}

func (p *GenericFreePeer) UnmarshalJSON(b []byte) error {
	inPeer := &innerGenericFreePeer{}
	err := json.Unmarshal(b, &inPeer)
	if err != nil {
		return err
	}

	p.IP = net.ParseIP(inPeer.IP)
	p.Port = inPeer.Port
	// Don't even bother hex-decoding if it's empty, else
	// "p.PubCertFingerprint" will equal `[]`, not `nil`, which messes with our
	// test suits
	if inPeer.PubCertFingerprint != "" {
		p.PubCertFingerprint, err = hex.DecodeString(inPeer.PubCertFingerprint)
		if err != nil {
			return errors.Wrap(err, "failed to decode pubcert")
		}
	}
	return nil
}

func (p *GenericFreePeer) ToIpPort() string {
	return net.JoinHostPort(p.IP.String(), strconv.Itoa(p.Port))
}

func (p *GenericFreePeer) String() string {
	return p.ToIpPort()
}
