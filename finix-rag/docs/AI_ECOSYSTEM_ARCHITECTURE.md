# FINIX — AI Ecosystem Production Architecture

**Status:** Design (approved decisions below)
**Date:** 2026-07-23
**Scope:** Linking `finix-rag` (Python AI ecosystem) to the Go backend so the RAG LLM can (1) read Postgres read-only, (2) consume ML model outputs, and (3) validate risk scores / warning popups against ground-truth data — while keeping the AI/ML ecosystem fully separated from the transactional backend.

---

## 1. Context & problem

FINIX has two runtimes:

- **Go backend** (`backend/`, `:8080`) — auth, payments, goals, portfolio, dashboards. Runs the **in-process ONNX/heuristic risk engine** (`internal/domain/transaction/risk_engine.go`) that returns an `Assessment{Level, Score, Reason}` in <10 ms on the payment hot path. Owns the Postgres **primary**.
- **finix-rag** (`finix-rag/`, `:8000`, Python/FastAPI) — secure retrieval RAG: Qdrant + Vault + OPA + Redis + Groq LLM, BGE-M3 embeddings/reranker. Currently document-chat only.

**Requirement:** all AI features live in the Python ecosystem, separate from Go. The LLM must gain read-only Postgres access, see ML outputs, and **validate** them — cross-check a transaction's risk score and warning popup against real data before the decision is trusted.

**Hard constraint:** an ML risk decision returns in <10 ms; an LLM call takes 1–4 s. Therefore the LLM must **never** sit synchronously in front of a payment. It is a *validation/governance* layer, not an inline gate.

---

## 2. Core principle — two planes

```
DECISION PLANE  (Go, synchronous, <50ms)      — authoritative for what the user sees NOW
VALIDATION PLANE (Python AI ecosystem, async) — a second opinion that audits & corrects ML output
```

The ML model produces the score + warning popup (authoritative, instant). The LLM later reads Postgres ground truth + the ML output + policy docs and issues a **verdict**: was that score/popup justified? Discrepancies feed back as escalations or false-positive suppressions.

## Approved decisions

1. **Validation timing:** Async second-opinion. ML decision shown instantly; LLM validates after and may escalate/suppress within the ack/cooling-off window.
2. **DB access:** Streaming **read replica** + read-only role + PII-masked whitelisted **views**.
3. **Missing writers:** **Add persistence in Go** so `realtime_detections`, `health_score_snapshots`, `fraud_reports` are actually written and available as ground truth.

---

## 3. Architecture diagram

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                         Flutter App (Android / iOS)                          │
└───────────────────────────────────┬──────────────────────────────────────────┘
                                    │  HTTPS (TLS1.3 + X25519MLKEM768)
                                    ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                              Caddy edge (:443)                               │
└───────────────────────────────────┬──────────────────────────────────────────┘
                                    │
        ═══════════════════════════ DECISION PLANE ═══════════════════════════
                                    ▼
╔════════════════════════════════════════════════════════════════════════════════╗
║                          Go Backend  (:8080)                                   ║
║  chi router · auth · goals · portfolio · payments · dashboards                 ║
║                                                                                ║
║  POST /v1/transactions/initiate                                                ║
║        │                                                                       ║
║        ▼                                                                       ║
║  ┌──────────────────────────────────────────────────────────┐                  ║
║  │  IN-PROCESS ML  (transaction/risk_engine.go)             │  <10ms           ║
║  │  RiskSignal[9] → ONNX or heuristic → Assessment          │  synchronous     ║
║  │  {Level, Score 0-100, Reason}                            │  AUTHORITATIVE   ║
║  │  → TransactionResult{status: success|warning_ack|blocked}│                  ║
║  └──────────────────────────────────────────────────────────┘                  ║
║        │ writes                          │ publishes (fire-and-forget)         ║
║        ▼                                 ▼                                     ║
║  ┌──────────────┐              ┌────────────────────────┐                      ║
║  │  Postgres    │◄─repl──┐     │  Outbox → Redis Stream │  "risk.decision"     ║
║  │  PRIMARY     │        │     │  (existing outbox      │  {tx_id,user,score,  ║
║  │  (R/W, OLTP) │        │     │   pattern)             │   level,status}      ║
║  └──────────────┘        │     └────────────┬───────────┘                      ║
╚══════════════════════════╪══════════════════╪══════════════════════════════════╝
                           │ streaming repl   │ event bus (Redis Streams)
        ═══════════════════╪══════════════════╪═══ VALIDATION PLANE ═════════════
                           ▼                  ▼
