package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func generateTestKeyPairAndID(t *testing.T) (*ecdsa.PrivateKey, string, string) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key pair failed: %v", err)
	}
	pubKeyBytes := elliptic.Marshal(privKey.Curve, privKey.PublicKey.X, privKey.PublicKey.Y)
	pubKeyB64 := base64.StdEncoding.EncodeToString(pubKeyBytes)

	keyIDBytes := make([]byte, 8)
	rand.Read(keyIDBytes)
	keyID := "key_" + base64.StdEncoding.EncodeToString(keyIDBytes)

	return privKey, pubKeyB64, keyID
}

func signChallengeBase64(t *testing.T, privKey *ecdsa.PrivateKey, challengeB64 string) string {
	t.Helper()
	challengeBytes, err := base64.StdEncoding.DecodeString(challengeB64)
	if err != nil {
		t.Fatalf("decode challenge failed: %v", err)
	}
	digest := sha256.Sum256(challengeBytes)
	sig, err := ecdsa.SignASN1(rand.Reader, privKey, digest[:])
	if err != nil {
		t.Fatalf("sign challenge failed: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}
