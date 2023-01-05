package broflake

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomSTUNs(t *testing.T) {
	listSize := 100
	stuns := make([]string, 0, listSize)
	for i := 0; i < listSize; i++ {
		stuns = append(stuns, fmt.Sprintf("stun%v.example.com", i))
	}

	getBatch := RandomSTUNs(stuns)
	for batchSize := 0; batchSize < listSize+1; batchSize++ {
		seen := make([]string, 0, batchSize)
		batch, err := getBatch(uint32(batchSize))
		if !assert.NoError(t, err) {
			return
		}
		for _, v := range batch {
			assert.Contains(t, stuns, v)
			assert.NotContains(t, seen, v)
			seen = append(seen, v)
		}
		assert.Equal(t, len(seen), batchSize)
	}

	batch, err := getBatch(uint32(listSize * 5))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, len(batch), listSize)
	assert.ElementsMatch(t, batch, stuns)

	getBatchZero := RandomSTUNs([]string{})
	batch, err = getBatchZero(100)
	assert.Equal(t, len(batch), 0)
	if !assert.NoError(t, err) {
		return
	}
}
