# FINIX Complete Architecture - ASCII Diagrams

Generated from: D:/FINIX_PRODUCTION/backend (124 Go files) + D:/FINIX_PRODUCTION/finix-rag/src (18 Python files)

---

## 1. SYSTEM-WIDE TOPOLOGY (ASCII)

```
┌─────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                    FINIX SECURE BANKING AI PLATFORM                                  │
│                                              v0.2.0-production                                       │
└─────────────────────────────────────────────────────────────────────────────────────────────────────┘

┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│   CLIENTS        │     │   LOAD BALANCER  │     │   API GATEWAY    │
│  ┌────────────┐  │     │  ┌────────────┐  │     │  ┌────────────┐  │
│  │  Flutter   │──┼────►│  │  nginx /   │──┼────►│  │  Chi Router │  │
│  │  Mobile    │  │     │  │  Traefik   │  │     │  │  (Go 1.21+) │  │
│  └────────────┘  │     │  └────────────┘  │     │  └──────┬──────┘  │
│  ┌────────────┐  │     │                 │     │         │         │
│  │  React     │──┼─────┘                 │     │         ▼         │
│  │  Web       │  │                       │     │  ┌────────────┐  │
│  └────────────┘  │                       │     │  │ MIDDLEWARE │  │
│  ┌────────────┐  │                       │     │  │  STACK     │  │
│  │  Admin     │──┼───────────────────────┼────►│  └────────────┘  │
│  │  Panel     │  │                       │     │                 │
│  └────────────┘  │                       │     └─────────────────┘
└──────────────────┘                       │
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                    GO BACKEND - PLATFORM SERVICE                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────────────────────┐   │
│  │  main.go ──► config.Load() ──► cfg.Validate() ──► platform.MustHaveKeys()                   │   │
│  │         │           │               │                   │                                    │   │
│  │         ▼           ▼               ▼                   ▼                                    │   │
│  │  ┌──────────────────────────────────────────────────────────────────────────────────────┐   │   │
│  │  │                         PLATFORM SERVICE (service.go)                                 │   │   │
│  │  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │   │   │
│  │  │  │ User Mgmt   │  │ Transaction │  │ Auth/       │  │ Goals/      │  │ AI/ML       │  │   │   │
│  │  │  │ (Users,     │  │ Engine      │  │ Security    │  │ Portfolio   │  │ (Chatbot,   │  │   │   │
│  │  │  │  KYC, Auth) │  │ (Risk,      │  │ (JWT, PQC,  │  │ (Invest,    │  │  RAG,       │  │   │   │
│  │  │  │             │  │  Override)  │  │  Dilithium) │  │  Insurance) │  │  ScanSMS)   │  │   │   │
│  │  │  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  │   │   │
│  │  │         │               │               │               │               │         │   │   │
│  │  │         └───────────────┼───────────────┼───────────────┼───────────────┼─────────┘   │   │   │
│  │  │                         ▼               ▼               ▼               ▼             │   │   │
│  │  │  ┌──────────────────────────────────────────────────────────────────────────────────┐  │   │   │
│  │  │  │                         INFRASTRUCTURE LAYER                                      │  │   │   │
│  │  │  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │  │   │   │
│  │  │  │  │ Postgres │ │  Redis   │ │ Qdrant   │ │ Fabric   │ │  Vault   │ │  ONNX    │  │  │   │   │
│  │  │  │  │ (ACID,   │ │ (Streams,│ │ (Vector  │ │ (Ledger) │ │ (Secrets,│ │ (Fraud   │  │  │   │   │
│  │  │  │  │ Migr.)   │ │ RateLim) │ │ Store)   │ │          │ │  Encrypt)│ │  Model)  │  │  │   │   │
│  │  │  │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘  │  │   │   │
│  │  │  └───────┼────────────┼────────────┼────────────┼────────────┼────────────┼─────────┘  │   │   │
│  │  └──────────┼────────────┼────────────┼────────────┼────────────┼────────────┼────────────┘  │   │   │
│  └─────────────┼────────────┼────────────┼────────────┼────────────┼────────────┼────────────────┘   │
│                │            │            │            │            │            │                   │
│                ▼            ▼            ▼            ▼            ▼            ▼                   │
│  ┌──────────────────────────────────────────────────────────────────────────────────────────┐   │
│  │                              OUTBOX PATTERN                                               │   │
│  │  Platform.Outbox ──► Redis Streams (risk.decision) ──► Python RAG Consumer               │   │
│  │       │                                                                                  │   │
│  │       └──► Fabric (WriteEventStrict) ──► Immutable Ledger                                │   │
│  └──────────────────────────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────────────────────────────┘

                                           │
                    ┌──────────────────────┼──────────────────────┐
                    ▼                      ▼                      ▼
           ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
           │  POSTGRESQL     │    │     REDIS       │    │  HYPERLEDGER    │
           │  (Primary DB)   │    │  (Streams +     │    │  FABRIC         │
           │  ┌────────────┐ │    │   Rate Limit)   │    │  ┌────────────┐ │
           │  │ users      │ │    │  ┌───────────┐  │    │  │ Chaincode: │ │
           │  │ accounts   │ │    │  │ INCR +    │  │    │  │  finix     │ │
           │  │ transactions   │    │  │ EXPIRE    │  │    │  │ Channel:   │ │
           │  │ goals      │ │    │  └───────────┘  │    │  │  finix-    │ │
           │  │ investments│ │    │                 │    │  │  channel   │ │
           │  │ insurance  │ │    │  Rate Limiter   │    │  └────────────┘ │
           │  │ loans      │ │    │  (distributed)  │    │                 │
           │  │ ... (27    │ │    │                 │    │  Events:        │
           │  │  migrations)│   │  Outbox Worker    │    │  consent-record │
           │  └────────────┘ │    │  (500ms ticker) │    │  insurance-claim│
           └─────────────────┘    └─────────────────┘    │  loan-repayment │
                                                         │  investment-trade│
                                                         │  cooling-off    │
                                                         └─────────────────┘
                    │                      │                      │
                    └──────────────────────┼──────────────────────┘
                                           ▼
                              ┌─────────────────────────┐
                              │   PYTHON RAG SERVICE    │
                              │   (FastAPI :8000)       │
                              │  ┌───────────────────┐  │
                              │  │  /query           │  │──► SecureQueryEngine
                              │  │  /ingest          │  │──► SecureIngestor
                              │  │  /health          │  │──► HybridRetriever
                              │  │  /audit/logs      │  │──► Reranker (BGE-v2-M3)
                              │  │  /admin/rotate    │  │──► Guardrails (NeMo+
                              │  └───────────────────┘  │     Programmatic)
                              │          │              │
                              │          ▼              │
                              │  ┌───────────────────┐  │
                              │  │ INFRASTRUCTURE    │  │
                              │  │ Qdrant :6333      │  │
                              │  │ Groq API (LLM)    │  │
                              │  │ Vault (Secrets)   │  │
                              │  │ OPA (AuthZ)       │  │
                              │  └───────────────────┘  │
                              └─────────────────────────┘

```

