// Package geo provides geolocation functions.
package geo

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/getlantern/goexpr"
	"github.com/getlantern/golog"
	"github.com/getlantern/msgpack"
	"github.com/hashicorp/golang-lru"
	geoip2 "github.com/oschwald/geoip2-golang"
)

const (
	// DefaultCacheSize determines the default size for the ip cache
	DefaultCacheSize = 100000

	dbURL = "https://s3.amazonaws.com/lantern/GeoLite2-City.mmdb.gz"
)

var (
	log = golog.LoggerFor("goexpr.geo")

	db               *geoip2.Reader
	cityCache        *lru.Cache
	regionCache      *lru.Cache
	regionCityCache  *lru.Cache
	countryCodeCache *lru.Cache
	dbMutex          sync.RWMutex
)

func init() {
	msgpack.RegisterExt(101, &city{})
	msgpack.RegisterExt(102, &region{})
	msgpack.RegisterExt(103, &regionCity{})
	msgpack.RegisterExt(104, &countryCode{})
}

// Init initializes the Geolocation subsystem, storing the database file at the
// given dbFile location. It will periodically fetch updates from the maxmind
// website.
func Init(dbFile string, cacheSize int) error {
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
		log.Debugf("Defaulted ip cache size to %v", cacheSize)
	}
	_db, dbDate, err := readDbFromFile(dbFile)
	if err != nil {
		_db, dbDate, err = readDbFromWeb(dbFile)
		if err != nil {
			return fmt.Errorf("Unable to read DB from file or web: %v", err)
		}
	}
	dbMutex.Lock()
	cityCache, _ = lru.New(cacheSize)
	regionCache, _ = lru.New(cacheSize)
	regionCityCache, _ = lru.New(cacheSize)
	countryCodeCache, _ = lru.New(cacheSize)
	db = _db
	dbMutex.Unlock()
	go keepDbCurrent(dbFile, dbDate)
	return nil
}

// CITY returns the city name for the IP, e.g. "Austin"
func CITY(ip goexpr.Expr) goexpr.Expr {
	return &city{ip}
}

type city struct {
	IP goexpr.Expr
}

func (e *city) Eval(params goexpr.Params) interface{} {
	_ip := e.IP.Eval(params)
	switch ip := _ip.(type) {
	case string:
		cityName, found := cityCache.Get(ip)
		if found {
			return cityName
		}
		city := cityFor(ip)
		if city == nil {
			return nil
		}
		return city.City.Names["en"]
	}
	return nil
}

func (e *city) WalkParams(cb func(string)) {
	e.IP.WalkParams(cb)
}

func (e *city) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *city) WalkLists(cb func(goexpr.List)) {
	e.IP.WalkLists(cb)
}

func (e *city) String() string {
	return fmt.Sprintf("CITY(%v)", e.IP)
}

// REGION returns the region name for the IP, e.g. "Texas"
func REGION(ip goexpr.Expr) goexpr.Expr {
	return &region{ip}
}

type region struct {
	IP goexpr.Expr
}

func (e *region) Eval(params goexpr.Params) interface{} {
	_ip := e.IP.Eval(params)
	switch ip := _ip.(type) {
	case string:
		region, found := regionCache.Get(ip)
		if found {
			return region
		}
		city := cityFor(ip)
		if city == nil {
			return nil
		}
		if len(city.Subdivisions) == 0 {
			return nil
		}
		return city.Subdivisions[0].Names["en"]
	}
	return nil
}

func (e *region) WalkParams(cb func(string)) {
	e.IP.WalkParams(cb)
}

func (e *region) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *region) WalkLists(cb func(goexpr.List)) {
	e.IP.WalkLists(cb)
}

func (e *region) String() string {
	return fmt.Sprintf("REGION(%v)", e.IP)
}

// REGION_CITY returns the region and city name for the IP, e.g. "Texas, Austin"
func REGION_CITY(ip goexpr.Expr) goexpr.Expr {
	return &regionCity{ip}
}

type regionCity struct {
	IP goexpr.Expr
}

func (e *regionCity) Eval(params goexpr.Params) interface{} {
	_ip := e.IP.Eval(params)
	switch ip := _ip.(type) {
	case string:
		regionCity, found := regionCityCache.Get(ip)
		if found {
			return regionCity
		}
		city := cityFor(ip)
		if city == nil {
			return nil
		}
		cityName := city.City.Names["en"]
		if len(city.Subdivisions) > 0 {
			return city.Subdivisions[0].Names["en"] + ", " + cityName
		}
		return cityName
	}
	return nil
}

func (e *regionCity) WalkParams(cb func(string)) {
	e.IP.WalkParams(cb)
}

func (e *regionCity) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *regionCity) WalkLists(cb func(goexpr.List)) {
	e.WalkLists(cb)
}

