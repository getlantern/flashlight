// Functions in this file allows hole punching through UPNP
package upnp

import (
	"context"
	"net"

	"github.com/getlantern/golog"
	"github.com/huin/goupnp/dcps/internetgateway2"
	"golang.org/x/sync/errgroup"
)

var log = golog.LoggerFor("replica-pkg")

const defaultLeaseWindow = 120 // seconds

type RouterClient interface {
	AddPortMapping(
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
		NewInternalPort uint16,
		NewInternalClient string,
		NewEnabled bool,
		NewPortMappingDescription string,
		NewLeaseDuration uint32,
	) (err error)

	GetExternalIPAddress() (
		NewExternalIPAddress string,
		err error,
	)
}

// ForwardPortWithUpnp is the entrypoint for using upnp: provide a context and
// the port you'd need to punch throw.
//
// You'll need to call this function every 'defaultLeaseWindow' seconds, else
// the port forwarding will expire
func ForwardPortWithUpnp(ctx context.Context, port uint16) (atleastOneIpWorked bool, errs []error) {
	cl, err := pickRouterClient(ctx)
	if err != nil {
		return false, []error{log.Errorf("picking router client")}
	}
	log.Debugf("Got UPNP router client: %+v", cl)

	for _, proto := range []string{"ip4", "ip6"} {
		ips, err := getRoutableLocalIPs(proto)
		if err != nil {
			return false, []error{log.Errorf("getting routable local ips")}
		}
		log.Debugf("Got host's local IPs for proto [%v]: %v", proto, ips)

		for _, ip := range ips {
			if err := cl.AddPortMapping(
				"",                 // Ignore
				port,               // External port
				"TCP",              // Proto
				port,               // Internal port
				ip,                 // IP addr in the LAN to forward packets to
				true,               // Enabled.
				"lantern-p2p",      // Description
				defaultLeaseWindow, // Lease window. Expire this port map after X seconds
				// XXX <10-01-22, soltzen> Because of the lease window, we need to call
				// this function every X seconds to keep the UPNP port alive
			); err != nil {
				errs = append(errs, log.Errorf("adding port map to client: %v", err))
			}
			atleastOneIpWorked = true
		}
	}
	return atleastOneIpWorked, errs
}

func pickRouterClient(ctx context.Context) (RouterClient, error) {
	// We don't really need the 2nd return value here
	tasks, _ := errgroup.WithContext(ctx)
	var ip1Clients []*internetgateway2.WANIPConnection1
	tasks.Go(func() error {
		var err error
		ip1Clients, _, err = internetgateway2.NewWANIPConnection1Clients()
		return err
	})
	var ip2Clients []*internetgateway2.WANIPConnection2
	tasks.Go(func() error {
		var err error
		ip2Clients, _, err = internetgateway2.NewWANIPConnection2Clients()
		return err
	})
	var ppp1Clients []*internetgateway2.WANPPPConnection1
	tasks.Go(func() error {
		var err error
		ppp1Clients, _, err = internetgateway2.NewWANPPPConnection1Clients()
		return err
	})

	if err := tasks.Wait(); err != nil {
		return nil, err
	}

	// TODO <14-01-22, soltzen> Returns the first device. Is this a problem for
	// home setups?
	switch {
	case len(ip2Clients) == 1:
		return ip2Clients[0], nil
	case len(ip1Clients) == 1:
		return ip1Clients[0], nil
	case len(ppp1Clients) == 1:
		return ppp1Clients[0], nil
	default:
		return nil, log.Errorf("multiple or no services found")
	}
}

func getRoutableLocalIPs(proto string) (addrs []string, err error) {
	flags := net.FlagUp | net.FlagBroadcast
	switch proto {
	case "ip", "ip4", "ip6":
	default:
		return nil, log.Errorf("bad proto: %v", proto)
	}
	inets, err := net.Interfaces()
	if err != nil {
		return nil, log.Errorf("fetching interfaces: %v", err)
	}
	var targetInet *net.Interface
	for _, inet := range inets {
		if inet.Flags&flags != flags {
			continue
		}
		if !checkIfInterfaceHasRoutableIps(proto, &inet) {
			continue
		}
		targetInet = &inet
		break
	}
	if targetInet == nil {
		return nil, log.Errorf("no network interfaces found")
	}

	inetAddrs, err := targetInet.Addrs()
	if err != nil {
		return nil, log.Errorf("getting interface addresses")
	}
	for _, addr := range inetAddrs {
		ipNetAddr, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		switch proto {
		case "ip4":
			if ipNetAddr.IP.To4() != nil {
				addrs = append(addrs, ipNetAddr.IP.String())
			}
		case "ip6":
			if ipNetAddr.IP.To16() != nil && ipNetAddr.IP.To4() == nil {
				addrs = append(addrs, ipNetAddr.IP.String())
			}
		default:
			addrs = append(addrs, ipNetAddr.IP.String())
		}
	}
	if len(addrs) == 0 {
		return nil, log.Errorf("found no routable IPs")
	}
	return addrs, nil
}

func checkIfInterfaceHasRoutableIps(proto string, inet *net.Interface) bool {
	ifat, err := inet.Addrs()
	if err != nil {
		return false
	}
	for _, ifa := range ifat {
		switch ifa := ifa.(type) {
		case *net.IPAddr:
			if ip := makeRoutableIp(proto, ifa.IP); ip != nil {
				return true
			}
		case *net.IPNet:
			if ip := makeRoutableIp(proto, ifa.IP); ip != nil {
				return true
			}
		}
	}
	return false
}

func makeRoutableIp(proto string, ip net.IP) net.IP {
	if !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsGlobalUnicast() {
		return nil
	}
	switch proto {
	case "ip4":
		if ip := ip.To4(); ip != nil {
			return ip
		}
	case "ip6":
		if ip.IsLoopback() { // addressing scope of the loopback address depends on each implementation
			return nil
		}
		if ip := ip.To16(); ip != nil && ip.To4() == nil {
			return ip
		}
	default:
		if ip := ip.To4(); ip != nil {
			return ip
		}
		if ip := ip.To16(); ip != nil {
			return ip
		}
	}
	return nil
}