---

## 2. GO BACKEND - DETAILED COMPONENT MAP

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                                      GO BACKEND (124 .go files)                         │
│                    D:/FINIX_PRODUCTION/backend                                          │
└────────────────────────────────────────────────────────────────────────────────────────┘

cmd/
└── server/
    └── main.go                          ← ENTRY POINT
        ├── config.Load()
        ├── cfg.Validate()                    [C-7/C-8: Refuse boot w/o secrets]
        ├── platform.MustHaveDataEncryptionKey()
        ├── security.MustHaveAadhaarHashKey()
        ├── db.New() ───► pgxpool + Migrate()
        ├── infra/ai.NewRagClient()
        ├── infra/bank (mock|razorpay)
        ├── infra/kyc (mock|nsdl)
        ├── infra/sim (mock|telecom)
        ├── infra/fabric.NewClient()
        ├── api.NewRouter() ───► Chi Router
        │   └── Middleware Stack (12 layers):
        │       1. RequestID
        │       2. RealIP
        │       3. Recoverer
        │       4. Logger
        │       5. CORS
        │       6. SecurityHeaders
        │       7. CSRF (Double-Submit Cookie)
        │       8. BodyLimit (1MB)
        │       9. RateLimit (240/min global)
        │      10. FlowTracking
        │      11. CertPinning (mTLS + X-Cert-Fingerprint)
        │      12. JWT Auth (+ Legacy fallback)
        │
        ├── Outbox Worker (500ms ticker)
        │   └── Redis XADD "risk.decision"
        └── Graceful Shutdown (signal.NotifyContext)

