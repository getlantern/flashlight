package chained

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetRandomSubset(t *testing.T) {
	listSize := 100
	uniqueStrings := make([]string, 0, listSize)
	for i := 0; i < listSize; i++ {
		uniqueStrings = append(uniqueStrings, fmt.Sprintf("foo%v", i))
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for subsetSize := 0; subsetSize < listSize+1; subsetSize++ {
		seen := make([]string, 0, subsetSize)
		subset := getRandomSubset(uint32(subsetSize), rng, uniqueStrings)

		for _, s := range subset {
			assert.Contains(t, uniqueStrings, s)
			assert.NotContains(t, seen, s)
			seen = append(seen, s)
		}
		assert.Equal(t, len(seen), subsetSize)
	}

	subset := getRandomSubset(uint32(listSize*10), rng, uniqueStrings)
	assert.Equal(t, len(subset), listSize)
	assert.ElementsMatch(t, subset, uniqueStrings)

	nullSet := []string{}
	subset = getRandomSubset(uint32(100), rng, nullSet)
	assert.Equal(t, len(subset), 0)
}
