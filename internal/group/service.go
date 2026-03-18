package group

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

// EncryptSecret encrypts plaintext using AES-256-GCM with the provided key.
// The returned string is hex-encoded: nonce || ciphertext || tag.
// key must be exactly 32 bytes (derived from config security.secret_key).
func EncryptSecret(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(sealed), nil
}

// DecryptSecret reverses EncryptSecret.
func DecryptSecret(cipherHex string, key []byte) (string, error) {
	data, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", fmt.Errorf("hex decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	plain, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plain), nil
}

// GenerateWebhookSecret generates a cryptographically random 32-byte hex string
// used as the per-app HMAC Authorization token.
func GenerateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// WebhookURL builds the full webhook URL for an app.
// baseURL is the externally reachable address of the proxy (e.g. "http://flowgate:8080").
func WebhookURL(baseURL, groupName, appName string) string {
	return fmt.Sprintf("%s/webhook/%s/%s", baseURL, groupName, appName)
}

// DeriveKey converts a hex or plain-text secret_key string into a 32-byte AES key.
// If the string is 64 hex chars it is decoded directly; otherwise it is zero-padded/truncated.
func DeriveKey(secretKey string) ([]byte, error) {
	if len(secretKey) == 64 {
		decoded, err := hex.DecodeString(secretKey)
		if err == nil {
			return decoded, nil
		}
	}
	// Pad or truncate to 32 bytes
	key := make([]byte, 32)
	copy(key, []byte(secretKey))
	return key, nil
}
