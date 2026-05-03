package crypto

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const masterKeySize = 32

var ErrInvalidMasterKey = errors.New("crypto: invalid master key")

// ParseMasterKey decodes a base64url-encoded 32-byte master key.
func ParseMasterKey(s string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: not base64url", ErrInvalidMasterKey)
	}
	if len(b) != masterKeySize {
		return nil, fmt.Errorf("%w: length %d, want %d", ErrInvalidMasterKey, len(b), masterKeySize)
	}
	return b, nil
}

// DeriveKey expands the master key into a sub-key for a specific domain (HKDF-Expand).
// The label provides domain separation between purposes (e.g. "credentials.v1").
func DeriveKey(masterKey []byte, label string, size int) ([]byte, error) {
	r := hkdf.Expand(sha256New, masterKey, []byte(label))
	out := make([]byte, size)
	if _, err := io.ReadFull(r, out); err != nil {
		return nil, fmt.Errorf("hkdf expand: %w", err)
	}
	return out, nil
}
