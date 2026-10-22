package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteIPUsesLastForwardedForEntryWhenTrusted(t *testing.T) {
	for _, tc := range []struct {
		name       string
		forwarded  []string
		trustProxy bool
		want       string
	}{
		{"header ignored unless trusted", []string{"198.51.100.7"}, false, "192.0.2.10"},
		{"no header", nil, true, "192.0.2.10"},
		{"single entry", []string{"198.51.100.7"}, true, "198.51.100.7"},
		{"forged leading entry", []string{"203.0.113.9, 198.51.100.7"}, true, "198.51.100.7"},
		{"proxy adds its own line", []string{"203.0.113.9", "198.51.100.7"}, true, "198.51.100.7"},
		{"ipv6", []string{"2001:db8::1"}, true, "2001:db8::1"},
		{"invalid last entry", []string{"198.51.100.7, unknown"}, true, "192.0.2.10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/login", nil)
			req.RemoteAddr = "192.0.2.10:4321"
			for _, v := range tc.forwarded {
				req.Header.Add("X-Forwarded-For", v)
			}
			if got := remoteIP(req, tc.trustProxy); got != tc.want {
				t.Fatalf("remoteIP = %q, want %q", got, tc.want)
			}
		})
	}
}
