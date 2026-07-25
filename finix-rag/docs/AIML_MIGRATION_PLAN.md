# FINIX — AIML Strip / Migration Plan (Go backend → Python AI ecosystem)

**Decision (2026-07-23):** move all AIML out of the Go backend and have the Python
`finix-rag` service own it; Go routes **proxy** to Python `:8000`.

This document records the *reality gap* found during implementation and the
staged, non-breaking path to complete the migration. A feature-flagged proxy seam
is already in place in Go (`internal/api/aiml_proxy.go`).

---

## 1. Reality check — you cannot proxy to endpoints that don't exist yet

The Python service today exposes **only**: `/health`, `/auth/login`, `/auth/refresh`,
`/ingest`, `/query`, `/audit/logs`, `/admin/rotate-keys`. Its single AI endpoint is
`/query` (RAG document chat), and it authenticates with its **own** `TokenPayload`
(separate JWT + OPA identity), not the Go user JWT.

There is **no** Python endpoint for any of the Go AIML surface:

| Go route (backend) | Kind | Python target | Status |
|---|---|---|---|
| `POST /v1/chatbot/query` | LLM chat | `POST /query` (shape differs) | endpoint exists, **needs a request/response adapter + identity bridge** |
| `POST /v1/chatbot/query/voice`, history | LLM chat | — | **missing** |
| `POST /v1/aiml/behaviour/predict` | ML | — | **missing** |
| `POST /v1/aiml/synthetic/generate` | ML/LLM | — | **missing** |
| `POST /v1/aiml/investments/recommend` | ML | — | **missing** |
| `POST /v1/aiml/behaviour/persona/contest` | ML | — | **missing** |
| `GET  /v1/admin/aiml/bias-audit` | ML | — | **missing** |
| **In-process risk engine** (`transaction/risk_engine.go`, ONNX/heuristic) | ML, **synchronous payment hot path** | — | **missing — see §3** |

So a literal "remove + proxy everything now" would 404 the entire AI surface and
break payments. The proxy is therefore **staged and OFF by default**.

---

## 2. The proxy seam (already implemented, reversible)

`internal/api/aiml_proxy.go` + wiring in `router.go`:

- Config: `AIML_PROXY_ENABLED` (default **false**), `AIML_UPSTREAM_URL` (default
  `http://localhost:8000`).
- `api.aiml.route(pythonPath, localHandler)` wraps a route: when enabled it forwards
  to `upstream+pythonPath` (safe-header allow-list + `X-FINIX-User`); when disabled
  **or on any forward error** it serves the existing Go handler → graceful fallback.
- Reference wiring: `POST /v1/chatbot/query → api.aiml.route("/query", api.chat)`.
  Every other AIML route becomes the same one-liner once its Python endpoint exists.

Flip on with `AIML_PROXY_ENABLED=true` **only after** the matching Python endpoint
and the identity bridge (below) are in place — do it one route at a time.

---

## 3. The risk engine is the hard part — recommendation

The in-process ONNX/heuristic risk engine returns `Assessment{Level,Score,Reason}`
in <10 ms **synchronously on the payment path**. Two options:

- **(Recommended) Keep it in Go.** This matches the approved
  `AI_ECOSYSTEM_ARCHITECTURE.md` (ML in Go / LLM in Python). Payments stay fast and
  keep working when the AI ecosystem is down. The Python plane still *validates* ML
  output asynchronously (the second-opinion "validation plane" already designed).
- **Port it to Python and proxy.** Requires (a) porting the ONNX model + `RiskSignal`
  feature build to a Python `POST /risk/score`, (b) accepting a network hop on every
  payment, and (c) a mandatory Go-side fallback to a local scorer when Python is slow/
  down (or payments fail). This contradicts the <10 ms + availability goals.

**If you still want a full strip:** build `POST /risk/score` in Python, keep a Go
local-scorer fallback behind the same seam, and load-test the payment path before
enabling. Until then the risk engine stays in Go and is *not* stripped.

---

## 4. Identity bridge (required before enabling any route)

Python endpoints authenticate with their own `TokenPayload`. The Go backend must
present a credential the Python side trusts — options: (a) a shared internal
service JWT (HS256) verified by `finix-rag`, or (b) mint a short-lived Python JWT
per request from the Go user identity. Add this to the proxy `forward()` before
enabling. `X-FINIX-User` is already forwarded for attribution.

---

## 5. Sequenced steps to finish the migration

1. Build the missing Python endpoints (chatbot, `/aiml/*`, and — only if chosen —
   `/risk/score`), reusing `SecureQueryEngine` where relevant.
2. Implement the identity bridge in `aiml_proxy.forward()`.
3. For chatbot: add a request/response adapter (Go `ChatRequest` ↔ Python `/query`).
4. Enable per route (`AIML_PROXY_ENABLED=true`), verify, then delete the now-dead Go
   handler bodies (keep the route as a pure proxy).
5. Decide the risk engine per §3; keep in Go unless the latency/availability
   trade-off is explicitly accepted.
