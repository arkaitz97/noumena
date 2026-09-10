package auth

import (
	"errors"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"noumena/internal/config"
)

// PasswordPolicyError describes why a password failed the policy.
type PasswordPolicyError struct {
	Reason string
}

func (e *PasswordPolicyError) Error() string { return e.Reason }

// HashPassword generates a bcrypt hash using the configured cost (>= 12).
func HashPassword(password string) (string, error) {
	cost := config.Default.BcryptCost
	if cost < 12 {
		cost = 12
	}
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// VerifyPassword returns true when the bcrypt hash matches the supplied
// password.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NeedsRehash reports whether the stored hash uses a bcrypt cost factor
// below the currently configured cost. Callers should rehash on next
// successful login when this returns true.
func NeedsRehash(hash string) bool {
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		// Unrecognised hash format — treat as needing rehash.
		return true
	}
	return cost < config.Default.BcryptCost
}

// ValidatePasswordPolicy enforces the minimum password complexity:
// >= 8 characters, >= 1 uppercase letter, >= 1 digit. Returns nil when the
// password meets the policy, or a *PasswordPolicyError otherwise.
func ValidatePasswordPolicy(pw string) error {
	if len(pw) < 8 {
		return &PasswordPolicyError{Reason: "password must be at least 8 characters"}
	}
	var hasUpper, hasDigit bool
	for _, r := range pw {
		hasUpper = hasUpper || unicode.IsUpper(r)
		hasDigit = hasDigit || unicode.IsDigit(r)
	}
	if !hasUpper {
		return &PasswordPolicyError{Reason: "password must contain at least one uppercase letter"}
	}
	if !hasDigit {
		return &PasswordPolicyError{Reason: "password must contain at least one digit"}
	}
	return nil
}

// IsPasswordPolicyError returns true if err is a *PasswordPolicyError.
func IsPasswordPolicyError(err error) bool {
	var pe *PasswordPolicyError
	return errors.As(err, &pe)
}
