package httpx

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

var traceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func captureCorrelation(t *testing.T, request *http.Request) (http.Header, string, trace.SpanContext) {
	t.Helper()
	var contextTrace string
	var spanContext trace.SpanContext
	handler := RequestID()(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contextTrace = GetTraceID(r.Context())
		spanContext = trace.SpanContextFromContext(r.Context())
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Header(), contextTrace, spanContext
}

func TestRequestIDCreatesW3CCorrelationWhenExporterIsAbsent(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/assets", nil)
	headers, contextTrace, spanContext := captureCorrelation(t, request)
	traceID := headers.Get(HeaderTraceID)
	if !traceIDPattern.MatchString(traceID) || contextTrace != traceID {
		t.Fatalf("trace correlation mismatch: header=%q context=%q", traceID, contextTrace)
	}
	if !spanContext.IsValid() || spanContext.TraceID().String() != traceID {
		t.Fatalf("invalid span context: %v", spanContext)
	}
	if request.Header.Get("traceparent") == "" || headers.Get(HeaderSpanID) == "" {
		t.Fatal("traceparent and response span identity must be present")
	}
}

func TestRequestIDTraceparentWinsOverConflictingLegacyHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/assets", nil)
	request.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
	request.Header.Set(HeaderTraceID, "33333333333333333333333333333333")
	headers, contextTrace, _ := captureCorrelation(t, request)
	if headers.Get(HeaderTraceID) != "11111111111111111111111111111111" || contextTrace != headers.Get(HeaderTraceID) {
		t.Fatalf("traceparent did not remain authoritative: %#v", headers)
	}
}

func TestRequestIDRejectsMalformedTraceHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/assets", nil)
	request.Header.Set(HeaderTraceID, "trace-with-newline-and-wrong-shape")
	headers, _, _ := captureCorrelation(t, request)
	if !traceIDPattern.MatchString(headers.Get(HeaderTraceID)) || headers.Get(HeaderTraceID) == request.Header.Get(HeaderTraceID) {
		t.Fatalf("malformed trace ID was not replaced: %q", headers.Get(HeaderTraceID))
	}
}

func TestRequestIDAcceptsValidLegacyTraceAndSynthesizesTraceparent(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/assets", nil)
	request.Header.Set(HeaderTraceID, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	headers, contextTrace, _ := captureCorrelation(t, request)
	if headers.Get(HeaderTraceID) != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || contextTrace != headers.Get(HeaderTraceID) {
		t.Fatalf("valid legacy trace ID did not propagate: %#v", headers)
	}
	if got := request.Header.Get("traceparent"); len(got) != 55 || got[:3] != "00-" {
		t.Fatalf("unexpected synthesized traceparent: %q", got)
	}
}
