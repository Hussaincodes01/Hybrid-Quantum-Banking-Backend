package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"golang.org/x/crypto/hkdf"
)

type EncryptedPayload struct {
	Ciphertext string `json:"ciphertext"`
	Nonce      string `json:"nonce"`
}

func EncryptAESGCM(key []byte, plaintext []byte) (EncryptedPayload, error) {
	if len(key) != 32 {
		return EncryptedPayload{}, errors.New("key must be 32 bytes for AES-256-GCM")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedPayload{}, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedPayload{}, err
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return EncryptedPayload{}, err
	}

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, nil)

	return EncryptedPayload{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
	}, nil
}

func DecryptAESGCM(key []byte, payload EncryptedPayload) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes for AES-256-GCM")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return nil, errors.New("invalid base64 ciphertext")
	}

	nonce, err := base64.StdEncoding.DecodeString(payload.Nonce)
	if err != nil {
		return nil, errors.New("invalid base64 nonce")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func DeriveKeyFromSharedSecret(sharedSecret []byte) ([]byte, error) {
	if len(sharedSecret) == 0 {
		return nil, errors.New("shared secret must not be empty")
	}

	hkdfReader := hkdf.New(sha256.New, sharedSecret, nil, []byte("FINIX-AES256-GCM-KEY"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, err
	}

	return key, nil
}
