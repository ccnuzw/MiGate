package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteCounterEscapesLabelValues(t *testing.T) {
	c := NewCounter()
	c.Inc(`a,b"c`)
	var sb strings.Builder
	writeCounter(&sb, "test_total", "help", c, []string{"result"})
	if want := `test_total{result="a,b\"c"} 1`; !strings.Contains(sb.String(), want) {
		t.Fatalf("output missing %q:\n%s", want, sb.String())
	}
}

func TestHistogramBucketsAreCumulative(t *testing.T) {
	h := NewHistogram([]float64{1, 0.3})
	for _, v := range []float64{0.25, 0.5, 4} {
		h.Observe(v, "GET")
	}
	var sb strings.Builder
	writeHistogram(&sb, "d", "help", h, []string{"method"})
	out := sb.String()
	for _, want := range []string{
		`d_bucket{method="GET",le="0.3"} 1`,
		`d_bucket{method="GET",le="1"} 2`,
		`d_bucket{method="GET",le="+Inf"} 3`,
		`d_sum{method="GET"} 4.75`,
		`d_count{method="GET"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestHandlerServesPrometheusText(t *testing.T) {
	RecordLoginAttempt("success")
	rec := httptest.NewRecorder()
	Handler()(rec, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Fatalf("content type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), `migate_login_attempts_total{result="success"}`) {
		t.Fatalf("missing login metric:\n%s", rec.Body.String())
	}
}
