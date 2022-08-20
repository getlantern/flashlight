// This file focuses on a structure called ProxiedFlow. It's useful for us to
// run multiple roundtrippers in parallel and have a "preference" for one or
// more of them.
//
// To breakdown this concept further, let's go with an example: Assume we want
// to run a request through our Lantern proxies (called the "chained
// roundtripper") **and** through domain fronting (called "fronted"
// roundtripper) where the fastest response is taken.
//
// Let's also assume we want to have a preference for "chained roundtripper",
// meaning that if running a request through the "chained roundtripper" was the
// fastest roundtripper we've found (as opposed to the "fronted roundtripper"
// in this example), the **next** request you run will automatically go through
// "chained", and we wouldn't bother "fronted" roundtripper, unless it fails.
//
// The code for this example will look like this:
//
//     chainedRoundTripper, err := proxied.ChainedNonPersistent("")
//     require.NoError(t, err)
//
//     req, err := http.NewRequest("GET", "http://example.com", nil)
//     require.NoError(t, err)
//     flow := NewProxiedFlow(
//     	&ProxiedFlowInput{
//     		AddDebugHeaders: true,
//     	},
//     )
//
// 	   flow.
// 	     Add(proxied.FlowComponentID_Chained, chained, true).
//       Add(proxied.FlowComponentID_Fronted, proxied.Fronted(masqueradeTimeout), false)
//     resp, err := flow.RoundTrip(req)
//     require.Error(t, err)
package proxied

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
)

const roundTripperHeaderKey = "Roundtripper"

type FlowComponentID string

// Enum of most used roundtrippers
var (
	FlowComponentID_P2P     FlowComponentID = "p2p"
	FlowComponentID_Fronted FlowComponentID = "fronted"
	FlowComponentID_Chained FlowComponentID = "chained"
)

func (id FlowComponentID) String() string {
	return string(id)
}

// ProxiedFlowComponent is basically a wrapper around an http.RoundTripper that
// includes an ID and some additional flags
type ProxiedFlowComponent struct {
	http.RoundTripper
	id              FlowComponentID
	addDebugHeaders bool
	shouldPrefer    bool
}

// ProxiedFlowResponse is a wrapper around an http.Response and an error, both
// coming from ProxiedFlowComponent.RoundTrip()
type ProxiedFlowResponse struct {
	id   FlowComponentID
	resp *http.Response
	err  error
}

// OnStartRoundTrip is called by the flow when it starts a new roundtrip.
type OnStartRoundTrip func(FlowComponentID, *http.Request)

// OnCompleteRoundTrip is called by the flow when it completes a roundtrip.
type OnCompleteRoundTrip func(FlowComponentID)

// ProxiedFlowInput is the input to NewProxiedFlow()
type ProxiedFlowInput struct {
	// Can be set to true to add the value of
	// "roundTripperHeaderKey" to the response headers (not request). It's purely
	// used for assertions during unit tests.
	AddDebugHeaders bool
	// Runs when a flow component is about to start roundtripping
	OnStartRoundTripFunc OnStartRoundTrip
	// Runs when a flow component is is done roundtripping
	OnCompleteRoundTripFunc OnCompleteRoundTrip
}

type ProxiedFlow struct {
	// List of components in the flow
	components []*ProxiedFlowComponent
	input      *ProxiedFlowInput

	// Most preferred component. Can be nil, which means either that no
	// component wants to be preferred or that this flow was never run
	// successfully before
	preferredComponent *ProxiedFlowComponent
}

// NewProxiedFlow returns a new ProxiedFlow
func NewProxiedFlow(input *ProxiedFlowInput) *ProxiedFlow {
	return &ProxiedFlow{input: input}
}

// Add adds new roundtrippers to the flow.
// The highest priority components should be added first (i.e., 0 is the
// highest priority)
func (f *ProxiedFlow) Add(
	id FlowComponentID,
	rt http.RoundTripper,
	shouldPrefer bool,
) *ProxiedFlow {
	f.components = append(f.components, &ProxiedFlowComponent{
		RoundTripper:    rt,
		id:              id,
		shouldPrefer:    shouldPrefer,
		addDebugHeaders: f.input.AddDebugHeaders,
	})
	// Returning f so function calls can be chained nicely in a builder pattern
	return f
}

// SetPreferredComponent sets the component with "id" as the preferred
// component.
// This function doesn't fail if the component doesn't exist.
//
// Return *ProxiedFlow to chain function calls in a builder pattern.
func (f *ProxiedFlow) SetPreferredComponent(id FlowComponentID) *ProxiedFlow {
	for _, c := range f.components {
		if c.id == id {
			f.preferredComponent = c
			break
		}
	}
	return f
}

