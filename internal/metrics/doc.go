// Package metrics keeps a few in-process counters and histograms (HTTP
// requests, login attempts, rate-limit blocks) and renders them in the
// Prometheus text format via Handler.
//
// Package web serves them at GET /api/metrics. The route is AuthRequired, so
// with panel authentication enabled it needs a valid panel session.
package metrics
