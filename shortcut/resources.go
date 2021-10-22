package shortcut

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed resources/GeoLite2-Country-Locations-en.csv
var resources_geolite2_country_locations_en_csv []byte

//go:embed resources/ae_ipv4.txt
var resources_ae_ipv4_txt []byte

//go:embed resources/ae_ipv6.txt
var resources_ae_ipv6_txt []byte

//go:embed resources/cn_ipv4.txt
var resources_cn_ipv4_txt []byte

//go:embed resources/cn_ipv6.txt
var resources_cn_ipv6_txt []byte

//go:embed resources/default_ipv4.txt
var resources_default_ipv4_txt []byte

//go:embed resources/default_ipv4_ir.txt
var resources_default_ipv4_ir_txt []byte

//go:embed resources/default_ipv6.txt
var resources_default_ipv6_txt []byte

//go:embed resources/ir_ipv4.txt
var resources_ir_ipv4_txt []byte

//go:embed resources/ir_ipv6.txt
var resources_ir_ipv6_txt []byte

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if bytes, ok := _bindata[cannonicalName]; ok {
		return bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// _bindata is a table, holding each asset, mapped to its name.
var _bindata = map[string][]byte{
	"resources/GeoLite2-Country-Locations-en.csv": resources_geolite2_country_locations_en_csv,
	"resources/ae_ipv4.txt":                       resources_ae_ipv4_txt,
	"resources/ae_ipv6.txt":                       resources_ae_ipv6_txt,
	"resources/cn_ipv4.txt":                       resources_cn_ipv4_txt,
	"resources/cn_ipv6.txt":                       resources_cn_ipv6_txt,
	"resources/default_ipv4.txt":                  resources_default_ipv4_txt,
	"resources/default_ipv4_ir.txt":               resources_default_ipv4_ir_txt,
	"resources/default_ipv6.txt":                  resources_default_ipv6_txt,
	"resources/ir_ipv4.txt":                       resources_ir_ipv4_txt,
	"resources/ir_ipv6.txt":                       resources_ir_ipv6_txt,
}
