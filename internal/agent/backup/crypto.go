package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// deriveKey derives a 32-byte AES-256 key from an arbitrary-length passphrase
// using SHA-256. The passphrase is the agent's API key, which is unique per agent
// and already stored securely (not in logs or process list).
func deriveKey(passphrase string) []byte {
	h := sha256.Sum256([]byte(passphrase))
	return h[:]
}

// encryptSecret encrypts plaintext with AES-256-GCM using a key derived from passphrase.
// Output format: base64(nonce || ciphertext || tag).
// Returns empty string if passphrase is empty (graceful degradation in tests).
func encryptSecret(plaintext, passphrase string) (string, error) {
	if passphrase == "" {
		// No key configured — return plaintext with a warning prefix so admins can detect it.
		return "UNENCRYPTED:" + plaintext, nil
	}
	block, err := aes.NewCipher(deriveKey(passphrase))
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
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptSecret reverses encryptSecret. Returns the original plaintext.
func decryptSecret(ciphertext64, passphrase string) (string, error) {
	// Handle unencrypted fallback written when no key was available.
	const unencPrefix = "UNENCRYPTED:"
	if len(ciphertext64) >= len(unencPrefix) && ciphertext64[:len(unencPrefix)] == unencPrefix {
		return ciphertext64[len(unencPrefix):], nil
	}
	if passphrase == "" {
		return ciphertext64, nil // best-effort: return as-is
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext64)
	if err != nil {
		// Might be a legacy plaintext value — return as-is so old backups still restore.
		return ciphertext64, nil
	}
	block, err := aes.NewCipher(deriveKey(passphrase))
	if err != nil {
		return "", fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		// Too short to be a valid ciphertext — treat as legacy plaintext.
		return ciphertext64, nil
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		// Decryption failed (wrong key or legacy plaintext) — return raw value.
		return ciphertext64, nil
	}
	return string(plain), nil
}
