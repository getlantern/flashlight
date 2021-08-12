package util

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDynamicEndpoint(t *testing.T) {
	// Setup
	// -------
	type Interval struct {
		endpoint    string
		returnedErr error
		shouldFail  bool
	}
	defaultEndpoint := "aaa.com"
	conditionChan := make(chan bool, 5)
	defaultSleepDuration := 1 * time.Second
	intervals := []Interval{
		Interval{
			endpoint:    "bbb.com",
			returnedErr: nil,
			shouldFail:  false,
		},
		Interval{
			endpoint:    "",
			returnedErr: errors.New("whatever error"),
			shouldFail:  true, // Because of the returned error
		},
		Interval{
			endpoint:    "ddd.com",
			returnedErr: nil,
		},
		Interval{
			endpoint:    "bunny\nfoo\nfoo", // This is not a parseable URL
			returnedErr: nil,
			shouldFail:  true, // Because of the unparseable URL
		},
		Interval{
			endpoint:    "eee.com",
			returnedErr: nil,
			shouldFail:  false,
		},
		Interval{
			endpoint:    "fff.com",
			returnedErr: nil,
			shouldFail:  false,
		},
	}
	// Refresh conditionChan every X seconds
	go func() {
		for range intervals {
			time.Sleep(defaultSleepDuration)
			conditionChan <- true
		}
	}()

	// Action
	// -------
	// Make a new DynamicEndpoint that assigns the ith interval value
	i := 0
	dynamicEndpoint, err := NewDynamicEndpoint(
		defaultEndpoint,
		conditionChan,
		nil,
		false,
		func() (string, error) {
			defer func() { i += 1 }()
			return intervals[i].endpoint, intervals[i].returnedErr
		},
	)
	require.NoError(t, err)

	// Assertions
	// -------------
	// Assert the sequence of values
	//
	// If we encounter an error from the setter function, assert that
	// dynamicEndpoint.Get() is still the last successful value we had
	var lastSuccessfulEndpoint string
	require.Equal(t, defaultEndpoint, dynamicEndpoint.Get().String())
	for _, interval := range intervals {
		time.Sleep(defaultSleepDuration + 10*time.Millisecond)
		if interval.shouldFail {
			require.Equal(t, lastSuccessfulEndpoint, dynamicEndpoint.Get().String())
		} else {
			require.Equal(t, interval.endpoint, dynamicEndpoint.Get().String())
			lastSuccessfulEndpoint = dynamicEndpoint.Get().String()
		}
		// log.Printf("%+v\n", dynamicEndpoint)
	}
}

func TestConstantDynamicEndpoint(t *testing.T) {
	dynamicEndpoint, err := NewConstantDynamicEndpoint("bunnyfoofoo.com")
	require.NoError(t, err)
	require.Equal(t, "bunnyfoofoo.com", dynamicEndpoint.Get())
	time.Sleep(5 * time.Second)
	require.Equal(t, "bunnyfoofoo.com", dynamicEndpoint.Get())
}