// RoundTrip makes ProxiedFlow implement the http.RoundTripper interface.
// This function works in two ways:
// - the "runner" code occurs in "f.runAllComponents()" and it's responsible
//   for running all the roundtrippers in the flow (or just the preferred one, if
//   one exists) and send the responses through "recvFlowRespCh"
// - the "reciever" code occurs in the "looper" block below and it's
//   responsible for handling responses and errors
//
// This function respects the request's original context
func (f *ProxiedFlow) RoundTrip(
	originalReq *http.Request,
) (*http.Response, error) {
	recvFlowRespCh := make(chan *ProxiedFlowResponse, len(f.components))
	go f.runAllComponents(originalReq, recvFlowRespCh)

	collectedErrors := []error{}
looper:
	select {
	case flowResp := <-recvFlowRespCh:
		fmt.Printf("flowResp = %+v\n", flowResp)
		if flowResp.err != nil {
			var url string
			if originalReq.URL != nil {
				url = originalReq.URL.String()
			} else {
				url = "nil"
			}

			log.Errorf(
				"Error from FlowComponent %s during request: %v: %v",
				flowResp.id, url, flowResp.err)
			collectedErrors = append(collectedErrors, flowResp.err)
		}
		if flowResp.resp != nil {
			// Set this component as a preferredComponent, only if the
			// component wants to (i.e., shouldPrefer is true)
			for _, c := range f.components {
				if c.id == flowResp.id && c.shouldPrefer {
					f.preferredComponent = c
				}
			}
			return flowResp.resp, nil
		}
	case <-originalReq.Context().Done():
		// If we're done, we need to exit now.
		// Try to return the highest priority response we've seen.
		// Else, try to return the latest response we've seen.
		// Else, return an error that all roundtrippers failed.
		collectedErrors = append(collectedErrors, originalReq.Context().Err())
		return nil, fmt.Errorf(
			"flow.go:RoundTrip: All roundtrippers failed with errs: %+v",
			collectedErrors,
		)
	}
	goto looper
}

// Run runs component "comp" by basically cloning the request and
// then roundtripping
func (comp *ProxiedFlowComponent) Run(
	originalReq *http.Request,
	originalReqMu *sync.Mutex,
	onStartRoundTripFunc OnStartRoundTrip,
	onCompleteRoundTripFunc OnCompleteRoundTrip,
) *ProxiedFlowResponse {
	// Copy original request
	originalReqMu.Lock()
	_, copiedReq, err := copyRequest(originalReq)
	originalReqMu.Unlock()
	if err != nil {
		return &ProxiedFlowResponse{
			resp: nil,
			err: fmt.Errorf(
				"flow.go:runAllComponents while copying request [%+v]: %w",
				originalReq, err,
			), id: comp.id}
	}

	// Setup the onStart and onComplete callbacks
	if onStartRoundTripFunc != nil {
		onStartRoundTripFunc(comp.id, copiedReq)
	}
	defer func() {
		if onCompleteRoundTripFunc != nil {
			onCompleteRoundTripFunc(comp.id)
		}
	}()

	// Get the URL (useful for logs and debugging)
	var url string
	if copiedReq.URL != nil {
		url = copiedReq.URL.String()
	} else {
		url = "nil"
	}

	// Run the roundtripper
	resp, err := comp.RoundTripper.RoundTrip(copiedReq)

	// Handle errors and whatnots
	if err != nil {
		return &ProxiedFlowResponse{
			resp: nil,
			err: fmt.Errorf(
				"with roundtripper [%v] during FlowRoundTrip towards [%v]: %v",
				comp.id,
				url,
				err,
			),
			id: comp.id}
	}
	if resp == nil {
		return &ProxiedFlowResponse{
			resp: nil,
			err: fmt.Errorf(
				"with roundtripper [%v] during FlowRoundTrip towards [%v]: no response",
				comp.id,
				url,
			),
			id: comp.id}
	}
	if resp.StatusCode >= 400 {
		body := "nil"
		if copiedReq.Body != nil {
			b, err := io.ReadAll(resp.Body)
			if err == nil {
				body = string(b)
				resp.Body.Close()
			}
		}
		return &ProxiedFlowResponse{
			resp: nil,
			err: fmt.Errorf(
				"with roundtripper [%v] during FlowRoundTrip towards [%v]: status code [%v]: body: %v",
				comp.id,
				url,
				resp.StatusCode,
				body,
			),
			id: comp.id}
	}

	// Add a header mentioning the used roundtripper.
	// Only useful for tests.
	if comp.addDebugHeaders {
		resp.Header.Set(roundTripperHeaderKey, comp.id.String())
	}

	return &ProxiedFlowResponse{
		resp: resp,
		err:  nil,
		id:   comp.id}
}

func copyRequest(req *http.Request) (*http.Request, *http.Request, error) {
	req2 := req.Clone(req.Context())
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, nil, fmt.Errorf("while reading request body %v", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(b))
		req2.Body = io.NopCloser(bytes.NewReader(b))
	}
	return req, req2, nil
}

// runAllComponents runs the components in parallel, while favoring the
// "preferred" component
func (f *ProxiedFlow) runAllComponents(
	originalReq *http.Request,
	recvFlowRespCh chan<- *ProxiedFlowResponse,
) {
	// If there's a preferred component, run it first
	var originalReqMu sync.Mutex
	if f.preferredComponent != nil {
		flowResp := f.preferredComponent.Run(
			originalReq, &originalReqMu,
			f.input.OnStartRoundTripFunc,
			f.input.OnCompleteRoundTripFunc,
		)
		recvFlowRespCh <- flowResp
		if flowResp.err != nil {
			// If it failed, remove it as our preferred component
			f.preferredComponent = nil
		} else if flowResp.resp != nil {
			// If it succeeded, just go with it
			return
		}
	}

	// Else, run the rest of the components asynchronously
	for _, _comp := range f.components {
		comp := _comp
		go func() {
			recvFlowRespCh <- comp.Run(
				originalReq, &originalReqMu,
				f.input.OnStartRoundTripFunc,
				f.input.OnCompleteRoundTripFunc)
		}()
	}
}
