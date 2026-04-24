package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

func deriveKey() []byte {
	hostname, _ := os.Hostname()
	data := hostname + "|" + os.Getenv("USERNAME") + "|" + os.Getenv("COMPUTERNAME")
	hash := sha256.Sum256([]byte(data))
	return hash[:]
}

func Encrypt(plaintext string) (string, error) {
	key := deriveKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(sealed), nil
}

func Decrypt(ciphertext string) (string, error) {
	key := deriveKey()
	data, err := hex.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode hex: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, sealed := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

const (
	pbkdf2Iterations = 100000
	saltSize         = 32
)

// EncryptWithPassword encrypts plaintext using a user-provided password.
// Returns "salt:nonce:ciphertext" as hex-encoded segments.
func EncryptWithPassword(plaintext, password string) (string, error) {
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := aesGCM.Seal(nil, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(nonce) + ":" + hex.EncodeToString(sealed), nil
}

// DecryptWithPassword decrypts ciphertext produced by EncryptWithPassword.
func DecryptWithPassword(ciphertext, password string) (string, error) {
	parts := strings.SplitN(ciphertext, ":", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid encrypted format")
	}

	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode salt: %w", err)
	}
	nonce, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}
	sealed, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("decode data: %w", err)
	}

	key := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create GCM: %w", err)
	}

	plaintext, err := aesGCM.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
