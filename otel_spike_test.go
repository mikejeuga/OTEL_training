package main

import (
	"context"
	"github.com/adamluzsi/testcase/assert"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	traceSDK "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newExporter(w io.Writer) (traceSDK.SpanExporter, error) {
	return stdouttrace.New(
		stdouttrace.WithWriter(w),
		// Use human-readable output.
		stdouttrace.WithPrettyPrint(),
		// Do not print timestamps for the demo.
		stdouttrace.WithoutTimestamps(),
	)
}

func TestSpikeOTEL(t *testing.T) {
	var spanExporter traceSDK.SpanExporter = tracetest.NewNoopExporter()
	spanExporter = newSpyExporter(t)

	tracerProvider := traceSDK.NewTracerProvider(
		traceSDK.WithSpanProcessor(&SpySpanProcessor{TB: t,
			SpanProcessor: traceSDK.NewSimpleSpanProcessor(&SpySpanExporter{TB: t,
				SpanExporter: spanExporter})}))

	propagator := propagation.TraceContext{}

	t.Run("WIP", func(t *testing.T) {
		defer tracerProvider.ForceFlush(context.Background())
		ExportWIP(t, tracerProvider, propagator)
	})
	t.Run("ok", func(t *testing.T) {
		defer tracerProvider.ForceFlush(context.Background())
		ExportWorks(t, tracerProvider, propagator)
	})
	t.Run("nok", func(t *testing.T) {
		defer tracerProvider.ForceFlush(context.Background())
		ExportDoesNotWorksV1(t, tracerProvider, propagator)
	})
	t.Run("ExportDoesNotWorksV2", func(t *testing.T) {
		defer tracerProvider.ForceFlush(context.Background())
		ExportDoesNotWorksV2(t, tracerProvider, propagator)
	})
}

func ExportWorks(tb testing.TB, tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator) {
	ctx := context.Background()

	ctx, span := tracerProvider.
		Tracer("name").        // TODO: check tracer options
		Start(ctx, "spanName") // TODO: check span options
	defer span.End() // TODO: check end options?

	debugSpan(tb, ctx)

	span.AddEvent("test this out")

	tb.Log(trace.SpanContextFromContext(ctx).TraceID().String())
}

func ExportWIP(tb testing.TB, tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator) {
	ctx := context.Background()

	tID, sID := newTraceID()
	inboundRequestMeta := FakeCarrier{"traceparent": traceIDToHeader(tID, sID)}
	ctx = propagator.Extract(ctx, inboundRequestMeta)

	tb.Log(trace.SpanContextFromContext(ctx).TraceID().String())

	ctx, span := tracerProvider.
		Tracer("name").        // TODO: check tracer options
		Start(ctx, "spanName") // TODO: check span options
	defer span.End() // TODO: check end options?

	tb.Log(trace.SpanContextFromContext(ctx).TraceID().String())

	span.AddEvent("test-asdf")

	outboundRequest := FakeCarrier{}
	propagator.Inject(ctx, outboundRequest)
	tb.Logf("%#v", outboundRequest)
}

func ExportDoesNotWorksV1(tb testing.TB, tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator) {
	ctx := context.Background()

	tID, sID := newTraceID()
	inboundRequestMeta := FakeCarrier{"traceparent": traceIDToHeader(tID, sID)}
	ctx = propagator.Extract(ctx, inboundRequestMeta)

	tb.Log(trace.SpanContextFromContext(ctx).TraceID().String())

	ctx, span := tracerProvider.
		Tracer("name").        // TODO: check tracer options
		Start(ctx, "spanName") // TODO: check span options
	defer span.End() // TODO: check end options?

	tb.Log(trace.SpanContextFromContext(ctx).TraceID().String())

	span.AddEvent("test-asdf")

	outboundRequest := FakeCarrier{}
	propagator.Inject(ctx, outboundRequest)
	tb.Logf("%#v", outboundRequest)
}

func ExportDoesNotWorksV2(tb testing.TB, tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator) {
	ctx := context.Background()

	ctx, span := tracerProvider.
		Tracer("name").        // TODO: check tracer options
		Start(ctx, "spanName") // TODO: check span options
	defer span.End() // TODO: check end options?

	tID, sID := newTraceID()
	inboundRequestMeta := FakeCarrier{"traceparent": traceIDToHeader(tID, sID)}
	ctx = propagator.Extract(ctx, inboundRequestMeta)

	tb.Logf("%#v", trace.SpanFromContext(ctx))

	debugSpan(tb, ctx)
	tb.Log(trace.SpanContextFromContext(ctx).TraceID().String())

	outboundRequest := FakeCarrier{}
	propagator.Inject(ctx, outboundRequest)

	assert.Must(tb).Contain(outboundRequest[headerKey], tID.String())
	assert.Must(tb).Contain(outboundRequest[headerKey], sID.String())

	tb.Logf("traceID:%s spanID:%s", tID, sID)
	tb.Log("outbound request tracing, and expected tracingID", outboundRequest[headerKey], tID.String())

	tb.Logf("%#v", outboundRequest)
}

func DummyTraceStartOpts() []trace.SpanStartOption {
	r := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	return []trace.SpanStartOption{
		trace.WithAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...),
		trace.WithAttributes(semconv.EndUserAttributesFromHTTPRequest(r)...),
		trace.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest("serviceName", "/users/{userID}", r)...),
		trace.WithSpanKind(trace.SpanKindServer),
	}
}

func debugSpan(tb testing.TB, ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		tb.Log("no span found in the current context")
		return
	}
	tb.Logf("%T %#v", span, span)
}

type FakeCarrier map[string]string

func (c FakeCarrier) Get(key string) string {
	return c[key]
}

func (c FakeCarrier) Set(key string, value string) {
	c[key] = value
}

func (c FakeCarrier) Keys() []string {
	var keys []string
	for k, _ := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// pkg agstracing
//func LookupTracingID(ctx context.Context) (string, bool)

// pkg agstracing/httpadapter
// -> httpadapter.NewRoundTripper
// ->