internal/
├── api/                           ← HTTP LAYER (12 files)
│   ├── router.go                  ← 2300+ lines, all routes + middleware
│   ├── middleware/
│   │   ├── auth.go                ← JWT verify + legacy fallback
│   │   ├── rate_limit.go          ← In-memory token bucket
│   │   ├── distributed_rate_limit.go ← Redis INCR sliding window
│   │   ├── cert_pinning.go        ← mTLS enforcement
│   │   ├── csrf.go                ← Double-submit cookie
│   │   ├── cooling_off.go         ← 423 Locked + Retry-After
│   │   ├── idempotency.go         ← Idempotency-Key header (24h TTL)
│   │   ├── otp_gate.go            ← OTP verification gate
│   │   ├── biometric_gate.go      ← WebAuthn/Passkey
│   │   ├── pqc.go                 ← Kyber-1024 session
│   │   ├── rbac.go                ← Role-based access
│   │   ├── scope_token.go         ← X-Internal-Token validation
│   │   ├── stepup.go              ← Step-up authentication
│   │   └── client_ip.go           ← X-Forwarded-For parsing
│   ├── validate.go                ← Request validation
│   ├── aiml_proxy.go              ← AI/ML endpoint proxies
│   └── flow_dashboard.go          ← Request flow observability
│
├── config/                        ← CONFIGURATION (2 files)
│   ├── config.go                  ← Load + Validate (STARTUP GUARDS)
│   │   └── Validate() checks:
│   │       ✓ FINIX_JWT_SECRET
│   │       ✓ FINIX_INTERNAL_TOKEN
│   │       ✓ AIML_UPSTREAM_URL (if AI_PROVIDER=remote)
│   │       ✓ FINIX_REVOCATION_FAIL_CLOSED (warn)
│   │       ✓ CERT_PINS (warn)
│   │       ✓ FINIX_PIN_ENFORCE (warn)
│   └── models.go                  ← All config structs
│
├── domain/                        ← CORE BUSINESS LOGIC (7 subdomains)
│   ├── platform/                  ← CENTRAL ORCHESTRATOR (service.go = 743+ lines)
│   │   ├── service.go             ← Platform struct + ALL business methods
│   │   ├── outbox.go              ← Event outbox (Enqueue, Flush, Retry)
│   │   ├── infrastructure.go      ← DB, Redis, Fabric, Vault wiring
│   │   ├── ledger_accounting.go   ← Double-entry bookkeeping
│   │   ├── chatbot.go             ← Local Groq chatbot
│   │   ├── aiml.go                ← RAG client integration
│   │   ├── bank_adapter.go        ← Bank integration interface
│   │   ├── data_protection.go     ← PII encryption via Vault
│   │   ├── oauth_redis_telemetry.go
│   │   └── dummy_bank.go          ← Mock bank
│   │
│   ├── transaction/               ← RISK ENGINE (5 files)
│   │   ├── risk_engine.go         ← RiskEngine.Evaluate() [ONNX + Heuristic]
│   │   ├── heuristic.go           ← Deterministic scoring (11 factors)
│   │   ├── onnx_predictor.go      ← ONNX Runtime inference
│   │   ├── safety.go              ← NaN/Inf guards
│   │   └── model.go               ← ModelPredictor interface
│   │
│   ├── security/                  ← CRYPTO & AUTH (10 files)
│   │   ├── jwt.go                 ← HS256, 15min TTL, revocation
│   │   ├── pqc.go                 ← Kyber-1024 KEM
│   │   ├── dilithium.go           ← Dilithium5 signatures
│   │   ├── biometric.go           ← WebAuthn/Passkey
│   │   ├── adaptive_auth.go       ← 5-tier auth (Passive→Manual)
│   │   ├── threat_response.go     ← Auto threat response
│   │   ├── otp.go                 ← OTP generation/verify
│   │   ├── pin.go                 ← Argon2id PIN hashing
│   │   ├── sim_binding.go         ← SIM challenge/response
│   │   └── passkey.go             ← Passkey management
│   │
│   ├── fraud/                     ← GNN FRAUD DETECTION (4 files)
│   │   ├── gnn.go                 ← FraudGraph (3-hop propagation, 0.6 decay)
│   │   ├── heuristic.go           ← Heuristic fraud scoring
│   │   ├── model.go               ← FraudModelPredictor interface
│   │   └── riskpropagation.go     ← Risk propagation algorithm
│   │
│   ├── healthscore/               ← FINANCIAL HEALTH (1 file)
│   │   └── calculator.go          ← 7 pillars, resilience modifier
│   │
│   ├── credit/                    ← CREDIT OPTIMIZATION (3 files)
│   │   ├── credit.go              ← Util alerts, BNPL detection
│   │   ├── credit_test.go
│   │   └── xai.go                 ← Explainable AI
│   │
│   └── debt/                      ← DEBT OPTIMIZATION (4 files)
│       ├── amortisation.go        ← Payment schedules
│       ├── optimise.go            ← Avalanche vs Snowball
│       ├── xai.go                 ← Explainable AI
│       └── *_test.go
│
├── infra/                         ← INFRASTRUCTURE ADAPTERS
│   ├── ai/rag_client.go           ← Python RAG HTTP client
│   │   ├── Chat() ───► /v1/rag/chat
│   │   ├── ScanSMS() ───► /v1/rag/sms
│   │   ├── RiskScore() ───► /v1/rag/tool/risk
│   │   └── Health() ───► /health
│   ├── db/postgres.go             ← pgxpool + migrations
│   ├── fabric/                    ← Hyperledger Fabric SDK
│   │   ├── ledger.go              ← Chaincode: consent, insurance, loan, trade, cooling-off
│   │   └── ledger_rules_test.go
│   ├── bank/                      ← Bank integrations (Interface + Razorpay + Mock)
│   ├── kyc/                       ← KYC (NSDL + Mock)
│   ├── sim/                       ← SIM binding (Telecom + Mock)
│   └── repo/                      ← REPOSITORIES (17 files)
│       ├── user.go                ← Users, AuthProfiles, Sessions
│       ├── account.go             ← Bank accounts
│       ├── transaction.go         ← Transactions (with model_version)
│       ├── goal.go                ← Goals + contributions
│       ├── investment.go          ← Holdings, SIPs, dividends
│       ├── insurance.go           ← Policies, premiums
│       ├── loan.go                ← Loans, EMIs
│       ├── payment.go             ← Payments, receipts
│       ├── audit.go               ← Audit events
│       ├── blockchain.go          ← Fabric events
│       ├── beneficiary.go         ← Beneficiaries
│       ├── challenge.go           ← Auth challenges
│       ├── consent.go             ← Consent grants
│       ├── emergency_contact.go   ← Emergency contacts
│       ├── feature_flag.go        ← Feature flags
│       ├── freeze.go              ← Freeze states
│       ├── net_worth.go           ← Assets/liabilities
│       ├── notification.go        ← Notifications
│       ├── pqc.go                 ← PQC sessions
│       ├── session.go             ← Sessions
│       ├── sim.go                 ← Simulations
│       └── phase7_repos.go        ← NEW: realtime_detections, health_snapshots, risk_validations
│
└── pkg/money/                     ← MONEY UTILITIES
    └── money.go                   ← Integer paise, formatting
```

---

## 3. PYTHON RAG SERVICE - DETAILED COMPONENT MAP

```
┌────────────────────────────────────────────────────────────────────────────────────────┐
│                                    PYTHON RAG SERVICE (18 .py files)                    │
│                         D:/FINIX_PRODUCTION/finix-rag/src                               │
└────────────────────────────────────────────────────────────────────────────────────────┘

