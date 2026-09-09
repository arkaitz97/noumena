package auth_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"noumena/internal/auth"
	"noumena/internal/config"
)

// setKeys installs the encryption keys on config.Default for a test and
// restores the previous values on cleanup. config.Load normally does this
// at startup; these tests drive the auth package directly.
func setKeys(t *testing.T, current, previous []byte) {
	t.Helper()
	prevCur := config.Default.EncryptionKey
	prevPrev := config.Default.EncryptionKeyPrevious
	config.Default.EncryptionKey = current
	config.Default.EncryptionKeyPrevious = previous
	t.Cleanup(func() {
		config.Default.EncryptionKey = prevCur
		config.Default.EncryptionKeyPrevious = prevPrev
	})
}

const (
	keyCurrent = "0123456789abcdef0123456789abcdef" // 32 bytes
	keyOld     = "fedcba9876543210fedcba9876543210" // 32 bytes
)

func TestEncryptDecryptRoundTripWithCurrentKey(t *testing.T) {
	setKeys(t, []byte(keyCurrent), nil)
	plain := "ya29.some-oauth-token-secret"
	enc, err := auth.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, "v1:") {
		t.Errorf("ciphertext missing version prefix: %q", enc)
	}
	if strings.Contains(enc, plain) {
		t.Error("ciphertext leaked plaintext")
	}
	got, err := auth.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Errorf("round trip: got %q want %q", got, plain)
	}
}

func TestDecryptWithPreviousKeyAfterRotation(t *testing.T) {
	// Phase 1: data encrypted under the old key.
	setKeys(t, []byte(keyOld), nil)
	plain := "refresh-token-1//secret"
	oldEnc, err := auth.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt (old key): %v", err)
	}

	// Phase 2: rotate — old key becomes previous, new key is current.
	setKeys(t, []byte(keyCurrent), []byte(keyOld))

	got, err := auth.Decrypt(oldEnc)
	if err != nil {
		t.Fatalf("Decrypt old ciphertext with previous key: %v", err)
	}
	if got != plain {
		t.Errorf("decrypted: got %q want %q", got, plain)
	}

	// New writes use the current key and decrypt with it.
	newEnc, err := auth.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt (current key): %v", err)
	}
	got2, err := auth.Decrypt(newEnc)
	if err != nil {
		t.Fatalf("Decrypt current ciphertext: %v", err)
	}
	if got2 != plain {
		t.Errorf("decrypted current: got %q want %q", got2, plain)
	}
}

func TestLegacyCiphertextStillDecrypts(t *testing.T) {
	// Values written before key versioning have no prefix. They must
	// still decrypt with the current key (no key change) and with the
	// previous key after a rotation.
	setKeys(t, []byte(keyOld), nil)

	// Produce a legacy (unprefixed) value the way the old code did.
	legacy, err := legacyEncrypt([]byte(keyOld), "legacy-secret-token")
	if err != nil {
		t.Fatalf("legacyEncrypt: %v", err)
	}
	if strings.HasPrefix(legacy, "v1:") {
		t.Fatal("legacy fixture must not be versioned")
	}

	if got, err := auth.Decrypt(legacy); err != nil || got != "legacy-secret-token" {
		t.Errorf("legacy decrypt (same key): got %q err %v", got, err)
	}

	setKeys(t, []byte(keyCurrent), []byte(keyOld))
	if got, err := auth.Decrypt(legacy); err != nil || got != "legacy-secret-token" {
		t.Errorf("legacy decrypt (after rotation, previous key): got %q err %v", got, err)
	}
}

func TestTamperedPrefixFails(t *testing.T) {
	setKeys(t, []byte(keyCurrent), []byte(keyOld))
	enc, err := auth.Encrypt("hello")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Unknown key ID.
	unknown := "v1:deadbeef:" + strings.SplitN(enc, ":", 3)[2]
	if _, err := auth.Decrypt(unknown); err == nil {
		t.Error("expected error for unknown key id, got nil")
	}

	// Malformed header (empty key id).
	if _, err := auth.Decrypt("v1::AAAA"); err == nil {
		t.Error("expected error for malformed key header, got nil")
	}

	// Truncated payload.
	if _, err := auth.Decrypt(enc[:len(enc)-6]); err == nil {
		t.Error("expected error for truncated ciphertext, got nil")
	}

	// Garbage base64 under a valid key id.
	validID := strings.SplitN(strings.TrimPrefix(enc, "v1:"), ":", 2)[0]
	if _, err := auth.Decrypt("v1:" + validID + ":!!!!"); err == nil {
		t.Error("expected error for corrupted payload, got nil")
	}
}

// legacyEncrypt reproduces the pre-key-versioning ciphertext format:
// base64(nonce || ciphertext) with no version prefix.
func legacyEncrypt(rawKey []byte, text string) (string, error) {
	block, err := aes.NewCipher(rawKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(text), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func TestDecryptFailsWithoutPreviousKey(t *testing.T) {
	// Ciphertext keyed with the previous key must fail when the previous
	// key is not configured (i.e. it was discarded).
	setKeys(t, []byte(keyOld), nil)
	enc, err := auth.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	setKeys(t, []byte(keyCurrent), nil)
	if _, err := auth.Decrypt(enc); err == nil {
		t.Error("expected error decrypting with discarded previous key")
	}
}

func TestEncryptUnsetKeyErrors(t *testing.T) {
	setKeys(t, nil, nil)
	if _, err := auth.Encrypt("secret"); err == nil {
		t.Error("expected error when ENCRYPTION_KEY is not configured")
	}
	if _, err := auth.Decrypt("v1:abc:AAAA"); err == nil {
		t.Error("expected error when ENCRYPTION_KEY is not configured")
	}
}
