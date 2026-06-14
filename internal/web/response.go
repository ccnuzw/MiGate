package web

import (
	"encoding/json"
	"net/http"
)

func writeJSONError(w http.ResponseWriter, status int, code string, fields ...map[string]interface{}) {
	payload := map[string]interface{}{"error": code}
	for _, extra := range fields {
		for k, v := range extra {
			payload[k] = v
		}
	}
	writeJSON(w, status, payload)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
}

// decodeJSONBody wraps r.Body with a 512KB MaxBytesReader and decodes JSON
// into v. Returns an error if the body is too large or invalid JSON.
func decodeJSONBody(r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<19) // 512KB
	return json.NewDecoder(r.Body).Decode(v)
}
