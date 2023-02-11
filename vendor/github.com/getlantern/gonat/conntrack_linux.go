package gonat

import (
	"syscall"

	"github.com/ti-mo/conntrack"
)

// Since we're using unconnected raw sockets, the kernel doesn't create ip_conntrack
// entries for us. When we receive a SYNACK packet from the upstream end in response
// to the SYN packet that we forward from the client, the kernel automatically sends
// an RST packet because it doesn't see a connection in the right state. We can't
// actually fake a connection in the right state, however we can manually create an entry
// in ip_conntrack which allows us to use a single iptables rule to safely drop
// all outbound RST packets for tracked tcp connections. The rule can be added like so:
//
//   iptables -A OUTPUT -p tcp -m conntrack --ctstate ESTABLISHED --ctdir ORIGINAL --tcp-flags RST RST -j DROP
//
func (s *server) createConntrackEntry(upFT FiveTuple) error {
	flow := s.ctFlowFor(true, upFT)
	log.Tracef("Creating conntrack entry for %v", upFT)
	return s.ctrack.Create(flow)
}

func (s *server) deleteConntrackEntry(upFT FiveTuple) {
	flow := s.ctFlowFor(false, upFT)
	if err := s.ctrack.Delete(flow); err != nil {
		if log.IsTraceEnabled() {
			// The below error is expected if the remote end closed the connection already, because
			// the OS automatically deletes the conntrack entry if it received an RST packet.
			log.Errorf("Unable to delete conntrack entry for %v: %v", upFT, err)
		}
	}
}

func (s *server) ctFlowFor(create bool, upFT FiveTuple) conntrack.Flow {
	var ctTimeout uint32
	var status conntrack.StatusFlag
	if create {
		// we set the status to ASSURED so that the kernel doesn't destroy the conntrack entry
		// prior to its expiration. It would do this because the connection doesn't look right and
		// very quickly transitions into a CLOSED state, at which point it would be eligible for
		// destruction even before its timeout.
		status = conntrack.StatusConfirmed | conntrack.StatusAssured
		ctTimeout = s.ctTimeout
	}

	flow := conntrack.NewFlow(
		upFT.IPProto, status,
		upFT.Src.IP(), upFT.Dst.IP(),
		upFT.Src.Port, upFT.Dst.Port,
		ctTimeout, 0)
	if create && upFT.IPProto == syscall.IPPROTO_TCP {
		flow.ProtoInfo.TCP = &conntrack.ProtoInfoTCP{
			State: 3, // ESTABLISHED
		}
	}

	return flow
}