finix_rag/
├── api.py                           ← FASTAPI APP (364 lines)
│   ├── lifespan()                   ← Initialize: TokenManager, OPAAuthorizer,
│   │                                 SecureIngestor, SecureQueryEngine
│   ├── Middleware:
│   │   └── CORSMiddleware
│   ├── Auth: HTTPBearer (JWT)
│   └── Endpoints:
│       ├── POST /query              ← Main RAG query (with auth, rate limit)
│       ├── POST /ingest             ← Document ingestion (admin/analyst)
│       ├── GET  /health             ← Health check
│       ├── GET  /audit/logs         ← Audit logs (admin)
│       ├── POST /admin/rotate-keys  ← Key rotation (admin)
│       └── GET  /metrics            ← Prometheus metrics
│
├── embedding_config.py              ← BGE-M3 EMBEDDING CONFIG
│   └── get_embedding_config()
│       ├── model: BAAI/bge-m3
│       ├── device: cuda/cpu
│       ├── batch_size: 32
│       └── normalize: true
│
├── query/                           ← QUERY PIPELINE (4 files)
│   ├── __init__.py
│   ├── secure_query_engine.py       ← MAIN PIPELINE (600+ lines)
│   │   ├── query()                  ← Full pipeline orchestration
│   │   │   1. Authorization (OPA)
│   │   │   2. Guardrails.validate_input()
│   │   │   3. Query expansion (47 banking synonyms)
│   │   │   4. PII masking (regex + NER)
│   │   │   5. HybridRetriever.retrieve()
│   │   │   6. Auth-level filtering
│   │   │   7. Reranker.rerank_with_nodes()
│   │   │   8. HMAC integrity check
│   │   │   9. Vault decryption
│   │   │  10. Groq LLM completion
│   │   │  11. PII mask answer
│   │   │  12. Guardrails.validate_output()
│   │   │  13. Confidence calculation
│   │   │  14. Audit log
│   │   └── QueryResponse:
│   │       answer, citations, confidence, classification
│   │       pii_masked, authorization_checked, retrieval_stats
│   │
│   ├── hybrid_retriever.py          ← BM25 + DENSE + RRF FUSION
│   │   ├── retrieve()               ← Top 20 each, RRF → top 5
│   │   ├── _dense_search()          ← Qdrant vector search
│   │   ├── _bm25_search()           ← Qdrant BM25
│   │   ├── _rrf_fusion()            ← Reciprocal Rank Fusion
│   │   └── RetrievalResult(nodes, scores, metadata)
│   │
│   └── reranker.py                  ← BGE-RERANKER-V2-M3 (Thread-Safe!)
│       ├── _reranker_instance       ← Global singleton
│       ├── _reranker_lock           ← threading.Lock()
│       ├── _get_reranker()          ← Double-checked locking
│       ├── rerank_with_nodes()      ← Cross-encoder scoring
│       ├── _sigmoid_normalize()     ← Sigmoid + stretch [0.1, 0.95]
│       └── score_threshold: 0.10
│
├── ingestion/                       ← DOCUMENT INGESTION (5 files)
│   ├── __init__.py
│   ├── secure_ingestor.py           ← MAIN INGESTION ORCHESTRATOR
│   │   ├── ingest()                 ← OCR → PII Mask → Chunk → Embed → Store
│   │   ├── _extract_text()          ← Native PDF + OCR fallback
│   │   ├── _mask_pii()              ← Regex + NER masking
│   │   ├── _chunk()                 ← Semantic chunking (400 tokens, 50 overlap)
│   │   └── _store()                 ← Qdrant upsert with payload
│   │
│   ├── document_processor.py        ← PROCESSING PIPELINE
│   │   ├── process()                ← Multi-format support
│   │   └── semantic_chunking()
│   │
│   ├── ocr_engine.py                ← PADDLE OCR
│   │   ├── extract()                ← Image → Text
│   │   └── preprocess()
│   │
│   └── text_extractor.py            ← NATIVE PDF EXTRACTION
│       ├── extract_pdf()            ← pymupdf (fitz)
│       └── extract_text()
│
├── guardrails/                      ← CONTENT SAFETY (1 file)
│   └── __init__.py                  ← NEMO GUARDRAILS + PROGRAMMATIC
│       ├── _init_rails()            ← Lazy init with fallback
│       ├── validate_input()         ← Pre-query guardrails
│       ├── validate_output()        ← Post-LLM guardrails
│       ├── _programmatic_fallback() ← Rules: PII, financial advice, injection
│       └── GuardrailResult(action, reason, confidence)
│
├── security/                        ← SECURITY LAYER (5 files)
│   ├── __init__.py
│   ├── auth.py                      ← JWT TOKEN MANAGEMENT
│   │   ├── TokenManager (HS256, 30min)
│   │   ├── TokenPayload (sub, role, permissions, exp)
│   │   └── UserRole (admin, analyst, user)
│   │
│   ├── crypto.py                    ← ENCRYPTION
│   │   ├── encrypt()/decrypt()      ← AES-256-GCM
│   │   ├── derive_key()             ← PBKDF2
│   │   └── hmac_sign()/verify()     ← HMAC-SHA256
│   │
│   ├── opa_client.py                ← OPA AUTHORIZATION
│   │   ├── OPAAuthorizer
│   │   ├── is_allowed(user, resource, action)
│   │   └── close()
│   │
│   ├── pii.py                       ← PII DETECTION/MASKING
│   │   ├── detect_pii()             ← Regex patterns (PAN, Aadhaar, phone, email)
│   │   ├── mask_pii()               ← Replacement tokens
│   │   └── unmask_pii()             ← Vault lookup
│   │
│   └── vault_client.py              ← HASHICORP VAULT
│       ├── VaultClient
│       ├── read_secret()
│       ├── write_secret()
│       ├── encrypt_data()
│       └── decrypt_data()
│
└── __init__.py
```

---

## 4. REQUEST FLOW: TRANSACTION INITIATION (ASCII SEQUENCE)

```
CLIENT                          GO BACKEND                          PYTHON RAG
  │                                 │                                    │
  │ POST /v1/transactions/initiate  │                                    │
  │────────────────────────────────►│                                    │
  │                                 │ Chi Router                         │
  │                                 │   │                                │
  │                                 │   ▼ Middleware Stack               │
  │                                 │   ├─ RequestID                     │
  │                                 │   ├─ RealIP                        │
  │                                 │   ├─ Recovery                      │
  │                                 │   ├─ Logger                        │
  │                                 │   ├─ CORS                          │
  │                                 │   ├─ SecurityHeaders               │
  │                                 │   ├─ CSRF                          │
  │                                 │   ├─ BodyLimit(1MB)                │
  │                                 │   ├─ RateLimit(240/min)            │
  │                                 │   ├─ FlowTracking                  │
  │                                 │   ├─ CertPinning (mTLS check)      │
  │                                 │   └─ JWT Auth (or legacy)          │
  │                                 │   │                                │
  │                                 │   ▼                                │
  │                                 │ Platform.InitiateTransaction()     │
  │                                 │   │                                │
  │                                 │   ├─ Check Freeze State            │
  │                                 │   ├─ Check Idempotency Key         │
  │                                 │   ├─ Build RiskSignal              │
  │                                 │   │   ├─ AmountVsAverage           │
  │                                 │   │   ├─ IsNewRecipient            │
  │                                 │   │   ├─ HourOfDay                 │
  │                                 │   │   ├─ FailedPINAttempts         │
  │                                 │   │   ├─ VelocityCount1Hr          │
  │                                 │   │   ├─ BalanceImpact             │
  │                                 │   │   ├─ RecipientGNNScore         │
  │                                 │   │   ├─ BehaviourDrift            │
  │                                 │   │   └─ SessionTrustScore         │
  │                                 │   │                                │
  │                                 │   ├─ RiskEngine.Evaluate()         │
  │                                 │   │   ├─ Normalize (clip NaN/Inf)  │
  │                                 │   │   ├─ ONNX Predict (if avail)   │
  │                                 │   │   │   └─ 9 features → 3 probs  │
  │                                 │   │   └─ Heuristic Fallback        │
  │                                 │   │       └─ 11-factor scoring     │
  │                                 │   │                                │
  │                                 │   ├─ Assessment{level, score, reason,│
  │                                 │   │          model_version}        │
  │                                 │   │                                │
  │                                 │   ├─ AdaptiveAuth.Decide()         │
  │                                 │   │   ├─ Tier 1: Passive           │
  │                                 │   │   ├─ Tier 2: PIN               │
  │                                 │   │   ├─ Tier 3: PIN+OTP           │
  │                                 │   │   ├─ Tier 4: Biometric         │
  │                                 │   │   └─ Tier 5: Manual Review     │
  │                                 │   │                                │
  │                                 │   ├─ Create Transaction (status)   │
  │                                 │   ├─ Dilithium5 Sign               │
  │                                 │   │   payload: txID:user:amt:rcpt:ts│
  │                                 │   ├─ Fabric.WriteEventStrict()     │
  │                                 │   │   (smart contract)             │
  │                                 │   ├─ Outbox.Enqueue("risk.decision",│
  │                                 │   │   {tx_id, risk_level, score,   │
  │                                 │   │    reason, model_version, ts}) │
  │                                 │   ├─ Debit Account (if success)    │
  │                                 │   ├─ Persistence Callback          │
  │                                 │   └─ Return Result                 │
  │                                 │                                    │
  │◄────────────────────────────────│ TransactionResult                  │
  │                                 │                                    │
  │                                 │  ═══════════════════════════════   │
  │                                 │  BACKGROUND OUTBOX WORKER (500ms)  │
  │                                 │  ═══════════════════════════════   │
  │                                 │                                    │                                    │
                                    │ Outbox.Flush()                     │
                                    │   │                                │
                                    │   └─ Redis.XAdd("risk.decision")   │
                                    │                                    │
                                    │                         Redis Stream │
                                    │                         ──────────►│
                                    │                         (Consumer) │
                                    │                                    │
                                    │                                    │ SecureQueryEngine
                                    │                                    │   .validate_risk()
                                    │                                    │   (Internal call)
                                    │                                    │
                                    │                    POST /v1/internal/risk-validation
                                    │                    (X-Internal-Token)           │
                                    │◄────────────────────────────────────────────────│
                                    │                     {agrees_with_ml: false,      │
                                    │                      suggested_level: "high",   │
                                    │                      confidence: 0.92,          │
                                    │                      rationale: "...",          │
                                    │                      recommended_action: "step_up"│
                                    │                      citations: [...],          │
                                    │                      xai_explanation: "..."}    │
                                    │                                    │
                                    │ Platform.ValidateRisk()                       │
                                    │   ├─ Lock transaction row                     │
                                    │   ├─ Check cooling-off window                 │
                                    │   ├─ Apply verdict: block/step_up/dismiss     │
                                    │   ├─ Update risk_level if suggested           │
                                    │   ├─ Outbox.Enqueue("risk_validation_applied")│
                                    │   └─ Persist via callback                     │
                                    │                                    │
                                    │ 200 OK {applied: true, new_status} ─────────►│
