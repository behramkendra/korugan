// Package crypto seals secrets (LLM keys, provider tokens) with
// AES-256-GCM before they touch PostgreSQL. The master key never leaves
// process memory; ciphertext and nonce are what the store persists.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

type Sealer struct {
	aead cipher.AEAD
}

// NewSealer accepts a base64-encoded 32-byte master key
// (KORUGAN_MASTER_KEY).
func NewSealer(masterKeyB64 string) (*Sealer, error) {
	if masterKeyB64 == "" {
		return nil, errors.New("crypto: empty master key")
	}
	key, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("crypto: master key is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: master key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// GenerateMasterKey returns a fresh base64 key for first boot.
func GenerateMasterKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func (s *Sealer) Seal(plaintext []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return s.aead.Seal(nil, nonce, plaintext, nil), nonce, nil
}

func (s *Sealer) Open(ciphertext, nonce []byte) ([]byte, error) {
	if len(nonce) != s.aead.NonceSize() {
		return nil, errors.New("crypto: bad nonce length")
	}
	return s.aead.Open(nil, nonce, ciphertext, nil)
}

// Mask renders a secret for UI/logs: first 5 + last 4 characters at most.
func Mask(secret string) string {
	if len(secret) <= 9 {
		return "****"
	}
	return secret[:5] + "…" + secret[len(secret)-4:]
}
