package main

import (
	"bytes"
	"context"
	"github.com/adamluzsi/testcase/assert"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	traceSDK "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"io"
	"log"
	"testing"
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
	ctx := context.Background()

	buf := &bytes.Buffer{}
	l := log.New(buf, "", 0)
	spanExporter, _ := newExporter(l.Writer())

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceNameKey.String("ags-test")))
	assert.Must(t).Nil(err)

	tracerProvider := traceSDK.NewTracerProvider(
		traceSDK.WithSpanProcessor(&DebugSpanProcessor{TB: t,
			SpanProcessor: traceSDK.NewSimpleSpanProcessor(&DebugSpanExporter{TB: t,
				SpanExporter: spanExporter})}),
		traceSDK.WithResource(res),
	)

	func() {
		// we never investigated the options they pass to the .Tracer call.
		// hmmm
		_, span := tracerProvider.
			Tracer("name").        // TODO: check tracer options
			Start(ctx, "spanName") // TODO: check span options

		defer span.End() // TODO: check end options?
	}()

	t.Log(buf.String())
}

// pkg agstracing
//func LookupTracingID(ctx context.Context) (string, bool)

// pkg agstracing/httpadapter
// -> httpadapter.NewRoundTripper
// ->
