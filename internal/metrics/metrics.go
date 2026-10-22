package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// labelSep joins label values into map keys. The label values recorded here
// (HTTP methods, route templates, status codes, fixed result names) never
// contain it, unlike a comma.
const labelSep = "\xff"

// Counter is a monotonically increasing value, optionally split by labels.
type Counter struct {
	value  uint64
	mu     sync.RWMutex
	labels map[string]uint64
}

// Histogram counts observations into cumulative buckets, optionally split by labels.
type Histogram struct {
	mu      sync.RWMutex
	sum     float64
	count   uint64
	buckets []float64
	counts  []uint64
	labels  map[string]*histogramData
}

type histogramData struct {
	sum    float64
	count  uint64
	counts []uint64
}

var (
	HTTPRequestsTotal   = NewCounter()
	HTTPRequestDuration = NewHistogram([]float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0})
	LoginAttempts       = NewCounter()
	RateLimitBlocked    = NewCounter()
)

// NewCounter returns an empty counter.
func NewCounter() *Counter {
	return &Counter{labels: make(map[string]uint64)}
}

func (c *Counter) Inc(labels ...string) {
	c.Add(1, labels...)
}

func (c *Counter) Add(delta uint64, labels ...string) {
	if len(labels) == 0 {
		atomic.AddUint64(&c.value, delta)
		return
	}
	key := strings.Join(labels, labelSep)
	c.mu.Lock()
	c.labels[key] += delta
	c.mu.Unlock()
}

func (c *Counter) Get(labels ...string) uint64 {
	if len(labels) == 0 {
		return atomic.LoadUint64(&c.value)
	}
	key := strings.Join(labels, labelSep)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.labels[key]
}

// NewHistogram returns a histogram with the given upper bounds; +Inf is implicit.
func NewHistogram(buckets []float64) *Histogram {
	sorted := make([]float64, len(buckets))
	copy(sorted, buckets)
	sort.Float64s(sorted)
	return &Histogram{
		buckets: sorted,
		counts:  make([]uint64, len(sorted)+1),
		labels:  make(map[string]*histogramData),
	}
}

func (h *Histogram) Observe(value float64, labels ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(labels) == 0 {
		h.sum += value
		h.count++
		observeInto(h.buckets, h.counts, value)
		return
	}
	key := strings.Join(labels, labelSep)
	data, ok := h.labels[key]
	if !ok {
		data = &histogramData{counts: make([]uint64, len(h.buckets)+1)}
		h.labels[key] = data
	}
	data.sum += value
	data.count++
	observeInto(h.buckets, data.counts, value)
}

// observeInto increments every bucket whose upper bound is >= value, plus
// +Inf, so bucket counts are cumulative as Prometheus expects.
func observeInto(buckets []float64, counts []uint64, value float64) {
	for i, bound := range buckets {
		if value <= bound {
			counts[i]++
		}
	}
	counts[len(buckets)]++
}

// Handler serves all metrics in the Prometheus text exposition format.
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder
		writeCounter(&sb, "migate_http_requests_total", "Total HTTP requests", HTTPRequestsTotal, []string{"method", "path", "status"})
		writeHistogram(&sb, "migate_http_request_duration_seconds", "HTTP request duration in seconds", HTTPRequestDuration, []string{"method", "path"})
		writeCounter(&sb, "migate_login_attempts_total", "Total login attempts by result", LoginAttempts, []string{"result"})
		writeCounter(&sb, "migate_rate_limit_blocked_total", "Total requests blocked by rate limiting", RateLimitBlocked, []string{"limiter"})
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(sb.String()))
	}
}

func writeCounter(sb *strings.Builder, name, help string, c *Counter, labelNames []string) {
	fmt.Fprintf(sb, "# HELP %s %s\n# TYPE %s counter\n", name, help, name)
	if len(labelNames) == 0 {
		fmt.Fprintf(sb, "%s %d\n", name, c.Get())
		return
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for key, value := range c.labels {
		fmt.Fprintf(sb, "%s{%s} %d\n", name, formatLabels(labelNames, key), value)
	}
}

func writeHistogram(sb *strings.Builder, name, help string, h *Histogram, labelNames []string) {
	fmt.Fprintf(sb, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(labelNames) == 0 {
		writeHistogramSeries(sb, name, "", h.buckets, h.counts, h.sum, h.count)
		return
	}
	for key, data := range h.labels {
		writeHistogramSeries(sb, name, formatLabels(labelNames, key), h.buckets, data.counts, data.sum, data.count)
	}
}

func writeHistogramSeries(sb *strings.Builder, name, labels string, buckets []float64, counts []uint64, sum float64, count uint64) {
	prefix := ""
	if labels != "" {
		prefix = labels + ","
	}
	for i, bound := range buckets {
		fmt.Fprintf(sb, "%s_bucket{%sle=\"%g\"} %d\n", name, prefix, bound, counts[i])
	}
	fmt.Fprintf(sb, "%s_bucket{%sle=\"+Inf\"} %d\n", name, prefix, counts[len(buckets)])
	if labels == "" {
		fmt.Fprintf(sb, "%s_sum %g\n%s_count %d\n", name, sum, name, count)
		return
	}
	fmt.Fprintf(sb, "%s_sum{%s} %g\n%s_count{%s} %d\n", name, labels, sum, name, labels, count)
}

// formatLabels turns a labelSep-joined key back into name="value" pairs.
func formatLabels(names []string, key string) string {
	values := strings.Split(key, labelSep)
	pairs := make([]string, 0, len(names))
	for i, name := range names {
		if i < len(values) {
			pairs = append(pairs, name+`="`+escapeLabelValue(values[i])+`"`)
		}
	}
	return strings.Join(pairs, ",")
}

func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return strings.ReplaceAll(s, "\n", `\n`)
}

// RecordHTTPRequest records one HTTP request and its duration.
func RecordHTTPRequest(method, path string, status int, duration time.Duration) {
	HTTPRequestsTotal.Inc(method, path, strconv.Itoa(status))
	HTTPRequestDuration.Observe(duration.Seconds(), method, path)
}

// RecordLoginAttempt records a login attempt by result (success, failure, ...).
func RecordLoginAttempt(result string) {
	LoginAttempts.Inc(result)
}

// RecordRateLimitBlock records a request rejected by the named rate limiter.
func RecordRateLimitBlock(limiter string) {
	RateLimitBlocked.Inc(limiter)
}
