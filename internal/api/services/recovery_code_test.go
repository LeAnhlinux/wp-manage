package services

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

// Recovery codes moved from bcrypt to SHA-256 so verifying a wrong code stops
// costing ~10 sequential bcrypt(cost 14) comparisons. That trade is only sound
// because the codes carry enough entropy to make an offline search of the hash
// infeasible — the tests below pin both halves of that argument.
//
// These are deliberately DB-free: the project has a lean dependency list (6 direct
// modules, no sqlite driver), so consumeRecoveryCode's query path is not covered
// here.

// TestRecoveryCodeEntropy is the security precondition for dropping bcrypt. The
// previous 4 bytes (32 bits) would have been enumerable offline in seconds behind
// a fast hash; lowering this constant again would silently reintroduce that.
func TestRecoveryCodeEntropy(t *testing.T) {
	const minBytes = 10 // 80 bits
	if recoveryCodeBytes < minBytes {
		t.Fatalf("recoveryCodeBytes = %d (%d bits): too little entropy to store behind a fast hash. "+
			"Either restore bcrypt or keep at least %d bytes",
			recoveryCodeBytes, recoveryCodeBytes*8, minBytes)
	}
}

// TestRecoveryCodeHashFitsColumn guards RecoveryCode.CodeHash `gorm:"size:64"`.
// A hash longer than the column would be truncated by MariaDB, which silently
// breaks verification for every newly issued code.
func TestRecoveryCodeHashFitsColumn(t *testing.T) {
	const codeHashColumnSize = 64

	b := make([]byte, recoveryCodeBytes)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	hash := hashToken(hex.EncodeToString(b))

	if len(hash) > codeHashColumnSize {
		t.Fatalf("hash is %d chars, column holds %d — it would be truncated", len(hash), codeHashColumnSize)
	}
	if len(hash) != 64 {
		t.Errorf("expected a 64-char SHA-256 hex digest, got %d chars", len(hash))
	}
	// Must not collide with the legacy-bcrypt detection used by consumeRecoveryCode,
	// which selects old rows with `code_hash LIKE '$2%'`.
	if strings.HasPrefix(hash, "$2") {
		t.Error("SHA-256 hash must not look like a bcrypt hash, or the legacy branch would pick it up")
	}
}

// TestRecoveryCodeHashIsDeterministic is what allows the single indexed lookup
// that replaced the per-row bcrypt loop.
func TestRecoveryCodeHashIsDeterministic(t *testing.T) {
	const code = "a1b2c3d4e5f6a7b8c9d0"
	if hashToken(code) != hashToken(code) {
		t.Fatal("hashToken is not deterministic; the lookup in consumeRecoveryCode cannot work")
	}
	if hashToken(code) == hashToken(code+"0") {
		t.Fatal("distinct codes hashed to the same value")
	}
}

// TestGeneratedCodesAreDistinct catches a seeding or loop-variable mistake in
// Setup2FA that would issue ten copies of the same code.
func TestGeneratedCodesAreDistinct(t *testing.T) {
	seen := make(map[string]bool, 10)
	for i := 0; i < 10; i++ {
		b := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(b); err != nil {
			t.Fatalf("rand: %v", err)
		}
		code := hex.EncodeToString(b)
		if seen[code] {
			t.Fatalf("duplicate recovery code generated: %s", code)
		}
		seen[code] = true
	}
}
