package trafficlog

import (
	"fmt"
	"io"

	"github.com/getlantern/errors"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// LinkType denotes a possible format of link layer packets.
type LinkType int

// Possible link layer types.
const (
	LinkTypeEthernet LinkType = iota
	LinkTypeLoopback LinkType = iota
)

func linkTypeFrom(lt layers.LinkType) LinkType {
	switch lt {
	case layers.LinkTypeEthernet:
		return LinkTypeEthernet
	case layers.LinkTypeLoop, layers.LinkTypeNull:
		return LinkTypeLoopback
	default:
		panic(fmt.Sprint("unsupported link type ", lt))
	}
}

func (lt LinkType) gopacketLayerType() gopacket.LayerType {
	switch lt {
	case LinkTypeEthernet:
		return layers.LayerTypeEthernet
	case LinkTypeLoopback:
		return layers.LayerTypeLoopback
	default:
		panic("unknown link type")
	}
}

func (lt LinkType) gopacketLinkType() layers.LinkType {
	switch lt {
	case LinkTypeEthernet:
		return layers.LinkTypeEthernet
	case LinkTypeLoopback:
		return layers.LinkTypeNull
	default:
		panic("unknown link type")
	}
}

// PacketMutator is a function which takes in link-layer packets (e.g. ethernet frames) and
// writes out some manipulation of those packets. The result must also be a link-layer packet.
//
// PacketMutators should not hold on to the packet or the writer after returning.
type PacketMutator func(linkPkt []byte, w io.Writer) error

// A MutatorFactory is used to produce new packet mutators.
type MutatorFactory interface {
	MutatorFor(LinkType) PacketMutator
}

// NoOpFactory implements MutatorFactory, producing PacketMutators which do not perform any
// mutations on input packets.
type NoOpFactory struct{}

// MutatorFor implements the MutatorFactory interface.
func (f NoOpFactory) MutatorFor(_ LinkType) PacketMutator {
	return func(pkt []byte, w io.Writer) error { _, err := w.Write(pkt); return err }
}

// AppStripperFactory implements MutatorFactory, producing PacketMutators which strip application
// layer data out of input packets.
type AppStripperFactory struct{}

// MutatorFor implements the MutatorFactory interface, producing PacketMutators which strip
// application layer data out of input packets.
func (f AppStripperFactory) MutatorFor(linkType LinkType) PacketMutator {
	var (
		eth     layers.Ethernet
		lb      layers.Loopback
		ip4     layers.IPv4
		ip6     layers.IPv6
		tcp     layers.TCP
		udp     layers.UDP
		payload gopacket.Payload

		decoded = make([]gopacket.LayerType, 4)
		parser  = gopacket.NewDecodingLayerParser(
			linkType.gopacketLayerType(), &eth, &lb, &ip4, &ip6, &tcp, &udp, &payload,
		)
	)
	return func(linkPkt []byte, w io.Writer) error {
		decodeErr := parser.DecodeLayers(linkPkt, &decoded)
		var link, network, transport gopacket.Layer
		for _, layerType := range decoded {
			switch layerType {
			case layers.LayerTypeEthernet:
				link = &eth
			case layers.LayerTypeLoopback:
				link = &lb
			case layers.LayerTypeIPv4:
				network = &ip4
			case layers.LayerTypeIPv6:
				network = &ip6
			case layers.LayerTypeTCP:
				transport = &tcp
			case layers.LayerTypeUDP:
				transport = &udp
			}
		}
		if decodeErr != nil && (transport == nil || network == nil || link == nil) {
			// Note: we ignore decoding errors if we were still able to decode the expected layers.
			return errors.New("decoding error: %v", decodeErr)
		}
		for _, layer := range []gopacket.Layer{link, network, transport} {
			if _, err := w.Write(layer.LayerContents()); err != nil {
				return errors.New("write failed: %v", err)
			}
		}
		return nil
	}
}
