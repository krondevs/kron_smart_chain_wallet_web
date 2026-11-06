package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	KeySize   = 32
	NonceSize = 12
)

// Manager provides authenticated encryption for wallet passphrases and TOTP
// secrets. The master key never lives in the database.
type Manager struct {
	key []byte
}

// LoadOrCreate resolves the master key in this order:
//  1. settings.json master_key (base64, 32 bytes)
//  2. settings.json master_key_file
//  3. <dataDir>/master.key, generated on first run
//
// The generated file is convenient for development but should be replaced by
// a KMS/secret manager in production.
func LoadOrCreate(encoded, file, dataDir string) (*Manager, error) {
	var key []byte
	var err error

	switch {
	case strings.TrimSpace(encoded) != "":
		key, err = decodeKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("settings master_key: %w", err)
		}
	case strings.TrimSpace(file) != "":
		key, err = readKeyFile(file)
		if err != nil {
			return nil, fmt.Errorf("settings master_key_file: %w", err)
		}
	default:
		path := filepath.Join(dataDir, "master.key")
		key, err = readKeyFile(path)
		if errors.Is(err, os.ErrNotExist) {
			key, err = generateAndSave(path)
		}
		if err != nil {
			return nil, fmt.Errorf("master key: %w", err)
		}
	}

	if len(key) != KeySize {
		return nil, fmt.Errorf("master key must be %d bytes, got %d", KeySize, len(key))
	}
	return &Manager{key: key}, nil
}

func decodeKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := hex.DecodeString(s); err == nil {
		return b, nil
	}
	return nil, errors.New("expected base64 or hex encoded 32-byte key")
}

func readKeyFile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeKey(string(b))
}

func generateAndSave(path string) ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(key) + "\n"
	if err := os.WriteFile(path, []byte(encoded), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (m *Manager) Encrypt(plaintext []byte, aad string) (ciphertextB64, nonceB64 string, err error) {
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}
	ct := gcm.Seal(nil, nonce, plaintext, []byte(aad))
	return base64.StdEncoding.EncodeToString(ct), base64.StdEncoding.EncodeToString(nonce), nil
}

func (m *Manager) Decrypt(ciphertextB64, nonceB64, aad string) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, errors.New("invalid ciphertext encoding")
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, errors.New("invalid nonce encoding")
	}
	block, err := aes.NewCipher(m.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid nonce size")
	}
	pt, err := gcm.Open(nil, nonce, ct, []byte(aad))
	if err != nil {
		return nil, errors.New("decryption failed")
	}
	return pt, nil
}

func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func RandomToken(n int) (string, error) {
	b, err := RandomBytes(n)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func HashRecoveryCode(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
