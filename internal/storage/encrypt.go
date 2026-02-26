package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	encPrefix   = "enc:v1:"
	keyFileName = ".encryption_key"
	keySize     = 32 // AES-256
)

// Encryptor provides AES-256-GCM encryption for sensitive fields stored in SQLite.
// A nil *Encryptor is safe to use — all methods become no-op passthroughs.
type Encryptor struct {
	aead cipher.AEAD
}

// NewEncryptor creates an Encryptor using a key from the CLICKNEST_ENCRYPTION_KEY
// env var (hex-encoded, 64 chars). If the env var is empty it falls back to an
// auto-generated key file at <dataDir>/.encryption_key (0600 permissions).
func NewEncryptor(dataDir string) (*Encryptor, error) {
	key, err := loadKey(dataDir)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	return &Encryptor{aead: aead}, nil
}

// Encrypt encrypts plaintext and returns a string with the "enc:v1:" prefix.
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if e == nil {
		return plaintext, nil
	}

	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := e.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a value previously produced by Encrypt.
// If the value does not have the "enc:v1:" prefix it is returned as-is
// (legacy plaintext passthrough).
func (e *Encryptor) Decrypt(value string) (string, error) {
	if e == nil {
		return value, nil
	}

	if !strings.HasPrefix(value, encPrefix) {
		return value, nil // legacy plaintext
	}

	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}

	nonceSize := e.aead.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := e.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting: %w", err)
	}

	return string(plaintext), nil
}

// EncryptPtr is a convenience wrapper for *string fields.
func (e *Encryptor) EncryptPtr(p *string) (*string, error) {
	if e == nil || p == nil {
		return p, nil
	}
	enc, err := e.Encrypt(*p)
	if err != nil {
		return nil, err
	}
	return &enc, nil
}

// DecryptPtr is a convenience wrapper for *string fields.
func (e *Encryptor) DecryptPtr(p *string) (*string, error) {
	if e == nil || p == nil {
		return p, nil
	}
	dec, err := e.Decrypt(*p)
	if err != nil {
		return nil, err
	}
	return &dec, nil
}

// loadKey reads the encryption key from CLICKNEST_ENCRYPTION_KEY env var
// or auto-generates a key file.
func loadKey(dataDir string) ([]byte, error) {
	if envKey := os.Getenv("CLICKNEST_ENCRYPTION_KEY"); envKey != "" {
		key, err := hex.DecodeString(envKey)
		if err != nil {
			return nil, fmt.Errorf("CLICKNEST_ENCRYPTION_KEY is not valid hex: %w", err)
		}
		if len(key) != keySize {
			return nil, fmt.Errorf("CLICKNEST_ENCRYPTION_KEY must be %d bytes (%d hex chars), got %d bytes", keySize, keySize*2, len(key))
		}
		return key, nil
	}

	keyPath := filepath.Join(dataDir, keyFileName)

	// Try to read existing key file.
	data, err := os.ReadFile(keyPath)
	if err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			return nil, fmt.Errorf("reading key file %s: invalid hex: %w", keyPath, err)
		}
		if len(key) != keySize {
			return nil, fmt.Errorf("key file %s: expected %d bytes, got %d", keyPath, keySize, len(key))
		}
		return key, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading key file %s: %w", keyPath, err)
	}

	// Auto-generate a new key.
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating encryption key: %w", err)
	}

	if err := os.WriteFile(keyPath, []byte(hex.EncodeToString(key)+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("writing key file %s: %w", keyPath, err)
	}

	return key, nil
}