func (e *regionCity) String() string {
	return fmt.Sprintf("REGION_CITY(%v)", e.IP)
}

// COUNTRY_CODE returns the 2 digit ISO country code for the ip, e.g. "US"
func COUNTRY_CODE(ip goexpr.Expr) goexpr.Expr {
	return &countryCode{ip}
}

type countryCode struct {
	IP goexpr.Expr
}

func (e *countryCode) Eval(params goexpr.Params) interface{} {
	_ip := e.IP.Eval(params)
	switch ip := _ip.(type) {
	case string:
		countryCode, found := countryCodeCache.Get(ip)
		if found {
			return countryCode
		}
		city := cityFor(ip)
		if city == nil {
			return nil
		}
		return city.Country.IsoCode
	}
	return nil
}

func (e *countryCode) WalkParams(cb func(string)) {
	e.IP.WalkParams(cb)
}

func (e *countryCode) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *countryCode) WalkLists(cb func(goexpr.List)) {
	e.IP.WalkLists(cb)
}

func (e *countryCode) String() string {
	return fmt.Sprintf("COUNTRY_CODE(%v)", e.IP)
}

func cityFor(ip string) *geoip2.City {
	d := getDB()
	parsedIP := net.ParseIP(ip)
	city, err := d.City(parsedIP)
	if err != nil {
		return nil
	}
	return city
}

func getDB() *geoip2.Reader {
	dbMutex.RLock()
	_db := db
	dbMutex.RUnlock()
	return _db
}

// readDbFromFile reads the MaxMind database and timestamp from a file
func readDbFromFile(dbFile string) (*geoip2.Reader, time.Time, error) {
	dbData, err := ioutil.ReadFile(dbFile)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("Unable to read db file %s: %s", dbFile, err)
	}
	fileInfo, err := os.Stat(dbFile)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("Unable to stat db file %s: %s", dbFile, err)
	}
	dbDate := fileInfo.ModTime()
	_db, err := openDb(dbData)
	if err != nil {
		return nil, time.Time{}, err
	}
	return _db, dbDate, nil
}

// keepDbCurrent checks for new versions of the database on the web every minute
// by issuing a HEAD request.  If a new database is found, this downloads it and
// submits it to server.dbUpdate for the run() routine to pick up.
func keepDbCurrent(dbFile string, dbDate time.Time) {
	for {
		time.Sleep(1 * time.Minute)
		headResp, err := http.Head(dbURL)
		if err != nil {
			log.Errorf("Unable to request modified of %s: %s", dbURL, err)
			continue
		}
		lm, err := lastModified(headResp)
		if err != nil {
			log.Errorf("Unable to parse modified date for %s: %s", dbURL, err)
			continue
		}
		if lm.After(dbDate) {
			log.Debug("Updating database from web")
			_db, _dbDate, err := readDbFromWeb(dbFile)
			if err != nil {
				log.Errorf("Unable to update database from web: %s", err)
				continue
			}
			dbDate = _dbDate
			dbMutex.Lock()
			db = _db
			cityCache.Purge()
			regionCache.Purge()
			regionCityCache.Purge()
			countryCodeCache.Purge()
			dbMutex.Unlock()
		}
	}
}

// readDbFromWeb reads the MaxMind database and timestamp from the web
func readDbFromWeb(dbFile string) (*geoip2.Reader, time.Time, error) {
	dbResp, err := http.Get(dbURL)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("Unable to get database from %v: %v", dbURL, err)
	}
	gzipDbData, err := gzip.NewReader(dbResp.Body)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("Unable to open gzip reader on response body: %v", err)
	}
	defer gzipDbData.Close()
	dbData, err := ioutil.ReadAll(gzipDbData)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("Unable to fetch database from HTTP response: %v", err)
	}
	dbDate, err := lastModified(dbResp)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("Unable to parse Last-Modified header %v: %v", dbDate, err)
	}
	err = ioutil.WriteFile(dbFile, dbData, 0644)
	if err != nil {
		log.Errorf("Unable to save dbfile: %v", err)
	}
	db, err := openDb(dbData)
	if err != nil {
		return nil, time.Time{}, err
	}
	return db, dbDate, nil
}

// lastModified parses the Last-Modified header from a response
func lastModified(resp *http.Response) (time.Time, error) {
	lastModified := resp.Header.Get("Last-Modified")
	return http.ParseTime(lastModified)
}

// openDb opens a MaxMind in-memory db using the geoip2.Reader
func openDb(dbData []byte) (*geoip2.Reader, error) {
	db, err := geoip2.FromBytes(dbData)
	if err != nil {
		return nil, fmt.Errorf("Unable to open database: %s", err)
	}
	return db, nil
}