```

---

## 5. REQUEST FLOW: RAG QUERY (ASCII SEQUENCE)

```
CLIENT                          GO BACKEND                          PYTHON RAG
  │                                 │                                    │
  │ POST /query                     │                                    │
  │ {question, max_sources}         │                                    │
  │─────────────────────────────────►                                    │
  │                                 │ Chi Router + Middleware            │
  │                                 │   └─ JWT Auth                      │
  │                                 │                                    │
  │                                 │ Platform.Chat()                    │
  │                                 │   │                                │
  │                                 │   └─ ragClient.Chat()              │
  │                                 │       │                            │
  │                                 │       ▼                            │
  │                                 │ HTTP POST /v1/rag/chat             │
  │                                 │──────────────────────────────────►│
  │                                 │                                    │ FastAPI /query
  │                                 │                                    │   ├─ Auth (Bearer)
  │                                 │                                    │   ├─ RateLimit
  │                                 │                                    │   └─ SecureQueryEngine.query()
  │                                 │                                    │       │
  │                                 │                                    │       ├─ OPA Authorize
  │                                 │                                    │       ├─ Guardrails.validate_input()
  │                                 │                                    │       ├─ Query Expansion (47 synonyms)
  │                                 │                                    │       ├─ PII Mask Query
  │                                 │                                    │       ├─ HybridRetriever.retrieve()
  │                                 │                                    │       │   ├─ Dense Search (Qdrant)
  │                                 │                                    │       │   ├─ BM25 Search (Qdrant)
  │                                 │                                    │       │   └─ RRF Fusion → Top 5
  │                                 │                                    │       │
  │                                 │                                    │       ├─ Auth Filter (OPA)
  │                                 │                                    │       ├─ Reranker.rerank_with_nodes()
  │                                 │                                    │       │   └─ BGE-Reranker-v2-M3 (Cross-Encoder)
  │                                 │                                    │       │       Thread-safe singleton!
  │                                 │                                    │       │
  │                                 │                                    │       ├─ HMAC Integrity Check
  │                                 │                                    │       ├─ Vault Decrypt Metadata
  │                                 │                                    │       ├─ Groq LLM (SYSTEM_PROMPT + Context)
  │                                 │                                    │       ├─ PII Mask Answer
  │                                 │                                    │       ├─ Guardrails.validate_output()
  │                                 │                                    │       ├─ Confidence Calculation
  │                                 │                                    │       │   0.40×top1 + 0.15×spread
  │                                 │                                    │       │   + 0.15×diversity + 0.30×alignment
  │                                 │                                    │       └─ Audit Log
  │                                 │                                    │
  │                                 │◄──────────────────────────────────│ ChatResponse
  │                                 │   {answer, citations, confidence, │
  │                                 │    validation_summary, ...}       │
  │                                 │                                    │
  │◄─────────────────────────────────│ ChatResponse                      │
  │  {answer, citations,              │                                    │
  │   confidence, ...}                │                                    │
