package chained

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChooseBucketForUser(t *testing.T) {
	const numIDs = 100

	bucketsToWeights := map[int]int{
		1: 40,
		2: 30,
		3: 20,
		4: 10,
	}
	weightsTotal := 0
	for _, weight := range bucketsToWeights {
		weightsTotal += weight
	}
	// Sanity check as the test won't work otherwise.
	require.Equal(t, numIDs, weightsTotal)

	bucketsToChoices := map[int]int{}
	for id := 0; id < numIDs; id++ {
		bucketsToChoices[chooseBucketForUser(int64(id), bucketsToWeights)]++
	}
	for bucket, choices := range bucketsToChoices {
		require.Equal(t, bucketsToWeights[bucket], choices)
	}
}
