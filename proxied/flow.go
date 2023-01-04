// This file focuses on a structure called ProxiedFlow. It's useful for us to
// run multiple roundtrippers, possibly in parallel, and have a "preference"
// for one or more of them.
//
// To breakdown this concept further, let's go with an example: Assume we want
// to run a request through our Lantern proxies (called the "chained
// roundtripper") **and** through domain fronting (called "fronted"
// roundtripper) where the fastest response is taken.
//
// Let's also assume we want to have a preference for "chained roundtripper",
// meaning that if running a request through the "chained roundtripper" is
// working relatively well (as opposed to the "fronted roundtripper"
// in this example), the **next** request you run will automatically go through
// "chained", and we wouldn't bother "fronted" roundtripper, unless it fails.
//
// The code for this example will look like this:
//
//     req, err := http.NewRequest("GET", "http://example.com", nil)
//     flow := NewProxiedFlow(
//         &ProxiedFlowOptions{
//             ParallelMethods: []{http.MethodGet, http.MethodHead, http.MethodOptions},
//     	   })
//
// 	   flow.
// 	     Add(proxied.FlowComponentID_Chained, true).
//       Add(proxied.FlowComponentID_Fronted, false)
//     resp, err := flow.RoundTrip(req)
//
// TODO: there is no support for ops tracing
//
package proxied

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const anyHTTPMethodMarker = "Any"

type FlowComponentID string

// Enum of most used roundtrippers
var (
	FlowComponentID_None     FlowComponentID = ""
	FlowComponentID_Direct   FlowComponentID = "direct"
	FlowComponentID_P2P      FlowComponentID = "p2p"
	FlowComponentID_Fronted  FlowComponentID = "fronted"
	FlowComponentID_Chained  FlowComponentID = "chained"
	FlowComponentID_Broflake FlowComponentID = "broflake"

	registry            *componentRegistry
	componentNotEnabled = errors.New("component not enabled")
	proxiedFlowEnabled  uint64

	AnyMethod = []string{anyHTTPMethodMarker}
)

func init() {
	registry = &componentRegistry{
		roundTrippers: make(map[FlowComponentID]http.RoundTripper),
	}
	registry.enableComponent(FlowComponentID_Direct, &http.Transport{})
	registry.enableComponent(FlowComponentID_Chained, &chainedRoundTripper{})
	registry.enableComponent(FlowComponentID_Fronted, Fronted(DefaultMasqueradeTimeout))

	forces := 0
	for _, id := range []FlowComponentID{FlowComponentID_P2P, FlowComponentID_Fronted, FlowComponentID_Chained, FlowComponentID_Broflake} {
		exclusive, _ := strconv.ParseBool(os.Getenv(fmt.Sprintf("FORCE_%s", strings.ToUpper(string(id)))))
		if exclusive {
			log.Debugf("Forcing exclusive use of %s for ProxiedFlow", id)
			registry.exclusivelyUse(id)
			forces += 1
		}
	}
	// also accept this for domain fronting
	frontOnly, _ := strconv.ParseBool(os.Getenv(forceDF))
	if frontOnly {
		log.Debugf("Forcing exclusive use of %s for ProxiedFlow", FlowComponentID_Fronted)
		registry.exclusivelyUse(FlowComponentID_Fronted)
		forces += 1
	}
	if forces > 1 {
		log.Debugf("Warning, multiple 'exclusive' methods requested for ProxiedFlow (using %s)", registry.exclusive)
	}
}

// Enables use of ProxiedFlow roundtrippers where they are optional
func SetProxiedFlowFeatureEnabled(enabled bool) {
	if enabled {
		atomic.StoreUint64(&proxiedFlowEnabled, 1)
		log.Debugf("ProxiedFlow roundtrippers enabled.")
	} else {
		atomic.StoreUint64(&proxiedFlowEnabled, 0)
		log.Debugf("ProxiedFlow roundtrippers disabled.")
	}
}

// MaybeFlowRoundTripper is an http.RoundTripper that uses
// a ProxiedFlow RoundTripper when the ProxiedFlow feature is
// enabled, and a default RoundTripper otherwise.
type MaybeProxiedFlowRoundTripper struct {
	Default http.RoundTripper
	Flow    http.RoundTripper
}

func (rt *MaybeProxiedFlowRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if atomic.LoadUint64(&proxiedFlowEnabled) == 1 {
		return rt.Flow.RoundTrip(req)
	} else {
		return rt.Default.RoundTrip(req)
	}
}

type noDefaultRoundTripper struct {
	id FlowComponentID
}

func (rt *noDefaultRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("no default RoundTripper for %v", rt.id)
}

