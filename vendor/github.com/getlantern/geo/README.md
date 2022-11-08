[![Go Actions Status](https://github.com/getlantern/geo/actions/workflows/go.yml/badge.svg)](https://github.com/getlantern/geo/actions)
# geo
Looks up geoinfo from MaxMind GeoLite2 Database fetched from web. Currently only country code is supported but later can be extend to city, ISP, etc.

Example:
```
  // Keep in sync with the database from geolite2_url every day, without
  // keeping a local copy in disk.
  lookup = geo.New(geolite2_url, 24*time.Hour, "")
  // or local copy so it's available immediately when the service restarts.
  lookup = geo.New(geolite2_url, 24*time.Hour, "local-file-path")
  country = lookup.CountryCode(net.ParseIP("1.1.1.1"))
```

