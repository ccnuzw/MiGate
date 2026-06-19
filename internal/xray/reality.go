package xray

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// DeriveRealityPublicKey derives the REALITY public key from a private key.
func DeriveRealityPublicKey(privateKey string) (string, error) {
	return deriveRealityPublicKeyLocal(privateKey)
}

// GenerateRealityKey generates a REALITY X25519 key pair.
// Returns the private key and public key on success.
func GenerateRealityKey() (privateKey, publicKey string, err error) {
	return generateRealityKeyLocal()
}

func generateRealityKeyLocal() (privateKey, publicKey string, err error) {
	privateBytes := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateBytes); err != nil {
		return "", "", fmt.Errorf("generate x25519 private key: %w", err)
	}
	publicBytes, err := curve25519.X25519(privateBytes, curve25519.Basepoint)
	if err != nil {
		return "", "", fmt.Errorf("derive x25519 public key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(privateBytes), base64.RawURLEncoding.EncodeToString(publicBytes), nil
}

func deriveRealityPublicKeyLocal(privateKey string) (string, error) {
	privateBytes, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil {
		return "", err
	}
	if len(privateBytes) != curve25519.ScalarSize {
		return "", fmt.Errorf("invalid x25519 private key length: %d", len(privateBytes))
	}
	publicBytes, err := curve25519.X25519(privateBytes, curve25519.Basepoint)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(publicBytes), nil
}
