package domainrouting

import (
	"testing"

	"github.com/getlantern/domains"
	"github.com/stretchr/testify/assert"
)

type change struct {
	oldRule Rule
	newRule Rule
}

var (
	oldRules = domains.NewTreeFromMap(domains.Map{
		".d1": Direct,
		".p1": Proxy,
		".p2": Proxy,
	})

	newRules = Rules{
		"D1": Proxy,
		"P1": Direct,
		"P3": Proxy,
	}

	expectedResult = domains.Map{
		".d1": Proxy,
		".p1": Direct,
		".p3": Proxy,
	}
)

func TestUpdateNil(t *testing.T) {
	changes := make(map[string]change, 0)
	result := update(nil, newRules, func(domain string, oldRule, newRule Rule) {
		changes[domain] = change{oldRule, newRule}
	})

	expectedChanges := map[string]change{
		"d1": change{None, Proxy},
		"p1": change{None, Direct},
		"p3": change{None, Proxy},
	}

	assert.EqualValues(t, expectedResult, result.ToMap())
	assert.EqualValues(t, expectedChanges, changes)
}
func TestUpdate(t *testing.T) {
	changes := make(map[string]change, 0)
	result := update(oldRules, newRules, func(domain string, oldRule, newRule Rule) {
		changes[domain] = change{oldRule, newRule}
	})

	expectedChanges := map[string]change{
		"d1": change{Direct, Proxy},
		"p1": change{Proxy, Direct},
		"p3": change{None, Proxy},
		"p2": change{Proxy, None},
	}

	assert.EqualValues(t, expectedResult, result.ToMap())
	assert.EqualValues(t, expectedChanges, changes)
}
