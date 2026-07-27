# FINIX Backend API - Flutter Developer Handoff

## Quick Start

- **Base URL**: `http://localhost:8080`
- **All user endpoints** are under `/v1/` and require `Authorization: Bearer <token>` header
- **Content-Type**: `application/json` for all requests
- **Currency**: All monetary amounts are in **paise** (INR × 100). ₹100.00 = `10000` paise

## Auth Flow

```
POST /v1/auth/register          → get userId + ubt
POST /v1/auth/ekyc/verify       → verify PAN/Aadhaar
POST /v1/auth/biometric/register → register device key
POST /v1/auth/login/challenge   → get challenge
POST /v1/auth/login/verify      → sign challenge → get accessToken
```

**Alternate PIN flow:**
```
POST /v1/auth/login/pin/set     → set PIN (authed)
POST /v1/auth/login/pin         → get accessToken with PIN
```

## Token Usage

```
Authorization: Bearer swt_<hex>
```

Token expires after **15 minutes**. Device fingerprint binding enforced.

## Rate Limits

| Scope | Limit | Window |
|-------|-------|--------|
| Global | 240 req | 1 min |
| Auth endpoints | 20 req | 1 min |
| Registration | 10 req | 1 min |

## Special Headers

| Header | Used In | Purpose |
|--------|---------|---------|
| `Authorization` | All /v1/* | Bearer token |
| `Idempotency-Key` | transactions, freeze, unfreeze | Prevent duplicate ops |
| `X-Biometric-Challenge-Id` | step-up auth | Challenge ID |
| `X-Biometric-Challenge` | step-up auth | Signed challenge (base64) |
| `X-Device-Fingerprint` | auth | Device identifier |
| `X-Device-Trusted` | sensitive ops | "true" for known device |
| `X-PQC-Session` | all /v1/* (optional) | PQC session ID |

## Files in This Folder

| File | Content |
|------|---------|
| `base_url.json` | Server config, CORS, headers |
| `auth.json` | Registration, login, KYC, biometric |
| `dashboard.json` | Dashboard, net worth history, market snapshot |
| `accounts.json` | Bank account linking, UPI, aggregator |
| `transactions.json` | Initiate, history, risk, cooling-off |
| `goals.json` | Create, contribute, pause, dissolve goals |
| `portfolio.json` | Investments, insurance, loans, net worth |
| `security.json` | Freeze, unfreeze, SMS scan, fraud, contacts |
| `tax.json` | Tax dashboard, regime compare, deductions |
| `insights.json` | Feed, market, persona, simulation |
| `chatbot.json` | Chat query, history |
| `settings.json` | Profile, consent, notifications, audit |
| `models.json` | All request/response type definitions |
| `middleware.json` | Headers, rate limits, auth rules |

## Error Format

```json
{
  "error": "error message string"
}
```

Status codes: `400` bad request, `401` unauthorized, `403` forbidden, `404` not found, `423` locked/cooling-off.
