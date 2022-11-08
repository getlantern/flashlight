package isp

import (
	"github.com/hashicorp/golang-lru"
)

func withCaching(prov Provider, cacheSize int) Provider {
	c1, _ := lru.New(cacheSize)
	c2, _ := lru.New(cacheSize)
	c3, _ := lru.New(cacheSize)
	c4, _ := lru.New(cacheSize)

	caching := &cachingProvider{
		Provider:    prov,
		ispCache:    c1,
		orgCache:    c2,
		asnCache:    c3,
		asnameCache: c4,
	}

	return caching
}

type cachingProvider struct {
	Provider    Provider
	ispCache    *lru.Cache
	orgCache    *lru.Cache
	asnCache    *lru.Cache
	asnameCache *lru.Cache
}

func (c *cachingProvider) ISP(ip string) (isp string, found bool) {
	_isp, _found := c.ispCache.Get(ip)
	if !_found {
		_isp, _found = c.Provider.ISP(ip)
		c.ispCache.Add(ip, _isp)
	}
	found = _isp != ""
	if found {
		isp = _isp.(string)
	}
	return
}

func (c *cachingProvider) ORG(ip string) (org string, found bool) {
	_org, _found := c.orgCache.Get(ip)
	if !_found {
		_org, _found = c.Provider.ORG(ip)
		c.orgCache.Add(ip, _org)
	}
	found = _org != ""
	if found {
		org = _org.(string)
	}
	return
}

// ASN looks up the Autonomous System Number corresponding to the given ip.
func (c *cachingProvider) ASN(ip string) (asn int, found bool) {
	_asn, _found := c.asnCache.Get(ip)
	if !_found {
		_asn, _found = c.Provider.ASN(ip)
		c.asnCache.Add(ip, _asn)
	}
	found = _asn != 0
	if found {
		asn = _asn.(int)
	}
	return
}

func (c *cachingProvider) ASName(ip string) (asnName string, found bool) {
	_asnName, _found := c.asnameCache.Get(ip)
	if !_found {
		_asnName, _found = c.Provider.ASName(ip)
		c.asnameCache.Add(ip, _asnName)
	}
	found = _asnName != ""
	if found {
		asnName = _asnName.(string)
	}
	return
}
