package chained

import (
	"context"
	"fmt"
	"net"

	"github.com/getlantern/lampshade"
	"github.com/opentracing/opentracing-go"

	otlog "github.com/opentracing/opentracing-go/log"
)

func newLampshadeLifecycleListener(proxyName string) lampshade.ClientLifecycleListener {
	return &lifecycleListener{
		proxyName: proxyName,
		ctx:       context.Background(),
	}
}

func newLampshadeSessionListener(ctx context.Context) lampshade.SessionLifecycleListener {
	return &sessionLifecycleListener{
		ctx: ctx,
	}
}

func newLampshadeStreamListener(ctx context.Context, ll *sessionLifecycleListener) lampshade.StreamLifecycleListener {
	return &streamListener{
		ctx: ctx,
		ll:  ll,
	}
}

type hostKey string

var upstreamHostKey = hostKey("uhk")

type lifecycleListener struct {
	proxyName string
	ctx       context.Context
}

type sessionLifecycleListener struct {
	ctx context.Context
}

type streamListener struct {
	ctx context.Context
	ll  *sessionLifecycleListener
}

func (ll *lifecycleListener) OnStart() {
	span := opentracing.StartSpan("lampshade-" + ll.proxyName)
	defer span.Finish()
	ll.ctx = opentracing.ContextWithSpan(ll.ctx, span)
}

func (ll *lifecycleListener) OnTCPStart() lampshade.SessionLifecycleListener {
	_, ctx := opentracing.StartSpanFromContext(ll.ctx, "lampshade-TCP-"+ll.proxyName)
	return newLampshadeSessionListener(ctx)
}

func (sll *sessionLifecycleListener) OnSessionInit() {}

func (sll *sessionLifecycleListener) OnTCPConnectionError(err error) {
	if span := opentracing.SpanFromContext(sll.ctx); span != nil {
		// We finish the span here because otherwise child spans will be orphaned.
		defer span.Finish()
		span.SetOperationName("tcp-error-" + err.Error())
		span.SetTag("error", "tcp")
	}
}
func (sll *sessionLifecycleListener) OnTCPEstablished(conn net.Conn) {
	if span := opentracing.SpanFromContext(sll.ctx); span != nil {
		// We finish the span here because otherwise child spans will be orphaned.
		defer span.Finish()
		span.SetTag("established", true)
		span.LogFields(otlog.String("established", "true"))
	}
}

func (sll *sessionLifecycleListener) OnTCPClosed() {
	if span := opentracing.SpanFromContext(sll.ctx); span != nil {
		span.LogFields(otlog.String("closed", "true"))
	}
}
func (sll *sessionLifecycleListener) OnClientInitWritten() {}

func (sll *sessionLifecycleListener) OnSessionError(err1 error, err2 error) {
	if span := opentracing.SpanFromContext(sll.ctx); span != nil {
		logError(span, "sessionerror", err1)
		logError(span, "sessionerror", err2)
		span.SetTag("sessionerror", "true")
	}
}

func (sll *sessionLifecycleListener) OnStreamStart(httpContext context.Context, id uint16) lampshade.StreamLifecycleListener {
	if parentSpan := opentracing.SpanFromContext(sll.ctx); parentSpan != nil {
		origin := httpContext.Value(upstreamHostKey)
		opts := []opentracing.StartSpanOption{opentracing.ChildOf(parentSpan.Context())}
		span := opentracing.StartSpan(fmt.Sprintf("stream-%v-%v", id, origin), opts...)
		ctx := opentracing.ContextWithSpan(sll.ctx, span)
		return newLampshadeStreamListener(ctx, sll)
	}
	return lampshade.NoopStreamLifecycleListener()
}

func (ls *streamListener) OnStreamRead(num int) {
	if span := opentracing.SpanFromContext(ls.ctx); span != nil {
		span.LogFields(otlog.Int("r", num))
	}
}

func (ls *streamListener) OnStreamWrite(num int) {
	if span := opentracing.SpanFromContext(ls.ctx); span != nil {
		span.LogFields(otlog.Int("w", num))
	}
}

func (ls *streamListener) OnStreamReadError(err error) {
	if span := opentracing.SpanFromContext(ls.ctx); span != nil {
		logError(span, "ioerror", err)
		span.SetTag("ioerror", "true")
	}
}

func (ls *streamListener) OnStreamWriteError(err error) {
	if span := opentracing.SpanFromContext(ls.ctx); span != nil {
		logError(span, "ioerror", err)
		span.SetTag("ioerror", "true")
	}
}

func (ls *streamListener) OnStreamClose() {
	if span := opentracing.SpanFromContext(ls.ctx); span != nil {
		span.Finish()
	}
}

func logError(span opentracing.Span, name string, err error) {
	if err != nil {
		span.LogFields(otlog.String(name, err.Error()))
	}
}
