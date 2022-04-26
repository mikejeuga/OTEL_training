package main

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"log"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

const name = "ASG"

type App struct {
	l      *log.Logger
	client *http.Client
	host   string
}

func NewHTTPHandler(host string, l *log.Logger) http.Handler {

	// configure the open telemetry
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	)
	propagator := otel.GetTextMapPropagator()

	app := &App{
		l:    l,
		host: host,
		client: &http.Client{
			Transport: rtFn(func(req *http.Request) (*http.Response, error) {
				// shovel the tracing ID from the context into the outgoing HTTP Request

				// open telnet TextMap Propagator with Carrier for the round tripped request to inject headers.
				ctx := req.Context()                             // contains the tracingID
				carrier := propagation.HeaderCarrier(req.Header) // mapping to the outgoing headers, that will carry the tracing ID
				propagator.Inject(ctx, carrier)                  // put the tracing ID from context into the http.Header (HeaderCarrier)

				return http.DefaultTransport.RoundTrip(req)
			}),
		},
	}
	// wrap App with open telemetry middleware
	return traceIDMiddleware(app, propagator)
}

func traceIDMiddleware(next http.Handler, propagator propagation.TextMapPropagator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// shovel the tracing ID from the incoming HTTP request into the next HTTP Handler's request context.
		fmt.Println(r.Header.Get("traceparent"))
		traceID, err := trace.TraceIDFromHex(r.Header.Get("traceparent"))

		fmt.Printf("%#v - %v\n", traceID, err)
		carrier := HeaderCarrier(r.Header) // source of truth

		ctx := propagator.Extract(r.Context(), carrier) // creating a new context with tracing ID in it

		fmt.Printf("%#v\n", otel.GetTextMapPropagator())
		next.ServeHTTP(w, r.WithContext(ctx)) // call next http.Handler with the context that has the tracingID
	})
}

type rtFn func(req *http.Request) (*http.Response, error)

func (fn rtFn) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func (a *App) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	_ = a.someSubStackScopeCall(r.Context())
}

func (a *App) someSubStackScopeCall(ctx context.Context) error {
	// make external request with tracing
	req, _ := http.NewRequest(http.MethodGet, a.host+"/", strings.NewReader("Hello, world!"))
	req = req.WithContext(ctx)
	fmt.Println(a.client.Do(req))

	// take span from context -> take tracing id from span
	span := trace.SpanFromContext(ctx)
	ctxFromSpan := span.SpanContext()
	traceID := ctxFromSpan.TraceID()
	a.l.Println("trace_id:", traceID.String())
	return nil
}

// HeaderCarrier adapts http.Header to satisfy the TextMapCarrier interface.
type HeaderCarrier http.Header

// Get returns the value associated with the passed key.
func (hc HeaderCarrier) Get(key string) string {
	fmt.Println("header key:", key)
	return http.Header(hc).Get(key)
}

// Set stores the key-value pair.
func (hc HeaderCarrier) Set(key string, value string) {
	fmt.Println("header key:", key)
	http.Header(hc).Set(key, value)
}

// Keys lists the keys stored in this carrier.
func (hc HeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(hc))
	for k := range hc {
		keys = append(keys, k)
	}
	return keys
}
