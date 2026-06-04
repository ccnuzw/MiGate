package xray

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GenerateRealityKey generates a REALITY X25519 key pair using xray x25519.
// Returns the private key and public key on success.
func GenerateRealityKey() (privateKey, publicKey string, err error) {
	cmd := exec.Command("xray", "x25519")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("xray x25519: %w", err)
	}
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PrivateKey:") {
			privateKey = strings.TrimSpace(strings.TrimPrefix(line, "PrivateKey:"))
		} else if strings.HasPrefix(line, "Password (PublicKey):") {
			publicKey = strings.TrimSpace(strings.TrimPrefix(line, "Password (PublicKey):"))
		}
	}
	if privateKey == "" {
		return "", "", fmt.Errorf("could not parse xray x25519 output: %s", out.String())
	}
	return privateKey, publicKey, nil
}
