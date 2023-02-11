// Functions in this file allows hole punching through UPNP
package upnp

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/huin/goupnp/dcps/internetgateway2"
	"github.com/pkg/errors"
)

const defaultLeaseWindow = 24 * 60 * 60 // 24 hours

type routerClient interface {
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

type Client struct {
	lastWorkingRouterClients []routerClient
}

func New() *Client { return &Client{} }

// ForwardPortWithUpnp is the entrypoint for using this package: provide a
// context and the port you'd need to punch through.
//
// You'll need to call this function every 'defaultLeaseWindow' seconds, else
// the port forwarding will expire.
//
// layer4Proto can be either "TCP" or "UDP" values.
//
// Returns:
// - if at least **one** portforwarding attempt succeeded on
//   **any** routeable IP on **any** RouterClient, return nil.
// - Else, return **all** the errors of for all portforwarding attempts.
//
// TODO <23-02-22, soltzen> Would be nice to have a context here, though that's
// not easy to add since RouterClients don't include it
func (ucl *Client) ForwardPortWithUpnp(port uint16, layer4Proto string) error {
	// layer 4 protocols must be capitalized in this function
	layer4Proto = strings.ToUpper(layer4Proto)
	if layer4Proto != "TCP" && layer4Proto != "UDP" {
		return errors.Errorf(
			"Bad layer4Proto: %s. Only TCP and UDP are accepted", layer4Proto)
	}

	// Fetch all routable IPs. If we can't, we can't proceed
	ips, err := fetchAllRoutableLocalIps()
	if len(ips) == 0 {
		return errors.Wrapf(err, "failed to fetch any routable ip: %v", err)
	}

	// Try to use the last working RouterClients we used.
	// Since picking a RouterClient takes about 2 seconds, it's best to
	// remember and reuse old clients first:
	// https://github.com/getlantern/flashlight/pull/1183#discussion_r789162172
	successfulClients, failedClients, err := portforwardWithAllClients(
		ucl.lastWorkingRouterClients, ips, port, layer4Proto)
	if len(successfulClients) != 0 {
		Log.Infof("Portforward with a previously-working RouterClient worked")
		// Remove the ones that didn't work
		removeRouterClientsFromSlice(ucl.lastWorkingRouterClients, failedClients)
		// Ignore any errors we find, if at least one successful portforwarding
		// attempt worked
		return nil
	}
	if err != nil {
		Log.Infof(
			"failed to portforward with RouterClients that previously worked %+v: %v. Will fetch new clients",
			ucl.lastWorkingRouterClients, err)
	}
	// If we reach here, nullify the list of previously-working clients since
	// none of them worked
	ucl.lastWorkingRouterClients = nil

	// Attempt to fetch new RouterClients
	cls, err := fetchAllRouterClients()
	if len(cls) == 0 {
		return errors.Wrapf(err, "failed to fetch any router clients: %v", err)
	}
	Log.Infof("Got UPNP router clients: %+v", cls)

	// Attempt to portforward through new clients
	successfulClients, failedClients,
		err = portforwardWithAllClients(cls, ips, port, layer4Proto)
	if len(successfulClients) != 0 {
		Log.Infof("Portforward with a new RouterClient worked")
		// Add the ones that worked
		ucl.lastWorkingRouterClients = successfulClients
		// Ignore any errors we find, if at least one successful portforwarding
		// attempt worked
		return nil
	}
	// If we reach here, nothing worked and we failed to portforward
	return errors.Wrapf(err,
		"failed to portforward with any RouterClient through any routable ip [%+v]: %v",
		ips, err)
}

func fetchAllRoutableLocalIps() ([]string, error) {
	var allIps []string
	var errs []error
	for _, layer3Proto := range []string{"ip4", "ip6"} {
		ips, err := getRoutableLocalIPs(layer3Proto)
		if err != nil {
			errs = append(errs, err)
		} else {
			allIps = append(allIps, ips...)
		}
	}
	if len(errs) != 0 {
		return allIps, fmt.Errorf("%+v", errs)
	}
	return allIps, nil
}

// Loop over all RouterClients we have and try to portforward using the
// routable ips we fetched.
func portforwardWithAllClients(
	cls []routerClient,
	ips []string,
	port uint16,
	layer4Proto string,
) (successfulClients []routerClient,
	failedClients []routerClient,
	err error) {
	errs := []error{}
	for _, cl := range cls {
		ok, err := portforwardWithClient(cl, ips, port, layer4Proto)
		if ok {
			successfulClients = append(successfulClients, cl)
			// If we had at least one successful portforward with one IP, we've
			// succeeded and can safely ignore the other errors from this
			// client
		} else {
			// We failed, count all the errors we got from this client
			failedClients = append(failedClients, cl)
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		return successfulClients, failedClients, errors.Wrapf(err, "%+v", errs)
	}
	return successfulClients, failedClients, nil
}

func portforwardWithClient(
	cl routerClient,
	ips []string,
	port uint16,
	layer4Proto string,
) (didSucceedOnce bool, err error) {
	errs := []error{}
	for _, ip := range ips {
		err := cl.AddPortMapping(
			"",                 // Ignore
			port,               // External port
			layer4Proto,        // Proto
			port,               // Internal port
			ip,                 // IP addr in the LAN to forward packets to
			true,               // Enabled.
			"lantern-p2p",      // Description
			defaultLeaseWindow, // Lease window. Expire this port map after X seconds
		)
		if err != nil {
			errs = append(errs, fmt.Errorf(
				"Error while adding forwarding port %s:%d to client [%+v] with IP [%v]: %v. Using the next available IP",
				layer4Proto, port, cl, ip, err))
		} else {
			didSucceedOnce = true
			Log.Infof(
				"Successfully forwarded port %s:%d to client [%+v]",
				layer4Proto, port, cl)
		}
	}
	if len(errs) != 0 {
		return didSucceedOnce, fmt.Errorf("%+v", errs)
	}
	return didSucceedOnce, nil
}

func fetchAllRouterClients() ([]routerClient, error) {
	var wg sync.WaitGroup
	routerClientChan := make(chan routerClient)
	errChan := make(chan error)

	// TODO <23-02-22, soltzen> Generics would be very nice here.
	//
	// Enumerate all possible RouterClients asynchronously.
	// If we find something, send it over routerClientChan.
	// If we get an error, send it over errChan.
	wg.Add(1)
	go func() {
		cls, _, err := internetgateway2.NewWANIPConnection1Clients()
		if err != nil {
			errChan <- err
		} else {
			for _, cl := range cls {
				routerClientChan <- cl
			}
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		cls, _, err := internetgateway2.NewWANIPConnection2Clients()
		if err != nil {
			errChan <- err
		} else {
			for _, cl := range cls {
				routerClientChan <- cl
			}
		}
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		cls, _, err := internetgateway2.NewWANPPPConnection1Clients()
		if err != nil {
			errChan <- err
		} else {
			for _, cl := range cls {
				routerClientChan <- cl
			}
		}
		wg.Done()
	}()
	// Wait for all enumerations to conclude, then close the channels, so we
	// can exit the range-loops over the channels below
	go func() {
		wg.Wait()
		close(routerClientChan)
		close(errChan)
	}()

	// Drain all channels
	var cls []routerClient
	var errs []error
	for cl := range routerClientChan {
		cls = append(cls, cl)
	}
	for err := range errChan {
		errs = append(errs, err)
	}

	// If we find any errors, concatenate them to one error
	if len(errs) != 0 {
		return cls, fmt.Errorf("%+v", errs)
	} else {
		return cls, nil
	}
}

func getRoutableLocalIPs(proto string) (addrs []string, err error) {
	flags := net.FlagUp | net.FlagBroadcast
	switch proto {
	case "ip", "ip4", "ip6":
	default:
		return nil, errors.Errorf("bad proto: %v", proto)
	}
	inets, err := net.Interfaces()
	if err != nil {
		return nil, errors.Wrapf(err, "fetching interfaces: %v", err)
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
		return nil, errors.New("no network interfaces found")
	}

	inetAddrs, err := targetInet.Addrs()
	if err != nil {
		return nil, errors.New("getting interface addresses")
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
		return nil, errors.New("found no routable IPs")
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
			if isRoutableIp(proto, ifa.IP) {
				return true
			}
		case *net.IPNet:
			if isRoutableIp(proto, ifa.IP) {
				return true
			}
		}
	}
	return false
}

func isRoutableIp(proto string, ip net.IP) bool {
	if !ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsGlobalUnicast() {
		return false
	}
	switch proto {
	case "ip4":
		if ip := ip.To4(); ip != nil {
			return true
		}
	case "ip6":
		if ip.IsLoopback() {
			return false
		}
		if ip := ip.To16(); ip != nil && ip.To4() == nil {
			return true
		}
	default:
		if ip := ip.To4(); ip != nil {
			return true
		}
		if ip := ip.To16(); ip != nil {
			return true
		}
	}
	return false
}

// removeRouterClientsFromSlice remove all elements in 'sliceToRemove' from
// 'baseSlice'
func removeRouterClientsFromSlice(
	baseSlice []routerClient,
	sliceToRemove []routerClient) {
	for _, sliceToRemoveCl := range sliceToRemove {
		for idx, baseSliceCl := range baseSlice {
			if sliceToRemoveCl != baseSliceCl {
				continue
			}
			// Slice trick to remove indexed elements fast:
			// - Set the index of the target element to be equal to the last
			//   element
			baseSlice[idx] = baseSlice[len(baseSlice)-1]
			// - Make the slice forget the last element, which is now the
			//   target element
			baseSlice = baseSlice[:len(baseSlice)-1]
		}
	}
}
