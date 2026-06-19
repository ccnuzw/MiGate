package xray_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/imzyb/MiGate/internal/xray"
)

func TestGenerateRealityKeyGeneratesValidKeyPair(t *testing.T) {
	privateKey, publicKey, err := xray.GenerateRealityKey()
	if err != nil {
		t.Fatalf("generate reality key: %v", err)
	}
	if len(privateKey) == 0 {
		t.Fatal("expected non-empty private key")
	}
	if len(publicKey) == 0 {
		t.Fatal("expected non-empty public key")
	}
	if len(privateKey) != 43 {
		t.Fatalf("expected 43-char base64 private key, got %d chars: %q", len(privateKey), privateKey)
	}
	if len(publicKey) != 43 {
		t.Fatalf("expected 43-char base64 public key, got %d chars: %q", len(publicKey), publicKey)
	}
	derivedPublicKey, err := xray.DeriveRealityPublicKey(privateKey)
	if err != nil {
		t.Fatalf("derive reality public key: %v", err)
	}
	if derivedPublicKey != publicKey {
		t.Fatalf("derived public key mismatch: got %q want %q", derivedPublicKey, publicKey)
	}
	privateBytes, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil {
		t.Fatalf("decode private key: %v", err)
	}
	if len(privateBytes) != 32 {
		t.Fatalf("expected 32-byte private key, got %d", len(privateBytes))
	}
	publicBytes, err := base64.RawURLEncoding.DecodeString(publicKey)
	if err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	if len(publicBytes) != 32 {
		t.Fatalf("expected 32-byte public key, got %d", len(publicBytes))
	}
}

func TestDeriveRealityPublicKeyFromPrivateKey(t *testing.T) {
	privateKey := "dKdt8fbC0s2mASfADxazWqIsOtlNHfbYuj4o8tSIxG8"
	wantPublicKey := "Q8Eqhqi1-1wRWZiZ9FYrO5__8ihMO1KrC5W2V-qOp28"
	got, err := xray.DeriveRealityPublicKey(privateKey)
	if err != nil {
		t.Fatalf("derive reality public key: %v", err)
	}
	if got != wantPublicKey {
		t.Fatalf("derived public key = %q, want %q", got, wantPublicKey)
	}
}

func TestRealityKeyGenerationWorksWithoutXrayCLI(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty-bin"))
	if err := os.MkdirAll(os.Getenv("PATH"), 0755); err != nil {
		t.Fatalf("create empty PATH dir: %v", err)
	}
	privateKey, publicKey, err := xray.GenerateRealityKey()
	if err != nil {
		t.Fatalf("generate reality key without xray CLI: %v", err)
	}
	derivedPublicKey, err := xray.DeriveRealityPublicKey(privateKey)
	if err != nil {
		t.Fatalf("derive reality public key without xray CLI: %v", err)
	}
	if derivedPublicKey != publicKey {
		t.Fatalf("derived public key mismatch without CLI: got %q want %q", derivedPublicKey, publicKey)
	}
}
