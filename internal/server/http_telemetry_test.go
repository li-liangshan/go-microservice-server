package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOTelRouteName(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { _ = tracerProvider.Shutdown(context.Background()) }()

	router := chi.NewRouter()
	router.Use(otelhttp.NewMiddleware("http.server", otelhttp.WithTracerProvider(tracerProvider)))
	router.Use(otelRouteName())
	router.Get("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	router.ServeHTTP(httptest.NewRecorder(), request)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if got := spans[0].Name(); got != "GET /users/{id}" {
		t.Errorf("span name: got %q, want %q", got, "GET /users/{id}")
	}

	var route string
	for _, attr := range spans[0].Attributes() {
		if attr.Key == "http.route" {
			route = attr.Value.AsString()
		}
	}
	if route != "/users/{id}" {
		t.Errorf("http.route: got %q, want %q", route, "/users/{id}")
	}
}
