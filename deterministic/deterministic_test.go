package deterministic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testChoice int

func (tc testChoice) Weight() int {
	return int(tc)
}

func TestMakeChoice(t *testing.T) {
	const numDecisions = 100
	choices := []interface{}{1, 2, 3, 4}

	// Sanity check as the test won't work otherwise.
	require.Equal(t, 0, numDecisions%len(choices))

	choiceToTimesChosen := map[int]int{}
	for i := int64(0); i < numDecisions; i++ {
		choiceToTimesChosen[MakeChoice(i, choices...).(int)]++
	}
	for _, timesChosen := range choiceToTimesChosen {
		require.Equal(t, numDecisions/len(choices), timesChosen)
	}
	for _, choice := range choices {
		_, ok := choiceToTimesChosen[choice.(int)]
		require.True(t, ok)
	}
	require.Equal(t, len(choices), len(choiceToTimesChosen))
}

func TestMakeWeightedChoice(t *testing.T) {
	const numIDs = 100

	choices := []WeightedChoice{
		testChoice(40),
		testChoice(30),
		testChoice(20),
		testChoice(10),
	}

	totalWeight := 0
	for _, choice := range choices {
		totalWeight += choice.Weight()
	}
	// Sanity check as the test won't work otherwise.
	require.Equal(t, numIDs, totalWeight)

	weightsToTimesChosen := map[int]int{}
	for id := 0; id < numIDs; id++ {
		choice := MakeWeightedChoice(int64(id), choices)
		weightsToTimesChosen[choice.Weight()]++
	}
	for weight, timesChosen := range weightsToTimesChosen {
		require.Equal(t, weight, timesChosen)
	}
}