```

---

## 6. DATA LAYER ARCHITECTURE

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                                         DATA LAYER                                           │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│   POSTGRESQL     │  │     REDIS        │  │     QDRANT       │  │  HYPERLEDGER     │
│   (Primary DB)   │  │ (Streams + Cache)│  │ (Vector Store)   │  │    FABRIC        │
│  ┌────────────┐  │  │  ┌────────────┐  │  │  ┌────────────┐  │  │  ┌────────────┐  │
│  │ 27 Tables  │  │  │  │ Streams:   │  │  │  │ Collection:│  │  │  │ Chaincode: │  │
│  │            │  │  │  │            │  │  │  │ finix_     │  │  │  │ finix      │  │
│  │ Core:      │  │  │  │ risk.decision     │  │ documents  │  │  │  │ Channel:   │  │
│  │ users      │  │  │  │ validation  │  │  │  │            │  │  │  │ finix-     │  │
│  │ accounts   │  │  │  │ applied     │  │  │  │ Vectors:   │  │  │  │ channel    │  │
│  │ sessions   │  │  │  │             │  │  │  │ BGE-M3     │  │  │  │            │  │
│  │            │  │  │  │ Outbox:     │  │  │  │ 1024-dim   │  │  │  │ Events:    │  │
│  │ Auth:      │  │  │  │ (pg table)  │  │  │  │            │  │  │  │ consent-   │  │
│  │ auth_      │  │  │  │             │  │  │  │ Payload:   │  │  │  │ record     │  │
│  │ profiles   │  │  │  │ Rate Limit: │  │  │  │ text       │  │  │  │ insurance- │  │
│  │ challenges │  │  │  │ INCR + EXPIRE    │  │ metadata   │  │  │  │ claim      │  │
│  │ consent_   │  │  │  │ (sliding    │  │  │  │ (encrypted │  │  │  │ loan-      │  │
│  │ grants     │  │  │  │  window)    │  │  │  │  PII)      │  │  │  │ repayment  │  │
│  │            │  │  │  │             │  │  │  │            │  │  │  │ investment-│  │
│  │ Financial: │  │  │  │ Session:    │  │  │  │ Index:     │  │  │  │ trade      │  │
│  │ transactions   │  │  │ telemetry   │  │  │  │ HNSW       │  │  │  │ cooling-   │  │
│  │ goals      │  │  │  │             │  │  │  │            │  │  │  │ off        │  │
│  │ investments│  │  │  │ OAuth:      │  │  │  │ Search:    │  │  │  └────────────┘  │
│  │ insurance  │  │  │  │ state       │  │  │  │ Dense +    │  │  └────────────┘      │
│  │ loans      │  │  │  │             │  │  │  │ BM25       │  │                       │
│  │ payments   │  │  │  └────────────┘  │  │  └────────────┘  │                       │
│  │ receipts   │  │  └──────────────────┘  └──────────────────┘                       │
│  │ net_worth  │  │                         │                         │                 │
│  │            │  │                         ▼                         ▼                 │
│  │ Security:  │  │              ┌──────────────────┐    ┌──────────────────┐           │
│  │ audit_     │  │              │     VAULT        │    │     ONNX         │           │
│  │ events     │  │              │ (Secrets + Encrypt)│   │  (Fraud Model)   │           │
│  │ freeze_    │  │              │  ┌────────────┐  │    │  ┌────────────┐  │           │
│  │ states     │  │              │  │ Transit:   │  │    │  │ model.onnx │  │           │
│  │ jwt_       │  │              │  │ AES-256-GCM│  │    │  │ 9 features │  │           │
│  │ revocations│  │              │  │ Encrypt/   │  │    │  │ 3-class    │  │           │
│  │            │  │              │  │ Decrypt    │  │    │  │ probs      │  │           │
│  │ Graph:     │  │              │  ├────────────┤  │    │  └────────────┘  │           │
│  │ beneficiary_│  │              │  │ KV v2:     │  │    └──────────────────┘           │
│  │ links      │  │              │  │ Secrets    │  │                                 │
│  │            │  │              │  ├────────────┤  │                                 │
│  │ Blockchain:│  │              │  │ Transit:   │  │                                 │
│  │ blockchain_│  │              │  │ Sign/Verify│  │                                 │
│  │ events     │  │              │  └────────────┘  │                                 │
│  │            │  │              └──────────────────┘                                 │
│  │ AI:        │  │                                                                    │
│  │ chatbot_   │  │                                                                    │
│  │ history    │  │                                                                    │
│  │            │  │                                                                    │
│  │ NEW Sprint1:│  │                                                                    │
│  │ realtime_  │  │                                                                    │
│  │ detections │  │                                                                    │
│  │ health_    │  │                                                                    │
│  │ snapshots  │  │                                                                    │
│  │ risk_      │  │                                                                    │
│  │ validations│  │                                                                    │
│  │ fraud_     │  │                                                                    │
│  │ reports    │  │                                                                    │
│  └────────────┘  │                                                                    │
└──────────────────┘                                                                    │
```

