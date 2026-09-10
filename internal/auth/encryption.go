package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"noumena/internal/config"
)

// versionedPrefix marks ciphertext produced by Encrypt since the
// fix-crypto-http-hardening change. Payload format:
//
//	v1:<keyID>:<base64(nonce || ciphertext)>
//
// keyID is a stable short identifier derived from the key itself, so
// decrypt can select the right key without a schema column. Legacy
// ciphertext written before this change has no prefix; Decrypt falls
// back to trying the current then previous key for those values.
const versionedPrefix = "v1:"

// keySizeValid reports whether raw bytes are a legal AES key size.
func keySizeValid(key []byte) bool {
	return len(key) == 16 || len(key) == 24 || len(key) == 32
}

// keyIDFor returns the first 8 hex chars of sha256(key): a stable,
// collision-resistant identifier that never exposes the key material.
func keyIDFor(key []byte) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:4])
}

// encryptionKeys returns the configured key material. The current key
// is mandatory (config.Load refuses to boot without it); the previous
// key is optional and used only for decryption during rotation.
func encryptionKeys() (current, previous []byte, err error) {
	cfg := config.Default
	if cfg == nil || len(cfg.EncryptionKey) == 0 {
		return nil, nil, errors.New("ENCRYPTION_KEY is not configured; refusing to encrypt")
	}
	if !keySizeValid(cfg.EncryptionKey) {
		return nil, nil, fmt.Errorf("ENCRYPTION_KEY must be 16, 24, or 32 bytes (got %d)", len(cfg.EncryptionKey))
	}
	if prev := cfg.EncryptionKeyPrevious; len(prev) > 0 && !keySizeValid(prev) {
		return nil, nil, fmt.Errorf("ENCRYPTION_KEY_PREVIOUS must be 16, 24, or 32 bytes (got %d)", len(prev))
	}
	return cfg.EncryptionKey, cfg.EncryptionKeyPrevious, nil
}

// gcmFor builds an AES-GCM AEAD for the given raw key bytes.
func gcmFor(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt encrypts a string using AES-GCM with the current key and
// returns a versioned, base64-encoded string. Empty input yields empty
// output (the callers treat empty as "nothing stored").
func Encrypt(text string) (string, error) {
	if text == "" {
		return "", nil
	}

	current, _, err := encryptionKeys()
	if err != nil {
		return "", err
	}

	aesGCM, err := gcmFor(current)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(text), nil)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return versionedPrefix + keyIDFor(current) + ":" + encoded, nil
}

// decryptWith decrypts raw nonce||ciphertext bytes with a single key.
func decryptWith(aesGCM cipher.AEAD, encoded string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, body := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// Decrypt decrypts a versioned or legacy base64-encoded AES-GCM string.
// Versioned payloads select the key by ID; legacy payloads (no prefix)
// are tried against the current key first, then the previous key so that
// rows written before a rotation remain readable.
func Decrypt(encodedText string) (string, error) {
	if encodedText == "" {
		return "", nil
	}

	current, previous, err := encryptionKeys()
	if err != nil {
		return "", err
	}
	currentGCM, err := gcmFor(current)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(encodedText, versionedPrefix) {
		plain, err := decryptWith(currentGCM, encodedText)
		if err == nil {
			return plain, nil
		}
		if len(previous) > 0 {
			prevGCM, gerr := gcmFor(previous)
			if gerr != nil {
				return "", gerr
			}
			if plain, perr := decryptWith(prevGCM, encodedText); perr == nil {
				return plain, nil
			}
		}
		return "", errors.New("decryption failed: corrupted ciphertext or unknown key")
	}

	rest := strings.TrimPrefix(encodedText, versionedPrefix)
	sep := strings.IndexByte(rest, ':')
	if sep <= 0 || sep == len(rest)-1 {
		return "", errors.New("decryption failed: malformed key version header")
	}
	return decryptVersioned(rest[:sep], rest[sep+1:], current, previous, currentGCM)
}

// decryptVersioned selects the key by ID and decrypts the payload.
func decryptVersioned(keyID, payload string, current, previous []byte, currentGCM cipher.AEAD) (string, error) {
	if keyID == keyIDFor(current) {
		return decryptChecked(currentGCM, payload)
	}
	if keyID == keyIDFor(previous) {
		return decryptWithPrevious(previous, payload)
	}
	return "", fmt.Errorf("decryption failed: no key matches ciphertext key id %q (key rotated and old key discarded?)", keyID)
}

// decryptChecked decrypts with a resolved AEAD; any failure means corrupted
// ciphertext.
func decryptChecked(aesGCM cipher.AEAD, payload string) (string, error) {
	plain, err := decryptWith(aesGCM, payload)
	if err != nil {
		return "", errors.New("decryption failed: corrupted ciphertext")
	}
	return plain, nil
}

func decryptWithPrevious(previous []byte, payload string) (string, error) {
	if len(previous) == 0 {
		return "", errors.New("decryption failed: ciphertext keyed with ENCRYPTION_KEY_PREVIOUS which is not configured")
	}
	prevGCM, err := gcmFor(previous)
	if err != nil {
		return "", err
	}
	return decryptChecked(prevGCM, payload)
}
