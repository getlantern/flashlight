package proxied

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProxiedFlowCancellaton(t *testing.T) {
	// Make a cancellable context inside a request (doesn't matter
	// what's the domain or method: it'll never be triggered. We just
	// want to make a context for a request here)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://whatever.com",
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, req)

	// Sleep a bit and then cancel the request
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	// Assert that, after we cancel, since our only roundtripper "aaa"
	// is waiting forever, we won't get any response and we'll get an
	// error
	resp, err := NewProxiedFlow(
		&ProxiedFlowInput{
			AddDebugHeaders: true,
		},
	).
		// Make a roundtripper that never finishes
		Add("aaa",
			&mockRoundTripper_Return200{
				id:             FlowComponentID("aaa"),
				processingTime: 999 * time.Second,
			},
			false, // isPreferred
		).RoundTrip(req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
	require.Nil(t, resp)
}

func TestProxiedFlowPreference(t *testing.T) {
	type testCase struct {
		name                                   string
		numOfRequests                          int
		initFlow                               func(f OnStartRoundTrip) *ProxiedFlow
		mapOfRoundTripperNamesToNumTimesCalled map[string]int
		winningRoundTripperPerRequest          []string
	}

	for _, tc := range []testCase{
		{
			name:          "Parallel components. Prefer none. All roundtrippers should be triggered",
			numOfRequests: 5,
			initFlow: func(f OnStartRoundTrip) *ProxiedFlow {
				return NewProxiedFlow(&ProxiedFlowInput{
					AddDebugHeaders:      true,
					OnStartRoundTripFunc: f,
				}).
					Add("aaa",
						&mockRoundTripper_Return200{id: "aaa", processingTime: 100 * time.Millisecond},
						false).
					Add("bbb",
						&mockRoundTripper_Return200{id: "bbb", processingTime: 300 * time.Millisecond},
						false).
					Add("ccc",
						&mockRoundTripper_Return200{id: "ccc", processingTime: 300 * time.Millisecond},
						false)
			},
			winningRoundTripperPerRequest: []string{
				// "aaa" always wins since it's the fastest
				"aaa",
				"aaa",
				"aaa",
				"aaa",
				"aaa",
			},
			mapOfRoundTripperNamesToNumTimesCalled: map[string]int{
				"aaa": 5,
				"bbb": 5,
				"ccc": 5,
			},
		},

		{
			name:          "Parallel components, prefer one. Have the preferred one come first and the rest should NOT be triggered for subsequent runs if no errors occur",
			numOfRequests: 5,
			initFlow: func(f OnStartRoundTrip) *ProxiedFlow {
				return NewProxiedFlow(&ProxiedFlowInput{
					AddDebugHeaders:      true,
					OnStartRoundTripFunc: f,
				}).
					Add("aaa",
						&mockRoundTripper_Return200{id: "aaa", processingTime: 100 * time.Millisecond},
						true).
					Add("bbb",
						&mockRoundTripper_Return200{id: "bbb", processingTime: 300 * time.Millisecond},
						false).
					Add("ccc",
						&mockRoundTripper_Return200{id: "ccc", processingTime: 300 * time.Millisecond},
						false)
			},
			winningRoundTripperPerRequest: []string{
				// "aaa" always wins since it's the fastest
				"aaa",
				"aaa",
				"aaa",
				"aaa",
				"aaa",
			},
			mapOfRoundTripperNamesToNumTimesCalled: map[string]int{
				"aaa": 5,
				"bbb": 1,
				"ccc": 1,
			},
		},

		{
			name:          "Parallel components. Prefer one. The preferred component fails and the rest are triggered in the same run",
			numOfRequests: 5,
			initFlow: func(f OnStartRoundTrip) *ProxiedFlow {
				flow := NewProxiedFlow(&ProxiedFlowInput{
					AddDebugHeaders:      true,
					OnStartRoundTripFunc: f,
				}).
					Add("aaa",
						&mockRoundTripper_FailOnceAndThenReturn200{
							id: "aaa", processingTime: 500 * time.Millisecond},
						true).
					Add("bbb",
						&mockRoundTripper_Return200{id: "bbb",
							processingTime: 100 * time.Millisecond,
						},
						false).
					Add("ccc",
						&mockRoundTripper_Return200{id: "ccc",
							processingTime: 400 * time.Millisecond},
						false)

					// Set "aaa" as the preferredComponent
				for _, c := range flow.components {
					if c.id == "aaa" {
						flow.preferredComponent = c
						break
					}
				}
				return flow
			},
			winningRoundTripperPerRequest: []string{
				// "bbb" always wins since it's the fastest, regardless of the
				// fact if other components are preferred or not
				"bbb",
				"bbb",
				"bbb",
				"bbb",
				"bbb",
			},
			mapOfRoundTripperNamesToNumTimesCalled: map[string]int{
				// Once for the first request that failed when "aaa" was the
				// preferred component.
				//
				// And five more for the rest of the requests. Yes, we've ran
				// this component **twice** for one request. That's fine.
				"aaa": 6,
				// One for each request in this round
				"bbb": 5,
				"ccc": 5,
			},
		},

		{
			name:          "Parallel components. Prefer none. Have all of them fail",
			numOfRequests: 5,
			initFlow: func(f OnStartRoundTrip) *ProxiedFlow {
				flow := NewProxiedFlow(&ProxiedFlowInput{
					AddDebugHeaders:      true,
					OnStartRoundTripFunc: f,
				}).
					Add("aaa",
						&mockRoundTripper_Return400{
							id: "aaa", processingTime: 100 * time.Millisecond},
						false).
					Add("bbb",
						&mockRoundTripper_Return400{id: "bbb",
							processingTime: 100 * time.Millisecond,
						},
						false).
					Add("ccc",
						&mockRoundTripper_Return400{id: "ccc",
							processingTime: 100 * time.Millisecond},
						false)

				return flow
			},
			winningRoundTripperPerRequest: []string{
				"",
				"",
				"",
				"",
				"",
			},
			mapOfRoundTripperNamesToNumTimesCalled: map[string]int{
				// Once for the first request that failed when "aaa" was the
				// preferred component.
				//
				// And five more for the rest of the requests. Yes, we've ran
				// this component **twice** for one request. That's fine.
				"aaa": 5,
				// One for each request in this round
				"bbb": 5,
				"ccc": 5,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var collectedIdsMu sync.Mutex
			collectedIds := []FlowComponentID{}
			flow := tc.initFlow(func(id FlowComponentID, _ *http.Request) {
				collectedIdsMu.Lock()
				defer collectedIdsMu.Unlock()
				collectedIds = append(collectedIds, id)
			})
			for i := 0; i < tc.numOfRequests; i++ {
				// Request doesn't matter since our mock roundtrippers don't do
				// any HTTP work
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(
					ctx,
					http.MethodGet,
					"http://whatever.com",
					nil,
				)
				require.NoError(t, err)
				resp, err := flow.RoundTrip(req)
				winnerRTName := tc.winningRoundTripperPerRequest[i]
				if winnerRTName == "" {
					require.Error(t, err)
					require.Nil(t, resp)
				} else {
					require.NoError(t, err)
					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.StatusCode)
					require.Equal(
						t,
						winnerRTName,
						resp.Header.Get(roundTripperHeaderKey),
						"Expected the winning round tripper for request #%d to be %s, but got %s",
						i,
						winnerRTName,
						resp.Header.Get(roundTripperHeaderKey),
					)
				}
			}

			// Assert that, if the preferred RT succeeds, we didn't even try
			// the rest
			collectedIdsMu.Lock()
			for roundTripperName, numOfTimesItShouldBeCalled := range tc.mapOfRoundTripperNamesToNumTimesCalled {
				assertArrHasValInCorrectQuanitity[FlowComponentID](
					t, collectedIds,
					FlowComponentID(roundTripperName), numOfTimesItShouldBeCalled)
			}
			collectedIdsMu.Unlock()
		})
	}
}

func assertArrHasValInCorrectQuanitity[T comparable](
	t *testing.T,
	arr []T,
	inputVal T, numOfTimes int) {
	t.Helper()
	seen := 0
	for _, val := range arr {
		if inputVal == val {
			seen++
		}
	}
	require.Equal(t,
		numOfTimes, seen,
		"Expected %v to be seen %v times, but it was seen %v times", inputVal, numOfTimes, seen)
}