---

## 7. SECURITY ARCHITECTURE (DEFENSE IN DEPTH)

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                                        SECURITY LAYERS                                        │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

LAYER 1: NETWORK
├── mTLS (Kyber-1024 KEM + Dilithium5 signatures)
├── Certificate Pinning (CERT_PINS env, FINIX_PIN_ENFORCE)
├── TLS 1.3 everywhere
└── Internal service mesh (future)

LAYER 2: TRANSPORT
├── Security Headers (CSP, HSTS, X-Frame-Options, X-Content-Type-Options)
├── CSRF Protection (Double-Submit Cookie)
├── Body Size Limits (1MB default)
└── CORS (Explicit origins only)

LAYER 3: APPLICATION - RATE LIMITING
├── Global: 240 req/min/IP (in-memory token bucket)
├── Auth endpoints: 20 req/min
├── Registration: 10 req/min
├── Distributed: Redis INCR sliding window
│   Headers: X-RateLimit-Limit, Remaining, Reset, Retry-After
└── Per-endpoint custom limits

LAYER 4: APPLICATION - AUTHENTICATION
├── JWT HS256 (15-min TTL, 7-day refresh)
│   Claims: sub, role, device_fp, iat, exp, jti
├── Device Fingerprint Binding
├── Token Revocation (In-memory + Postgres persistence)
│   Fail-Open default (bounded by TTL)
│   FINIX_REVOCATION_FAIL_CLOSED=true for production
├── Legacy Opaque Token Fallback
├── PQC Session (Kyber-1024) - Optional/Required
├── Dilithium5 Transaction Signatures (Non-repudiation)
├── WebAuthn/Passkey (Biometric)
├── OTP (SMS/Email)
├── PIN (Argon2id)
├── SIM Binding Challenge
└── Adaptive Auth (5 Tiers: Passive → Manual Review)

LAYER 5: APPLICATION - AUTHORIZATION
├── OPA (Open Policy Agent) - Policy-as-Code
│   Rego policies for: resource access, data classification
├── RBAC (Role-Based Access Control)
│   Roles: admin, analyst, user
├── Scoped Internal Tokens (X-Internal-Token)
│   For service-to-service (RAG validation callback)
└── Feature Flags

LAYER 6: DATA PROTECTION
├── PII Encryption at Rest (Vault Transit - AES-256-GCM)
│   Fields: phone, email, PAN, Aadhaar, etc.
├── PII Masking in Logs/Responses (Regex + NER)
├── HMAC Integrity Checks (RAG retrieval)
├── Argon2id for PIN/Password Hashing
├── Dilithium5 for Transaction Non-repudiation
├── Kyber-1024 for Post-Quantum Key Exchange
└── Audit Logging (All authz decisions, sensitive ops)

LAYER 7: CONTENT SAFETY (RAG)
├── NeMo Guardrails (Input/Output validation)
├── Programmatic Fallback Rules:
│   ├─ PII leakage detection
│   ├─ Financial advice disclaimer injection
│   ├─ Prompt injection detection
│   └─ Off-topic rejection
├── Contextual Advice Patterns (User-addressed)
└── Citation Enforcement (Every claim sourced)
```

---

## 8. DEPLOYMENT TOPOLOGY

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                                        PRODUCTION DEPLOYMENT                                  │
└─────────────────────────────────────────────────────────────────────────────────────────────┘

                           ┌─────────────────┐
                           │   INTERNET      │
                           └────────┬────────┘
                                    │
                           ┌────────▼────────┐
                           │  WAF / DDoS     │
                           │  Protection     │
                           └────────┬────────┘
                                    │
                           ┌────────▼────────┐
                           │  LOAD BALANCER  │
                           │  (nginx/Traefik)│
                           │  TLS Termination│
                           └────────┬────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
      ┌───────▼───────┐     ┌───────▼───────┐     ┌───────▼───────┐
      │  BACKEND #1   │     │  BACKEND #2   │     │  BACKEND #N   │
      │  (Go :8080)   │     │  (Go :8080)   │     │  (Go :8080)   │
      │  ┌──────────┐ │     │  ┌──────────┐ │     │  ┌──────────┐ │
      │  │ Platform │ │     │  │ Platform │ │     │  │ Platform │ │
      │  │ Service  │ │     │  │ Service  │ │     │  │ Service  │ │
      │  └──────────┘ │     │  └──────────┘ │     │  └──────────┘ │
      └───────┬───────┘     └───────┬───────┘     └───────┬───────┘
              │                     │                     │
              └─────────────────────┼─────────────────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
      ┌───────▼───────┐     ┌───────▼───────┐     ┌───────▼───────┐
      │  POSTGRESQL   │     │    REDIS      │     │   QDRANT      │
      │  (Primary)    │     │  (Cluster)    │     │  (Cluster)    │
      │  ┌──────────┐ │     │  ┌──────────┐ │     │  ┌──────────┐ │
      │  │ Read     │ │     │  │ Master   │ │     │  │ Leader   │ │
      │  │ Replica  │ │     │  │ Replicas │ │     │  │ Replicas │ │
      │  └──────────┘ │     │  └──────────┘ │     │  └──────────┘ │
      └───────────────┘     └───────────────┘     └───────────────┘
              │                     │                     │
              └─────────────────────┼─────────────────────┘
                                    │
              ┌─────────────────────┼─────────────────────┐
              │                     │                     │
      ┌───────▼───────┐     ┌───────▼───────┐     ┌───────▼───────┐
      │  VAULT        │     │  FABRIC       │     │  PYTHON RAG   │
      │  (HA Cluster) │     │  (Orderer +   │     │  (K8s Deploy) │
      │               │     │   Peers)      │     │  ┌──────────┐  │
      │  Transit + KV │     │               │     │  │ Pod #1   │  │
      └───────────────┘     └───────────────┘     │  │ Pod #2   │  │
                                                   │  │ Pod #N   │  │
                                                   │  └──────────┘  │
                                                   └───────────────┘
                                    │
                                    ▼
                          ┌─────────────────┐
                          │  MONITORING     │
                          │  ┌───────────┐  │
                          │  │ Prometheus│  │  ◄── /metrics (always exposed)
                          │  │ Grafana   │  │
                          │  │ AlertMgr  │  │
                          │  └───────────┘  │
                          └─────────────────┘

KEY PRODUCTION CONFIGS:
├── FINIX_ENV=production
├── FINIX_REVOCATION_FAIL_CLOSED=true
├── FINIX_PIN_ENFORCE=true
├── CERT_PINS=sha256/AAAA...,sha256/BBBB...
├── AI_PROVIDER=remote
├── AIML_UPSTREAM_URL=https://rag.finix.app
├── RAG_PG_DSN=postgres://finix_rag_ro:***@replica:5432/finix
├── REDIS_URL=redis://redis-cluster:6379 (TLS)
├── PGSSLMode=verify-full
└── QDRANT_TLS=true
```

