package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
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

func TestE2E(t *testing.T) {
	tID, sID := newTraceID()
	expectedTraceIDHeader := traceIDToHeader(tID, sID)
	t.Log("expectedTraceID Header:", expectedTraceIDHeader)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			t.Logf("%s: %s", k, v)
		}

		actualTraceID := r.Header.Get(headerKey)
		assert.Should(t).NotEmpty(actualTraceID, "we should have a tracing id received in the request")
		assert.Should(t).Contain(actualTraceID, tID.String(), "the initial parent tracing ID should be present")
	}))
	defer srv.Close()

	var buf bytes.Buffer
	logger := log.New(&buf, "", log.LstdFlags)
	handler := NewHTTPHandler(srv.URL, logger)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerKey, expectedTraceIDHeader)
	handler.ServeHTTP(rr, req)
	assert.Must(t).Equal(http.StatusOK, rr.Code)
	assert.Must(t).Contain(buf.String(), tID.String())
}

func TestE2E_noTraceIDSent_TraceIDReceived(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Should(t).NotEmpty(r.Header.Get(headerKey))
	})

	logger := log.New(&bytes.Buffer{}, "", log.LstdFlags)
	handler := NewHTTPHandler(srv.URL, logger)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rr, req)
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
