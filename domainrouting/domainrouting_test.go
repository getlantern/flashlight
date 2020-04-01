package domainrouting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type change struct {
	domain  string
	oldRule Rule
	newRule Rule
}

func TestUpdateNil(t *testing.T) {

}
func TestUpdate(t *testing.T) {
	oldRules := Rules{
		"d1": Direct,
		"p1": Proxy,
		"p2": Proxy,
	}

	newRules := Rules{
		"d1": Proxy,
		"p1": Direct,
		"p3": Proxy,
	}

	changes := make([]change, 0)
	result := oldRules.Update(newRules, func(domain string, oldRule, newRule Rule) {
		changes = append(changes, change{domain, oldRule, newRule})
	})

	expectedChanges := []change{
		change{"d1", Direct, Proxy},
		change{"p1", Proxy, Direct},
		change{"p3", None, Proxy},
		change{"p2", Proxy, None},
	}

	assert.EqualValues(t, newRules, result)
	assert.EqualValues(t, expectedChanges, changes)
}
