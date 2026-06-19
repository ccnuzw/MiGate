package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorUsesStandardErrorResponseShape(t *testing.T) {
	response := httptest.NewRecorder()
	WriteError(response, http.StatusBadRequest, APIError{
		Code:    "invalid_json",
		Message: "Invalid JSON body",
		Detail:  "body is malformed",
		Fields:  map[string]interface{}{"line": float64(1)},
	}, map[string]interface{}{"line": 1, "detail": "legacy detail must not override"})

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	errorObject, ok := payload["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error must be object, got %#v", payload["error"])
	}
	if errorObject["code"] != "invalid_json" || errorObject["message"] != "Invalid JSON body" || errorObject["detail"] != "body is malformed" {
		t.Fatalf("unexpected error object: %#v", errorObject)
	}
	errorFields, ok := errorObject["fields"].(map[string]interface{})
	if !ok || errorFields["line"] != float64(1) {
		t.Fatalf("unexpected error fields: %#v", errorObject["fields"])
	}
	if _, ok := payload["detail"]; ok {
		t.Fatalf("detail must only be nested under error: %#v", payload)
	}
	if _, ok := payload["line"]; ok {
		t.Fatalf("fields must only be nested under error.fields: %#v", payload)
	}
}

func TestWriteErrorCopiesAdditionalFieldsIntoStandardErrorFields(t *testing.T) {
	response := httptest.NewRecorder()
	WriteError(response, http.StatusBadRequest, APIError{
		Code:    "invalid_json",
		Message: "Invalid JSON body",
	}, map[string]interface{}{"line": 1, "detail": "legacy detail must not become field"})

	var payload map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	errorObject, ok := payload["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error must be object, got %#v", payload["error"])
	}
	errorFields, ok := errorObject["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("error.fields missing: %#v", errorObject)
	}
	if errorFields["line"] != float64(1) {
		t.Fatalf("error.fields.line = %#v, want 1", errorFields["line"])
	}
	if _, ok := errorFields["detail"]; ok {
		t.Fatalf("legacy detail must not be copied into error.fields: %#v", errorFields)
	}
	if _, ok := payload["line"]; ok {
		t.Fatalf("fields must not be emitted at top level: %#v", payload)
	}
}
