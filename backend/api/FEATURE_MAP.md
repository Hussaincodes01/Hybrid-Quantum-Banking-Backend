# FINIX Feature Map

This document maps major PRD capabilities to implemented API routes in the backend scaffold.

## 1. Onboarding & Identity

- `POST /v1/auth/register`
- `POST /v1/auth/ekyc/verify`
- `POST /v1/auth/biometric/register`
- `POST /v1/auth/login/challenge`
- `POST /v1/auth/login/verify`
- `POST /v1/accounts/link`

## 2. Dashboard

- `GET /v1/dashboard`

Includes net worth, health score, market snapshot, and freeze status.

## 3. Transaction Engine & Risk Scoring

- `POST /v1/transactions/initiate`
- `POST /v1/transactions/override`
- `GET /v1/transactions/history`

Implemented behaviors:

- AI-like risk scoring over behavioral/session/recipient signals
- Low/Medium/High outcomes
- Cooling-off timestamp for high risk
- Idempotency key enforcement
- Freeze state enforcement

## 4. Financial Health Score

- `GET /v1/health-score`

Implemented behaviors:

- 7-pillar weighted score
- 300-900 output scale
- resilience modifier with 6-month weighting

## 5. Goals

- `POST /v1/goals`
- `GET /v1/goals`
- `GET /v1/goals/{goalID}`
- `POST /v1/goals/{goalID}/contribute`
- `POST /v1/goals/{goalID}/close`

## 6. Portfolio

- `GET /v1/portfolio/summary`
- `GET /v1/portfolio/investments`
- `GET /v1/portfolio/insurance`
- `GET /v1/portfolio/loans`
- `GET /v1/portfolio/audit-logs`

## 7. Simulation Engine

- `POST /v1/simulations/run`

## 8. Insights

- `GET /v1/insights/feed`
- `GET /v1/insights/market`
- `GET /v1/insights/persona`

## 9. Security Corner

- `GET /v1/security/health`
- `POST /v1/security/sms/scan`
- `POST /v1/security/emergency-freeze`
- `POST /v1/security/unfreeze`
- `POST /v1/security/report-fraud`

## 10. Tax

- `GET /v1/tax/dashboard`
- `GET /v1/tax/regime-compare`
- `GET /v1/tax/deductions`
- `GET /v1/tax/capital-gains`

## 11. AI Chatbot

- `POST /v1/chatbot/query`

Guardrail included: generated response text is filtered to avoid using disallowed certainty language.

## 12. Settings & Consent

- `GET /v1/settings/consent/privacy_policy/status` | id=`consent_pp_status` | label=Privacy policy consent status
- `POST /v1/settings/consent/privacy_policy` | label=Privacy policy consent submission (public + optional authenticated enrichment)
- `GET /v1/settings/profile`
- `POST /v1/settings/consent/{consentType}`
- `POST /v1/settings/nudge-preference`
- `GET /v1/settings/notifications`
- `POST /v1/settings/feedback`

## 13. Audit and Integrity

- `GET /v1/audit/logs`
- `GET /v1/audit/integrity`

Implemented behaviors:

- append-only in-app audit events
- blockchain-like hash-chain ledger events
- per-user merkle root integrity proof

## Security Middleware

- Auth: Bearer token validation in middleware
- Rate limiting: global per-remote-address window limit
- Idempotency enforcement: required on transaction/freeze mutation routes

## Current Scope Note

This implementation is a production-ready scaffold with domain logic and route coverage for the full product surface. External integrations (NPCI/AA/NSDL/TRAI/RBI feeds, full cryptographic hardware attestation, and live blockchain network) are represented with deterministic local logic and can be swapped with live adapters module-by-module.