// EnableComponent enables use of the flow component specified in any ProxiedFlow that
// it was Add()'ed to.  If non-nil, the given RoundTripper is registered as the default
// RoundTripper for the type.
func EnableComponent(id FlowComponentID, rt http.RoundTripper) error {
	if rt == nil {
		rt = &noDefaultRoundTripper{id: id}
	}
	return registry.enableComponent(id, rt)
}

func ExclusivelyUse(id FlowComponentID) {
	registry.exclusivelyUse(id)
}

type componentRegistry struct {
	roundTrippers map[FlowComponentID]http.RoundTripper
	exclusive     FlowComponentID
	mx            sync.RWMutex
}

func (r *componentRegistry) enableComponent(id FlowComponentID, rt http.RoundTripper) error {
	r.mx.Lock()
	defer r.mx.Unlock()
	r.roundTrippers[id] = rt
	return nil
}

func (r *componentRegistry) exclusivelyUse(id FlowComponentID) {
	r.mx.Lock()
	r.exclusive = id
	r.mx.Unlock()
}

func (r *componentRegistry) clear() {
	r.mx.Lock()
	r.exclusive = FlowComponentID_None
	r.roundTrippers = make(map[FlowComponentID]http.RoundTripper)
	r.mx.Unlock()
}

func (r *componentRegistry) defaultRoundTripper(id FlowComponentID) (http.RoundTripper, error) {
	r.mx.RLock()
	defer r.mx.RUnlock()
	if r.exclusive != FlowComponentID_None && r.exclusive != id {
		return nil, fmt.Errorf("%v: %w", id, componentNotEnabled)
	}
	rt := r.roundTrippers[id]
	if rt == nil {
		return nil, fmt.Errorf("%v: %w", id, componentNotEnabled)
	}
	return rt, nil
}

func (r *componentRegistry) isEnabled(id FlowComponentID) bool {
	rt, err := r.defaultRoundTripper(id)
	if rt == nil || errors.Is(err, componentNotEnabled) {
		return false
	}
	return true
}

func (id FlowComponentID) String() string {
	return string(id)
}

// ProxiedFlowComponent is basically a wrapper around an http.RoundTripper that
// includes an ID and some additional flags
type ProxiedFlowComponent struct {
	id           FlowComponentID
	shouldPrefer bool
	rt           http.RoundTripper
}

// ProxiedFlowResponse is a wrapper around an http.Response and an error, both
// coming from ProxiedFlowComponent.RoundTrip()
type ProxiedFlowResponse struct {
	id      FlowComponentID
	resp    *http.Response
	err     error
	elapsed time.Duration
}

func (r *ProxiedFlowResponse) IsFailure() bool {
	if r.err != nil || r.resp == nil {
		return true
	}
	status := r.resp.StatusCode
	if status < 200 || (status >= 400 && status != http.StatusUpgradeRequired) {
		return true
	}
	return false
}

func (r *ProxiedFlowResponse) Close() {
	if r.resp != nil && r.resp.Body != nil {
		r.resp.Body.Close()
	}
}

// ProxiedFlowOptions is the input to NewProxiedFlow()
type ProxiedFlowOptions struct {
	// allows parallel execution for these HTTP request methods
	ParallelMethods []string
}

type ProxiedFlow struct {
	// List of components in the flow
	components []*ProxiedFlowComponent
	options    *ProxiedFlowOptions

	// Most preferred component. Can be nil, which means either that no
	// component wants to be preferred or that this flow was never run
	// successfully before
	preferredComponent   *ProxiedFlowComponent
	preferredComponentMx sync.RWMutex
}

// NewProxiedFlow returns a new ProxiedFlow
func NewProxiedFlow(options *ProxiedFlowOptions) *ProxiedFlow {
	return &ProxiedFlow{options: options}
}

// Add adds an allowed strategy for roundtripping http requests to the flow.
// The default RoundTripper registered for the component type is used to make
// requests.
//
// If shouldPrefer is true, the component may become the only
// request method used when it is succeeding.  Preferred components
// should be added first before fallback alternatives.
//
// If the component specified is not enabled at the time a request is made
// it is skipped.
func (f *ProxiedFlow) Add(
	id FlowComponentID,
	shouldPrefer bool,
) *ProxiedFlow {
	f.components = append(f.components, &ProxiedFlowComponent{
		id:           id,
		shouldPrefer: shouldPrefer,
	})
	return f
}

