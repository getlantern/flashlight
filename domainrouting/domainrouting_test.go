package domainrouting

import (
	"testing"

	"github.com/getlantern/domains"
	"github.com/getlantern/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTree(t *testing.T) {
	rules := RuleSet{
		"D1": RuleProxy{},
		"P1": RuleDirect{},
		"P3": RuleProxy{},
	}

	expectedResult := domains.Map{
		".d1": RuleProxy{},
		".p1": RuleDirect{},
		".p3": RuleProxy{},
	}

	result := buildTree(rules)

	assert.EqualValues(t, expectedResult, result.ToMap())
}

func TestYAMLRoundTrip(t *testing.T) {
	r := RuleSet{
		"direct-domain":   RuleDirect{},
		"proxied-domain":  RuleProxy{},
		"rerouted-domain": RuleReroute("another-domain"),
	}
	asYaml, err := yaml.Marshal(r)
	require.NoError(t, err)
	roundTripped := RuleSet{}
	require.NoError(t, yaml.Unmarshal(asYaml, roundTripped))
	require.Equal(t, r, roundTripped)
}
