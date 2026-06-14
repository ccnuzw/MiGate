package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordHashPrefix = "$migate$argon2id$v=19$"
	argon2Time         = uint32(3)
	argon2Memory       = uint32(64 * 1024)
	argon2Threads      = uint8(1)
	argon2KeyLen       = uint32(32)
	argon2SaltLen      = 16
)

func HashPanelPassword(password string) (string, error) {
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return fmt.Sprintf("%sm=%d,t=%d,p=%d$%s$%s",
		passwordHashPrefix,
		argon2Memory,
		argon2Time,
		argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func IsPanelPasswordHash(value string) bool {
	return strings.HasPrefix(value, passwordHashPrefix)
}

func VerifyPanelPassword(stored, password string) bool {
	if !IsPanelPasswordHash(stored) {
		return constantTimeStringEqual(password, stored)
	}
	memory, timeCost, threads, salt, want, ok := parsePanelPasswordHash(stored)
	if !ok {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func PanelPasswordUsesDefault(stored string) bool {
	return VerifyPanelPassword(stored, "admin")
}

func parsePanelPasswordHash(stored string) (uint32, uint32, uint8, []byte, []byte, bool) {
	rest := strings.TrimPrefix(stored, passwordHashPrefix)
	parts := strings.Split(rest, "$")
	if len(parts) != 3 {
		return 0, 0, 0, nil, nil, false
	}
	var memory, timeCost uint64
	var threads uint64
	for _, param := range strings.Split(parts[0], ",") {
		key, value, ok := strings.Cut(param, "=")
		if !ok {
			return 0, 0, 0, nil, nil, false
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return 0, 0, 0, nil, nil, false
		}
		switch key {
		case "m":
			memory = parsed
		case "t":
			timeCost = parsed
		case "p":
			threads = parsed
		default:
			return 0, 0, 0, nil, nil, false
		}
	}
	if memory == 0 || timeCost == 0 || threads == 0 || threads > 255 {
		return 0, 0, 0, nil, nil, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil || len(salt) == 0 {
		return 0, 0, 0, nil, nil, false
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(hash) == 0 {
		return 0, 0, 0, nil, nil, false
	}
	return uint32(memory), uint32(timeCost), uint8(threads), salt, hash, true
}
