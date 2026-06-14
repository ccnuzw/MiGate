package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

type cspNonceContextKey struct{}

func securityHeadersMiddleware(next http.Handler, cfg *routerConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := newCSPNonce()
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'nonce-"+nonce+"'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'")
		if requestIsHTTPS(r, cfg) {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), cspNonceContextKey{}, nonce)))
	})
}

func newCSPNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return base64.RawStdEncoding.EncodeToString(b[:])
}

func cspNonceFromRequest(r *http.Request) string {
	nonce, _ := r.Context().Value(cspNonceContextKey{}).(string)
	return nonce
}

func requestIsHTTPS(r *http.Request, cfg *routerConfig) bool {
	if r.TLS != nil {
		return true
	}
	if cfg == nil || !cfg.trustProxy {
		return false
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}
