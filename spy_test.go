package main

import (
	"bytes"
	"context"
	"github.com/adamluzsi/testcase/assert"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"testing"

	traceSDK "go.opentelemetry.io/otel/sdk/trace"
)

// traceSDK.SpanExporter
type SpySpanExporter struct {
	TB           testing.TB
	SpanExporter traceSDK.SpanExporter
}

var _ traceSDK.SpanExporter = &SpySpanExporter{}

func (exp SpySpanExporter) ExportSpans(ctx context.Context, spans []traceSDK.ReadOnlySpan) error {
	exp.TB.Helper()
	exp.TB.Logf("SpySpanExporter.ExportSpans:  %#v", spans)
	return exp.SpanExporter.ExportSpans(ctx, spans)
}

func (exp SpySpanExporter) Shutdown(ctx context.Context) error {
	exp.TB.Helper()
	exp.TB.Logf("SpySpanExporter is shutting down")
	return exp.SpanExporter.Shutdown(ctx)
}

type SpySpanProcessor struct {
	testing.TB
	traceSDK.SpanProcessor
}

func (sp *SpySpanProcessor) OnStart(parent context.Context, s traceSDK.ReadWriteSpan) {
	sp.TB.Helper()
	sp.TB.Logf("SpySpanProcessor.OnStart: %#v", s)
	sp.SpanProcessor.OnStart(parent, s)
}

func (sp *SpySpanProcessor) OnEnd(s traceSDK.ReadOnlySpan) {
	sp.TB.Helper()
	sp.TB.Logf("SpySpanProcessor.OnEnd: %#v", s)
	sp.SpanProcessor.OnEnd(s)
}

func (sp *SpySpanProcessor) Shutdown(ctx context.Context) error {
	sp.TB.Helper()
	sp.TB.Logf("SpySpanProcessor.Shutdown: %#v", ctx)
	return sp.SpanProcessor.Shutdown(ctx)
}

func (sp *SpySpanProcessor) ForceFlush(ctx context.Context) error {
	sp.TB.Helper()
	sp.TB.Logf("SpySpanProcessor.ForceFlush: %#v", ctx)
	return sp.SpanProcessor.ForceFlush(ctx)
}

func newSpyExporter(tb testing.TB) traceSDK.SpanExporter {
	tb.Helper()
	buf := &bytes.Buffer{}
	tb.Cleanup(func() {
		tb.Helper()
		tb.Logf("\n%s", buf.String())
	})
	se, err := stdouttrace.New(
		stdouttrace.WithWriter(buf),
		// Use human-readable output.
		stdouttrace.WithPrettyPrint(),
		// Do not print timestamps for the demo.
		stdouttrace.WithoutTimestamps(),
	)
	assert.Must(tb).Nil(err)
	tb.Cleanup(func() { se.Shutdown(context.Background()) })
	return se
}
