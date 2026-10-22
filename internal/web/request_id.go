package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/imzyb/MiGate/internal/logger"
)

const maxRequestIDLength = 64

// requestIDMiddleware adds a request ID to each request for log correlation.
// A well-formed X-Request-ID from an upstream proxy is kept; anything else is
// replaced, since the value is echoed in the response and written to logs.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if !validRequestID(requestID) {
			requestID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := logger.WithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// generateRequestID returns 16 hex characters. crypto/rand.Read never returns
// an error on Go 1.24+ (it crashes the process instead), so the error is ignored.
func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
