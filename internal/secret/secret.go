// Package secret encrypts small values (TLS keys, SSH keys, client secrets)
// before they are stored, using AES-256-GCM and the LOCCI_SECRET_KEY.
//
// Sealed values look like "v1:<base64(nonce|ciphertext)>"; the prefix leaves
// room for key rotation later.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
)

// ErrNoKey is returned when a secret must be stored or read but no key is set.
var ErrNoKey = errors.New("LOCCI_SECRET_KEY is not set; generate one with `openssl rand -base64 32` and restart the server")

const prefix = "v1:"

// Box seals and opens values. A nil *Box has no key and refuses both.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a base64-encoded 32-byte key. An empty key returns
// (nil, nil): the server still runs, it just can't store secrets.
func New(key string) (*Box, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("LOCCI_SECRET_KEY must be 32 random bytes, base64 encoded (`openssl rand -base64 32`)")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plain. Sealing "" returns "" so empty fields stay empty.
func (b *Box) Seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if b == nil {
		return "", ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.StdEncoding.EncodeToString(out), nil
}

// Open decrypts a value made by Seal.
func (b *Box) Open(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	if b == nil {
		return "", ErrNoKey
	}
	data, ok := strings.CutPrefix(sealed, prefix)
	if !ok {
		return "", errors.New("secret: unknown format")
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", errors.New("secret: corrupt value")
	}
	n := b.aead.NonceSize()
	plain, err := b.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		// Wrong key or tampered data; don't leak which.
		return "", errors.New("secret: can't decrypt (was LOCCI_SECRET_KEY changed?)")
	}
	return string(plain), nil
}
