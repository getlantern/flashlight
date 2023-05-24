package domainrouting

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getlantern/domains"
)

func TestBuildTree(t *testing.T) {
	rules := RulesMap{
		"D1": Proxy,
		"P1": Direct,
		"P3": Proxy,
	}

	expectedResult := domains.Map{
		".d1": Proxy,
		".p1": Direct,
		".p3": Proxy,
	}

	result := NewRules(rules)

	assert.EqualValues(t, expectedResult, result.tree.ToMap())
}

func TestIPHandling(t *testing.T) {
	rules := RulesMap{
		"1.2.3.4": MustDirect,
	}

	Configure(rules, &ProxiedSitesConfig{})

	assert.EqualValues(t, MustDirect, RuleFor("1.2.3.4"))

	RemoveRules(rules)
}

func TestTLDHandling(t *testing.T) {
	rules := RulesMap{
		"ir": MustDirect,
	}

	Configure(rules, &ProxiedSitesConfig{})

	assert.EqualValues(t, MustDirect, RuleFor("www.google.ir"))

	RemoveRules(rules)
}