╔═══════════════════════════════════════════════════════════════════════════════╗
║                    AI ECOSYSTEM  (separate compose stack, own network)        ║
║                                                                               ║
║  ┌──────────────┐        ┌──────────────────────────────────────────────────┐ ║
║  │  Postgres    │        │        finix-rag  (:8000, FastAPI)               │ ║
║  │  READ REPLICA│◄─SELECT│                                                  │ ║
║  │  (read-only) │  via   │  ┌────────────────────────────────────────────┐  │ ║
║  │  role _ro    │  views │  │  NEW: Validation worker (queue consumer)   │  │ ║
║  │  VIEWS ONLY  │        │  │   1. consume risk.decision event           │  │ ║
║  └──────────────┘        │  │   2. build ValidationContext (param SQL)   │  │ ║
║                          │  │   3. retrieve risk-policy docs (Qdrant)    │  │ ║
║                          │  │   4. PII-mask → LLM judge → verdict        │  │ ║
║                          │  │   5. write verdict + corrective action     │  │ ║
║                          │  └────────────────────────────────────────────┘  │ ║
║                          │  ┌────────────────────────────────────────────┐  │ ║
║                          │  │  EXISTING: SecureQueryEngine (RAG chat)    │  │ ║
║                          │  │  OPA authz → retrieve → rerank → Groq      │  │ ║
║                          │  └────────────────────────────────────────────┘  │ ║
║                          │  POST /validate   POST /query   POST /ingest     │ ║
║                          └───────┬───────────────┬──────────────┬───────────┘ ║
║                                  ▼               ▼              ▼             ║
║   Qdrant(:6333)   Vault(:8200)   OPA(:8181)   Redis(:6379)   Groq LLM (70B)   ║
║   vector store    transit enc    RBAC/ABAC    cache+queue    llama-3.3-70b    ║
╚═══════════════════════════════════════════════════════════════════════════════╝
                           │ verdict write-back
                           ▼
         Go: POST /internal/risk-validation  (service-to-service, mTLS + JWT)
         → risk_validations table → /ops/flow-dashboard + audit_events
```

**Boundary contract:** only two things cross between the planes — (1) a streaming **read replica** of Postgres, (2) a **Redis Streams** event bus. Go never calls Qdrant/Vault/OPA; Python never touches the primary DB. **ML stays in Go; the LLM stays in Python.**

---

## 4. Validation loop (sequence)

```
User pays ₹80,000 to a new payee
   │
Go ML engine → score 62 / MEDIUM → status=warning_ack_required   ← shown to user in <50ms
   │                                (ML is authoritative; popup shows immediately)
   ├─ persist transactions{risk_score,risk_level,xai_reason,status,cooling_off_until}
   └─ publish risk.decision → Redis Stream
                                   │
                 finix-rag validation worker consumes
                                   │
   reads READ-REPLICA via views:  user 90-day txn history, avg amount, this payee's
                                   prior transfers, freeze state, GNN recipient score,
                                   recent failed-PIN / velocity
                                   │
   retrieves from Qdrant:         RBI fraud-typology docs, internal risk policy,
                                   "new-payee + high-amount" guidance
                                   │
   PII-mask context → LLM judge → structured verdict:
       { agrees_with_ml: false, suggested_level: "high", confidence: 0.88,
         rationale: "payee matches mule pattern; amount 6× user avg; off-hours",
         recommended_action: "escalate_to_step_up",
         citations: [policy§3.2, RBI-2024-fraud-advisory] }
                                   │
   write-back → risk_validations + POST /internal/risk-validation
                                   │
   Go acts on verdict (policy-gated, within ack window):
       • escalate: warning → step-up / cooling-off
       • suppress false positive: downgrade, dismiss popup
       • agree: no-op, audit only
```

**Coverage policy (bounds LLM cost):** validate 100% of `blocked` + high-value; sample the low-risk `success` tail (~5%) for drift monitoring.

---

## 5. Read-only Postgres access (production-safe)

Three layers — the LLM never gets raw table access:

```
Postgres primary ──streaming repl──► READ REPLICA
                                        └── schema rag_ro  (VIEWS only, PII-masked,
                                        │                   secrets excluded)
                                        └── role finix_rag_ro  (GRANT SELECT on rag_ro.* only)
                                                └── asyncpg pool
                                                      default_transaction_read_only = on
                                                      statement_timeout = 2s
                                                      parameterized queries only
