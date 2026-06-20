package web

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type APIError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Detail  string                 `json:"detail,omitempty"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

type serviceError interface {
	error
	ServiceCode() string
	ServiceDetail() string
}

type serviceErrorFields interface {
	ServiceFields() map[string]interface{}
}

type StatusResponse struct {
	Status   string        `json:"status"`
	Warnings []interface{} `json:"warnings,omitempty"`
	Applied  *bool         `json:"applied,omitempty"`
}

func writeJSONError(w http.ResponseWriter, status int, code string, fields ...map[string]interface{}) {
	detail := ""
	merged := map[string]interface{}{}
	for _, extra := range fields {
		for k, v := range extra {
			if k == "detail" {
				detail = fmt.Sprint(v)
				continue
			}
			merged[k] = v
		}
	}
	WriteError(w, status, APIError{Code: code, Message: defaultErrorMessage(code), Detail: detail, Fields: merged}, merged)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	WriteJSON(w, status, payload)
}

func WriteJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, status int, apiError APIError, legacyFields map[string]interface{}) {
	if apiError.Message == "" {
		apiError.Message = defaultErrorMessage(apiError.Code)
	}
	apiError.Fields = mergeErrorFields(apiError.Fields, legacyFields)
	WriteJSON(w, status, ErrorResponse{Error: apiError})
}

func writeServiceError(w http.ResponseWriter, status int, err error) {
	if serviceErr, ok := err.(serviceError); ok {
		var fields map[string]interface{}
		if fieldErr, ok := err.(serviceErrorFields); ok {
			fields = fieldErr.ServiceFields()
		}
		WriteError(w, status, APIError{Code: serviceErr.ServiceCode(), Detail: serviceErr.ServiceDetail(), Fields: fields}, nil)
		return
	}
	WriteError(w, status, APIError{Code: "request_failed", Detail: err.Error()}, nil)
}

func mergeErrorFields(fields, legacyFields map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for k, v := range legacyFields {
		if k == "error" || k == "detail" {
			continue
		}
		merged[k] = v
	}
	for k, v := range fields {
		if k == "error" || k == "detail" {
			continue
		}
		merged[k] = v
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed")
}

// decodeJSONBody wraps r.Body with a 512KB MaxBytesReader and decodes JSON
// into v. Returns an error if the body is too large or invalid JSON.
func decodeJSONBody(r *http.Request, v interface{}) error {
	return DecodeJSONBody(r, v)
}

func DecodeJSONBody(r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<19) // 512KB
	return json.NewDecoder(r.Body).Decode(v)
}

func defaultErrorMessage(code string) string {
	switch code {
	case "invalid_json":
		return "Invalid JSON body"
	case "method_not_allowed":
		return "Method not allowed"
	case "confirmation_required":
		return "Explicit confirmation is required"
	default:
		if code == "" {
			return "Request failed"
		}
		return code
	}
}
