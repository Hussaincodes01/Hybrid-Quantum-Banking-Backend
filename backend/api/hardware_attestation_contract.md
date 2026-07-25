# FINIX Hardware Attestation API Contract

## Endpoint Specifications

---

### 1. `POST /v1/auth/register`

Register a new user with device attestation.

**Request:**

```json
{
  "email": "user@example.com",
  "phone": "+14155551234",
  "pin": "123456",
  "deviceIdFingerprint": "a1b2c3d4e5f6...sha256 hash...7890abcdef",
  "deviceInfo": {
    "deviceModel": "iPhone15,2",
    "osVersion": "17.4.1",
    "systemUptime": 86400,
    "identifierForVendor": "E621E1F8-C36C-495A-93FC-0C247A3E6E5F",
    "hardwareUUID": "F8E1C2D3-A4B5-6789-0123-456789ABCDEF",
    "secureEnclaveAvailable": true,
    "biometricType": "faceID",
    "screenResolution": "1179x2556",
    "totalDiskSpace": 128000000000,
    "androidId": "",
    "buildFingerprint": "",
    "keystoreAttestation": "",
    "simSerialNumber": ""
  }
}
```

**Response `201 Created`:**

```json
{
  "userId": "usr_8f14e45f-ceea-467f-a9f6-d7e3c0b7d8a1",
  "sessionToken": "eyJhbGciOiJFZERTQ...",
  "biometricRegistered": false,
  "createdAt": "2026-06-10T14:30:00Z"
}
```

---

### 2. `POST /v1/auth/biometric/register`

Register a biometric-bound public key for the device.

**Headers:**

```
Authorization: Bearer <sessionToken>
X-Device-Fingerprint: <sha256 hash>
```

**Request:**

```json
{
  "publicKey": "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...base64 SPKI...",
  "keyId": "b7e2f3a1-9c4d-4e8b-a6f2-1d3c5e7a9b0f",
  "biometricType": "faceID",
  "deviceIdFingerprint": "a1b2c3d4e5f6...sha256 hash...7890abcdef"
}
```

**Response `200 OK`:**

```json
{
  "registered": true,
  "keyId": "b7e2f3a1-9c4d-4e8b-a6f2-1d3c5e7a9b0f",
  "algorithm": "ES256",
  "registeredAt": "2026-06-10T14:31:00Z"
}
```

---

### 3. `POST /v1/auth/login/challenge`

Request a biometric authentication challenge.

**Request:**

```json
{
  "deviceIdFingerprint": "a1b2c3d4e5f6...sha256 hash...7890abcdef"
}
```

**Response `200 OK`:**

```json
{
  "challenge": "dGhpcyBpcyBhIGNoYWxsZW5nZQ...base64url...",
  "keyId": "b7e2f3a1-9c4d-4e8b-a6f2-1d3c5e7a9b0f",
  "expiresAt": "2026-06-10T14:35:00Z",
  "algorithm": "ES256"
}
```

**Error `404 Not Found`:**

```json
{
  "error": "no_biometric_key",
  "message": "No biometric key registered for this device"
}
```

---

### 4. `POST /v1/auth/login/verify`

Verify biometric signature against challenge.

**Request:**

```json
{
  "challenge": "dGhpcyBpcyBhIGNoYWxsZW5nZQ...base64url...",
  "signature": "MEUCIQD...base64 ECDSA signature...",
  "keyId": "b7e2f3a1-9c4d-4e8b-a6f2-1d3c5e7a9b0f",
  "deviceIdFingerprint": "a1b2c3d4e5f6...sha256 hash...7890abcdef"
}
```

**Response `200 OK`:**

```json
{
  "sessionToken": "eyJhbGciOiJFZERTQ...",
  "userId": "usr_8f14e45f-ceea-467f-a9f6-d7e3c0b7d8a1",
  "biometricRegistered": true,
  "expiresAt": "2026-06-10T22:30:00Z"
}
```

**Error `401 Unauthorized`:**

```json
{
  "error": "invalid_signature",
  "message": "Signature verification failed"
}
```

**Error `410 Gone`:**

```json
{
  "error": "challenge_expired",
  "message": "Challenge has expired; request a new one"
}
```

---

### 5. `GET /v1/auth/pqc/init`

Initialize post-quantum key exchange.

**Headers:**

```
Authorization: Bearer <sessionToken>
X-Device-Fingerprint: <sha256 hash>
```

**Response `200 OK`:**

```json
{
  "publicKey": "base64-encoded Kyber-1024 public key",
  "algorithm": "kyber-1024",
  "serverKeyId": "pqc_srv_9a8b7c6d",
  "expiresAt": "2026-06-10T14:40:00Z"
}
```

---

### 6. `POST /v1/auth/pqc/encapsulate`

Send client encapsulation to establish shared secret.

