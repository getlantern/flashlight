package domainrouting_test

import (
	"fmt"

	"github.com/getlantern/flashlight/domainrouting"
	"github.com/getlantern/yaml"
)

func ExampleRuleSet_yaml() {
	type myStruct struct {
		RoutingRules domainrouting.RuleSet
	}

	s := myStruct{
		domainrouting.RuleSet{
			"direct-domain":   domainrouting.RuleDirect{},
			"proxied-domain":  domainrouting.RuleProxy{},
			"rerouted-domain": domainrouting.RuleReroute("another-domain"),
		},
	}
	asYAML, err := yaml.Marshal(s)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(asYAML))

	// Output:
	//
	// routingrules:
	//   direct-domain: d
	//   proxied-domain: p
	//   rerouted-domain: another-domain
}
