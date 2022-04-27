package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"go.opentelemetry.io/otel/propagation"
	traceSDK "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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
	TraceBuf         *bytes.Buffer
	InMemoryExporter *tracetest.InMemoryExporter
	TracerProvider   *traceSDK.TracerProvider
}

func NewSubject(tb testing.TB, url string) Subject {
	logBuf := &bytes.Buffer{}
	inMemoryExporter := &tracetest.InMemoryExporter{}
	logger := log.New(logBuf, "", log.LstdFlags)
	propagator := propagation.TraceContext{}
	spanProcessorForExporting := traceSDK.NewBatchSpanProcessor(&DebugSpanExporter{TB: tb})
	tracerProvider := traceSDK.NewTracerProvider(traceSDK.WithSpanProcessor(spanProcessorForExporting))
	tracer := tracerProvider.Tracer("spikeTKI")

	return Subject{
		Handler:          NewHTTPHandler(url, logger, propagator, tracer),
		LoggerBuffer:     logBuf,
		InMemoryExporter: inMemoryExporter,
		TracerProvider:   tracerProvider,
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

	must.Nil(subject.TracerProvider.ForceFlush(context.Background()))
	t.Logf("%#v", subject.InMemoryExporter.GetSpans())
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

var _ traceSDK.SpanExporter = &DebugSpanExporter{}

// traceSDK.SpanExporter
type DebugSpanExporter struct{ TB testing.TB }

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
