package embeddedconfig

import (
	"bytes"
	_ "embed"
	"text/template"

	replicaConfig "github.com/getlantern/replica/config"
	"github.com/getlantern/yaml"

	globalConfig "github.com/getlantern/flashlight/v7/config/global"
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

// The replica options root extracted from the executed global config template. Replica options
// don't require any template data to derive so this can be done at init. This is used by
// getlantern/replica.
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
