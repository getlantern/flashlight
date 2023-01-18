// Largely inspired by https://unix.stackexchange.com/a/344997/5587. Note that we also use http
// because VPNs might block specialized DNS queries.
package publicip

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// This function is reused by dialers that want constrain the network with a suffix.
func dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	d := net.Dialer{
		// I think we want to disable switching to other networks, since this DNS response
		// depends on the network used.
		FallbackDelay: -1,
	}
	// Go DNS resolution doesn't bother to use a particular network, but we need to reapply our
	// constraint.
	suffix := ctx.Value(dialNetworkSuffixVar)
	if suffix != nil {
		network += suffix.(string)
	}
	return d.DialContext(ctx, network, address)
}

var openDnsResolver = &net.Resolver{
	// This ensures our Dial function will be used.
	PreferGo: true,
	Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		const resolverAddress = "resolver1.opendns.com:53"
		//log.Printf("replacing %q with %q for dns lookup", address, resolverAddress)
		return dialContext(ctx, network, resolverAddress)
	},
}

var dialNetworkSuffixVar = &struct{}{}

func withDialNetworkSuffix(ctx context.Context, suffix string) context.Context {
	return context.WithValue(ctx, dialNetworkSuffixVar, suffix)
}

func GetAll(ctx context.Context) ([]net.IPAddr, error) {
	res, errs := race(
		ctx,
		func(ctx context.Context) (interface{}, error) {
			// We know this passes "ip" to the LookupIP, and includes Zones in the return.
			return openDnsResolver.LookupIPAddr(ctx, "myip.opendns.com")
		},
		func(ctx context.Context) (interface{}, error) {
			ip, err := fromHttp(ctx)
			return []net.IPAddr{{IP: ip}}, err
		},
	)
	if len(errs) > 0 {
		return nil, fmt.Errorf("racing lookup: %v", errs)
	}
	return res.([]net.IPAddr), nil
}

// Network should be one of "ip", "ip4", or "ip6".
func Get(ctx context.Context, network string) ([]net.IP, error) {
	res, errs := race(
		ctx,
		func(ctx context.Context) (interface{}, error) {
			return openDnsResolver.LookupIP(ctx, network, "myip.opendns.com")
		},
		func(ctx context.Context) (interface{}, error) {
			ip, err := fromHttp(ctx)
			return []net.IP{ip}, err
		},
	)
	if len(errs) > 0 {
		return nil, fmt.Errorf("racing lookup:%s", strings.Join(func() (ret []string) {
			ret = append(ret, "")
			for _, e := range errs {
				ret = append(ret, e.Error())
			}
			return
		}(), "\n\t"))
	}
	return res.([]net.IP), nil
}

func Get4(ctx context.Context) (net.IP, error) {
	all, err := Get(withDialNetworkSuffix(ctx, "4"), "ip4")
	if err != nil {
		return nil, err
	}
	return all[0], nil
}

func Get6(ctx context.Context) (net.IP, error) {
	all, err := Get(withDialNetworkSuffix(ctx, "6"), "ip6")
	if err != nil {
		return nil, err
	}
	return all[0], nil
}
