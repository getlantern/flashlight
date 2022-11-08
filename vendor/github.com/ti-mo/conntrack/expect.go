package conntrack

import (
	"fmt"

	"github.com/mdlayher/netlink"
	"github.com/pkg/errors"
	"github.com/ti-mo/netfilter"
)

const (
	opUnExpectNAT = "ExpectNAT unmarshal"
)

// Expect represents an 'expected' connection, created by Conntrack/IPTables helpers.
// Active connections created by helpers are shown by the conntrack tooling as 'RELATED'.
type Expect struct {
	ID, Timeout uint32

	TupleMaster, Tuple, Mask Tuple

	Zone uint16

	HelpName, Function string

	Flags, Class uint32

	NAT ExpectNAT
}

// ExpectNAT holds NAT information about an expected connection.
type ExpectNAT struct {
	Direction bool
	Tuple     Tuple
}

// unmarshal unmarshals a netfilter.Attribute into an ExpectNAT.
func (en *ExpectNAT) unmarshal(ad *netlink.AttributeDecoder) error {

	if ad.Len() == 0 {
		return errors.Wrap(errNeedSingleChild, opUnExpectNAT)
	}

	for ad.Next() {
		switch expectNATType(ad.Type()) {
		case ctaExpectNATDir:
			en.Direction = ad.Uint32() == 1
		case ctaExpectNATTuple:
			ad.Nested(en.Tuple.unmarshal)
		default:
			return errors.Wrap(fmt.Errorf(errAttributeChild, ad.Type()), opUnExpectNAT)
		}
	}

	return ad.Err()
}

func (en ExpectNAT) marshal() (netfilter.Attribute, error) {

	nfa := netfilter.Attribute{Type: uint16(ctaExpectNAT), Nested: true, Children: make([]netfilter.Attribute, 2)}

	var dir uint32
	if en.Direction {
		dir = 1
	}

	nfa.Children[0] = netfilter.Attribute{Type: uint16(ctaExpectNATDir), Data: netfilter.Uint32Bytes(dir)}

	ta, err := en.Tuple.marshal(uint16(ctaExpectNATTuple))
	if err != nil {
		return nfa, err
	}
	nfa.Children[1] = ta

	return nfa, nil
}

// unmarshal unmarshals a list of netfilter.Attributes into an Expect structure.
func (ex *Expect) unmarshal(ad *netlink.AttributeDecoder) error {

	for ad.Next() {
		switch at := expectType(ad.Type()); at {
		case ctaExpectMaster:
			if !nestedFlag(ad.TypeFlags()) {
				return errors.Wrap(errNotNested, opUnTup)
			}
			ad.Nested(ex.TupleMaster.unmarshal)
		case ctaExpectTuple:
			if !nestedFlag(ad.TypeFlags()) {
				return errors.Wrap(errNotNested, opUnTup)
			}
			ad.Nested(ex.Tuple.unmarshal)
		case ctaExpectMask:
			if !nestedFlag(ad.TypeFlags()) {
				return errors.Wrap(errNotNested, opUnTup)
			}
			ad.Nested(ex.Mask.unmarshal)
		case ctaExpectTimeout:
			ex.Timeout = ad.Uint32()
		case ctaExpectID:
			ex.ID = ad.Uint32()
		case ctaExpectHelpName:
			ex.HelpName = ad.String()
		case ctaExpectZone:
			ex.Zone = ad.Uint16()
		case ctaExpectFlags:
			ex.Flags = ad.Uint32()
		case ctaExpectClass:
			ex.Class = ad.Uint32()
		case ctaExpectNAT:
			if !nestedFlag(ad.TypeFlags()) {
				return errors.Wrap(errNotNested, opUnExpectNAT)
			}
			ad.Nested(ex.NAT.unmarshal)
		case ctaExpectFN:
			ex.Function = ad.String()
		}
	}

	return ad.Err()
}

func (ex Expect) marshal() ([]netfilter.Attribute, error) {

	// Expectations need Tuple, Mask and TupleMaster filled to be valid.
	if !ex.Tuple.filled() || !ex.Mask.filled() || !ex.TupleMaster.filled() {
		return nil, errExpectNeedTuples
	}

	attrs := make([]netfilter.Attribute, 4, 10)

	tm, err := ex.TupleMaster.marshal(uint16(ctaExpectMaster))
	if err != nil {
		return nil, err
	}
	attrs[0] = tm

	tp, err := ex.Tuple.marshal(uint16(ctaExpectTuple))
	if err != nil {
		return nil, err
	}
	attrs[1] = tp

	ts, err := ex.Mask.marshal(uint16(ctaExpectMask))
	if err != nil {
		return nil, err
	}
	attrs[2] = ts

	attrs[3] = netfilter.Attribute{Type: uint16(ctaExpectTimeout), Data: netfilter.Uint32Bytes(ex.Timeout)}

	if ex.HelpName != "" {
		attrs = append(attrs, netfilter.Attribute{Type: uint16(ctaExpectHelpName), Data: []byte(ex.HelpName)})
	}

	if ex.Zone != 0 {
		attrs = append(attrs, netfilter.Attribute{Type: uint16(ctaExpectZone), Data: netfilter.Uint16Bytes(ex.Zone)})
	}

	if ex.Class != 0 {
		attrs = append(attrs, netfilter.Attribute{Type: uint16(ctaExpectClass), Data: netfilter.Uint32Bytes(ex.Class)})
	}

	if ex.Flags != 0 {
		attrs = append(attrs, netfilter.Attribute{Type: uint16(ctaExpectFlags), Data: netfilter.Uint32Bytes(ex.Flags)})
	}

	if ex.Function != "" {
		attrs = append(attrs, netfilter.Attribute{Type: uint16(ctaExpectFN), Data: []byte(ex.Function)})
	}

	if ex.NAT.Tuple.filled() {
		en, err := ex.NAT.marshal()
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, en)
	}

	return attrs, nil
}

// unmarshalExpect unmarshals an Expect from a netlink.Message.
// The Message must contain valid attributes.
func unmarshalExpect(nlm netlink.Message) (Expect, error) {

	var ex Expect

	_, ad, err := netfilter.DecodeNetlink(nlm)
	if err != nil {
		return ex, err
	}

	err = ex.unmarshal(ad)
	if err != nil {
		return ex, err
	}

	return ex, nil
}

// unmarshalExpects unmarshals a list of expected connections from a list of Netlink messages.
// This method can be used to parse the result of a dump or get query.
func unmarshalExpects(nlm []netlink.Message) ([]Expect, error) {

	// Pre-allocate to avoid re-allocating output slice on every op
	out := make([]Expect, 0, len(nlm))

	for i := 0; i < len(nlm); i++ {

		ex, err := unmarshalExpect(nlm[i])
		if err != nil {
			return nil, err
		}

		out = append(out, ex)
	}

	return out, nil
}
