package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

// ============================================================
// SERVER-SIDE DATA PROTECTION LAYER
// All encryption, masking, and tokenization is SERVER-SIDE ONLY.
// The frontend NEVER sees plaintext PII unless explicitly authorized.
// ============================================================

var (
	dataEncryptionKey []byte
	tokenStore        sync.RWMutex
	tokenMap          = make(map[string]string) // token → plaintext account number
	reverseTokenMap   = make(map[string]string) // plaintext → token
)

func init() {
	keyHex := strings.TrimSpace(os.Getenv("FINIX_DATA_ENCRYPTION_KEY"))
	if keyHex == "" {
		// C-7 fix: the old fallback derived the key deterministically from the
		// hostname, so a known hostname let an attacker derive the key and decrypt
		// all PII. Instead, generate a RANDOM ephemeral key at process start. This
		// keeps unit tests working (they never persist ciphertext across runs) while
		// being useless to an attacker. Production MUST set the env var and is forced
		// to via MustHaveDataEncryptionKey() (called from main); an ephemeral key
		// there would make previously-stored ciphertext undecryptable, which is a
		// loud, safe failure rather than a silent compromise.
		ephemeral := make([]byte, 32)
		if _, err := rand.Read(ephemeral); err != nil {
			log.Fatal("[SECURITY] FATAL: cannot generate ephemeral encryption key")
		}
		dataEncryptionKey = ephemeral
		return
	}
	var err error
	dataEncryptionKey, err = hex.DecodeString(keyHex)
	if err != nil || len(dataEncryptionKey) != 32 {
		log.Fatal("[SECURITY] FATAL: FINIX_DATA_ENCRYPTION_KEY must be a valid 64-char hex string (32 bytes).")
	}
}

// MustHaveDataEncryptionKey aborts the process if FINIX_DATA_ENCRYPTION_KEY is
// unset. C-7: called from main() (NOT init()) so production refuses to boot on the
// ephemeral key while unit tests are not killed.
func MustHaveDataEncryptionKey() {
	if strings.TrimSpace(os.Getenv("FINIX_DATA_ENCRYPTION_KEY")) == "" {
		log.Fatal("[SECURITY] FATAL: FINIX_DATA_ENCRYPTION_KEY is not set. " +
			"Set this to a 32-byte hex-encoded random key via Secrets Manager. " +
			"The server will not start without it.")
	}
}

// EncryptField encrypts a plaintext PII value using AES-256-GCM.
// Returns base64-encoded ciphertext. NEVER returns raw encrypted bytes.
// Server-side only — the frontend never encrypts or decrypts PII.
func EncryptField(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	block, err := aes.NewCipher(dataEncryptionKey)
	if err != nil {
		log.Printf("[SECURITY] EncryptField cipher error: %v", err)
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Printf("[SECURITY] EncryptField GCM error: %v", err)
		return ""
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		log.Printf("[SECURITY] EncryptField nonce error: %v", err)
		return ""
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

// DecryptField decrypts an AES-256-GCM encrypted field.
// Used server-side when PII must be displayed or processed.
// The frontend NEVER receives the decryption key.
func DecryptField(ciphertextB64 string) string {
	if ciphertextB64 == "" {
		return ""
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(dataEncryptionKey)
	if err != nil {
		return ""
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ""
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return ""
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return ""
	}
	return string(plaintext)
}

// MaskPhone masks a phone number for API responses.
// Returns last 4 digits visible: "+91 XXXXXX1234"
// Server-side: full number stored encrypted; API responses NEVER include plaintext.
// Admin/ComplianceOfficer role can request unmasked view (enforced by RBAC middleware).
func MaskPhone(phone string) string {
	if len(phone) < 4 {
		return "XXXX"
	}
	if strings.HasPrefix(phone, "+") && len(phone) >= 7 {
		return phone[:3] + "XXXXXX" + phone[len(phone)-4:]
	}
	return "XXXXXX" + phone[len(phone)-4:]
}

// MaskEmail masks an email address for API responses.
// Returns: "a***@domain.com", "ab***@domain.com"
// Server-side: full email stored encrypted; API responses NEVER include plaintext.
func MaskEmail(email string) string {
	if email == "" {
		return ""
	}
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return strings.Repeat("*", len(email))
	}
	local := parts[0]
	domain := parts[1]
	if len(local) <= 1 {
		return local + "***@" + domain
	}
	if len(local) <= 3 {
		return local[:1] + "***@" + domain
	}
	return local[:2] + "***@" + domain
}

// MaskPAN ensures PAN is never fully visible in API responses.
// Format: "XXXXXX1234F" (last 5 chars visible, rest masked)
func MaskPAN(pan string) string {
	if len(pan) < 5 {
		return strings.Repeat("X", len(pan))
	}
	return strings.Repeat("X", len(pan)-5) + pan[len(pan)-5:]
}

// MaskAadhaar ensures Aadhaar is never visible.
// Returns only hash-based reference: "XXXX-XXXX-1234"
func MaskAadhaar(ref string) string {
	if len(ref) < 4 {
		return "XXXX"
	}
	return "XXXX-XXXX-" + ref[len(ref)-4:]
}

// TokenizeAccount replaces an account number with a service-side token.
// Tokens are resolved server-side only; the frontend only sees tokens.
// Format: "acct_tok_" + 16 hex chars
func TokenizeAccount(accountID, plaintextAccountNumber string) string {
	tokenStore.RLock()
	if token, exists := reverseTokenMap[plaintextAccountNumber]; exists {
		tokenStore.RUnlock()
		return token
	}
	tokenStore.RUnlock()

	b := make([]byte, 8)
	rand.Read(b)
	token := "acct_tok_" + hex.EncodeToString(b)

	tokenStore.Lock()
	defer tokenStore.Unlock()
	// M-9 fix: double-check after upgrading to the write lock. Two goroutines could
	// both pass the RLock check and each mint a distinct token for the same
	// plaintext; the re-check ensures the first writer wins and is reused.
	if existing, exists := reverseTokenMap[plaintextAccountNumber]; exists {
		return existing
	}
	tokenMap[token] = plaintextAccountNumber
	reverseTokenMap[plaintextAccountNumber] = token

	return token
}

// ResolveToken converts a token back to an account number (server-side only).
func ResolveToken(token string) (string, bool) {
	tokenStore.RLock()
	defer tokenStore.RUnlock()
	v, ok := tokenMap[token]
	return v, ok
}

// SanitizeForResponse strips or masks PII based on the viewer's role.
// This is the single entry point for all API response PII sanitization.
// viewerRole: the role of the user viewing the data (from RBAC middleware).
// isOwner: true if the viewer is the data owner.
func SanitizeForResponse(field string, viewerRole string, isOwner bool) string {
	if isOwner {
		return field // owner sees their own data
	}
	switch viewerRole {
	case "admin", "auditor", "compliance_officer":
		return field // authorized roles see full data (per DPDP Act, §22.3)
	default:
		return strings.Repeat("*", len(field)) // all others see nothing
	}
}
