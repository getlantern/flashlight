package proxied

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/getlantern/errors"
	"github.com/getlantern/flashlight/p2p"
)

const DebugTransportIdResponseHeader = "TransportId"

type TransportId string
type InterceptorFunc func(TransportId, *http.Client, *http.Request)

var (
	TransportId_P2p     TransportId = "p2p"
	TransportId_Fronted TransportId = "fronted"
)

func FrontedAndP2p(
	p2pCtx *p2p.CensoredP2pCtx,
	masqueradeTimeout time.Duration,
	addDebugHeaders bool,
	interceptorFuncs ...InterceptorFunc,
) http.RoundTripper {
	return &frontedP2pRoundTripper{
		p2pCtx:              p2pCtx,
		shouldRunInParallel: true,
		addDebugHeaders:     addDebugHeaders,
		masqueradeTimeout:   masqueradeTimeout,
		interceptorFunc:     interceptorFuncs[0],
	}
}

type frontedP2pRoundTripper struct {
	p2pCtx              *p2p.CensoredP2pCtx
	shouldRunInParallel bool
	// Masquerade timeout duration for Fronted requests
	masqueradeTimeout time.Duration
	// Used exclusively in tests.
	// If true, call 'resp.Header.Set(DebugTransportIdResponseHeader, TransportId_xxx)':
	// - where 'xxx' is the TransportId of the RoundTripper that did the request
	// - and 'resp' is the request's response
	addDebugHeaders bool
	// Used exclusively in tests
	// Intercepts and modifies a request to test different situations.
	interceptorFunc InterceptorFunc
}

func (conn *frontedP2pRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Define a couple of helper functions
	doRequestFunc := func(
		ctx context.Context,
		transportId TransportId,
		interceptorFunc InterceptorFunc,
		cl *http.Client,
		req *http.Request,
		respChan chan *http.Response,
		errChan chan error,
		addDebugHeaders bool,
	) {
		// Run request
		if interceptorFunc != nil {
			interceptorFunc(transportId, cl, req)
		}
		resp, err := cl.Do(req)
		// Check If context is cancelled before anything: if so, exit silently
		select {
		case <-ctx.Done():
			log.Debugf("Ignoring returned response after ctx is done")
			return
		default:
		}
		if err != nil {
			errChan <- fmt.Errorf("during roundtrip of request [%v]: %w",
				req.URL, err)
			return
		}
		if addDebugHeaders {
			resp.Header.Set(
				DebugTransportIdResponseHeader,
				string(transportId))
		}
		respChan <- resp
	}

	respChan := make(chan *http.Response)
	errChan := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	// Prep Fronted request
	reqFronted := req.WithContext(ctx)
	clFronted := &http.Client{
		Transport: frontedRT{masqueradeTimeout: conn.masqueradeTimeout}}
	// Prep P2p Request
	reqP2p := reqFronted.Clone(ctx)
	clP2p := &http.Client{Transport: conn.p2pCtx}

	// Run Requests
	go doRequestFunc(ctx,
		TransportId_Fronted,
		conn.interceptorFunc,
		clFronted, reqFronted,
		respChan, errChan, conn.addDebugHeaders)
	go doRequestFunc(ctx,
		TransportId_P2p,
		conn.interceptorFunc,
		clP2p, reqP2p,
		respChan, errChan, conn.addDebugHeaders)

	// See whichever one comes first: the error or the response
	// - If we get a response
	//   - Cancel the context and close the channel: we don't need the 2nd
	//     roundtripper's response
	// - If we get an error:
	//   - And this is the 1st error, log it and loop again: the 2nd request is
	//     still pending
	//   - And this is the 2nd error, cancel everything and leave: both
	//     requests failed
	maxNumOfErrors := 2 // equal to number of RoundTrippers we have
looper:
	select {
	case err := <-errChan:
		if err != nil {
			log.Errorf("while waiting for roundtrippers: %v", err)
			maxNumOfErrors--
			if maxNumOfErrors == 0 {
				cancel()
				close(respChan)
				close(errChan)
				return nil, errors.New("All attempts failed")
			}
			goto looper
		}
	case resp := <-respChan:
		// We got one successful response: the 2nd doesn't matter. Cancel it
		// and leave
		cancel()
		close(respChan)
		close(errChan)
		return resp, nil
	}
	panic("unreachable code")
}
