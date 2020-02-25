package chained

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChooseBrowserForUser(t *testing.T) {
	const numIDs = 100

	browsers := []browserMarketShare{
		{chrome, .4},
		{safari, .3},
		{firefox, .2},
		{edge, .1},
	}
	weightsTotal := 0.0
	for _, browser := range browsers {
		weightsTotal += browser.marketShare
	}
	// Sanity check as the test won't work otherwise.
	assert.Equal(t, numIDs/100, weightsTotal)

	results := map[browser]int{}
	for id := 0; id < numIDs; id++ {
		results[chooseBrowserForUser(int64(id), browsers)]++
	}
	assert.Equal(t, 40, results[chrome])
	assert.Equal(t, 30, results[safari])
	assert.Equal(t, 20, results[firefox])
	assert.Equal(t, 10, results[edge])
}
