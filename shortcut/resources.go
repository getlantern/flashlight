package shortcut

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"
)

func bindata_read(data, name string) ([]byte, error) {
	var empty [0]byte
	sx := (*reflect.StringHeader)(unsafe.Pointer(&data))
	b := empty[:]
	bx := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	bx.Data = sx.Data
	bx.Len = len(data)
	bx.Cap = bx.Len
	return b, nil
}

func resources_geolite2_country_locations_en_csv() ([]byte, error) {
	return bindata_read(
		_resources_geolite2_country_locations_en_csv,
		"resources/GeoLite2-Country-Locations-en.csv",
	)
}


func resources_ae_ipv4_txt() ([]byte, error) {
	return bindata_read(
		_resources_ae_ipv4_txt,
		"resources/ae_ipv4.txt",
	)
}


func resources_ae_ipv6_txt() ([]byte, error) {
	return bindata_read(
		_resources_ae_ipv6_txt,
		"resources/ae_ipv6.txt",
	)
}


func resources_cn_ipv4_txt() ([]byte, error) {
	return bindata_read(
		_resources_cn_ipv4_txt,
		"resources/cn_ipv4.txt",
	)
}


func resources_cn_ipv6_txt() ([]byte, error) {
	return bindata_read(
		_resources_cn_ipv6_txt,
		"resources/cn_ipv6.txt",
	)
}


func resources_default_ipv4_txt() ([]byte, error) {
	return bindata_read(
		_resources_default_ipv4_txt,
		"resources/default_ipv4.txt",
	)
}


func resources_default_ipv4_ir_txt() ([]byte, error) {
	return bindata_read(
		_resources_default_ipv4_ir_txt,
		"resources/default_ipv4_ir.txt",
	)
}


func resources_default_ipv6_txt() ([]byte, error) {
	return bindata_read(
		_resources_default_ipv6_txt,
		"resources/default_ipv6.txt",
	)
}


func resources_ir_ipv4_txt() ([]byte, error) {
	return bindata_read(
		_resources_ir_ipv4_txt,
		"resources/ir_ipv4.txt",
	)
}


func resources_ir_ipv6_txt() ([]byte, error) {
	return bindata_read(
		_resources_ir_ipv6_txt,
		"resources/ir_ipv6.txt",
	)
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		return f()
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() ([]byte, error){
	"resources/GeoLite2-Country-Locations-en.csv": resources_geolite2_country_locations_en_csv,
	"resources/ae_ipv4.txt": resources_ae_ipv4_txt,
	"resources/ae_ipv6.txt": resources_ae_ipv6_txt,
	"resources/cn_ipv4.txt": resources_cn_ipv4_txt,
	"resources/cn_ipv6.txt": resources_cn_ipv6_txt,
	"resources/default_ipv4.txt": resources_default_ipv4_txt,
	"resources/default_ipv4_ir.txt": resources_default_ipv4_ir_txt,
	"resources/default_ipv6.txt": resources_default_ipv6_txt,
	"resources/ir_ipv4.txt": resources_ir_ipv4_txt,
	"resources/ir_ipv6.txt": resources_ir_ipv6_txt,
}
// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for name := range node.Children {
		rv = append(rv, name)
	}
	return rv, nil
}

type _bintree_t struct {
	Func func() ([]byte, error)
	Children map[string]*_bintree_t
}
var _bintree = &_bintree_t{nil, map[string]*_bintree_t{
	"resources": &_bintree_t{nil, map[string]*_bintree_t{
		"GeoLite2-Country-Locations-en.csv": &_bintree_t{resources_geolite2_country_locations_en_csv, map[string]*_bintree_t{
		}},
		"ae_ipv4.txt": &_bintree_t{resources_ae_ipv4_txt, map[string]*_bintree_t{
		}},
		"ae_ipv6.txt": &_bintree_t{resources_ae_ipv6_txt, map[string]*_bintree_t{
		}},
		"cn_ipv4.txt": &_bintree_t{resources_cn_ipv4_txt, map[string]*_bintree_t{
		}},
		"cn_ipv6.txt": &_bintree_t{resources_cn_ipv6_txt, map[string]*_bintree_t{
		}},
		"default_ipv4.txt": &_bintree_t{resources_default_ipv4_txt, map[string]*_bintree_t{
		}},
		"default_ipv4_ir.txt": &_bintree_t{resources_default_ipv4_ir_txt, map[string]*_bintree_t{
		}},
		"default_ipv6.txt": &_bintree_t{resources_default_ipv6_txt, map[string]*_bintree_t{
		}},
		"ir_ipv4.txt": &_bintree_t{resources_ir_ipv4_txt, map[string]*_bintree_t{
		}},
		"ir_ipv6.txt": &_bintree_t{resources_ir_ipv6_txt, map[string]*_bintree_t{
		}},
	}},
}}
