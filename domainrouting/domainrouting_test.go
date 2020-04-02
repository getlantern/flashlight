package domainrouting

import (
	"testing"

	"github.com/getlantern/domains"
	"github.com/stretchr/testify/assert"
)

func TestBuildTree(t *testing.T) {
	rules := Rules{
		"D1": Proxy,
		"P1": Direct,
		"P3": Proxy,
	}

	expectedResult := domains.Map{
		".d1": Proxy,
		".p1": Direct,
		".p3": Proxy,
	}

	result := buildTree(rules)

	assert.EqualValues(t, expectedResult, result.ToMap())
}
