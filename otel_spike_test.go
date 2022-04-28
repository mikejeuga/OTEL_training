package main

import (
	"context"
	"io"
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
	tracerProvider := traceSDK.NewTracerProvider(
		traceSDK.WithSpanProcessor(&SpySpanProcessor{TB: t,
			SpanProcessor: traceSDK.NewSimpleSpanProcessor(&SpySpanExporter{TB: t,
				SpanExporter: tracetest.NewNoopExporter()})}))

	propagator := propagation.TraceContext{}

	t.Run("ExportWorks", func(t *testing.T) {
		defer tracerProvider.ForceFlush(context.Background())
		ExportWorks(t, tracerProvider, propagator)
	})

	t.Run("ExportDoesNotWorks", func(t *testing.T) {
		defer tracerProvider.ForceFlush(context.Background())
		ExportDoesNotWorks(t, tracerProvider, propagator)
	})
}

func ExportWorks(tb testing.TB, tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator) {
	ctx := context.Background()

	ctx, span := tracerProvider.
		Tracer("name").        // TODO: check tracer options
		Start(ctx, "spanName") // TODO: check span options
	defer span.End() // TODO: check end options?

	tb.Log(trace.SpanContextFromContext(ctx).TraceID().String())
}

func ExportDoesNotWorks(tb testing.TB, tracerProvider trace.TracerProvider, propagator propagation.TextMapPropagator) {
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

	outboundRequest := FakeCarrier{}
	propagator.Inject(ctx, outboundRequest)
	tb.Logf("%#v", outboundRequest)
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
