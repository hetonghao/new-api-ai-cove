package common

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	MinAccountPasswordLength = 8
	MaxAccountPasswordLength = 128
	accountPasswordMemory    = 19 * 1024
	accountPasswordTime      = 2
	accountPasswordSaltBytes = 16
	accountPasswordKeyBytes  = 32
	// argon2idPHCPrefix is the algorithm identifier of the modular PHC string
	// format. Verification dispatches on it, so every Argon2id hash must carry it.
	argon2idPHCPrefix = "$argon2id$"
)

var (
	ErrAccountPasswordLength = errors.New("Password must contain between 8 and 128 characters.")
	ErrAccountPasswordSame   = errors.New("New password must be different from current password")
	ErrPasswordLegacyLimit   = errors.New("Long passwords are unavailable until the password storage upgrade is complete.")
)

// ValidateNewAccountPassword applies only when a user chooses a new password.
// Authentication must continue to accept historical passwords without applying
// the new policy. Do not normalize passwords, including surrounding whitespace.
func ValidateNewAccountPassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < MinAccountPasswordLength || utf8.RuneCountInString(password) > MaxAccountPasswordLength {
		return ErrAccountPasswordLength
	}
	return nil
}

// HashAccountPassword is for account passwords, not MFA backup codes. Argon2id
// is the default write format; ACCOUNT_PASSWORD_HASH_ALGORITHM=bcrypt keeps the
// legacy writer available for rollback. Existing hashes are never rewritten in
// bulk, and ValidatePasswordAndHash reads both formats.
func HashAccountPassword(password string) (string, error) {
	if err := ValidateNewAccountPassword(password); err != nil {
		return "", err
	}
	switch os.Getenv("ACCOUNT_PASSWORD_HASH_ALGORITHM") {
	case "bcrypt":
		if len(password) > 72 {
			return "", ErrPasswordLegacyLimit
		}
		return Password2Hash(password)
	case "", "argon2id":
	default:
		return "", errors.New("Unsupported account password hashing configuration.")
	}
	salt := make([]byte, accountPasswordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate account password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, accountPasswordTime, accountPasswordMemory, 1, accountPasswordKeyBytes)
	return fmt.Sprintf("%sv=19$m=19456,t=2,p=1$%s$%s", argon2idPHCPrefix, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func validateArgon2AccountPassword(password, encoded string) bool {
	// Bound both plaintext and parameters before invoking a memory-hard KDF.
	// Only the version/parameters emitted by this application are accepted.
	if len(password) > MaxAccountPasswordLength*utf8.UTFMax || len(encoded) > 256 {
		return false
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=19456,t=2,p=1" {
		return false
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != accountPasswordSaltBytes {
		return false
	}
	expected, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(expected) != accountPasswordKeyBytes {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, accountPasswordTime, accountPasswordMemory, 1, accountPasswordKeyBytes)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
