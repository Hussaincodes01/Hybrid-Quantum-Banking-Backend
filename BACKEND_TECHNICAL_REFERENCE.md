# FINIX Backend - Complete Technical Reference

> **Comprehensive guide to every module, component, and data flow in the FINIX Go backend.**

---

## 📋 Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Detailed Architecture Diagrams](#detailed-architecture-diagrams)
3. [Core Domain Modules](#core-domain-modules)
4. [Infrastructure Layer](#infrastructure-layer)
5. [API Layer](#api-layer)
6. [Security & Cryptography](#security--cryptography)
7. [Data Flow Diagrams](#data-flow-diagrams)
8. [Configuration & Deployment](#configuration--deployment)
9. [Testing & Quality](#testing--quality)

---

## 🏗️ Architecture Overview

### High-Level Component Diagram

```mermaid
graph TB
    subgraph "External"
        CLIENT[Flutter/React Native App]
        PYTHON_RAG[Python RAG Service :8000]
    end

    subgraph "HTTP Layer"
        ROUTER[Chi Router + Middleware Stack]
    end

    subgraph "Middleware Stack"
        CORS[CORS]
        RATE[RateLimit 240/min]
        CSRF[CSRF Defense]
        SEC[Security Headers]
        PIN[Cert Pinning]
        FLOW[Flow Tracker]
        AUTH_MW[Auth Middleware]
    end

    subgraph "Core Services"
        PLATFORM[Platform Service<br/>Core Business Logic]
        AUTH_SVC[Auth Service]
        TXN_SVC[Transaction Service]
        AI_GATEWAY[AI Gateway]
        LEDGER[Ledger Service]
    end

    subgraph "Domain Modules"
        TXN_DOM[Transaction Domain]
        SECURITY_DOM[Security Domain]
        FRAUD_DOM[Fraud Domain]
        HEALTH_DOM[HealthScore Domain]
        CREDIT_DOM[Credit Domain]
        DEBT_DOM[Debt Domain]
    end

    subgraph "Infrastructure"
        POSTGRES[(PostgreSQL)]
        REDIS[(Redis Streams)]
        FABRIC[(Hyperledger Fabric)]
        VAULT[HashiCorp Vault]
    end

    CLIENT --> ROUTER
    PYTHON_RAG --> ROUTER
    ROUTER --> CORS --> RATE --> CSRF --> SEC --> PIN --> FLOW --> AUTH_MW
    AUTH_MW --> PLATFORM
    PLATFORM --> AUTH_SVC
    PLATFORM --> TXN_SVC
    PLATFORM --> AI_GATEWAY
    PLATFORM --> LEDGER
    PLATFORM --> TXN_DOM
    PLATFORM --> SECURITY_DOM
    PLATFORM --> FRAUD_DOM
    PLATFORM --> HEALTH_DOM
    PLATFORM --> CREDIT_DOM
    PLATFORM --> DEBT_DOM
    PLATFORM --> POSTGRES
    PLATFORM --> REDIS
    PLATFORM --> FABRIC
    PLATFORM --> VAULT
```

### B. Module Dependency Graph
```mermaid
graph LR
    subgraph "Entry Points"
        HTTP[HTTP Handlers]
        SCHEDULER[Scheduled Jobs]
        OUTBOX[Outbox Worker]
    end

    subgraph "Platform Service"
        PS[Platform]
        AUTH[Auth]
        TXN[Transaction]
        RISK[Risk Engine]
    end

    subgraph "Domain"
        FRAUD[Fraud Graph]
        HEALTH[Health Score]
        CREDIT[Credit]
        DEBT[Debt]
        FINPROFILE[Financial Profile]
        GOAL[Goals]
        INVEST[Investment]
        INSUR[Insurance]
        LOAN[Loan]
    end

    subgraph "Security"
        JWT[JWT Manager]
        PQC[Post-Quantum]
        DILITHIUM[Dilithium]
        BIOMETRIC[Biometric]
        THREAT[Threat Response]
        ADAPTIVE[Adaptive Auth]
    end

    subgraph "Storage"
        PG[(PostgreSQL)]
        RED[(Redis)]
        FAB[(Fabric)]
        VAULT[(Vault)]
        ONNX[(ONNX Model)]
        QDRANT[(Qdrant)]
    end

    HTTP --> PS
    SCHEDULER --> PS
    OUTBOX --> PS
    PS --> AUTH
    PS --> TXN
    PS --> RISK
    TXN --> RISK
    RISK --> ONNX
    RISK --> FRAUD
    AUTH --> JWT
    AUTH --> PQC
    AUTH --> DILITHIUM
    AUTH --> BIOMETRIC
    AUTH --> ADAPTIVE
    AUTH --> THREAT
    AUTH --> RED
    TXN --> PG
    TXN --> FAB
    TXN --> VAULT
    FRAUD --> PG
    FRAUD --> RED
    HEALTH --> PG
    CREDIT --> PG
    DEBT --> PG
    GOAL --> PG
    INVEST --> PG
    INSUR --> PG
    LOAN --> PG
    PS --> PG
    PS --> RED
    PS --> FAB
    PS --> VAULT
```

### C. Data Flow: Transaction Lifecycle
```mermaid
sequenceDiagram
    participant Client
    participant Router
    participant Platform
    participant RiskEngine
    participant Outbox
    participant Redis
    participant PG[(PostgreSQL)]
    participant Fabric[(Hyperledger Fabric)]

    Client->>Router: POST /v1/transactions/initiate
    Router->>Platform: InitiateTransaction(userID, req)
    Platform->>Platform: Check freeze, idempotency
    Platform->>RiskEngine: Evaluate(RiskSignal)
    RiskEngine->>RiskEngine: Normalize (clip NaN/Inf)
    RiskEngine->>ONNX: Predict(features)
    ONNX-->>RiskEngine: probs [low, med, high]
    alt ONNX fails
        RiskEngine->>RiskEngine: Heuristic fallback
    end
    RiskEngine-->>Platform: Assessment{level, score, reason}
    Platform->>Platform: AdaptiveAuth decision
    Platform->>PG: Create Transaction{status}
    Platform->>Platform: Dilithium sign(txID:userID:amt:recipient:ts)
    Platform->>Fabric: WriteEventStrict(smart contract)
    Platform->>Outbox: Enqueue("risk.decision", payload)
    alt success
        Platform->>PG: Debit account
    end
    Platform-->>Client: TransactionResult

    Note over Outbox,Redis: Background (500ms ticker)
    Outbox->>Redis: XADD risk.decision {payload}
    Redis->>Python_RAG: Stream consumer
    Python_RAG->>Platform: POST /v1/internal/risk-validation
    Platform->>PG: Lock + find transaction
    Platform->>Platform: Apply verdict
    Platform-->>Python_RAG: 200 OK {applied, new_status}
    Python_RAG-->>Redis: Ack
```

### D. Data Flow: RAG Query Pipeline
```mermaid
sequenceDiagram
    participant Client
    participant Router
    participant Platform
    participant QEngine[Query Engine]
    participant Retriever[Hybrid Retriever]
    participant Reranker[Reranker]
    participant Guardrails
    participant Groq
    participant QDRANT[(Qdrant)]
    participant VAULT[(Vault)]

    Client->>Router: POST /query
    Router->>Platform: Chat(userID, req)
    Platform->>QEngine: query(question, user, ip)

    QEngine->>QEngine: Authorization (OPA)
    QEngine->>Guardrails: validate_input(question)
    QEngine->>QEngine: Expand query (domain synonyms)
    QEngine->>QEngine: PII mask query

    QEngine->>Retriever: retrieve(query)
    Retriever->>QDRANT: Dense + BM25 (top 20 each)
    QDRANT-->>Retriever: Nodes + scores
    Retriever->>Retriever: RRF fusion -> top 5
    Retriever-->>QEngine: RetrievalResult

    QEngine->>QEngine: Filter by auth level
    QEngine->>Reranker: rerank(query, nodes)
    Reranker->>QDRANT: Cross-encoder (BGE-Reranker-v2-M3)
    QDRANT-->>Reranker: Relevance scores
    Reranker-->>QEngine: Top 5 reranked docs

    QEngine->>QEngine: HMAC integrity check
    QEngine->>Vault: Decrypt sensitive metadata
    QEngine->>Groq: Complete (SYSTEM_PROMPT + context)
    Groq-->>QEngine: Answer
    QEngine->>QEngine: PII mask answer
    QEngine->>Guardrails: validate_output(answer)
    QEngine->>QEngine: Confidence calculation
    QEngine->>Platform: QueryResponse
    Platform-->>Client: ChatResponse
```

---

## 🎯 Core Domain Modules

### 1. Platform Service (`internal/domain/platform/service.go`)

The **central orchestrator** — a single instance per process that manages all application state.

#### Responsibilities
- User lifecycle (registration, authentication, profiles)
- Transaction orchestration (initiate, override, history)
- Account management (linking, balances, verification)
- Goal management (create, contribute, track)
- Investment & insurance tracking
- Loan management
- Net worth calculation
- Emergency contacts & notifications
- Feature flags
- Simulation engine (Monte Carlo)
- Market news aggregation
- Persona analysis
- Security tips

#### Key Data Structures

```go
type Service struct {
    // Core state
    users               map[string]*User
    authProfiles        map[string]*AuthProfile
    kycProfiles         map[string]*KYCProfile
    accounts            map[string][]Account
    transactions        map[string][]Transaction
    beneficiaries       map[string]map[string]struct{}

    // Domain modules
    riskEngine          transaction.RiskEngine
    healthCalculator    healthscore.Calculator
    fraudGraph          *fraud.FraudGraph
    adaptiveAuth        *security.AdaptiveAuthManager
    threatResponse      *security.ThreatResponseManager

    // Infrastructure
    outbox              *Outbox
    ledger              *blockchain.Ledger
    accountingLedger    *AccountingLedger
    db                  *ServicePostgres
    ragClient           *ai.RagClient
    chatbot             *ChatbotClient
    // ... more
}
```

#### Critical Methods

| Method | Purpose |
|--------|---------|
| `InitiateTransaction()` | Full payment flow with risk evaluation, Dilithium signing, Fabric event |
| `OverrideTransaction()` | Biometric+OTP override for blocked transactions |
| `ValidateRisk()` | Internal endpoint for Python RAG validation of ML risk decision |
| `Chat()` | AI chat with RAG fallback |
| `ScanSMS()` | SMS fraud detection with RAG enhancement |
| `HealthScore()` | Financial health calculation across 7 pillars |
| `RunSimulation()` | Monte Carlo portfolio simulation |

---

### 2. Transaction Domain (`internal/domain/transaction/`)

#### Files

| File | Purpose |
|------|---------|
| `risk_engine.go` | Main risk evaluation engine (ONNX + heuristic fallback) |
| `heuristic.go` | Deterministic heuristic scoring |
| `onnx_predictor.go` | ONNX ML model inference |
| `safety.go` | NaN/Inf guards, clamping |
| `model.go` | ModelPredictor interface |

#### Risk Engine (`risk_engine.go`)

```go
type RiskEngine struct {
    predictor   ModelPredictor      // ONNX or heuristic
    heuristic   *HeuristicPredictor
    amt         config.AmountTermParams
}

func (e RiskEngine) Evaluate(sig RiskSignal) Assessment {
    // 1. Normalize signals (clip NaN/Inf)
    sig = normaliseSignal(sig, e.amt)

    // 2. Try ONNX model first
    if e.predictor.IsAvailable() {
        features := signalToFeatures(sig)
        probs, err := e.predictor.Predict(features)
        if err == nil && len(probs) == 3 {
            return assessmentFromProbs(probs, sig)
        }
    }

    // 3. Fallback to heuristic
    return e.heuristic.assess(sig)
}
```

#### RiskSignal Input

```go
type RiskSignal struct {
    AmountVsAverage      float64  // tx amount / historical avg (floored at Rs 500)
    IsNewRecipient       bool     // Never transacted before
    HourOfDay            int      // 0-23
    FailedPINAttempts    int      // Recent failed PINs
    VelocityCount1Hr     int      // Tx count in last hour
    BalanceImpact        float64  // tx_amount / available_balance
    RecipientGNNScore    float64  // Fraud graph proximity score
    BehaviourDrift       float64  // Session anomaly score
    SessionTrustScore    float64  // Device/session trust 0-1
}
```

#### Assessment Output

```go
type Assessment struct {
    Level        RiskLevel  // low/medium/high
    Score        float64    // 0-100
    Reason       string     // Human-readable explanation
    ModelVersion string     // e.g. "srishti-math-v1+override"
}
```

#### Heuristic Scoring (`heuristic.go`)

| Factor | Weight | Threshold |
|--------|--------|-----------|
| Amount vs Average | K=12, Cap=30 | >1x average |
| New Recipient | +15 | Boolean |
| Unusual Hour (00-04, 23) | +10 | Time-based |
| Failed PIN Attempts | +5 each (max 15) | Count |
| Velocity (>3/hr) | +2 each (max 10) | Count |
| Balance Impact >80% | +10 | Ratio |
| Balance Impact >50% | +5 | Ratio |
| GNN Score | x20 (max 20) | 0-1 |
| Behaviour Drift | x20 (max 20) | 0-1 |
| Low Session Trust | +15 (<0.4) | Score |
| Medium Session Trust | +8 (<0.7) | Score |

**Risk Tiers:**
- **Low**: Score < 40
- **Medium**: 40 <= Score < 70
- **High**: Score >= 70

#### ONNX Predictor (`onnx_predictor.go`)

- **Model path**: `FINIX_RISK_MODEL_PATH` (default: `backend/models/security/fraud_label/v1/model.onnx`)
- **Input**: 9 normalized features (RiskSignal)
- **Output**: 3-class probabilities [low_prob, medium_prob, high_prob]
- **Fallback**: Heuristic on any error

#### Safety Module (`safety.go`)

- `safeFloat64()` — sanitises NaN/Inf to 0 via `math.IsNaN`/`math.IsInf` guardrails

---

### 3. Security Domain (`internal/domain/security/`)

#### Files

| File | Purpose |
|------|---------|
| `jwt.go` | JWT management (HS256, 15-min TTL) |
| `pqc.go` | Kyber-1024 KEM (post-quantum) |
| `dilithium.go` | Dilithium5 signatures |
| `biometric.go` | WebAuthn/Passkey support |
| `adaptive_auth.go` | 5-tier adaptive authentication |
| `threat_response.go` | Automated threat response |
| `otp.go` | OTP generation/verification |
| `pin.go` | PIN hashing (Argon2id) |
| `sim_binding.go` | SIM binding challenges |
| `passkey.go` | WebAuthn credential management |

#### JWT Manager (`jwt.go`)

```go
type JWTManager struct {
    secret     []byte                    // HS256 key
    revoked    map[string]int64          // jti -> expiry
    persist    func(jti string, exp time.Time)
    lookup     func(jti string) (bool, error)
    failClosed bool                        // Fail-closed on lookup error
}
```

**Token Claims:**
```go
type Claims struct {
    Sub      string `json:"sub"`       // userID
    Role     string `json:"role"`        // customer/admin
    DeviceFP string `json:"device_fp"` // bound device fingerprint
    IAT      int64  `json:"iat"`       // issued at
    EXP      int64  `json:"exp"`       // expires at (15 min)
    JTI      string `json:"jti"`       // unique token ID
}
```

**Revocation Policy:**
- In-memory map + optional Postgres persistence (via `SetPersistenceCallback`)
- Fail-open default (15-min TTL bounds exposure window)
- `FINIX_REVOCATION_FAIL_CLOSED=true` enables fail-closed
- Atomic rotation on refresh (prevents token doubling attacks)

#### Adaptive Authentication (`adaptive_auth.go`)

**5-Tier System:**

| Tier | Name | Requirements | Triggers |
|------|------|--------------|----------|
| 1 | Passive | None | Low risk, trusted device |
| 2 | PIN | PIN entry | Medium risk |
| 3 | PIN + OTP | PIN + SMS/Email OTP | High risk |
| 4 | Biometric | Fingerprint/FaceID | Critical risk |
| 5 | Manual Review | Compliance review | Extreme risk |

**Decision Factors:**
- Session risk profile (device trust, behaviour drift, failed auth count)
- Transaction risk profile (score, risk level, amount, new recipient)
- User role & KYC status

#### Post-Quantum Cryptography (`pqc.go`)

- **Kyber-1024** for key encapsulation (KEM)
- Session establishment via `/v1/auth/pqc/init` -> `/v1/auth/pqc/encapsulate`
- `FINIX_REQUIRE_PQC=true` enforces PQC-only sessions
- Keys rotated daily via cron

#### Dilithium Signatures (`dilithium.go`)

- **Dilithium5** for transaction non-repudiation
- Every transaction signed: `txID:userID:amount:recipient:timestamp`
- Public key verification via `/v1/auth/dilithium/public-key`

---

### 4. Fraud Domain (`internal/domain/fraud/`)

#### Files

| File | Purpose |
|------|---------|
| `gnn.go` | Graph Neural Network for mule detection |
| `heuristic.go` | Heuristic fraud scoring |
| `model.go` | Fraud model predictor interface |
| `riskpropagation.go` | Risk propagation algorithm |

#### Fraud Graph (`gnn.go`)

```go
type FraudGraph struct {
    mu       sync.RWMutex
    nodes    map[string]*GraphNode
    inEdges  map[string]map[string]*GraphEdge
    gen      uint64
    cache    map[string]cachedRisk
}

type GraphNode struct {
    ID         string
    IsFraud    bool
    IsMule     bool
    RiskScore  float64
    Edges      map[string]*GraphEdge
    LastUpdate time.Time
}

type GraphEdge struct {
    To        string
    Weight    float64  // structuringWeight (count + amount + velocity)
    TxCount   int
    TotalAmt  int64
    FirstTxAt time.Time
    LastTxAt  time.Time
}
```

**Risk Propagation Algorithm:**
- Max 3 hops from known fraud/mule nodes
- Decay factor: 0.6 per hop
- Edge weight = max(count_signal, amount_signal, velocity_signal)
- Reverse edges tracked for incoming fraud detection

**Structuring Weight Calculation:**
```go
func structuringWeight(e *GraphEdge) float64 {
    countSignal := float64(e.TxCount) / 10.0
    amtSignal   := float64(e.TotalAmt) / 100_000_000  // Rs 10L
    velSignal   := (float64(e.TxCount) / days) / 10.0  // tx/day
    return min(max(countSignal, amtSignal, velSignal), 1.0)
}
```

**Seeded Patterns:**
- 7 known fraud patterns (unknown-urgent, lottery-winner, etc.)
- 5 known mule accounts

---

### 5. Health Score Domain (`internal/domain/healthscore/calculator.go`)

#### 7 Pillars (weights sum to 1.0)

| Pillar | Weight | Metric |
|--------|--------|--------|
| Liquidity | 0.18 | Emergency fund months (>=6 = 100) |
| Debt Health | 0.18 | DTI ratio (<=0 = 100, >=1 = 0) |
| Savings Behaviour | 0.15 | Savings rate (>=35% = 100) |
| Investment Quality | 0.15 | Diversification score (0-100) |
| Protection Coverage | 0.15 | Insurance adequacy (0-100) |
| Goal Alignment | 0.12 | On-track ratio (>=1.0 = 100) |
| Financial Behaviour | 0.07 | Behaviour quality (0-100) |

#### Resilience Modifier (6-month weighted average)
- Weights: 0.05, 0.08, 0.10, 0.15, 0.25, 0.37 (earliest to latest)
- Score >= 80: +5% bonus
- Score >= 65: +2% bonus
- Score <= 40: -6% penalty

#### Output

```go
type Result struct {
    Score300To900 int      // 300-900 scale
    Band          string   // red/amber/green
    Pillars       []Pillar
}
```

---

### 6. Credit Domain (`internal/domain/credit/`)

- Credit utilization alerts (30% threshold)
- BNPL detection (provider list + MCC codes 6012, 6051)
- Utilization lead alerts (T-5 days)

---

### 7. Debt Domain (`internal/domain/debt/`)

- **Avalanche vs Snowball** optimization
- Refinancing analysis (repo transmission factor)
- Low adherence threshold (0.5) for snowball preference
- Foreclosure penalty calculation (RBI 2014 compliance)

---

## 🏗️ Infrastructure Layer

### 1. Database (`internal/infra/db/postgres.go`)

```go
type Config struct {
    Host, Port, Database, User, Password, SSLMode string
    MaxConns int32
}

func New(cfg Config) (*pgxpool.Pool, error)
func Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) error
```

**Migrations** (27 files in `migrations/`):
- Core: users, sessions, accounts, transactions, goals
- Security: audit_events, freeze_states, consent_grants, challenges
- Graph: beneficiary_links
- Blockchain: blockchain_events
- AI: chatbot_history
- Phase 7: net_worth, insurance, loans, emergency_contacts, feature_flags, notifications
- PQC sessions, payments, expanded transactions
- Ledger events, JWT revocations

---

### 2. Repository Layer (`internal/infra/repo/`)

| File | Entities |
|------|----------|
| `user.go` | Users, auth profiles, sessions |
| `account.go` | Bank accounts, balances |
| `transaction.go` | Transactions (with model_version) |
| `goal.go` | Goals, contributions |
| `investment.go` | Holdings, SIPs, dividends |
| `insurance.go` | Policies, premiums |
| `loan.go` | Loans, EMIs |
| `payment.go` | Payments, receipts |
| `phase7_repos.go` | Insurance, loans, fraud_reports, notifications, **realtime_detections, health_score_snapshots, risk_validations** |
| `audit.go` | Audit events |
| `blockchain.go` | Fabric events |
| `beneficiary.go` | Beneficiaries |
| `challenge.go` | Auth challenges |
| `consent.go` | Consent grants |
| `emergency_contact.go` | Emergency contacts |
| `feature_flag.go` | Feature flags |
| `freeze.go` | Freeze states |
| `net_worth.go` | Assets/liabilities |
| `notification.go` | Notifications |
| `pqc.go` | PQC sessions |
| `session.go` | Sessions |
| `sim.go` | Simulations |

---

### 3. Redis (`internal/infra/redis/` via go-redis/v9)

**Usage:**
- JWT revocation persistence (sorted sets with TTL)
- Rate limiting (distributed_rate_limit.go)
- Outbox event publishing (Redis Streams)
- OAuth state storage
- Session telemetry

---

### 4. Hyperledger Fabric (`internal/infra/fabric/`)

```go
type Client struct {
    gateway     *gateway.Gateway
    network     *gateway.Network
    contract    *gateway.Contract
}
```

**Chaincode:** `finix` on channel `finix-channel`
**Events:** `consent-record`, `insurance-claim`, `loan-repayment`, `investment-trade`, `cooling-off`

---

### 5. AI Gateway (`internal/infra/ai/rag_client.go`)

```go
type RagClient struct {
    baseURL       string
    internalToken string
    httpClient    *http.Client
}

// Methods:
Chat(ctx, ChatRequest)    -> ChatResponse      // /v1/rag/chat
ScanSMS(ctx, sender, msg) -> SMSBadgeResponse  // /v1/rag/sms
RiskScore(ctx, features)  -> RiskScoreResponse // /v1/rag/tool/risk
Health(ctx)               -> error               // /health
```

**Request/Response Types:**

```go
type ChatRequest struct {
    Query      string
    History    []ChatMessage
    UserID     string
    DeviceFP   string
    Scope      string        // wealth|fraud|general
    MaxSources int
}

type ChatResponse struct {
    Answer            string
    Citations         []Citation
    Confidence        float64
    ValidationSummary *ValidationSummary
    Recommendations   *Recommendations
}

type ValidationSummary struct {
    TxID               string
    AgreesWithML       bool
    SuggestedLevel     string
    Confidence         float64
    Rationale          string
    Action             string
    Citations          []string
    XAIExplanation     string
    RecommendedAction  string
}
```

---

### 6. External Integrations

| Adapter | File | Methods |
|---------|------|---------|
| Bank | `bank_adapter.go`, `razorpay.go`, `mock.go` | `VerifyUPI`, `VerifyIFSC`, `GetBalance`, `SubmitPayment` |
| KYC | `kyc/mock.go`, `nsdl.go` | `VerifyPAN`, `VerifyAadhaar` |
| SIM | `sim/mock.go`, `telecom.go` | `CreateChallenge`, `VerifyChallenge` |

---

## 🌐 API Layer (`internal/api/`)

### Router (`router.go`)

**Middleware Stack (order matters):**

```
1. chimw.RequestID      - Unique request IDs
2. chimw.RealIP          - Client IP extraction
3. chimw.Recoverer       - Panic recovery
4. chimw.Logger          - Structured logging
5. corsMiddleware         - CORS with explicit origins
6. securityHeaders        - CSP, HSTS, X-Frame-Options
7. csrfDefense            - Double-submit cookie
8. maxBodyBytes           - 1MB default
9. apimw.RateLimit(240/min) - Global rate limit
10. flow.Middleware       - Request flow tracking
11. PinVerificationMiddleware - Cert pinning
12. Auth                  - JWT + legacy fallback
```

**Route Groups:**

| Prefix | Middleware | Endpoints |
|--------|------------|-----------|
| `/v1/auth/*` | None (public) | register, login, PIN, biometric, OTP |
| `/v1/internal/*` | `RequireScopedToken(X-Internal-Token)` | risk-validation, blockchain |
| `/v1/compliance/*` | Admin token + JWT + Role | breach-notify |
| `/v1/admin/*` | Admin token + JWT + Role | AIML bias audit |
| `/v1/*` | JWT Auth + PQC (if enforced) | All user endpoints |
| `/metrics` | None (always) | Prometheus metrics |

**Key Endpoints:**

```
POST   /v1/auth/register                    # User registration
POST   /v1/auth/login/challenge             # Initiate login
POST   /v1/auth/login/verify                # Verify login
POST   /v1/auth/refresh                     # Refresh token
POST   /v1/transactions/initiate            # Initiate payment
POST   /v1/transactions/{id}/override       # Override blocked tx
POST   /v1/security/scan-sms                # SMS fraud scan
POST   /v1/security/emergency-freeze        # Freeze transactions
POST   /v1/security/unfreeze                # Unfreeze transactions
GET    /v1/dashboard                        # Financial dashboard
GET    /v1/transactions                     # List transactions
POST   /v1/goals                            # Create goal
GET    /v1/portfolio/summary                # Portfolio overview
POST   /v1/simulation/run                   # Monte Carlo sim
GET    /v1/aiml/behaviour/predict           # Behaviour prediction
GET    /v1/aiml/investments/recommend       # Investment recommendations
POST   /v1/internal/risk-validation         # Python RAG callback
GET    /metrics                             # Prometheus (always)
```

---

### Middleware Details

#### Auth Middleware (`middleware/auth.go`)

```go
func Auth(verify JWTVerify, authenticator Authenticator, roleFetcher RoleFetcher) func(http.Handler) http.Handler
```

- JWT first (structured errors: `token_expired`, `device_mismatch`, `token_revoked`)
- Legacy opaque token fallback for backward compatibility

#### Rate Limiting (`middleware/rate_limit.go`)

- In-memory token bucket per IP
- Global: 240 req/min
- Auth endpoints: 20 req/min
- Registration: 10 req/min
- Key: client IP (X-Forwarded-For aware)

#### Distributed Rate Limit (`middleware/distributed_rate_limit.go`)

- Redis-backed INCR with TTL (sliding window)
- Headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After`

#### Certificate Pinning (`middleware/cert_pinning.go`)

- `CERT_PINS` env: comma-separated SHA256 fingerprints
- mTLS preferred (`r.TLS.PeerCertificates`)
- Header fallback: `X-Cert-Fingerprint`
- `FINIX_PIN_ENFORCE=true` rejects missing fingerprint

#### Cooling Off (`middleware/cooling_off.go`)

- `EnforceCoolingOff(txIDParam, checker)`
- Returns 423 Locked with `Retry-After` header

#### Idempotency (`middleware/idempotency.go`)

- `RequireIdempotency(paths...)`
- `Idempotency-Key` header
- Redis/DB storage with 24h TTL
- 10,000 entry cap

---

## 🔐 Security & Cryptography

### Key Management

| Key | Purpose | Rotation |
|-----|---------|----------|
| `FINIX_JWT_SECRET` | HS256 signing | Manual (env) |
| `FINIX_INTERNAL_TOKEN` | Service-to-service auth | Manual |
| `FINIX_AADHAAR_HASH_KEY` | Aadhaar hashing | Manual |
| `FINIX_DATA_ENCRYPTION_KEY` | Field encryption | Manual |
| Kyber-1024 | PQC KEM | Daily cron |
| Dilithium5 | Transaction signatures | Daily cron |
| Vault keys | Field encryption | Vault rotation |

### Encryption Matrix

| Data | Method | When |
|------|--------|------|
| PII fields (phone, email, PAN, Aadhaar) | AES-256-GCM via Vault | At rest + transit |
| Passwords/PINs | Argon2id (memory-hard) | Storage |
| Transit | TLS 1.3 | Always |
| mTLS | Kyber-1024 + Dilithium5 | Session setup |
| At rest (fields) | Vault transit + AES-256 | Storage |
| JWT tokens | HS256 | Issuance |

---

## 🔄 Data Flow Diagrams

### 1. Transaction Initiation Flow

```
Client -> Chi Router -> Middleware Stack -> Platform.InitiateTransaction()
  -> RiskEngine.Evaluate()  ->  ONNX or Heuristic
  -> AdaptiveAuth.decide()  ->  Step-up if needed
  -> Transaction created    ->  status=blocked|success
  -> Dilithium signature    ->  Tamper-proof
  -> Fabric event           ->  Immutable ledger
  -> Outbox.enqueue()       ->  "risk.decision" event
  -> Account debit (if success)
  -> Response to Client

Background (500ms ticker):
  Outbox worker -> Redis Stream (XADD) -> Python RAG consumes
  Python RAG -> ValidateRisk() -> Go POST /v1/internal/risk-validation
  Platform applies verdict -> Persists to Postgres
```

### 2. RAG Query Flow

```
Client -> Chi Router -> Platform.Chat()
  -> QueryEngine.query()
    -> OPA authorization check
    -> Guardrails.validate_input()
    -> Query expansion (domain synonyms)
    -> PII mask query
    -> Hybrid Retriever.retrieve()
      -> Qdrant dense + BM25
      -> RRF fusion
    -> Auth-level filter
    -> Reranker.rerank() (BGE-Reranker-v2-M3)
      -> Top 5 documents
    -> HMAC integrity check
    -> Vault decrypt sensitive metadata
    -> Groq API (SYSTEM_PROMPT + context)
    -> PII mask answer
    -> Guardrails.validate_output()
    -> Confidence calculation
    -> Audit log
  -> Response to Client
```

### 3. Risk Validation Callback (Python -> Go)

```
Python RAG -> POST /v1/internal/risk-validation
  (X-Internal-Token header for auth)
  -> Platform.ValidateRisk(txID, req)
    -> Lock transaction row
    -> Check cooling-off window
    -> Apply verdict (block/step_up/dismiss)
    -> Update risk_level if suggested
    -> Enqueue "risk_validation_applied" event
    -> Persist to Postgres (async callback)
  -> 200 OK {applied, new_status}
```

---

## ⚙️ Configuration & Deployment

### Config Structure (`internal/config/config.go`)

```go
type Config struct {
    Server    ServerConfig
    Postgres  PostgresConfig
    Groq      GroqConfig
    Models    ModelsConfig
    AI        AIConfig
    Log       LogConfig
    Fabric    FabricConfig
    Providers ProvidersConfig
    Redis     RedisConfig      // Sprint 1 addition
}

type AIConfig struct {
    Provider      string // "local" | "remote"
    RagBaseURL    string // http://finix-rag:8000
    InternalToken string // FINIX_INTERNAL_TOKEN
    ReadOnlyDSN   string // RAG_PG_DSN
}
```

### Required Environment Variables

```bash
# Core (REQUIRED)
FINIX_JWT_SECRET=<32+ chars>
FINIX_INTERNAL_TOKEN=<service token>
FINIX_AADHAAR_HASH_KEY=<32+ chars>
FINIX_DATA_ENCRYPTION_KEY=<32+ chars>
GROQ_API_KEY=<api_key>

# Database
PGHOST=localhost
PGPORT=5432
PGDATABASE=finix
PGUSER=finix
PGPASSWORD=<password>

# Redis
REDIS_URL=redis://localhost:6379
REDIS_PASSWORD=<optional>

# AI/RAG
AI_PROVIDER=remote|local
AIML_UPSTREAM_URL=http://finix-rag:8000
RAG_PG_DSN=postgres://finix_rag_ro:***@replica:5432/finix

# Fabric (optional)
FABRIC_ENABLED=true
FABRIC_PEER_ENDPOINT=localhost:7051

# Security
CERT_PINS=sha256/AAAA...,sha256/BBBB...
FINIX_PIN_ENFORCE=true
FINIX_REVOCATION_FAIL_CLOSED=true
FINIX_ENV=production
```

### Production Startup Validation (`config.go:Validate()`)

1. `FINIX_JWT_SECRET` set (>= 16 chars)
2. `FINIX_INTERNAL_TOKEN` set
3. `AIML_UPSTREAM_URL` set if `AI_PROVIDER=remote`
4. `FINIX_REVOCATION_FAIL_CLOSED=true` (warn if not)
5. `CERT_PINS` set (warn if not)
6. `FINIX_PIN_ENFORCE=true` (warn if not)
7. `FINIX_REVOCATION_FAIL_CLOSED=true` for production

---

## 🚀 Deployment

### Docker Compose (Local)

```yaml
services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: finix
      POSTGRES_USER: finix
      POSTGRES_PASSWORD: ${PGPASSWORD}
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

  qdrant:
    image: qdrant/qdrant:v1.7
    ports: ["6333:6333", "6334:6334"]
    volumes: ["qdrant_data:/qdrant/storage"]

  finix-rag:
    build: ./finix-rag
    ports: ["8000:8000"]
    environment:
      QDRANT_HOST: qdrant
      GROQ_API_KEY: ${GROQ_API_KEY}
    depends_on: [qdrant]

  backend:
    build: ./backend
    ports: ["8080:8080"]
    environment:
      PGHOST: postgres
      REDIS_URL: redis://redis:6379
      AIML_UPSTREAM_URL: http://finix-rag:8000
    depends_on: [postgres, redis, finix-rag]
```

### Build & Run

```bash
# Go Backend
cd backend
go build ./cmd/server/
./server

# Python RAG
cd finix-rag
pip install -r requirements.txt
uvicorn finix_rag.api:app --host 0.0.0.0 --port 8000

# Or with Docker
docker-compose up --build
```

### Migrations

```bash
# Auto-run on startup (FINIX_MIGRATIONS_DIR defaults to ./migrations)
# Manual:
go run ./cmd/migrate -dir migrations -dsn "postgres://..."
```

---

## 🧪 Testing & Quality

### Run All Tests

```bash
# Go Backend
cd backend
go test ./... -v

# Specific packages
go test ./internal/domain/platform -run TestOutbox -v
go test ./internal/domain/transaction -run TestRiskEngine -v
go test ./internal/infra/repo -run TestTransactionRisk -v

# Python RAG
cd finix-rag
python -m py_compile src/finix_rag/**/*.py
python -m pytest tests/ -v

# Lint
cd backend
golangci-lint run
```

### Key Test Files

| Package | Test File | Coverage |
|---------|-----------|----------|
| `internal/domain/platform` | `outbox_test.go` | Outbox retry/purge |
| `internal/domain/transaction` | `risk_engine_test.go` | Low/High risk |
| `internal/domain/security` | `jwt_test.go` | JWT issuance/validation |
| `internal/infra/bank` | `mock_test.go` | Mock bank adapter |
| `internal/infra/fabric` | `ledger_test.go` | Fabric integration |

---

## 📊 Performance Characteristics

| Component | P50 Latency | P99 Latency | Throughput |
|-----------|-------------|-------------|------------|
| Embedding (BGE-M3) | ~15ms | ~40ms | ~50/s |
| Dense Retrieval | ~10ms | ~30ms | ~200/s |
| BM25 | ~5ms | ~15ms | ~500/s |
| Reranker (CPU) | ~80ms | ~200ms | ~10/s |
| Reranker (GPU) | ~20ms | ~50ms | ~50/s |
| Groq LLM | ~500ms | ~2000ms | 30 RPM |
| Risk Engine | ~2ms | ~5ms | ~1000/s |
| JWT Verify | ~1ms | ~3ms | ~5000/s |

### Rate Limits

| Endpoint | Limit | Window |
|----------|-------|--------|
| Global | 240 | minute |
| Auth endpoints | 20 | minute |
| Registration | 10 | minute |
| Transaction initiate | 60 | minute |

---

## 🔒 Security Checklist

- [ ] `FINIX_JWT_SECRET` >= 32 chars, rotated quarterly
- [ ] `FINIX_INTERNAL_TOKEN` rotated monthly
- [ ] `CERT_PINS` configured, `FINIX_PIN_ENFORCE=true`
- [ ] `FINIX_REVOCATION_FAIL_CLOSED=true`
- [ ] `FINIX_ENV=production`
- [ ] Redis TLS enabled
- [ ] Qdrant TLS enabled
- [ ] Postgres TLS enabled
- [ ] Vault TLS enabled
- [ ] Prometheus scraping `/metrics`
- [ ] Alerting: circuit breaker open, outbox lag > 100, error rate > 1%
- [ ] Log aggregation (structured JSON logs)
- [ ] Backup/restore tested for Postgres + Qdrant

---

## 📈 Monitoring Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /metrics` | Prometheus metrics (always available) |
| `GET /healthz` | Basic health check |
| `GET /readyz` | Readiness (DB, PQC, Dilithium) |
| `GET /v1/system/health-deep` | Deep health (dev only) |
| `GET /v1/system/metrics` | Legacy metrics (dev only) |
| `GET /v1/system/openapi.json` | OpenAPI spec (dev only) |

---

## 📚 Key Files Quick Reference

| Area | File | Purpose |
|------|------|---------|
| Entry | `cmd/server/main.go` | Composition root |
| Config | `internal/config/config.go` | All configuration |
| Core | `internal/domain/platform/service.go` | Main service orchestrator |
| Risk | `internal/domain/transaction/risk_engine.go` | Risk evaluation engine |
| Risk | `internal/domain/transaction/heuristic.go` | Heuristic scoring |
| Risk | `internal/domain/transaction/onnx_predictor.go` | ONNX inference |
| Fraud | `internal/domain/fraud/gnn.go` | GNN mule detection |
| Health | `internal/domain/healthscore/calculator.go` | Health score |
| Security | `internal/domain/security/jwt.go` | JWT management |
| Security | `internal/domain/security/pqc.go` | Kyber-1024 PQC |
| Security | `internal/domain/security/dilithium.go` | Dilithium5 signatures |
| Security | `internal/domain/security/adaptive_auth.go` | 5-tier auth |
| Infra | `internal/infra/db/postgres.go` | PG connection + migrations |
| Infra | `internal/infra/repo/*.go` | All repositories |
| Infra | `internal/infra/ai/rag_client.go` | RAG client |
| API | `internal/api/router.go` | HTTP router |
| API | `internal/api/middleware/*.go` | All middleware |
| Python | `finix-rag/src/finix_rag/api.py` | FastAPI app |
| Python | `finix-rag/src/finix_rag/query/secure_query_engine.py` | RAG pipeline |
| Python | `finix-rag/src/finix_rag/query/reranker.py` | BGE-Reranker |
| Python | `finix-rag/src/finix_rag/guardrails/__init__.py` | Guardrails |
| Config | `migrations/*.sql` | 27 SQL migrations |

---

## 🎯 Quick Start for New Developers

```bash
# 1. Clone & setup
git clone <repo>
cd FINIX_PRODUCTION/backend
cp .env.example .env  # Fill in secrets

# 2. Start dependencies
docker-compose up -d postgres redis qdrant

# 3. Run backend
go build ./cmd/server/
./server

# 4. In another terminal - Python RAG
cd ../finix-rag
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt
uvicorn finix_rag.api:app --reload --port 8000

# 5. Test
curl -X POST http://localhost:8080/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Test","mobile":"+919876543210","deviceIdFingerprint":"dev-1"}'
```

---

## 📝 Version History

| Version | Date | Changes |
|---------|------|---------|
| 0.2.0-production | 2024-07-25 | Sprint 1 complete: Outbox + Redis, Validation endpoint, Distributed rate limit, Config validation, Circuit breaker, Cert pinning, JWT fail-closed, Prometheus metrics, Reranker thread-safety, Guardrails production mode, Startup validation |

---

*Generated from FINIX backend codebase — D:/FINIX_PRODUCTION/backend*
