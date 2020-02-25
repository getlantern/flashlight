package chained

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testChoice int

func (tc testChoice) weight() int {
	return int(tc)
}

func TestWeightedChoiceForUser(t *testing.T) {
	const numIDs = 100

	choices := []weightedChoice{
		testChoice(40),
		testChoice(30),
		testChoice(20),
		testChoice(10),
	}
	totalWeight := 0
	for _, choice := range choices {
		totalWeight += choice.weight()
	}
	// Sanity check as the test won't work otherwise.
	require.Equal(t, numIDs, totalWeight)

	weightsToChoices := map[int]int{}
	for id := 0; id < numIDs; id++ {
		choice := weightedChoiceForUser(int64(id), choices)
		weightsToChoices[choice.weight()]++
	}
	for weight, timesChosen := range weightsToChoices {
		require.Equal(t, weight, timesChosen)
	}
}
