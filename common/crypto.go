package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	credentialCipherVersion = "v1"
	credentialCipherAAD     = "ai-cove:risk-provider-credential:v1"
)

func GenerateHMACWithKey(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateHMAC(data string) string {
	h := hmac.New(sha256.New, []byte(CryptoSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func Password2Hash(password string) (string, error) {
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	return string(hashedPassword), err
}

func ValidatePasswordAndHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func EncryptCredential(plaintext string) (string, error) {
	gcm, err := credentialCipher()
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate credential nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), []byte(credentialCipherAAD))
	return credentialCipherVersion + ":" + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func DecryptCredential(encoded string) (string, error) {
	version, payload, ok := strings.Cut(encoded, ":")
	if !ok || version != credentialCipherVersion {
		return "", errors.New("unsupported credential ciphertext")
	}

	sealed, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", errors.New("invalid credential ciphertext")
	}
	gcm, err := credentialCipher()
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("invalid credential ciphertext")
	}

	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(credentialCipherAAD))
	if err != nil {
		return "", errors.New("decrypt credential")
	}
	return string(plaintext), nil
}

func credentialCipher() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential gcm: %w", err)
	}
	return gcm, nil
}
