package crypto_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"mitm_transformation/internal/crypto"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("secret sensitive payload")

	ciphertext, nonce, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Fatal("ciphertext is empty")
	}

	decrypted, err := crypto.Decrypt(key, nonce, ciphertext)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("expected %s, got %s", plaintext, decrypted)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	wrongKey := make([]byte, 32)
	rand.Read(wrongKey)

	plaintext := []byte("secret")
	ciphertext, nonce, _ := crypto.Encrypt(key, plaintext)

	_, err := crypto.Decrypt(wrongKey, nonce, ciphertext)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}
