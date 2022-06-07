package proxied

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

const roundTripperHeaderKey = "Roundtripper"

type FlowComponentID string

var (
	FlowComponentID_P2P     FlowComponentID = "p2p"
	FlowComponentID_Fronted FlowComponentID = "fronted"
	FlowComponentID_Chained FlowComponentID = "chained"
)

func (id FlowComponentID) String() string {
	return string(id)
}

type ProxiedFlowComponent struct {
	http.RoundTripper
	id              FlowComponentID
	addDebugHeaders bool
	shouldPrefer    bool
}

type ProxiedFlowResponse struct {
	id   FlowComponentID
	resp *http.Response
	err  error
}

type OnStartRoundTrip func(FlowComponentID, *http.Request)
type OnCompleteRoundTrip func(FlowComponentID)

type ProxiedFlowInput struct {
	// Can be zero, in which case, ProxiedFlow.RoundTrip() basically disregards
	// preference and will return whatever response it gets.
	// WaitForPreferredRoundTripperTimeout time.Duration

	// Use to dynamically set WaitForPreferredRoundTripperTimeout to a
	// percentage of the context's deadline, if any.
	//
	// If no context deadline is available, nothing happens.
	//
	// So, if you know your client called their request with something like
	// this, if a Chained response came (the non-preferred one), it'll wait for
	// 5*time.Second (0.5 * 10*time.Second) for a Fronted response (the
	// preferred one). After that, ProxiedFlow.RoundTrip() will take what it
	// currently has (the Chained response) and return that
	//
	//      ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	//      defer cancel()
	//      req, err := http.NewRequestWithContext(ctx, "GET", "https://whatever.com", nil)
	//      dieIfErr(err)
	//
	//      resp, err := flow.NewProxiedFlow(*ProxiedFlowInput{WaitForPreferredRoundTripperTimeoutPerc: 0.5}).
	//        Add(FlowComponentID_Fronted, proxied.Fronted(), true).
	//        Add(FlowComponentID_Fronted, proxied.ChainedNonPersistent(), false).
	//        RoundTrip(req)
	//      // ...
	//
	// WaitForPreferredRoundTripperTimeoutPerc float64

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
	components         []*ProxiedFlowComponent
	input              *ProxiedFlowInput
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

// TODO <02-07-2022, soltzen> Make the docs prettier
// So there're 3 options:
// - Run a bunch of roundtrippers in sequence
// - Run parallel with no preference
// - Run parallel with preference
//
// We can achieve this with a priority and preference system.
// The priority system is used to determine which roundtripper sequence to run
// When a roundtripper returns, see if it is preferable. If so, just use its response and cancel the rest.
//
// With this, we can do the following combinations:
// - P2P AND Fronted AND Chained (preferred)
// - Fronted AND Chained (preferred)
// - Fronted AND Chained
// - etc.
//
// If there are two or more preferred roundtrippers, whichever one returns first will be used.
func (f *ProxiedFlow) RoundTrip(
	originalReq *http.Request,
) (*http.Response, error) {
	recvFlowRespCh := make(chan *ProxiedFlowResponse, len(f.components))
	go f.runComponents(originalReq, recvFlowRespCh)

	collectedErrors := []error{}
looper:
	select {
	case flowResp := <-recvFlowRespCh:
		// fmt.Printf("flowResp = %+v\n", flowResp)
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

func (comp *ProxiedFlowComponent) Run(req *http.Request) *ProxiedFlowResponse {
	resp, err := comp.RoundTrip(req)
	if err != nil {
		return &ProxiedFlowResponse{
			resp: nil,
			err: fmt.Errorf(
				"with roundtripper [%v] during FlowRoundTrip towards [%v]: %v",
				comp.id,
				req.URL,
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
				req.URL,
			),
			id: comp.id}
	}
	// Accept 1xx, 2xx and 3xx responses only
	if resp.StatusCode/100 >= 4 {
		body := "nil"
		if req.Body != nil {
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
				req.URL,
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

	// b, _ := httputil.DumpResponse(resp, true)
	// log.Debugf("Response from FlowComponent %s: %s", comp.id, string(b))

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

func (f *ProxiedFlow) runComponent(
	comp *ProxiedFlowComponent,
	req *http.Request,
) *ProxiedFlowResponse {
	if f.input.OnStartRoundTripFunc != nil {
		f.input.OnStartRoundTripFunc(comp.id, req)
	}
	defer func() {
		if f.input.OnCompleteRoundTripFunc != nil {
			f.input.OnCompleteRoundTripFunc(comp.id)
		}
	}()
	return comp.Run(req)
}

func (f *ProxiedFlow) runComponents(
	originalReq *http.Request,
	recvFlowRespCh chan<- *ProxiedFlowResponse,
) {
	// If there's a preferred component, run it first
	if f.preferredComponent != nil {
		// Copy the request
		originalReq, copiedReq, err := copyRequest(originalReq)
		if err != nil {
			recvFlowRespCh <- &ProxiedFlowResponse{
				resp: nil,
				err: fmt.Errorf(
					"flow.go:runComponents while copying request [%+v]: %w",
					originalReq, err,
				), id: f.preferredComponent.id}
			return
		}

		// Get the URL (useful for logs and debugging)
		var url string
		if copiedReq.URL != nil {
			url = copiedReq.URL.String()
		} else {
			url = "nil"
		}

		// Try to run the preferred component first.
		// If it fails, we'll try the other components.
		flowResp := f.runComponent(f.preferredComponent, copiedReq)
		if flowResp.resp != nil && flowResp.err == nil {
			log.Debugf(
				"Preferred component [%v] returned good response for request [%v]. continuing with it",
				f.preferredComponent.id,
				url,
			)
			recvFlowRespCh <- flowResp
			// No need to activate the rest of the components
			return
		} else {
			// If the preferred component failed, we'll run the rest of the
			// components.
			log.Debugf(
				"Preferred component [%v] failed for request [%v]. continuing with the rest of the components",
				f.preferredComponent.id,
				url,
			)
			f.preferredComponent = nil
		}
	}

	// Now, run the rest of the components asynchronously
	for _, _comp := range f.components {
		comp := _comp
		// Copy the request
		originalReq, copiedReq, err := copyRequest(originalReq)
		if err != nil {
			recvFlowRespCh <- &ProxiedFlowResponse{
				resp: nil,
				err: fmt.Errorf(
					"flow.go:runComponents while copying request [%+v]: %w",
					originalReq, err,
				), id: comp.id}
			return
		}
		// Run the component
		go func() { recvFlowRespCh <- f.runComponent(comp, copiedReq) }()
	}
}
