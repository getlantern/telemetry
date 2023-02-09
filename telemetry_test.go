package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/sdk/trace"
)

func TestForceSample(t *testing.T) {
	s := ForceableSampler(trace.NeverSample())
	tracer := trace.NewTracerProvider(trace.WithSampler(s)).Tracer("my-app")

	ctx := context.WithValue(context.Background(), forceSample, true)
	_, span := tracer.Start(ctx, "span name")
	assert.True(t, span.SpanContext().IsSampled())
}

func TestNewHandler(t *testing.T) {

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/apis", nil)
	req.Header.Add("traceme", "true")

	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(forceSample) == nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})
	h := NewHandler(base, requestFilterFunc(func(r *http.Request) bool {
		sampling := r.Header.Get("traceme") == "true"
		return sampling
	}))

	h.ServeHTTP(rr, req)

	assert.True(t, rr.Code == http.StatusOK)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/apis", nil)
	h.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)

}
