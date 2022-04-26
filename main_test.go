package main

import (
	"bytes"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adamluzsi/testcase/assert"
)

func TestE2E(t *testing.T) {
	const headerKey = "traceparent"
	rand.Seed(time.Now().UnixNano())
	expectedTraceID := "01000000000000000000000000000000"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range r.Header {
			t.Logf("%s: %s", k, v)
		}

		actualTraceID := r.Header.Get(headerKey)
		assert.Should(t).Equal(expectedTraceID, actualTraceID)
	}))
	t.Cleanup(srv.Close)

	var buf bytes.Buffer
	logger := log.New(&buf, "", log.LstdFlags)
	handler := NewHTTPHandler(srv.URL, logger)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerKey, expectedTraceID)
	handler.ServeHTTP(rr, req)
	assert.Must(t).Equal(http.StatusOK, rr.Code)
	assert.Must(t).Contain(buf.String(), expectedTraceID)
}
