package web

import (
	"bytes"
	"net/http"
	"sync"
	"time"
)

type coreStatusCache struct {
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	entries map[string]cachedCoreResponse
}

type cachedCoreResponse struct {
	statusCode int
	header     http.Header
	body       []byte
	expiresAt  time.Time
}

func newCoreStatusCache(ttl time.Duration) *coreStatusCache {
	return &coreStatusCache{ttl: ttl, now: time.Now, entries: map[string]cachedCoreResponse{}}
}

func (c *coreStatusCache) wrap(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c == nil || c.ttl <= 0 || r.Method != http.MethodGet {
			next(w, r)
			return
		}
		now := c.now()
		c.mu.Lock()
		entry, ok := c.entries[key]
		if ok && now.Before(entry.expiresAt) {
			c.mu.Unlock()
			copyHeader(w.Header(), entry.header)
			w.WriteHeader(entry.statusCode)
			_, _ = w.Write(entry.body)
			return
		}
		c.mu.Unlock()

		recorder := &cachedResponseWriter{header: http.Header{}, statusCode: http.StatusOK}
		next(recorder, r)
		entry = cachedCoreResponse{
			statusCode: recorder.statusCode,
			header:     cloneHeader(recorder.header),
			body:       append([]byte(nil), recorder.body.Bytes()...),
			expiresAt:  c.now().Add(c.ttl),
		}
		if entry.statusCode >= 200 && entry.statusCode < 300 {
			c.mu.Lock()
			c.entries[key] = entry
			c.mu.Unlock()
		}
		copyHeader(w.Header(), entry.header)
		w.WriteHeader(entry.statusCode)
		_, _ = w.Write(entry.body)
	}
}

func (c *coreStatusCache) invalidate(keys ...string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(keys) == 0 {
		clear(c.entries)
		return
	}
	for _, key := range keys {
		delete(c.entries, key)
	}
}

type cachedResponseWriter struct {
	header      http.Header
	body        bytes.Buffer
	statusCode  int
	wroteHeader bool
}

func (w *cachedResponseWriter) Header() http.Header {
	return w.header
}

func (w *cachedResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = statusCode
	w.wroteHeader = true
}

func (w *cachedResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(body)
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		dst[key] = append([]string(nil), values...)
	}
}

func cloneHeader(src http.Header) http.Header {
	dst := http.Header{}
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}
