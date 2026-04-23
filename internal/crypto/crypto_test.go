package crypto

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	original := "sk-1234567890abcdef-test-api-key"
	encrypted, err := Encrypt(original)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if encrypted == original {
		t.Fatal("encrypted should differ from original")
	}

	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decrypted != original {
		t.Fatalf("Decrypt() = %q, want %q", decrypted, original)
	}
}

func TestDecryptInvalidInput(t *testing.T) {
	_, err := Decrypt("not-valid-hex")
	if err == nil {
		t.Fatal("expected error for invalid hex input")
	}
}
