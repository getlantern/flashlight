package embeddedconfig

import _ "embed"

//go:embed global.yaml.tmpl
var GlobalTemplate string

//go:generate ./download_global.sh
//go:embed global.yaml
var Global []byte

var Proxies []byte
