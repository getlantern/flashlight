package embeddedconfig

import (
	"bytes"
	_ "embed"
	globalConfig "github.com/getlantern/flashlight/config/global"
	replicaConfig "github.com/getlantern/replica/config"
	"github.com/getlantern/yaml"
	"text/template"
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

// The replica options root extracted from the executed global config template. Replica options don't require any template data to derive so this can be done at init.
var GlobalReplicaOptions = func() (fos replicaConfig.ReplicaOptionsRoot) {
	var g globalConfig.Raw
	err := ExecuteAndUnmarshalGlobal(nil, &g)
	if err != nil {
		panic(err)
	}
	err = globalConfig.UnmarshalFeatureOptions(g, globalConfig.FeatureReplica, &fos)
	if err != nil {
		panic(err)
	}
	return
}()
