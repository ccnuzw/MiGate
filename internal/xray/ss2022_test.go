package xray

import (
	"encoding/base64"
	"testing"
)

func TestSS2022KeyGeneratesValidBase64Key(t *testing.T) {
	// Non-2022 methods should use the fallback UUID
	key := ss2022Key("aes-256-gcm", "my-uuid-here")
	if key != "my-uuid-here" {
		t.Fatalf("expected fallback, got %q", key)
	}

	// 2022-blake3-aes-128-gcm should return 16 bytes base64
	key128 := ss2022Key("2022-blake3-aes-128-gcm", "fallback")
	if key128 == "fallback" {
		t.Fatalf("should have generated a key, not fallback")
	}
	decoded, err := base64.StdEncoding.DecodeString(key128)
	if err != nil {
		t.Fatalf("key not valid base64: %v", err)
	}
	if len(decoded) != 16 {
		t.Fatalf("expected 16-byte key, got %d bytes", len(decoded))
	}

	// 2022-blake3-aes-256-gcm should return 32 bytes base64
	key256 := ss2022Key("2022-blake3-aes-256-gcm", "fallback")
	if key256 == "fallback" {
		t.Fatalf("should have generated a key, not fallback")
	}
	decoded, err = base64.StdEncoding.DecodeString(key256)
	if err != nil {
		t.Fatalf("key not valid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("expected 32-byte key, got %d bytes", len(decoded))
	}
}
