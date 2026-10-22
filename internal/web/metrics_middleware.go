package web

import (
	"net/http"
	"time"

	"github.com/imzyb/MiGate/internal/metrics"
)

// metricsHandler serves Prometheus metrics. It is registered in routeTable as
// an AuthRequired route, so it needs a panel session when auth is enabled.
func metricsHandler() http.HandlerFunc {
	serve := metrics.Handler()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		serve(w, r)
	}
}

// metricsMiddleware wraps handlers to automatically record request metrics.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		wrapper := &responseWrapper{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapper, r)

		duration := time.Since(start)
		metrics.RecordHTTPRequest(metricMethod(r.Method), metricPath(r.Method, r.URL.Path), wrapper.statusCode, duration)
	})
}

// metricPath uses route templates for known endpoints and a single bucket for
// everything else, so attacker-controlled paths cannot create unbounded series.
func metricPath(method, path string) string {
	if path == "/login" || path == "/" || path == "/favicon.svg" || path == "/favicon.ico" || path == "/assets/" || path == "/sub/" {
		return path
	}
	for _, route := range RouteContracts() {
		if route.Method == method && routePathMatches(route.Path, path) {
			return route.Path
		}
	}
	return "unmatched"
}

func metricMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	default:
		return "OTHER"
	}
}

// responseWrapper wraps http.ResponseWriter to capture the status code
type responseWrapper struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (w *responseWrapper) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWrapper) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// Flush preserves streaming handlers such as the traffic event stream.
func (w *responseWrapper) Flush() {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = http.StatusOK
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap lets http.ResponseController reach optional interfaces implemented by
// the underlying writer.
func (w *responseWrapper) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
