package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	traceSDK "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/adamluzsi/testcase/assert"
	"go.opentelemetry.io/otel/trace"
)

const headerKey = "traceparent"

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Subject struct {
	Handler          http.Handler
	LoggerBuffer     *bytes.Buffer
	InMemoryExporter *tracetest.InMemoryExporter
	TracerProvider   trace.TracerProvider
	TracingSubject   TracingSubject
}

func NewSubject(tb testing.TB, url string) Subject {
	tb.Helper()
	//exporterBuffer := &bytes.Buffer{}
	//exporter, err := newIOWriterExporter(exporterBuffer)
	//assert.Must(tb).Nil(err)
	//spanProcessorForExporting := traceSDK.NewSimpleSpanProcessor(exporter)

	//propagator := propagation.TraceContext{}
	//otel.SetTextMapPropagator(propagator)
	//
	//spanProcessorForExporting := &DebugSpanProcessor{TB: tb, SpanProcessor: traceSDK.NewSimpleSpanProcessor(&DebugSpanExporter{TB: tb})}
	//tracerProvider := traceSDK.NewTracerProvider(
	//	traceSDK.WithSpanProcessor(spanProcessorForExporting),
	//	traceSDK.WithResource(newResource(tb)),
	//)
	//otel.SetTracerProvider(tracerProvider)

	//// &tracetest.NoopExporter{}
	//traceExporter, err := otlptrace.New(context.Background())
	//if err != nil {
	//	zapctx.Error(ctx, "error failed to instantiate trace exporter", zap.Error(err))
	//
	//	return err
	//}
	//opts = append(opts, sdkTrace.WithSpanProcessor(sdkTrace.NewBatchSpanProcessor(traceExporter)))

	//tracer := tracerProvider.Tracer("spikeTKI")
	tracingSubject := makeTracingPropagation(tb)

	logBuf := &bytes.Buffer{}
	logger := log.New(logBuf, "", log.LstdFlags)

	return Subject{
		Handler:        NewHTTPHandler(url, logger, tracingSubject.TextMapPropagator, tracingSubject.TracerProvider),
		TracingSubject: tracingSubject,
	}
}

type TracingSubject struct {
	TextMapPropagator propagation.TextMapPropagator
	TracerProvider    trace.TracerProvider
	ExportIO          *bytes.Buffer
}

func makeTracingPropagation(tb testing.TB) TracingSubject {
	tb.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)

	// setup a custom exporter
	//spanExporter := &tracetest.NoopExporter{}

	buf := &bytes.Buffer{}
	tb.Cleanup(func() { tb.Log(buf.String()) })
	ppExporter, err := newIOWriterExporter(buf)

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceNameKey.String("ags-test")))
	assert.Must(tb).Nil(err)

	tracerProvider := traceSDK.NewTracerProvider(
		traceSDK.WithSpanProcessor(traceSDK.NewSimpleSpanProcessor(ppExporter)),
		traceSDK.WithResource(res),
	)
	tb.Cleanup(func() { tracerProvider.Shutdown(ctx) })

	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})

	// setup globals just to be sure :see_no_evil:
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagator)

	return TracingSubject{
		TextMapPropagator: propagator,
		TracerProvider:    tracerProvider,
		ExportIO:          buf,
	}
}

func TestE2E(t *testing.T) {
	must := assert.Must(t)

	tID, sID := newTraceID()
	expectedTraceIDHeader := traceIDToHeader(tID, sID)
	t.Log("expectedTraceID Header:", expectedTraceIDHeader)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actualTraceID := r.Header.Get(headerKey)
		assert.Should(t).NotEmpty(actualTraceID, "we should have a tracing id received in the request")
		assert.Should(t).Contain(actualTraceID, tID.String(), "the initial parent tracing ID should be present")
	}))
	defer srv.Close()

	subject := NewSubject(t, srv.URL)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerKey, expectedTraceIDHeader)
	subject.Handler.ServeHTTP(rr, req)
	must.Equal(http.StatusOK, rr.Code)
	must.Contain(subject.LoggerBuffer.String(), tID.String())
}

func TestE2E_spanExport(t *testing.T) {
	must := assert.Must(t)

	tID, sID := newTraceID()
	tracingParentHeader := traceIDToHeader(tID, sID)
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {})
	subject := NewSubject(t, srv.URL)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerKey, tracingParentHeader)
	subject.Handler.ServeHTTP(rr, req)
	must.Equal(http.StatusOK, rr.Code)

	t.Log(subject.TracingSubject.ExportIO.String())
	//must.Nil(subject.TracerProvider.ForceFlush(context.Background()))
	//subject.TracerProvider.Shutdown(context.Background())
	// t.Logf("%#v", subject.InMemoryExporter.GetSpans())

}

