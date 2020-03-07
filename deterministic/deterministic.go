// Package deterministic is used to make deterministic choices for users.
package deterministic

import "sort"

// A WeightedChoice represents some choice with an associated weight.
type WeightedChoice interface {
	Weight() int
}

type unweightedChoice struct {
	choice interface{}
}

func (uc unweightedChoice) Weight() int { return 1 }

// MakeChoice makes a choice for the given user ID. This choice is deterministic given constant
// inputs.
func MakeChoice(userID int64, choices ...interface{}) interface{} {
	_choices := make([]WeightedChoice, len(choices))
	for i, choice := range choices {
		_choices[i] = unweightedChoice{choice}
	}
	return MakeWeightedChoice(userID, _choices).(unweightedChoice).choice
}

// MakeWeightedChoice makes a choice for the given user ID. This choice is deterministic given
// constant inputs.
//
// To ensure proper distribution of users according to the weights, the following must be true:
// 	(1) The sum of the weights must be less than the maximum assigned user ID.
//	(2) User IDs must be evenly distributed across their range.
func MakeWeightedChoice(userID int64, choices []WeightedChoice) WeightedChoice {
	totalWeight := 0
	for i := range choices {
		totalWeight += choices[i].Weight()
	}
	sort.Slice(choices, func(i, j int) bool { return choices[i].Weight() < choices[j].Weight() })

	if userID < 0 {
		userID *= -1
	}
	choicePosition := userID % int64(totalWeight)
	for i, b := range choices {
		if choicePosition < int64(b.Weight()) {
			return choices[i]
		}
		choicePosition -= int64(b.Weight())
	}
	// Shouldn't be possible to reach this point unless choices is empty.
	return nil
}
