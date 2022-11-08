package dhtup

// List computed from https://gist.github.com/anacrolix/02f7d132866e78101a05bf267d3c7978.
//
// reasons for:
//   http: incase there are certificate/proxy issues, proxyable and for tcp.
//   https: security and tunneling, proxyable, and for tcp
//   ipv4: users that can't operate over ipv6
//   ipv6: fly.io limitation workaround
//   tcp: fly.io limitation workaround
//
// I'm not aware of a Russian HTTPS tracker. This list should be included in generated metainfo
// files, see the Makefile.
var DefaultTrackers = []string{
	// russia ipv4 ipv6
	"udp://opentor.org:2710/announce",
	// russia ipv6 tcp
	"http://tracker4.itzmx.com:2710/announce",
	// best tracker ipv4 only
	"udp://tracker.opentrackr.org:1337/announce",
	// ipv4 ipv6 tcp https
	"https://tracker.nanoha.org:443/announce",
	// france singapore ipv6 http tcp
	"http://t.nyaatracker.com:80/announce",
}
