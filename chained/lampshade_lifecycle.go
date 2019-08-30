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
	return &lampshadeLifecycleListener{
		proxyName: proxyName,
	}
}

func newLampshadeStreamListener(ctx context.Context, ll *lampshadeLifecycleListener) lampshade.StreamLifecycleListener {
	return &lampshadeStreamListener{
		ctx: ctx,
		ll:  ll,
	}
}

type hostKey string

var upstreamHostKey = hostKey("uhk")

type lampshadeLifecycleListener struct {
	proxyName    string
	totalRead    int64
	totalWritten int64
}

type lampshadeStreamListener struct {
	ctx context.Context
	ll  *lampshadeLifecycleListener
}

func (ll *lampshadeLifecycleListener) OnStart(ctx context.Context) context.Context {
	span := opentracing.StartSpan("lampshade-" + ll.proxyName)
	defer span.Finish()
	return opentracing.ContextWithSpan(ctx, span)
}

func (ll *lampshadeLifecycleListener) OnSessionInit(ctx context.Context) context.Context {
	return ctx
}

func (ll *lampshadeLifecycleListener) OnTCPStart(ctx context.Context) context.Context {
	if parentSpan := opentracing.SpanFromContext(ctx); parentSpan != nil {
		opts := []opentracing.StartSpanOption{opentracing.ChildOf(parentSpan.Context())}
		span := opentracing.StartSpan("lampshade-TCP-"+ll.proxyName, opts...)
		return opentracing.ContextWithSpan(ctx, span)
	}
	return ctx
}

func (ll *lampshadeLifecycleListener) OnTCPConnectionError(ctx context.Context, err error) context.Context {
	if span := opentracing.SpanFromContext(ctx); span != nil {
		// We finish the span here because otherwise child spans will be orphaned.
		defer span.Finish()
		span.SetOperationName("tcp-error-" + err.Error())
		span.SetTag("error", "tcp")
	}
	return ctx
}
func (ll *lampshadeLifecycleListener) OnTCPEstablished(ctx context.Context, conn net.Conn) context.Context {
	if span := opentracing.SpanFromContext(ctx); span != nil {
		// We finish the span here because otherwise child spans will be orphaned.
		defer span.Finish()
		span.SetTag("established", true)
		span.LogFields(otlog.String("established", "true"))
	}
	return ctx
}

func (ll *lampshadeLifecycleListener) OnTCPClosed(ctx context.Context) context.Context {
	if span := opentracing.SpanFromContext(ctx); span != nil {
		span.LogFields(otlog.String("closed", "true"))
	}
	return ctx
}
func (ll *lampshadeLifecycleListener) OnClientInitWritten(ctx context.Context) context.Context {
	return ctx
}

func (ll *lampshadeLifecycleListener) OnSessionError(ctx context.Context, err1 error, err2 error) context.Context {
	return ctx
}

func (ll *lampshadeLifecycleListener) OnStreamInit(lampshadeContext context.Context, httpContext context.Context, id uint16) lampshade.StreamLifecycleListener {
	if parentSpan := opentracing.SpanFromContext(lampshadeContext); parentSpan != nil {
		origin := httpContext.Value(upstreamHostKey)
		opts := []opentracing.StartSpanOption{opentracing.ChildOf(parentSpan.Context())}
		span := opentracing.StartSpan(fmt.Sprintf("stream-%v-%v", id, origin), opts...)
		lampshadeContext = opentracing.ContextWithSpan(lampshadeContext, span)
		return newLampshadeStreamListener(lampshadeContext, ll)
	}
	return lampshade.NoopStreamLifecycleListener()
}

func (ls *lampshadeStreamListener) OnStreamRead(num int) context.Context {
	if span := opentracing.SpanFromContext(ls.ctx); span != nil {
		span.LogFields(otlog.Int("r", num))
	}
	return ls.ctx
}

func (ls *lampshadeStreamListener) OnStreamWrite(num int) context.Context {
	if span := opentracing.SpanFromContext(ls.ctx); span != nil {
		span.LogFields(otlog.Int("w", num))
	}
	return ls.ctx
}

func (ls *lampshadeStreamListener) OnStreamClose() context.Context {
	if span := opentracing.SpanFromContext(ls.ctx); span != nil {
		span.Finish()
	}
	return ls.ctx
}