**Headers:**

```
Authorization: Bearer <sessionToken>
X-Device-Fingerprint: <sha256 hash>
```

**Request:**

```json
{
  "ciphertext": "base64-encoded Kyber ciphertext",
  "algorithm": "kyber-1024"
}
```

**Response `200 OK`:**

```json
{
  "sessionId": "pqc_sess_1a2b3c4d5e6f",
  "established": true,
  "expiresAt": "2026-06-10T15:30:00Z"
}
```

**Post-response:**
Both client and server derive an AES-256-GCM key from the shared secret using HKDF-SHA256. Subsequent requests on this session use `X-Session-Id` header and encrypted payloads.

---

### 7. `POST /v1/transactions/initiate`

Initiate a financial transaction; server may require biometric step-up.

**Headers:**

```
Authorization: Bearer <sessionToken>
X-Device-Fingerprint: <sha256 hash>
```

**Request:**

```json
{
  "toAddress": "wallet_7f8e9d0c-1b2a-3e4f-5a6b-7c8d9e0f1a2b",
  "amountCents": 150000,
  "currency": "USD",
  "deviceIdFingerprint": "a1b2c3d4e5f6...sha256 hash...7890abcdef",
  "idempotencyKey": "txn_req_a1b2c3d4e5f6"
}
```

**Response `200 OK` (no step-up):**

```json
{
  "transactionId": "txn_3f4e5d6c-7b8a-9f0e-1d2c-3b4a5f6e7d8c",
  "status": "completed",
  "stepUpRequired": false,
  "amountCents": 150000,
  "currency": "USD",
  "completedAt": "2026-06-10T14:32:00Z"
}
```

**Response `200 OK` (step-up required):**

```json
{
  "transactionId": "txn_3f4e5d6c-7b8a-9f0e-1d2c-3b4a5f6e7d8c",
  "status": "pending_approval",
  "stepUpRequired": true,
  "reason": "amount_threshold_exceeded",
  "amountCents": 150000,
  "currency": "USD"
}
```

**Response `403 Forbidden` (trust score too low):**

```json
{
  "error": "untrusted_device",
  "message": "Device trust score below threshold for this transaction amount",
  "requiredScore": 0.85,
  "currentScore": 0.62
}
```

---

### 8. `POST /v1/transactions/override`

Complete a step-up-protected transaction with biometric verification.

**Headers:**

```
Authorization: Bearer <sessionToken>
X-Device-Fingerprint: <sha256 hash>
```

**Request:**

```json
{
  "transactionId": "txn_3f4e5d6c-7b8a-9f0e-1d2c-3b4a5f6e7d8c",
  "biometricVerified": true,
  "deviceIdFingerprint": "a1b2c3d4e5f6...sha256 hash...7890abcdef"
}
```

**Response `200 OK`:**

```json
{
  "transactionId": "txn_3f4e5d6c-7b8a-9f0e-1d2c-3b4a5f6e7d8c",
  "status": "completed",
  "stepUpRequired": true,
  "biometricVerified": true,
  "amountCents": 150000,
  "currency": "USD",
  "completedAt": "2026-06-10T14:32:30Z"
}
```

**Error `401 Unauthorized`:**

```json
{
  "error": "biometric_not_verified",
  "message": "Biometric verification was not completed on this device"
}
```

**Error `409 Conflict`:**

```json
{
  "error": "transaction_already_completed",
  "message": "Transaction has already been finalized"
}
```

---

## Device Trust Score Calculation

The server computes a trust score (0.0–1.0) from:

| Factor | Weight | Description |
|--------|--------|-------------|
| Device fingerprint match | 0.30 | Consistent with registered device |
| Biometric key bound | 0.25 | Key registered and hardware-backed |
| Device attestation valid | 0.20 | SafetyNet/Play Integrity/DeviceCheck passed |
| Session freshness | 0.10 | Login within expected timeframe |
| Behavioral signals | 0.15 | Transaction pattern consistency |

Thresholds:

- **0.0–0.5**: Block transaction
- **0.5–0.7**: Require biometric step-up
- **0.7–1.0**: Allow transaction (or step-up for amounts > $10,000)

---

## Error Codes Summary

| HTTP | Code | Description |
|------|------|-------------|
| 400 | `invalid_attestation` | Device attestation data malformed |
| 401 | `invalid_signature` | Biometric signature verification failed |
| 401 | `biometric_not_verified` | Override called without biometric |
| 404 | `no_biometric_key` | No key registered for device |
| 409 | `challenge_expired` | Challenge TTL exceeded (5 min) |
| 409 | `transaction_already_completed` | Duplicate override attempt |
| 403 | `untrusted_device` | Trust score below threshold |
| 429 | `rate_limited` | Too many auth attempts; retry after `Retry-After` header |
