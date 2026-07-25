package security

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// MF1: the client-facing Dilithium key info must never carry the private key.
func TestDilithiumKeyInfoOmitsPrivateKey(t *testing.T) {
	dm, err := NewDilithiumManager()
	if err != nil {
		t.Fatalf("NewDilithiumManager: %v", err)
	}
	info := dm.GetCurrentKeyInfo()
	b, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	lower := strings.ToLower(string(b))
	for _, banned := range []string{"private", "secret"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("key info JSON leaks %q: %s", banned, b)
		}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["privateKey"]; ok {
		t.Fatalf("privateKey present in key info JSON: %s", b)
	}
	if info.KeyID == "" || info.Algorithm == "" || info.PublicKey == "" {
		t.Fatalf("expected keyId/algorithm/publicKey populated, got %+v", info)
	}
}

// MF3: the SIM binding challenge must not serialize its VNN proof code.
func TestSIMBindingChallengeOmitsVNNCode(t *testing.T) {
	m := NewSIMBindingManager()
	ch, err := m.CreateBindingChallenge("usr_x", "+911234567890", "dev_1")
	if err != nil {
		t.Fatalf("CreateBindingChallenge: %v", err)
	}
	if ch.VNNCode == "" {
		t.Fatal("expected a VNN code on the in-process struct")
	}
	b, err := json.Marshal(ch)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(strings.ToLower(string(b)), "vnn") {
		t.Fatalf("challenge JSON leaks the VNN proof code: %s", b)
	}
	var mp map[string]any
	if err := json.Unmarshal(b, &mp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := mp["vnnCode"]; ok {
		t.Fatalf("vnnCode present in challenge JSON: %s", b)
	}
}

// MF4: a well-formed but fabricated (unsigned) Aadhaar XML must parse yet NOT be
// marked verified — no UIDAI signature is checked.
func TestEKYCFabricatedXMLNotVerified(t *testing.T) {
	xml := `<?xml version="1.0"?><AadhaarData><UidData><Uid>999999990019</Uid>` +
		`<Poi><Poi-Name>Fabricated Person</Poi-Name></Poi></UidData></AadhaarData>`
	b64 := base64.StdEncoding.EncodeToString([]byte(xml))

	parsed, result, err := VerifyAadhaarOfflineXML(b64, "1234")
	if err != nil {
		t.Fatalf("well-formed XML should parse: %v", err)
	}
	if parsed == nil || result == nil {
		t.Fatal("expected parsed data and result")
	}
	if result.Verified {
		t.Fatal("fabricated XML must NOT be Verified (no signature validation)")
	}
	if result.VerificationMethod != "aadhaar_offline_xml_parse_only" {
		t.Fatalf("expected parse-only marker, got %q", result.VerificationMethod)
	}
}

// MF2: the passkey counter update must land on the correct registration even when
// a concurrent Register reallocates the backing slice mid-verification.
func TestWebAuthnConcurrentRegisterDuringVerify(t *testing.T) {
	pm := NewPasskeyManager()
	privA, pubA, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := pm.Register("user1", "keyA", "devA", pubA); err != nil {
		t.Fatalf("register keyA: %v", err)
	}

	ch, err := pm.CreateChallenge("user1")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	digest := sha256.Sum256(ch.Challenge)
	sigA, err := ecdsa.SignASN1(rand.Reader, privA, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 64; i++ {
			_, pub, err := GeneratePasskeyKeyPair()
			if err == nil {
				_ = pm.Register("user1", "extra"+strconv.Itoa(i), "d", pub)
			}
		}
	}()

	ok, err := pm.VerifyChallenge("user1", ch.ChallengeID, sigA, "keyA")
	wg.Wait()
	if err != nil || !ok {
		t.Fatalf("verify keyA: ok=%v err=%v", ok, err)
	}
	found := false
	for _, r := range pm.ListRegistrations("user1") {
		if r.KeyID == "keyA" {
			found = true
			if r.SignCount != 1 {
				t.Fatalf("keyA SignCount=%d, want 1 (lost update / stale pointer)", r.SignCount)
			}
		}
	}
	if !found {
		t.Fatal("keyA registration missing after concurrent registers")
	}
}

// N22: a failed signature attempt must not consume the challenge.
func TestWebAuthnBadSignatureDoesNotConsumeChallenge(t *testing.T) {
	pm := NewPasskeyManager()
	priv, pub, err := GeneratePasskeyKeyPair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := pm.Register("u", "k", "d", pub); err != nil {
		t.Fatalf("register: %v", err)
	}
	ch, err := pm.CreateChallenge("u")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}

	badDigest := sha256.Sum256([]byte("not the challenge"))
	badSig, _ := ecdsa.SignASN1(rand.Reader, priv, badDigest[:])
	if ok, err := pm.VerifyChallenge("u", ch.ChallengeID, badSig, "k"); ok || err != nil {
		t.Fatalf("bad sig: want ok=false err=nil, got ok=%v err=%v", ok, err)
	}

	// The correct signature must still succeed — the challenge was not burned.
	digest := sha256.Sum256(ch.Challenge)
	goodSig, _ := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if ok, err := pm.VerifyChallenge("u", ch.ChallengeID, goodSig, "k"); !ok || err != nil {
		t.Fatalf("good sig after bad attempt: want success, got ok=%v err=%v", ok, err)
	}
}
