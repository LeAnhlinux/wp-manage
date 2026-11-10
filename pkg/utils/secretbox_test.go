package utils

import (
	"strings"
	"testing"
)

func TestSecretRoundTrip(t *testing.T) {
	key := DeriveSecretKey("a-long-random-server-secret-0123456789")
	for _, plain := range []string{"cf-token-abcdefghijklmnopqrstuvwxyz012345", "user@example.com", "ключ 🔑", strings.Repeat("x", 300)} {
		enc, err := EncryptSecret(key, plain)
		if err != nil {
			t.Fatal(err)
		}
		if !IsEncryptedSecret(enc) || strings.Contains(enc, plain) {
			t.Fatalf("not encrypted: %q", enc)
		}
		got, err := DecryptSecret(key, enc)
		if err != nil || got != plain {
			t.Fatalf("round trip: %q %v", got, err)
		}
	}
}

func TestSecretEdgeCases(t *testing.T) {
	key := DeriveSecretKey("material-one-0123456789-0123456789")

	// empty stays empty in both directions
	if enc, err := EncryptSecret(key, ""); err != nil || enc != "" {
		t.Errorf("empty: %q %v", enc, err)
	}
	if got, err := DecryptSecret(key, ""); err != nil || got != "" {
		t.Errorf("empty decrypt: %q %v", got, err)
	}

	// legacy plaintext (no prefix) passes through unchanged
	if got, err := DecryptSecret(key, "plain-legacy-token"); err != nil || got != "plain-legacy-token" {
		t.Errorf("legacy: %q %v", got, err)
	}

	// same plaintext encrypts differently each time (random nonce)
	a, _ := EncryptSecret(key, "same")
	b, _ := EncryptSecret(key, "same")
	if a == b {
		t.Error("nonce reuse: two encryptions are identical")
	}

	// wrong key and tampering are errors, never garbage or the ciphertext
	enc, _ := EncryptSecret(key, "top-secret")
	if got, err := DecryptSecret(DeriveSecretKey("material-two-0123456789-0123456789"), enc); err == nil || got != "" {
		t.Errorf("wrong key must fail: %q %v", got, err)
	}
	tampered := enc[:len(enc)-2] + "AA"
	if got, err := DecryptSecret(key, tampered); err == nil || got != "" {
		t.Errorf("tampered must fail: %q %v", got, err)
	}
	if _, err := DecryptSecret(key, "enc:v1:!!!not-base64"); err == nil {
		t.Error("corrupt base64 must fail")
	}
	if _, err := DecryptSecret(key, "enc:v1:AAAA"); err == nil {
		t.Error("too-short payload must fail")
	}

	// the derived key is independent of other uses of the same material
	if string(DeriveSecretKey("m")) == string(DeriveSecretKey("n")) {
		t.Error("different material must give different keys")
	}
}
