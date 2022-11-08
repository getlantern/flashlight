// Package geo provides functionality for looking up country code and ISP name
// of the given IP address.
package geo

import (
	"io/ioutil"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/golog"
	"github.com/getlantern/keepcurrent"
	geoip2 "github.com/oschwald/geoip2-golang"
)

var (
	log = golog.LoggerFor("geo")
)

type Lookup interface {
	CountryLookup
	ISPLookup
	// Ready returns a channel which is closed when the lookup is ready to
	// serve requests.
	Ready() <-chan struct{}
}

// CountryLookup allows looking up the country for an IP address
type CountryLookup interface {
	// CountryCode looks up the 2 digit ISO 3166 country code in upper case for
	// the given IP address and returns "" if there was an error.
	CountryCode(ip net.IP) string
}

// ISPLookup allows looking up the ISP for an IP address
type ISPLookup interface {
	// ISP looks up the ISP name for the given IP address and returns "" if there was an error.
	ISP(ip net.IP) string
}

// NoLookup is a Lookup implementation which always return empty result.
type NoLookup struct{}

func (l NoLookup) CountryCode(ip net.IP) string { return "" }
func (l NoLookup) ISP(ip net.IP) string         { return "" }
func (l NoLookup) Ready() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

type lookup struct {
	runner    *keepcurrent.Runner
	db        atomic.Value
	ready     chan struct{}
	readyOnce sync.Once
}

// New constructs a new Lookup from the MaxMind GeoLite2 Country database
// fetched from the given URL and keeps in sync with it every syncInterval. If filePath
// is not empty, it saves the database file to filePath and uses the file if
// available.
func New(dbURL string, syncInterval time.Duration, filePath string) *lookup {
	return FromWeb(dbURL, "GeoLite2-Country.mmdb", syncInterval, filePath)
}

// FromWeb is same as New but allows downloading a different MaxMind database
func FromWeb(dbURL string, nameInTarball string, syncInterval time.Duration, filePath string) *lookup {
	source := keepcurrent.FromTarGz(keepcurrent.FromWeb(dbURL), nameInTarball)
	chDB := make(chan []byte)
	dest := keepcurrent.ToChannel(chDB)
	var runner *keepcurrent.Runner
	if filePath != "" {
		runner = keepcurrent.New(source, keepcurrent.ToFile(filePath), dest)
	} else {
		runner = keepcurrent.New(source, dest)
	}

	v := &lookup{runner: runner, ready: make(chan struct{})}
	go func() {
		for data := range chDB {
			db, err := geoip2.FromBytes(data)
			if err != nil {
				log.Errorf("Error loading geo database: %v", err)
			} else {
				v.db.Store(db)
				v.readyOnce.Do(func() { close(v.ready) })
			}
		}
	}()
	if filePath != "" {
		runner.InitFrom(keepcurrent.FromFile(filePath))
	}

	runner.OnSourceError = keepcurrent.ExpBackoffThenFail(time.Minute, 5, func(err error) {
		log.Errorf("Error fetching geo database: %v", err)
	})
	runner.Start(syncInterval)
	return v
}

// FromFile uses the local database file for lookup
func FromFile(filePath string) (*lookup, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}
	db, err := geoip2.FromBytes(data)
	if err != nil {
		return nil, err
	}
	v := &lookup{}
	v.db.Store(db)

	return v, nil
}

func (l *lookup) Ready() <-chan struct{} {
	return l.ready
}

func (l *lookup) CountryCode(ip net.IP) string {
	if db := l.db.Load(); db != nil {
		geoData, err := db.(*geoip2.Reader).Country(ip)
		if err != nil {
			log.Debugf("Unable to look up ip address %s: %s", ip, err)
			return ""
		}
		return geoData.Country.IsoCode
	}
	return ""
}

func (l *lookup) ISP(ip net.IP) string {
	if db := l.db.Load(); db != nil {
		geoData, err := db.(*geoip2.Reader).ISP(ip)
		if err != nil {
			log.Debugf("Unable to look up ip address %s: %s", ip, err)
			return ""
		}
		return geoData.ISP
	}
	return ""
}