// Add adds an allowed strategy for roundtripping http requests to the flow.
// The sepcific RoundTripper provided is used to make requests. If the
// component type is enabled.
//
// If shouldPrefer is true, the component may become the only
// request method used when it is succeeding.  Preferred components
// should be added first before fallback alternatives.
// If the component specified is not enabled at the time a request is made
// it is skipped.
func (f *ProxiedFlow) AddWithRoundTripper(
	id FlowComponentID,
	rt http.RoundTripper,
	shouldPrefer bool,
) *ProxiedFlow {
	f.components = append(f.components, &ProxiedFlowComponent{
		id:           id,
		rt:           rt,
		shouldPrefer: shouldPrefer,
	})
	return f
}

// SetPreferredComponent sets the component with "id" as the currently
// preferred component.  It will be tried first when it is succeeding.
// t will be removed as the preferred component if it is failing.
// This function doesn't fail if the component doesn't exist.
//
// Return *ProxiedFlow to chain function calls in a builder pattern.
func (f *ProxiedFlow) SetPreferredComponent(id FlowComponentID) *ProxiedFlow {
	for _, c := range f.components {
		if c.id == id {
			f.preferredComponentMx.Lock()
			f.preferredComponent = c
			f.preferredComponentMx.Unlock()
			break
		}
	}
	return f
}

// SetPreferredComponentIfEmpty sets the component with "id" as the currently
// preferred component if there is no current preferred component.
// It will be tried first when it is succeeding.
// t will be removed as the preferred component if it is failing.
// This function doesn't fail if the component doesn't exist.
//
// Return *ProxiedFlow to chain function calls in a builder pattern.
func (f *ProxiedFlow) SetPreferredComponentIfEmpty(id FlowComponentID) *ProxiedFlow {
	for _, c := range f.components {
		if c.id == id {
			f.preferredComponentMx.Lock()
			if f.preferredComponent == nil {
				f.preferredComponent = c
			}
			f.preferredComponentMx.Unlock()
			break
		}
	}
	return f
}

// ClearPreferredComponent removes any previously set preferred component
//
// Return *ProxiedFlow to chain function calls in a builder pattern.
func (f *ProxiedFlow) ClearPreferredComponent() *ProxiedFlow {
	f.preferredComponentMx.Lock()
	f.preferredComponent = nil
	f.preferredComponentMx.Unlock()
	return f
}

// ClearPreferredComponentIfIDMatches clears the preferred component if the current
// preferred component has the given id.
//
// Return *ProxiedFlow to chain function calls in a builder pattern.
func (f *ProxiedFlow) ClearPreferredComponentIfIDMatches(id FlowComponentID) *ProxiedFlow {
	f.preferredComponentMx.Lock()
	if f.preferredComponent != nil && f.preferredComponent.id == id {
		f.preferredComponent = nil
	}
	f.preferredComponentMx.Unlock()
	return f
}

func (f *ProxiedFlow) PreferredComponent() *ProxiedFlowComponent {
	f.preferredComponentMx.RLock()
	defer f.preferredComponentMx.RUnlock()
	return f.preferredComponent
}

// Returns preferred component, other components
// if a component is not enabled, it is omitted from the result.
func (f *ProxiedFlow) enabledComponents() (*ProxiedFlowComponent, []*ProxiedFlowComponent) {
	preferred := f.PreferredComponent()
	if preferred != nil && !preferred.isEnabled() {
		preferred = nil
	}
	rest := make([]*ProxiedFlowComponent, 0)
	for _, c := range f.components {
		if c != preferred && c.isEnabled() {
			rest = append(rest, c)
		}
	}
	return preferred, rest
}

func (f ProxiedFlow) ShouldRunParallel(req *http.Request) bool {
	for _, method := range f.options.ParallelMethods {
		if method == anyHTTPMethodMarker {
			return true
		}
		if method == req.Method {
			return true
		}
	}
	return false
}

