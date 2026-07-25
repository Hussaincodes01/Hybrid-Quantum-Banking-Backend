# FINIX - Secure Banking AI Platform

> **Production-ready secure banking AI platform with RAG-powered financial assistant, risk validation, and distributed architecture.**

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://golang.org)
[![Python Version](https://img.shields.io/badge/Python-3.11+-3776AB?logo=python)](https://python.org)
[![License](https://img.shields.io/badge/License-Proprietary-red)](#license)

---

## 🏗️ Architecture Overview

```mermaid
graph TB
    subgraph "Frontend Layer"
        FE[Flutter/React Native App]
    end

    subgraph "API Gateway"
        GW[Chi Router + Middleware]
    end

    subgraph "Go Backend Services"
        AUTH[Auth Service<br/>JWT + Biometric + PQC]
        TXN[Transaction Service<br/>Risk Engine + Outbox]
        AI[AI Gateway<br/>RAG Proxy + Guardrails]
        LEDGER[Ledger Service<br/>Double-Entry + Fabric]
    end

    subgraph "Python RAG Service"
        INGEST[Secure Ingestor<br/>OCR → PII Mask → Embed]
        QUERY[Secure Query Engine<br/>Hybrid Retrieval → Rerank → LLM]
        GUARD[FINIX Guardrails<br/>NeMo + Programmatic]
    end

    subgraph "Data Layer"
        PG[(PostgreSQL<br/>ACID + Migrations)]
        QDRANT[(Qdrant<br/>Vector Store)]
        REDIS[(Redis<br/>Streams + Rate Limit)]
        FABRIC[(Hyperledger Fabric<br/>Immutable Ledger)]
        VAULT[(HashiCorp Vault<br/>Secrets + Encryption)]
    end

    FE --> GW
    GW --> AUTH
    GW --> TXN
    GW --> AI
    GW --> LEDGER
    AI --> INGEST
    AI --> QUERY
    AI --> GUARD
    TXN --> PG
    TXN --> REDIS
    TXN --> FABRIC
    INGEST --> QDRANT
    INGEST --> VAULT
    QUERY --> QDRANT
    QUERY --> VAULT
    LEDGER --> PG
    LEDGER --> FABRIC
```

### Architecture Diagrams

The following diagrams show the complete backend architecture with all modules, data flows, and interactions:

#### C. Module Dependency Graph
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
    end

    subgraph "Storage"
        PG[(PostgreSQL)]
        RED[(Redis)]
        FAB[(Fabric)]
        VAULT[(Vault)]
        ONNX[(ONNX Model)]
    end

    HTTP --> PS
    SCHEDULER --> PS
    OUTBOX --> PS
    PS --> AUTH
    PS --> TXN
    PS --> RISK
    RISK --> ONNX
    RISK --> FRAUD
    AUTH --> JWT
    AUTH --> PQC
    AUTH --> DILITHIUM
    TXN --> PG
    TXN --> FAB
    TXN --> VAULT
    FRAUD --> PG
    FRAUD --> RED
    PS --> PG
    PS --> RED
    PS --> FAB
    PS --> VAULT
```

#### D. Data Flow: Transaction Lifecycle
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

#### E. Data Flow: RAG Query Pipeline
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

## 📁 Repository Structure

```
FINIX_PRODUCTION/
├── backend/                     # Go Backend (124 files)
│   ├── cmd/server/              # Entry point
│   ├── internal/
│   │   ├── api/                 # HTTP handlers, middleware
│   │   ├── config/              # Configuration management
│   │   ├── domain/
│   │   │   ├── platform/        # Core business logic
│   │   │   ├── transaction/     # Risk engine, assessments
│   │   │   ├── security/        # JWT, PQC, Dilithium, biometrics
│   │   │   ├── fraud/           # GNN fraud graph
│   │   │   ├── healthscore/     # Financial health calculator
│   │   │   ├── credit/          # Credit optimization
│   │   │   └── debt/            # Debt optimization
│   │   ├── infra/
│   │   │   ├── ai/              # RAG client (Go→Python)
│   │   │   ├── db/              # Postgres repository
│   │   │   ├── fabric/          # Hyperledger Fabric
│   │   │   └── repo/            # SQL repositories
│   ├── migrations/              # 27 SQL migrations
│   ├── go.mod / go.sum
│   └── server.exe               # Pre-built binary
│
├── finix-rag/                   # Python RAG Service
│   ├── src/finix_rag/
│   │   ├── api.py               # FastAPI endpoints
│   │   ├── query/
│   │   │   ├── secure_query_engine.py   # Main RAG pipeline
│   │   │   ├── hybrid_retriever.py      # BM25 + Dense + RRF
│   │   │   ├── reranker.py              # BGE-Reranker-v2-M3
│   │   │   └── secure_query_engine.py   # Guardrails integration
│   │   ├── ingestion/
│   │   │   ├── secure_ingestor.py       # OCR → PII → Embed
│   │   │   ├── document_processor.py    # Semantic chunking
│   │   │   ├── ocr_engine.py            # PaddleOCR
│   │   │   └── text_extractor.py        # Native PDF extraction
│   │   ├── guardrails/        # NeMo + Programmatic fallback
│   │   ├── security/          # Auth, PII, Vault, OPA
│   │   └── embedding_config.py          # BGE-M3 config
│   ├── config/                # OPA, Qdrant, Vault, Guardrails configs
│   ├── requirements.txt
│   └── docker-compose.yml
│
└── docs/                      # Architecture docs
    ├── AI_ECOSYSTEM_ARCHITECTURE.md
    ├── AIML_MIGRATION_PLAN.md
    ├── RUNBOOK.md
    └── SETUP_AND_TEST.md
```

---

## 🔐 Repository Rules & Governance

### 👑 Access Control

| Role | Permissions | Users |
|------|-------------|-------|
| **Owner** | Full write access, merge to main | **@Akshit Kumar** |
| **Maintainer** | Write to feature branches, create PRs | **@Akshit Kumar** |
| **Collaborator** | Read-only, assigned modules only | Assigned team members |
| **External** | No access | N/A |

### 📋 Branch & PR Rules

1. **Only @Akshit Kumar** has rights to change code directly
2. **All changes** must be pushed to separate feature branches with proper PR descriptions
3. **Rest of collaborators** can access code to understand assigned parts — **NO direct changes allowed**
4. **PRs from unauthorized collaborators** will be removed immediately
5. **Never modify main directly** — copy project, make changes in feature branch, then PR

### 🔀 Branch Naming Convention

```
feature/<module>-<short-description>    # New features
fix/<module>-<issue-number>             # Bug fixes
security/<module>-<vulnerability>       # Security patches
refactor/<module>-<scope>               # Code improvements
docs/<module>-<update>                  # Documentation
```

### ✅ PR Requirements

- [ ] Descriptive title following convention
- [ ] Detailed description of changes
- [ ] Linked issue/ticket
- [ ] All tests pass (`go test ./...` + `py_compile`)
- [ ] No lint errors (`golangci-lint run`)
- [ ] Security review for auth/payment/risk changes
- [ ] Documentation updated if API changes

---

## 🌐 API Reference

### Base URLs

| Environment | Go Backend | Python RAG |
|-------------|------------|------------|
| **Local** | `http://localhost:8080` | `http://localhost:8000` |
| **Staging** | `https://staging-api.finix.app` | `https://staging-rag.finix.app` |
| **Production** | `https://api.finix.app` | `https://rag.finix.app` |

### Authentication

| Method | Header | Description |
|--------|--------|-------------|
| **JWT** | `Authorization: Bearer <token>` | User authentication (15-min TTL) |
| **Device Fingerprint** | `X-Device-Fingerprint: <hash>` | Device binding |
| **Internal Service** | `X-Internal-Token: <token>` | Service-to-service (RAG validation) |
| **Certificate Pinning** | `X-Cert-Fingerprint: <sha256>` | mTLS pinning (production) |

---

### 🔑 Auth Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/auth/register` | Register new user |
| `POST` | `/v1/auth/login/challenge` | Initiate login (returns challenge) |
| `POST` | `/v1/auth/login/verify` | Verify login (PIN + biometric) |
| `POST` | `/v1/auth/refresh` | Refresh access token |
| `POST` | `/v1/auth/stepup/challenge` | Step-up authentication challenge |
| `POST` | `/v1/auth/otp/generate` | Generate OTP |

---

### 💳 Transaction Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/transactions/initiate` | Initiate payment (triggers risk engine) |
| `GET` | `/v1/transactions` | List user transactions |
| `GET` | `/v1/transactions/{id}` | Get transaction details |
| `POST` | `/v1/transactions/{id}/override` | Override blocked transaction (biometric + OTP) |
| `POST` | `/v1/transactions/{id}/verify-receipt` | Verify payment receipt |

**Risk Response Fields:**
```json
{
  "transaction_id": "txn_abc123",
  "status": "success|blocked|warning_ack_required",
  "risk_level": "low|medium|high",
  "risk_score": 42.5,
  "xai_reason": "Amount 3.2x your average; new recipient",
  "cooling_off_until": "2024-01-15T14:30:00Z",
  "step_up_required": true,
  "required_tier": "tier_3",
  "require_biometric": true,
  "require_otp": true,
  "dilithium_signature": {...}
}
```

---

### 🛡️ Security Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/security/scan-sms` | Scan SMS for fraud/phishing |
| `POST` | `/v1/security/emergency-freeze` | Freeze all outgoing transactions |
| `POST` | `/v1/security/unfreeze` | Unfreeze (biometric + OTP) |
| `POST` | `/v1/security/report-fraud` | Report fraudulent transaction |
| `GET` | `/v1/security/health` | Security health check |

**SMS Scan Response:**
```json
{
  "badge": "green|amber|red",
  "reason": "Sender and template match known safe banking patterns.",
  "confidence": 0.95,
  "indicators": ["short_url", "otp_solicitation"],
  "device_trust_score": 0.85
}
```

---

### 🤖 AI/RAG Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/query` | Main RAG query with full security pipeline |
| `POST` | `/ingest` | Ingest document (admin/analyst only) |
| `GET` | `/health` | Service health check |
| `GET` | `/audit/logs` | Audit logs (admin only) |
| `POST` | `/admin/rotate-keys` | Rotate encryption keys (admin) |

**Query Request:**
```json
{
  "question": "What's the current NPA ratio for SBI?",
  "max_sources": 5
}
```

**Query Response:**
```json
{
  "answer": "As of Q3 2024, SBI's Gross NPA ratio stands at 2.36%... [Source 1]",
  "citations": [
    {"source": "SBI Q3 2024 Investor Presentation", "doc_id": "doc_123", "relevance_score": 0.92}
  ],
  "confidence": 0.94,
  "classification": "internal",
  "pii_masked": true,
  "authorization_checked": true,
  "retrieval_stats": {
    "total_retrieved": 20,
    "after_auth_filter": 8,
    "after_rerank": 5,
    "query_expanded": true
  }
}
```

---

### 🏦 Account & Profile Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/accounts` | List linked accounts |
| `POST` | `/v1/accounts/link` | Link new bank account |
| `GET` | `/v1/dashboard` | Financial dashboard summary |
| `GET` | `/v1/auth/profile` | User profile |
| `GET` | `/v1/kyc/profile` | KYC profile |

---

### 📊 Portfolio & Goals

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/goals` | Create financial goal |
| `GET` | `/v1/goals` | List goals |
| `POST` | `/v1/goals/{id}/contribute` | Add to goal |
| `GET` | `/v1/portfolio/summary` | Portfolio overview |
| `GET` | `/v1/portfolio/investments` | Investment holdings |
| `POST` | `/v1/simulation/run` | Monte Carlo simulation |

---

### 🧠 AI/ML Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/aiml/behaviour/predict` | Behaviour prediction |
| `GET` | `/v1/aiml/investments/recommend` | Investment recommendations |
| `POST` | `/v1/aiml/synthetic/generate` | Generate synthetic data |
| `GET` | `/v1/aiml/bank/readiness` | Bank API readiness check |

---

### 🔍 Internal Validation (Service-to-Service)

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/v1/internal/risk-validation` | `X-Internal-Token` | Python RAG validates ML risk decision |

**Request:**
```json
{
  "tx_id": "txn_abc123",
  "agrees_with_ml": false,
  "suggested_level": "high",
  "confidence": 0.92,
  "rationale": "ML missed velocity anomaly",
  "recommended_action": "step_up",
  "citations": ["Velocity pattern doc_45"],
  "xai_explanation": "User made 5 transactions in 10 minutes..."
}
```

**Response:**
```json
{
  "applied": true,
  "tx_id": "txn_abc123",
  "new_status": "warning_ack_required",
  "old_status": "success",
  "message": "Risk validation applied successfully"
}
```

---

## ⚙️ Configuration

### Required Environment Variables

```bash
# Go Backend
FINIX_JWT_SECRET=<32+ char secret>          # REQUIRED
FINIX_INTERNAL_TOKEN=<service token>         # REQUIRED
FINIX_JWT_SECRET=<32+ chars>
FINIX_AADHAAR_HASH_KEY=<32+ chars>
FINIX_DATA_ENCRYPTION_KEY=<32+ chars>
GROQ_API_KEY=<api_key>                       # For chatbot
GROQ_MODEL=llama-3.1-8b-instant

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

---

## 🚀 Deployment

### Docker Compose (Local)

```yaml
# docker-compose.yml
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
      - QDRANT_HOST=qdrant
      - GROQ_API_KEY=${GROQ_API_KEY}
    depends_on: [qdrant]

  backend:
    build: ./backend
    ports: ["8080:8080"]
    environment:
      - PGHOST=postgres
      - REDIS_URL=redis://redis:6379
      - AIML_UPSTREAM_URL=http://finix-rag:8000
    depends_on: [postgres, redis, finix-rag]
```

### Production Checklist

- [ ] All required env vars set
- [ ] `FINIX_ENV=production`
- [ ] `FINIX_REVOCATION_FAIL_CLOSED=true`
- [ ] `CERT_PINS` configured
- [ ] `FINIX_PIN_ENFORCE=true`
- [ ] Postgres read-replica for RAG (`RAG_PG_DSN`)
- [ ] Redis TLS enabled
- [ ] Qdrant TLS enabled
- [ ] Vault configured for secrets
- [ ] Prometheus scraping `/metrics`
- [ ] Alerting on: circuit breaker open, outbox lag, error rate > 1%

---

## 📊 Monitoring & Observability

### Metrics Endpoint
```
GET /metrics                    # Prometheus format (always available)
GET /v1/system/health-deep      # Detailed health (dev only)
GET /v1/system/metrics          # Legacy (dev only)
```

### Key Metrics
| Metric | Type | Description |
|--------|------|-------------|
| `finix_requests_total` | Counter | Requests by route/method/status |
| `finix_request_duration_seconds` | Histogram | Latency by route |
| `finix_active_users` | Gauge | Active sessions |
| `finix_transactions_total` | Counter | Total transactions |
| `finix_outbox_pending` | Gauge | Unpublished events |
| `finix_fraud_graph_nodes` | Gauge | Graph size |
| `finix_circuit_breaker_state` | Gauge | 0=closed, 1=half-open, 2=open |

---

## 🧪 Testing

```bash
# Go Backend
cd backend
go test ./... -v              # All tests
go test ./internal/domain/platform -run TestOutbox -v

# Python RAG
cd finix-rag
python -m py_compile src/finix_rag/**/*.py
python -m pytest tests/ -v

# Lint
cd backend
golangci-lint run
```

---

## 📜 License

**Proprietary** — All rights reserved. Unauthorized copying, distribution, or modification is strictly prohibited.

---

## 👥 Contributors

| Role | Name | Access |
|------|------|--------|
| **Owner** | **@Hussaincodes01** | Full write access |
| **Maintainer** | **@Hussaincodes01** | Feature branches, PRs |
| **Security Review** | Assigned team | Auth/Payment/Risk modules |

---

## 🚀 Quick Start (Local Development)

### Prerequisites
- **Go 1.21+**
- **Python 3.11+**
- **Docker & Docker Compose** (for PostgreSQL, Redis, Qdrant, Vault)
- **OpenSSL** (for key generation)

### Option 1: Automated Setup (Recommended)

```bash
# 1. Clone and enter project
git clone <your-repo-url>
cd FINIX_PRODUCTION

# 2. Run automated setup script (Linux/macOS/Git Bash)
chmod +x scripts/start-local.sh
./scripts/start-local.sh

# OR Windows PowerShell/CMD
scripts\start-local.bat

# 3. Add your GROQ_API_KEY to .env (required for AI features)
# Edit .env and set: GROQ_API_KEY=your_key_from_console.groq.com

# 4. Start Go Backend (Terminal 1)
cd backend
go run ./cmd/server

# 5. Start Python RAG (Terminal 2)
cd finix-rag
python -m venv venv
source venv/bin/activate  # Linux/macOS/Git Bash
# venv\Scripts\activate    # Windows CMD
pip install -r requirements.txt
uvicorn finix_rag.api:app --reload --port 8000

# 6. Verify
curl http://localhost:8080/healthz
curl http://localhost:8000/health
```

### Option 2: Docker Compose (Full Stack)

```bash
# 1. Configure .env
cp .env.example .env
# Edit .env with your keys (GROQ_API_KEY, etc.)

# 2. Start all services
docker-compose --env-file .env up -d

# 3. View logs
docker-compose logs -f backend finix-rag

# 4. Stop
docker-compose down
```

### Option 3: Manual Step-by-Step

<details>
<summary>Click to expand manual setup</summary>

```bash
# 1. Start infrastructure only
docker-compose --env-file .env up -d postgres redis qdrant vault opa

# 2. Initialize Vault (first time only)
docker exec finix-vault vault secrets enable transit
docker exec finix-vault vault write -f transit/keys/finix-pii type=aes256-gcm96
docker exec finix-vault vault write -f transit/keys/finix-data type=aes256-gcm96

# 3. Configure .env
cp .env.example .env
# Generate secure keys:
# FINIX_JWT_SECRET=$(openssl rand -base64 48)
# FINIX_INTERNAL_TOKEN=$(openssl rand -base64 32)
# FINIX_AADHAAR_HASH_KEY=$(openssl rand -base64 32)
# FINIX_DATA_ENCRYPTION_KEY=$(openssl rand -base64 32)
# VAULT_TOKEN=$(openssl rand -base64 32)
# PGPASSWORD=$(openssl rand -base64 24)

# 4. Build & run Go backend
cd backend
go build -o finix-server ./cmd/server
./finix-server

# 5. Build & run Python RAG (separate terminal)
cd finix-rag
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt
uvicorn finix_rag.api:app --host 0.0.0.0 --port 8000 --reload
```
</details>

---

### Verify Installation

| Service | URL | Expected |
|---------|-----|----------|
| Go Backend Health | `http://localhost:8080/healthz` | `{"status":"ok"}` |
| Python RAG Health | `http://localhost:8000/health` | `{"status":"healthy",...}` |
| Prometheus | `http://localhost:9090` | Web UI |
| Grafana | `http://localhost:3000` | admin/admin |

### Test API Call

```bash
# Register user
curl -X POST http://localhost:8080/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Test User","mobile":"+919876543210","deviceIdFingerprint":"dev-123"}'

# Query RAG (requires auth token from login)
curl -X POST http://localhost:8000/query \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"question":"What is the current repo rate?","max_sources":3}'
```

---

> **Last Updated**: 2024-07-25  
> **Version**: 0.2.0-production  
> **Compatibility**: Go 1.21+, Python 3.11+, PostgreSQL 15+, Qdrant 1.7+, Redis 7+
> **Version**: 0.2.0-production  
> **Compatibility**: Go 1.21+, Python 3.11+, PostgreSQL 15+, Qdrant 1.7+, Redis 7+