```

1. **Read replica** — analytical LLM reads never load the OLTP primary's write path.
2. **Whitelisted VIEWS, not tables** — `rag_ro` schema of sanitized views that omit `password_hash`, `upi_pins`, `pqc_keys` and column-mask raw PII (`recipient`, `account_holder_name`, `upi_id`). RO role gets SELECT on views only.
3. **Read-only role + discipline** — dedicated `finix_rag_ro`, read-only transactions, statement timeout, **parameterized** query bundle (no free-form LLM SQL in v1).

**How the LLM sees data:** a `ValidationContext` builder runs a fixed bundle of parameterized queries keyed by `tx_id`/`user_id`, assembles a structured JSON snapshot, PII-masks it (reuse existing `PIIMasker`), and passes it as LLM context — deterministic, injection-proof, cacheable in Redis.

*Phase 2 (optional):* constrained tool-calling (`get_transaction`, `get_user_risk_history`) with llama-3.3-70b, each tool backed by the same views.

### Ground-truth surfaces
- Reliably written today: `transactions` (risk_score/level/xai_reason/status/cooling_off_until), `payments`, `audit_events`, `ledger_events` (JSONB payload + rule_passed/violation).
- Schema-only, **need Go writers added** (decision 3): `realtime_detections`, `health_score_snapshots`, `fraud_reports`.

---

## 6. Implementation plan

### 6a. Go backend changes
- **Persist ML outputs** (decision 3): add repo INSERTs so `DetectRealtimeFraud` writes `realtime_detections`, health-snapshot path writes `health_score_snapshots`, fraud reporting writes `fraud_reports`. Follow the existing circuit-breaker + outbox pattern (`internal/domain/platform/infrastructure.go`).
- **Publish risk decisions**: on `InitiateTransaction`/`DetectRealtimeFraud`, enqueue a `risk.decision` event to a **Redis Stream** via the existing Outbox (`{tx_id, user_id, score, level, status, model_version, ts}`).
- **Verdict intake**: new internal route `POST /internal/risk-validation` (service-to-service, mTLS + JWT, off the public `/v1` surface). Applies policy-gated escalate/suppress within the ack/cooling-off window; always audits.
- **Config**: `REDIS_STREAM_URL`, `RISK_VALIDATION_ENABLED` (default off → graceful degradation), internal-JWT secret. Add to `internal/config/config.go`.

### 6b. Read replica + DB grants (deploy)
- Add a streaming read replica to the deploy topology (compose/k8s).
- Migration for `rag_ro` schema (views) + `finix_rag_ro` role. Views PII-mask and exclude secret columns.

### 6c. finix-rag changes (new `src/finix_rag/validation/` package)
```
validation/
├── postgres_readonly.py   asyncpg RO pool → replica, view-scoped
├── data_context.py        parameterized query bundle → structured PII-masked context
├── ml_validator.py        ctx + ML output + Qdrant docs → LLM verdict
├── event_consumer.py      Redis Streams consumer for risk.decision
└── verdict.py             Verdict dataclass + write-back client (JWT to Go)
```
- Reuse existing security: OPA gains a `validate`/`db_read` action; `PIIMasker` masks DB rows; Vault stores the RO DSN; JWT for write-back.
- `api.py`: add `POST /validate` (sync s2s path) and start the event-consumer worker in `lifespan`.
- `requirements.txt`: add `asyncpg` (and `redis` is already present).
- **Fix noted during exploration:** `SecureQueryEngine` never actually passes `SYSTEM_PROMPT` to Groq (uses only an inline prompt) — wire it in so security rules apply to synthesis.

---

## 7. Why this is optimized

| Decision | Rationale |
|---|---|
| ML in-process, LLM async | Payment latency <50ms; a 2s LLM call never blocks a UPI transfer. |
| Event-driven (Redis Streams) | Decouples Go/Python; reuses existing Outbox; both stacks already run Redis. |
| Read replica + views + RO role | LLM reads never load OLTP primary; secrets/PII structurally unreachable. |
| Parameterized context, not free SQL | No injection surface; deterministic; cacheable. |
| Validate blocked/high-value 100%, sample the rest | Bounds LLM cost; still catches drift. |
| Graceful degradation | finix-rag down → ML decision stands; matches repo's "runs with/without external systems" ethos. |
| Model tiering | Small model triages obvious agreements; 70B reasons only on discrepancies. |
| Verdict write-back + dashboard | Every AI override audited (`risk_validations` + `audit_events`), visible in `/ops/flow-dashboard`. |

---

## 8. Verification (end-to-end)

1. **Unit:** Go — event published on initiate; verdict route applies escalate/suppress within ack window only. Python — `data_context` returns PII-masked structured snapshot; `ml_validator` yields a valid `Verdict`.
2. **DB isolation:** confirm `finix_rag_ro` cannot `SELECT password_hash / upi_pins / pqc_keys`, cannot write, and sees only `rag_ro` views. Confirm reads hit the replica, not primary.
3. **Integration:** initiate a high-value new-payee transfer → assert popup returns in <50 ms (ML), then within a few seconds a `risk_validations` row + audit event appears and (if escalated) the transaction state changes.
4. **Degradation:** stop finix-rag / Redis → payments still complete on ML decision; events buffer in the outbox and drain on recovery.
5. **Load:** validation worker keeps up with peak `risk.decision` throughput; primary DB write latency unaffected by LLM reads (replica isolation).