---

## 9. MODULE DEPENDENCY GRAPH (GO)

```
main.go
  │
  ├─► config.Load() ───► config.Validate() [STARTUP GUARDS]
  │
  ├─► db.New() ───► pgxpool + Migrate(migrations/)
  │
  ├─► infra/ai.NewRagClient() ───► HTTP Client → Python RAG :8000
  │
  ├─► infra/bank (Interface → Mock|Razorpay)
  ├─► infra/kyc (Interface → Mock|NSDL)
  ├─► infra/sim (Interface → Mock|Telecom)
  ├─► infra/fabric.NewClient() ───► Gateway → Network → Contract
  │
  ├─► platform.NewService() ───► Platform Struct (743+ lines)
  │     │
  │     ├─► domain/transaction.RiskEngine
  │     │     ├─► ONNX Predictor (models/security/fraud_label/v1/model.onnx)
  │     │     └─► Heuristic Predictor (11 factors)
  │     │
  │     ├─► domain/security
  │     │     ├─► JWTManager (HS256, revocation, persistence)
  │     │     ├─► PQC (Kyber-1024 KEM)
  │     │     ├─► Dilithium5 (Signatures)
  │     │     ├─► AdaptiveAuth (5 tiers)
  │     │     ├─► ThreatResponse
  │     │     ├─► Biometric (WebAuthn)
  │     │     ├─► OTP, PIN (Argon2id), SIM Binding, Passkey
  │     │
  │     ├─► domain/fraud.FraudGraph (GNN, 3-hop, 0.6 decay)
  │     ├─► domain/healthscore.Calculator (7 pillars)
  │     ├─► domain/credit (BNPL, utilization alerts)
  │     ├─► domain/debt (Avalanche vs Snowball)
  │     │
  │     ├─► Outbox (Enqueue → Redis Streams)
  │     ├─► Ledger (Fabric WriteEventStrict)
  │     ├─► AccountingLedger (Double-entry)
  │     ├─► DB Repositories (17 repo files)
  │     ├─► RagClient (ai package)
  │     ├─► Chatbot (Local Groq)
  │     ├─► DataProtection (Vault encryption)
  │     └─► Bank/KYC/SIM Adapters
  │
  └─► api.NewRouter() ───► Chi Router
        │
        ├─► Middleware Stack (12 layers)
        │
        ├─► /v1/auth/* (Public)
        ├─► /v1/internal/* (X-Internal-Token)
        ├─► /v1/compliance/* (Admin + JWT + Role)
        ├─► /v1/admin/* (Admin + JWT + Role)
        ├─► /v1/* (JWT + PQC if enforced)
        └─► /metrics (Prometheus - ALWAYS)
```

---

## 10. MODULE DEPENDENCY GRAPH (PYTHON RAG)

```
api.py (FastAPI)
  │
  ├─► lifespan()
  │     ├─► TokenManager (auth.py)
  │     ├─► OPAAuthorizer (opa_client.py)
  │     ├─► SecureIngestor (ingestion/secure_ingestor.py)
  │     │     ├─► Qdrant Client
  │     │     ├─► TextExtractor (text_extractor.py)
  │     │     ├─► OCREngine (ocr_engine.py)
  │     │     ├─► DocumentProcessor (document_processor.py)
  │     │     └─► VaultClient (security/vault_client.py)
  │     │
  │     └─► SecureQueryEngine (query/secure_query_engine.py)
  │           ├─► HybridRetriever (query/hybrid_retriever.py)
  │           │     ├─► Qdrant Dense Search
  │           │     ├─► Qdrant BM25 Search
  │           │     └─► RRF Fusion
  │           │
  │           ├─► Reranker (query/reranker.py) [THREAD-SAFE SINGLETON]
  │           │     └─► BGE-Reranker-v2-M3 (FlagEmbedding)
  │           │
  │           ├─► Guardrails (guardrails/__init__.py)
  │           │     ├─► NeMo Guardrails (optional)
  │           │     └─► Programmatic Fallback (PII, injection, advice)
  │           │
  │           ├─► OPAAuthorizer (Authorization)
  │           ├─► PII Masking (security/pii.py)
  │           ├─► VaultClient (Decrypt metadata)
  │           ├─► Groq API (LLM)
  │           └─► HMAC (crypto.py)
  │
  └─► Endpoints:
        ├─► POST /query → SecureQueryEngine.query()
        ├─► POST /ingest → SecureIngestor.ingest()
        ├─► GET /health → Health check
        ├─► GET /audit/logs → Audit log query
        └─► POST /admin/rotate-keys → Key rotation
```

---

*Generated from complete codebase scan: 124 Go files + 18 Python source files*
*Location: D:/FINIX_PRODUCTION/backend + D:/FINIX_PRODUCTION/finix-rag/src*
*Date: 2024-07-25*