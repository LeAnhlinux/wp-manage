package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// Encryption at rest for third-party credentials the panel must be able to
// read back (DNS provider tokens and the like). Unlike passwords and the
// panel's own API keys, these cannot be hashed: the panel has to present the
// original value to the provider.

// secretPrefix marks a value encrypted by EncryptSecret. Anything without it
// is treated as legacy plaintext, which is how rows written before encryption
// existed keep working until the startup migration rewrites them.
const secretPrefix = "enc:v1:"

// DeriveSecretKey turns key material (a long random string from the
// environment) into a 32-byte AES-256 key. The label keeps this key different
// from any other use of the same material, such as signing JWTs.
func DeriveSecretKey(material string) []byte {
	sum := sha256.Sum256([]byte("wp-manage/secrets/v1\x00" + material))
	return sum[:]
}

// IsEncryptedSecret reports whether a stored value was produced by EncryptSecret.
func IsEncryptedSecret(stored string) bool {
	return strings.HasPrefix(stored, secretPrefix)
}

// EncryptSecret encrypts plaintext with AES-256-GCM. The empty string stays
// empty, so optional fields remain visibly empty in the database.
func EncryptSecret(key []byte, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("secret key: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return secretPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptSecret reverses EncryptSecret. A value without the prefix is returned
// unchanged (legacy plaintext). A value that cannot be decrypted — wrong key or
// tampered data — is an error, never silently returned as-is.
func DecryptSecret(key []byte, stored string) (string, error) {
	if !IsEncryptedSecret(stored) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, secretPrefix))
	if err != nil {
		return "", fmt.Errorf("stored secret is corrupted")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("secret key: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("stored secret is corrupted")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("stored secret cannot be decrypted with the current key")
	}
	return string(plain), nil
}