func TestE2E_noTraceIDSent_TraceIDReceived(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Should(t).NotEmpty(r.Header.Get(headerKey))
	})

	subject := NewSubject(t, srv.URL)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	subject.Handler.ServeHTTP(rr, req)
	assert.Must(t).Equal(http.StatusOK, rr.Code)
}

func newServer(tb testing.TB, h http.HandlerFunc) *httptest.Server {
	srv := httptest.NewServer(h)
	tb.Cleanup(srv.Close)
	return srv
}

func traceIDToHeader(tID trace.TraceID, sID trace.SpanID) string {
	return fmt.Sprintf("00-%s-%s-00", tID.String(), sID.String())
}

func newTraceID() (trace.TraceID, trace.SpanID) {
	return newRandomIDGenerator().NewIDs(nil)
}

type randomIDGenerator struct {
	sync.Mutex
	randSource *rand.Rand
}

// NewSpanID returns a non-zero span ID from a randomly-chosen sequence.
func (gen *randomIDGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	gen.Lock()
	defer gen.Unlock()
	sid := trace.SpanID{}
	gen.randSource.Read(sid[:])
	return sid
}

// NewIDs returns a non-zero trace ID and a non-zero span ID from a
// randomly-chosen sequence.
func (gen *randomIDGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	gen.Lock()
	defer gen.Unlock()
	tid := trace.TraceID{}
	gen.randSource.Read(tid[:])
	sid := trace.SpanID{}
	gen.randSource.Read(sid[:])
	return tid, sid
}

func newRandomIDGenerator() *randomIDGenerator {
	gen := &randomIDGenerator{}
	var rngSeed int64
	_ = binary.Read(crand.Reader, binary.LittleEndian, &rngSeed)
	gen.randSource = rand.New(rand.NewSource(rngSeed))
	return gen
}

// traceSDK.SpanExporter
type DebugSpanExporter struct{ TB testing.TB }

var _ traceSDK.SpanExporter = &DebugSpanExporter{}

func (exp DebugSpanExporter) ExportSpans(ctx context.Context, spans []traceSDK.ReadOnlySpan) error {
	exp.TB.Helper()
	exp.TB.Logf("DebugSpanExporter.ExportSpans:  %#v", spans)
	return nil
}

func (exp DebugSpanExporter) Shutdown(ctx context.Context) error {
	exp.TB.Helper()
	exp.TB.Logf("DebugSpanExporter is shutting down")
	return nil
}

type DebugSpanProcessor struct {
	testing.TB
	traceSDK.SpanProcessor
}

func (sp *DebugSpanProcessor) OnStart(parent context.Context, s traceSDK.ReadWriteSpan) {
	sp.TB.Helper()
	sp.TB.Logf("DebugSpanProcessor.OnStart: %#v", s)
	sp.SpanProcessor.OnStart(parent, s)
}

func (sp *DebugSpanProcessor) OnEnd(s traceSDK.ReadOnlySpan) {
	sp.TB.Helper()
	sp.TB.Logf("DebugSpanProcessor.OnEnd: %#v", s)
	sp.SpanProcessor.OnEnd(s)
}

func (sp *DebugSpanProcessor) Shutdown(ctx context.Context) error {
	sp.TB.Helper()
	sp.TB.Logf("DebugSpanProcessor.Shutdown: %#v", ctx)
	return sp.SpanProcessor.Shutdown(ctx)
}

func (sp *DebugSpanProcessor) ForceFlush(ctx context.Context) error {
	sp.TB.Helper()
	sp.TB.Logf("DebugSpanProcessor.ForceFlush: %#v", ctx)
	return sp.SpanProcessor.ForceFlush(ctx)
}

func newIOWriterExporter(w io.Writer) (traceSDK.SpanExporter, error) {
	return stdouttrace.New(
		stdouttrace.WithWriter(w),
		// Use human-readable output.
		stdouttrace.WithPrettyPrint(),
		// Do not print timestamps for the demo.
		stdouttrace.WithoutTimestamps(),
	)
}

// newResource returns a resource describing this application.
func newResource(tb testing.TB) *resource.Resource {
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("fib"),
			semconv.ServiceVersionKey.String("v0.1.0"),
			attribute.String("environment", "demo"),
		),
	)
	assert.Must(tb).Nil(err)
	return r
}
