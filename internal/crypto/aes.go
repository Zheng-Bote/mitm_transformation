/**
 * SPDX-FileComment: Transformation Layer Crypto Helpers
 * SPDX-FileType: SOURCE
 * SPDX-FileContributor: ZHENG Robert
 * SPDX-FileCopyrightText: 2026 ZHENG Robert
 * SPDX-License-Identifier: Apache-2.0
 *
 * @file aes.go
 * @brief AES-GCM cryptographic functions for envelope encryption.
 * @version 1.0.0
 * @date 2026-06-06
 *
 * @author ZHENG Robert (robert@hase-zheng.net)
 * @copyright Copyright (c) 2026 ZHENG Robert
 * @license Apache-2.0
 */

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-GCM with the provided key.
// It returns the ciphertext (with the auth tag appended) and the generated nonce.
func Encrypt(key []byte, plaintext []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext using AES-GCM with the provided key and nonce.
func Decrypt(key []byte, nonce []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(nonce) != aesGCM.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size: expected %d, got %d", aesGCM.NonceSize(), len(nonce))
	}

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// EnvelopeDecrypt decrypts a payload using a wrapped DEK and a KEK
func EnvelopeDecrypt(kek []byte, wrappedKey []byte, payloadNonce []byte, payload []byte) ([]byte, error) {
	if len(kek) != 32 {
		adjusted := make([]byte, 32)
		copy(adjusted, kek)
		kek = adjusted
	}

	if len(wrappedKey) < 12 {
		return nil, fmt.Errorf("wrapped DEK too short")
	}
	dekNonce := wrappedKey[:12]
	wrappedCipher := wrappedKey[12:]

	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return nil, err
	}
	dek, err := kekGCM.Open(nil, dekNonce, wrappedCipher, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return nil, err
	}
	return dekGCM.Open(nil, payloadNonce, payload, nil)
}

// EnvelopeEncrypt encrypts a payload using a wrapped DEK and a KEK. Returns encrypted payload and new nonce.
func EnvelopeEncrypt(kek []byte, wrappedKey []byte, plaintext []byte) ([]byte, []byte, error) {
	if len(kek) != 32 {
		adjusted := make([]byte, 32)
		copy(adjusted, kek)
		kek = adjusted
	}

	if len(wrappedKey) < 12 {
		return nil, nil, fmt.Errorf("wrapped DEK too short")
	}
	dekNonce := wrappedKey[:12]
	wrappedCipher := wrappedKey[12:]

	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return nil, nil, err
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return nil, nil, err
	}
	dek, err := kekGCM.Open(nil, dekNonce, wrappedCipher, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return nil, nil, err
	}
	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return nil, nil, err
	}

	payloadNonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, payloadNonce); err != nil {
		return nil, nil, err
	}

	ciphertext := dekGCM.Seal(nil, payloadNonce, plaintext, nil)
	return ciphertext, payloadNonce, nil
}

// GenerateWrappedDEK generates a new random 32-byte DEK and wraps it with the provided KEK
func GenerateWrappedDEK(kek []byte) ([]byte, error) {
	if len(kek) != 32 {
		adjusted := make([]byte, 32)
		copy(adjusted, kek)
		kek = adjusted
	}

	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, err
	}

	dekNonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, dekNonce); err != nil {
		return nil, err
	}

	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return nil, err
	}
	wrappedCipher := kekGCM.Seal(nil, dekNonce, dek, nil)

	wrappedKey := make([]byte, len(dekNonce)+len(wrappedCipher))
	copy(wrappedKey, dekNonce)
	copy(wrappedKey[len(dekNonce):], wrappedCipher)

	return wrappedKey, nil
}
