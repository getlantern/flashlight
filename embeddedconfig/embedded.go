package embeddedconfig

import (
	"bytes"
	_ "embed"
	"text/template"

	"github.com/getlantern/yaml"
)

//go:embed global.yaml.tmpl
var GlobalTemplate string

//go:generate ./download_global.sh
//go:embed global.yaml
var Global []byte

var Proxies []byte

func ExecuteAndUnmarshalGlobal(data, g any) (err error) {
	var w bytes.Buffer
	err = template.Must(template.New("").Parse(GlobalTemplate)).Execute(&w, data)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(w.Bytes(), g)
	return
}
