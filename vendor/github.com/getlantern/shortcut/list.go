package shortcut

import (
	"bytes"
	"net"
	"sort"

	"github.com/getlantern/golog"
)

var (
	log = golog.LoggerFor("shortcut")
)

// sort IPNet in asc order, smaller first. If two networks overlap, the one
// with larger IP space goes first, to match as many IPs as possible.
type sorter []*net.IPNet

func (s sorter) Len() int      { return len(s) }
func (s sorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s sorter) Less(i, j int) bool {
	r := bytes.Compare(s[i].IP, s[j].IP)
	switch r {
	case -1:
		return true
	case 1:
		return false
	default:
		return bytes.Compare(s[i].Mask, s[j].Mask) > 0
	}
}

type SortList struct {
	sorted sorter
}

// NewSortList creates a shortcut list from a list of CIDR subnets in
// "a.b.c.d/24" or "2001:db8::/32" format. Each subnet string in one list
// should be in the same format, i.e., IPv4 only or IPv6 only, but not mixed.
func NewSortList(subnets []string) *SortList {
	nets := make([]*net.IPNet, 0, len(subnets))
	for _, s := range subnets {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			log.Debugf("Skip adding %s: %v", s, err)
			continue
		}

		nets = append(nets, n)
	}
	sort.Sort(sorter(nets))
	return &SortList{nets}
}

// Contains checks if the ip belongs to one of the subnets in the list.
// Note that the byte length of ip should match the format of the subnets,
// i.e., call To4() before checking against an IPv4 list, and To16() for an
// IPv6 list.
func (l *SortList) Contains(ip net.IP) bool {
	index := sort.Search(len(l.sorted), func(i int) bool {
		res := bytes.Compare(ip, l.sorted[i].IP)
		return res < 0
	})
	// find the smallest network address that is larger than the IP. The one before it would be fit.
	index--
	return index >= 0 && index < len(l.sorted) && l.sorted[index].Contains(ip)
}
