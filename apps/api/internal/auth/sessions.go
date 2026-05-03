package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const sessionTokenSize = 32

// generateSessionToken returns a base64url-encoded 256-bit random token suitable
// for a cookie value, plus the SHA-256 hash that should be stored in DB.
func generateSessionToken() (token string, hash []byte, err error) {
	raw := make([]byte, sessionTokenSize)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("rand: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	return token, sum[:], nil
}

// hashCookieToken returns the SHA-256 hash that DB stores for a cookie value.
func hashCookieToken(token string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("token decode: %w", err)
	}
	if len(raw) != sessionTokenSize {
		return nil, fmt.Errorf("token length %d, want %d", len(raw), sessionTokenSize)
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}