// RoundTrip makes ProxiedFlow implement the http.RoundTripper interface.
// This function respects the request's original context
func (f *ProxiedFlow) RoundTrip(req *http.Request) (*http.Response, error) {
	ch := make(chan *ProxiedFlowResponse, 1)
	go func() {
		flow := newFlowRunner(req, f)
		ch <- flow.Run()
	}()
	select {
	case r := <-ch:
		return r.resp, r.err
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

func (comp *ProxiedFlowComponent) run(f *flowRunner) *ProxiedFlowResponse {
	var err error
	rt := comp.rt
	if rt == nil {
		rt, err = registry.defaultRoundTripper(comp.id)
		if err != nil {
			return &ProxiedFlowResponse{err: err, id: comp.id}
		}
	}
	req := f.copyRequest()
	start := time.Now()
	resp, err := rt.RoundTrip(req)
	elapsed := time.Since(start)
	if resp == nil {
		err = errors.New("no response")
	}
	if err != nil {
		err = fmt.Errorf("%v.RoundTrip %v: %w", comp.id, req.URL, err)
	}

	return &ProxiedFlowResponse{
		resp:    resp,
		err:     err,
		id:      comp.id,
		elapsed: elapsed}
}

func (comp *ProxiedFlowComponent) isEnabled() bool {
	return registry.isEnabled(comp.id)
}

// a flowRunner executes a single request using the strategies
// configured in a ProxiedFlow.  A new flowRunner is
// created for each request processed by a ProxiedFlow.
type flowRunner struct {
	firstTime   uint64
	originalReq *http.Request
	mx          sync.Mutex
	body        []byte
	proxiedFlow *ProxiedFlow
}

func newFlowRunner(req *http.Request, p *ProxiedFlow) *flowRunner {
	return &flowRunner{
		originalReq: req,
		proxiedFlow: p,
	}
}

// run picks the appropriate strategy for executing the request
// based on request method and other state
func (f *flowRunner) Run() *ProxiedFlowResponse {
	if err := f.readRequestBody(); err != nil {
		return &ProxiedFlowResponse{err: err}
	}

	preferred, rest := f.proxiedFlow.enabledComponents()
	if preferred == nil && len(rest) == 0 {
		return &ProxiedFlowResponse{err: fmt.Errorf("no components enabled to perform request.")}
	}

	if preferred != nil {
		resp := f.runComponent(preferred)
		if !resp.IsFailure() {
			return resp
		}
	}

	if f.proxiedFlow.ShouldRunParallel(f.originalReq) {
		return f.runParallel(rest)
	}
	return f.runSequential(rest)
}

// runParallel runs all components at once and returns
// the first non-error response or an error if all
// components fail.
func (f *flowRunner) runParallel(components []*ProxiedFlowComponent) *ProxiedFlowResponse {
	if len(components) == 0 {
		return &ProxiedFlowResponse{err: fmt.Errorf("no components")}
	}

	results := make(chan *ProxiedFlowResponse, len(components))
	first := make(chan *ProxiedFlowResponse, 1)

	for _, _comp := range components {
		comp := _comp
		go func() {
			results <- f.runComponent(comp)
		}()
	}

	go func() {
		rs := []*ProxiedFlowResponse{}
		sent := false

		for i := 0; i < len(components); i++ {
			res := <-results
			if !sent && !res.IsFailure() {
				first <- res
				sent = true
			} else {
				rs = append(rs, res)
			}
		}

		if !sent {
			first <- rs[0]
			rs = rs[1:]
		}
		for _, r := range rs {
			r.Close()
		}
	}()

	return <-first
}

// runSequential tries each of the components
// in order until a sucessful response is received or all
// components fail.
func (f *flowRunner) runSequential(components []*ProxiedFlowComponent) *ProxiedFlowResponse {
	r := &ProxiedFlowResponse{err: componentNotEnabled}
	for _, comp := range components {
		r.Close() // prior Response if any
		r = f.runComponent(comp)
		if !r.IsFailure() {
			return r
		}
	}
	return r
}

func (f *flowRunner) runComponent(comp *ProxiedFlowComponent) *ProxiedFlowResponse {
	r := comp.run(f)
	f.updatePreferred(comp, r)
	return r
}

func (f *flowRunner) updatePreferred(comp *ProxiedFlowComponent, r *ProxiedFlowResponse) {
	if r.IsFailure() {
		f.proxiedFlow.ClearPreferredComponentIfIDMatches(comp.id)
	} else {
		elapsed := uint64(r.elapsed)
		atomic.CompareAndSwapUint64(&f.firstTime, 0, elapsed)
		if comp.shouldPrefer {
			firstTime := atomic.LoadUint64(&f.firstTime)
			// 3 is arbitraily chosen to check if preferred speed is reasonable
			// compared to best/first time seen for a success.
			if elapsed <= 3*firstTime {
				f.proxiedFlow.SetPreferredComponentIfEmpty(comp.id)
			}
		}
	}
}

func (f *flowRunner) readRequestBody() error {
	req := f.originalReq
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("while reading request body: %w", err)
		}
		_ = req.Body.Close()
		f.body = b
	}
	return nil
}

// copyRequest creates a new copy of the original request
// readRequestBody should be called before calling this.
func (f *flowRunner) copyRequest() *http.Request {
	f.mx.Lock()
	req := f.originalReq.Clone(f.originalReq.Context())
	f.mx.Unlock()
	if len(f.body) > 0 {
		req.Body = io.NopCloser(bytes.NewReader(f.body))
	}
	return req
}
