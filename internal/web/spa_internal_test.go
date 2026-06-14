package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPrecompressedStaticHandlerServesAcceptedEncoding(t *testing.T) {
	handler := precompressedStaticHandler("/assets/", fstest.MapFS{
		"app.js":    {Data: []byte("plain")},
		"app.js.gz": {Data: []byte("gzip-data")},
		"app.js.br": {Data: []byte("br-data")},
	})

	br := httptest.NewRecorder()
	brReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	brReq.Header.Set("Accept-Encoding", "br, gzip")
	handler.ServeHTTP(br, brReq)
	if br.Code != http.StatusOK {
		t.Fatalf("expected br response 200, got %d: %s", br.Code, br.Body.String())
	}
	if br.Header().Get("Content-Encoding") != "br" || br.Body.String() != "br-data" {
		t.Fatalf("expected br asset, encoding=%q body=%q", br.Header().Get("Content-Encoding"), br.Body.String())
	}
	if contentType := br.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Fatalf("expected original JS content type, got %q", contentType)
	}
	if cache := br.Header().Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected cache header: %q", cache)
	}

	gzip := httptest.NewRecorder()
	gzipReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	gzipReq.Header.Set("Accept-Encoding", "gzip")
	handler.ServeHTTP(gzip, gzipReq)
	if gzip.Header().Get("Content-Encoding") != "gzip" || gzip.Body.String() != "gzip-data" {
		t.Fatalf("expected gzip asset, encoding=%q body=%q", gzip.Header().Get("Content-Encoding"), gzip.Body.String())
	}

	gzipOnly := httptest.NewRecorder()
	gzipOnlyReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	gzipOnlyReq.Header.Set("Accept-Encoding", "br;q=0, gzip")
	handler.ServeHTTP(gzipOnly, gzipOnlyReq)
	if gzipOnly.Header().Get("Content-Encoding") != "gzip" || gzipOnly.Body.String() != "gzip-data" {
		t.Fatalf("expected q=0 br to be skipped, encoding=%q body=%q", gzipOnly.Header().Get("Content-Encoding"), gzipOnly.Body.String())
	}

	gzipPreferred := httptest.NewRecorder()
	gzipPreferredReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	gzipPreferredReq.Header.Set("Accept-Encoding", "br;q=0.1, gzip;q=1")
	handler.ServeHTTP(gzipPreferred, gzipPreferredReq)
	if gzipPreferred.Header().Get("Content-Encoding") != "gzip" || gzipPreferred.Body.String() != "gzip-data" {
		t.Fatalf("expected higher gzip q to be selected, encoding=%q body=%q", gzipPreferred.Header().Get("Content-Encoding"), gzipPreferred.Body.String())
	}

	qZeroDecimal := httptest.NewRecorder()
	qZeroDecimalReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	qZeroDecimalReq.Header.Set("Accept-Encoding", "br;q=0.00")
	handler.ServeHTTP(qZeroDecimal, qZeroDecimalReq)
	if qZeroDecimal.Header().Get("Content-Encoding") != "" || qZeroDecimal.Body.String() != "plain" {
		t.Fatalf("expected q=0.00 br to be skipped, encoding=%q body=%q", qZeroDecimal.Header().Get("Content-Encoding"), qZeroDecimal.Body.String())
	}

	brFallbackHandler := precompressedStaticHandler("/assets/", fstest.MapFS{
		"app.js":    {Data: []byte("plain")},
		"app.js.br": {Data: []byte("br-data")},
	})
	brFallback := httptest.NewRecorder()
	brFallbackReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	brFallbackReq.Header.Set("Accept-Encoding", "br;q=0.5, gzip;q=1")
	brFallbackHandler.ServeHTTP(brFallback, brFallbackReq)
	if brFallback.Header().Get("Content-Encoding") != "br" || brFallback.Body.String() != "br-data" {
		t.Fatalf("expected lower-q br fallback when gzip file is missing, encoding=%q body=%q", brFallback.Header().Get("Content-Encoding"), brFallback.Body.String())
	}

	plain := httptest.NewRecorder()
	handler.ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if plain.Header().Get("Content-Encoding") != "" || plain.Body.String() != "plain" {
		t.Fatalf("expected plain asset, encoding=%q body=%q", plain.Header().Get("Content-Encoding"), plain.Body.String())
	}
}
