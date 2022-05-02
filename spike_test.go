package main

import (
	"encoding/hex"
	"regexp"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

const (
	supportedVersion  = 0
	maxVersion        = 254
	traceparentHeader = "traceparent"
	tracestateHeader  = "tracestate"
)

var traceCtxRegExp = regexp.MustCompile("^(?P<version>[0-9a-f]{2})-(?P<traceID>[a-f0-9]{32})-(?P<spanID>[a-f0-9]{16})-(?P<traceFlags>[a-f0-9]{2})(?:-.*)?$")

func TestSpikeExtract(t *testing.T) {
	headerValue := traceIDToHeader(newTraceID())
	headerValue = "00-d41c1b69fdcf0b087fc0cdf0df436689-07c3d2d11ca3dca5-00"

	carrier := make(HeaderCarrier)
	carrier.Set("traceparent", headerValue)

	h := carrier.Get(traceparentHeader)
	if h == "" {
		t.Fatal("err case")
	}

	matches := traceCtxRegExp.FindStringSubmatch(h)

	t.Log(matches)
	if len(matches) == 0 {
		t.Fatal("err case")
	}

	if len(matches) < 5 { // four subgroups plus the overall match
		t.Fatal("err case")
	}

	if len(matches[1]) != 2 {
		t.Fatal("err case")
	}
	ver, err := hex.DecodeString(matches[1])
	if err != nil {
		t.Fatal("err case")
	}
	version := int(ver[0])
	if version > maxVersion {
		t.Fatal("err case")
	}

	if version == 0 && len(matches) != 5 { // four subgroups plus the overall match
		t.Fatal("err case")
	}

	if len(matches[2]) != 32 {
		t.Fatal("err case")
	}

	var scc trace.SpanContextConfig

	scc.TraceID, err = trace.TraceIDFromHex(matches[2][:32])
	if err != nil {
		t.Fatal("err case")
	}

	if len(matches[3]) != 16 {
		t.Fatal("err case")
	}
	scc.SpanID, err = trace.SpanIDFromHex(matches[3])
	if err != nil {
		t.Fatal("err case")
	}

	if len(matches[4]) != 2 {
		t.Fatal("err case")
	}
	opts, err := hex.DecodeString(matches[4])
	if err != nil || len(opts) < 1 || (version == 0 && opts[0] > 2) {
		t.Fatal("err case")
	}
	// Clear all flags other than the trace-context supported sampling bit.
	scc.TraceFlags = trace.TraceFlags(opts[0]) & trace.FlagsSampled

	// Ignore the error returned here. Failure to parse tracestate MUST NOT
	// affect the parsing of traceparent according to the W3C tracecontext
	// specification.
	scc.TraceState, _ = trace.ParseTraceState(carrier.Get(tracestateHeader))
	scc.Remote = true

	sc := trace.NewSpanContext(scc)
	if !sc.IsValid() {
		t.Fatal("fail")
	}

	t.Logf("%#v", sc)
}
