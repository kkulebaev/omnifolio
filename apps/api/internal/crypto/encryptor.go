package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
)

const (
	credentialsLabel = "credentials.v1"
	credentialsKeyVersion = 1
	gcmNonceSize     = 12
)

var (
	ErrCipherInvalid = errors.New("crypto: cipher invalid")
)

// sha256New is exposed as a hash.Hash factory for hkdf.
func sha256New() hash.Hash { return sha256.New() }

// Encryptor encrypts and decrypts blobs using AES-256-GCM with a HKDF-derived key.
// nonce is generated randomly per call and stored alongside ciphertext.
type Encryptor struct {
	gcm        cipher.AEAD
	keyVersion int
}

func NewEncryptor(masterKey []byte) (*Encryptor, error) {
	credKey, err := DeriveKey(masterKey, credentialsLabel, 32)
	if err != nil {
		return nil, fmt.Errorf("derive credentials key: %w", err)
	}
	block, err := aes.NewCipher(credKey)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	return &Encryptor{gcm: gcm, keyVersion: credentialsKeyVersion}, nil
}

// KeyVersion returns the version associated with the current credentials key.
func (e *Encryptor) KeyVersion() int { return e.keyVersion }

// Encrypt seals plaintext, returning ciphertext and the random nonce used.
// aad is bound into authentication; pass deterministic data (e.g. account_id bytes)
// to prevent ciphertext substitution between rows.
func (e *Encryptor) Encrypt(plaintext, aad []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, gcmNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("rand nonce: %w", err)
	}
	ct := e.gcm.Seal(nil, nonce, plaintext, aad)
	return ct, nonce, nil
}

// Decrypt opens ciphertext using nonce and aad. Returns ErrCipherInvalid on auth fail.
func (e *Encryptor) Decrypt(ciphertext, nonce, aad []byte) ([]byte, error) {
	if len(nonce) != gcmNonceSize {
		return nil, fmt.Errorf("%w: nonce size %d", ErrCipherInvalid, len(nonce))
	}
	pt, err := e.gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCipherInvalid, err)
	}
	return pt, nil
}
