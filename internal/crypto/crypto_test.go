package crypto

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := []byte("12345678901234567890123456789012") // 32 bytes for AES-256
	plaintext := "my-secret-token-12345"

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Encrypted text is empty")
	}

	if encrypted == plaintext {
		t.Fatal("Encrypted text is the same as plaintext")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	plaintext := ""

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted != "" {
		t.Errorf("Encrypted = %q, want empty string", encrypted)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != "" {
		t.Errorf("Decrypted = %q, want empty string", decrypted)
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	key := []byte("12345678901234567890123456789012")

	_, err := Decrypt("not-valid-base64!!!", key)
	if err == nil {
		t.Error("Expected error for invalid base64, got nil")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := []byte("12345678901234567890123456789012")
	key2 := []byte("98765432109876543210987654321098")
	plaintext := "my-secret-token"

	encrypted, err := Encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(encrypted, key2)
	if err == nil {
		t.Error("Expected error when decrypting with wrong key, got nil")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	key := []byte("12345678901234567890123456789012")

	// Create a base64 string that's too short
	_, err := Decrypt("AAAA", key)
	if err == nil {
		t.Error("Expected error for ciphertext too short, got nil")
	}
}

func TestEncrypt_DifferentNonces(t *testing.T) {
	key := []byte("12345678901234567890123456789012")
	plaintext := "same-plaintext"

	encrypted1, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 1 failed: %v", err)
	}

	encrypted2, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt 2 failed: %v", err)
	}

	// Same plaintext should produce different ciphertext (due to random nonce)
	if encrypted1 == encrypted2 {
		t.Error("Same plaintext produced identical ciphertext (nonces not random)")
	}

	// But both should decrypt to the same plaintext
	decrypted1, _ := Decrypt(encrypted1, key)
	decrypted2, _ := Decrypt(encrypted2, key)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Decrypted values don't match original plaintext")
	}
}
