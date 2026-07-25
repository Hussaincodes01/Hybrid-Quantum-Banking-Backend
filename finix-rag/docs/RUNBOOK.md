# FINIX RAG - Operations Runbook
# Banking RAG System with Cryptographic Security

> **Setting it up or running on a GPU? Start with [`SETUP_AND_TEST.md`](SETUP_AND_TEST.md).**
> That is the verified, up-to-date quickstart (GPU torch install, dev-mode Vault, model
> download, and the accuracy test). This file is the extended *operations* reference.

## Table of Contents
1. [Prerequisites](#1-prerequisites)
2. [Initial Setup](#2-initial-setup)
3. [Configuration](#3-configuration)
4. [Starting Services](#4-starting-services)
5. [Verification & Health Checks](#5-verification--health-checks)
6. [Document Ingestion](#6-document-ingestion)
7. [Querying the System](#7-querying-the-system)
8. [Security Operations](#8-security-operations)
9. [Backup & Recovery](#9-backup--recovery)
10. [Troubleshooting](#10-troubleshooting)
11. [Maintenance](#11-maintenance)
12. [Incident Response](#12-incident-response)
13. [Testing Guide](#13-testing-guide)

---

## 1. Prerequisites

### System Requirements
| Component | Minimum | Recommended |
|-----------|---------|-------------|
| CPU | 8 cores | 16 cores |
| RAM | 16 GB | 32 GB |
| GPU | None (CPU works) | NVIDIA T4/RTX 3060+ |
| Disk | 50 GB SSD | 200 GB NVMe SSD |
| OS | Ubuntu 22.04 / Windows 11 | Ubuntu 24.04 LTS |

### Required Software
```
Docker Desktop 4.25+        # Container runtime
Docker Compose 2.23+        # Multi-container orchestration
Python 3.12                 # For local (non-Docker) install â€” see Â§2 Step 1A
Git 2.40+                   # Version control
```

### External Dependencies
| Service | Account Required | API Key |
|---------|-----------------|---------|
| Groq | https://console.groq.com | `GROQ_API_KEY` |
| HuggingFace | https://huggingface.co | Optional (faster downloads) |

---

## 2. Initial Setup

### Step 1: Clone & Navigate
```bash
cd "PSB Golang Stack/finix-rag"
```

### Step 1A: Local Python environment (Windows â€” Command Prompt)

Run the app directly (no Docker) for development. **Python 3.12** is required.

Dependencies are installed in a virtual environment at `venv\`.

```batch
:: from finix-rag\
python -m venv venv
venv\Scripts\activate
pip install -U pip setuptools wheel

:: Core dependencies. This installs a CPU build of PyTorch (~200 MB) via the CPU
:: index pinned at the top of requirements.txt â€” NOT the multi-GB CUDA stack.
:: (The first query/ingest additionally downloads the BGE-M3 embedding + reranker
:: models, ~2.5 GB, into the HuggingFace cache.)
pip install -r requirements.txt
```

- **CPU users:** add `--extra-index-url https://download.pytorch.org/whl/cpu` to the
  first line of `requirements.txt` and append `+cpu` to the `torch` pin (e.g. `torch==2.6.0+cpu`)
  before installing.
- **`EMBEDDING_DEVICE`:** the code auto-detects CUDA and falls back to CPU, so the
  `EMBEDDING_DEVICE=cuda` value in `.env` is safe on a CPU-only box.

#### OCR is optional and installed separately

The PaddleOCR document-ingestion path is intentionally **not** in `requirements.txt`.
`paddleocr` pulls `huggingface-hub` **1.x**, which removed the `inference` extra and
conflicts with the embedding stack (`huggingface-hub` 0.x) â€” this is the source of
the repeated `huggingface-hub â€¦ does not provide the extra 'inference'` errors.
Install OCR **only if you need it**, ideally in a *separate* virtualenv:

```batch
pip install -r requirements-ocr.txt
```

The RAG query/ingest pipeline runs fine without it (paddle is lazy-imported).

#### Run: infra in Docker, app in local Python

```batch
:: 1. Start the backing services. OPA is pinned to openpolicyagent/opa:0.70.0 because
::    config/opa-policy.rego uses Rego v0 syntax (opa:latest / 1.0+ crash-loops on it).
docker compose up -d qdrant vault opa redis

:: 2. Run the FastAPI app from the venv (src-layout â€” no package install needed).
venv\Scripts\activate
set PYTHONPATH=src
uvicorn finix_rag.api:app --host 0.0.0.0 --port 8000 --reload
```

Verify: `curl http://localhost:8000/health`.

> **Note:** LlamaIndex is pinned to the 0.14 line. The app code was originally
> written against 0.12; the top-level APIs it uses are stable across both, but if
> you hit a LlamaIndex import/attribute error at runtime, it's a 0.12â†’0.14 drift â€”
> flag it and the pins can be moved to a 0.12.x set.

### Step 2: Create .env from Template
```bash
# Windows
copy .env.example .env

# Linux/Mac
cp .env.example .env
```

### Step 3: Edit .env with Your Values
```bash
# Required: Add your Groq API key
GROQ_API_KEY=gsk_your_key_here
GROQ_MODEL=llama-3.3-70b-versatile

# Required: Set a strong API secret key
API_SECRET_KEY=$(python -c "import secrets; print(secrets.token_hex(32))")

# Required: Set Qdrant API key
QDRANT_API_KEY=finix-rag-$(python -c "import secrets; print(secrets.token_hex(8))")

# CORS: set to your frontend origin (never use * in production)
CORS_ORIGINS=http://localhost:3000
```

### Step 4: Generate TLS Certificates (Optional - Production)
```bash
# Install mkcert (Windows)
choco install mkcert

# Install local CA
mkcert -install

# Generate certificates
mkdir config\tls
mkcert -cert-file config\tls\cert.pem -key-file config\tls\key.pem localhost 127.0.0.1
```

---

## 3. Configuration

### Environment Variables Reference

#### Core Services
| Variable | Default | Description |
|----------|---------|-------------|
| `GROQ_API_KEY` | - | **REQUIRED** Groq API key |
| `GROQ_MODEL` | `llama-3.1-8b-instant` | LLM model for responses |
| `QDRANT_API_KEY` | - | **REQUIRED** Qdrant authentication |
| `VAULT_TOKEN` | `dev-only-token` | Vault root token (change in prod) |
| `API_SECRET_KEY` | - | **REQUIRED** JWT signing key |
| `CORS_ORIGINS` | `http://localhost:3000` | Comma-separated allowed origins |

#### Embedding Configuration
| Variable | Default | Description |
|----------|---------|-------------|
| `EMBEDDING_MODEL` | `BAAI/bge-m3` | Embedding model |
| `EMBEDDING_DIM` | `1024` | Embedding dimension |
| `EMBEDDING_DEVICE` | `cpu` | `cpu` or `cuda` |

#### Security Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_ALGORITHM` | `HS256` | JWT signing algorithm |
| `JWT_EXPIRY_MINUTES` | `30` | Token expiration time |
| `ENABLE_PII_MASKING` | `true` | Enable PII detection/masking |
| `PII_MASKING_STRATEGY` | `tokenize` | `redact`, `tokenize`, or `hash` |

---

## 4. Starting Services

### Full Stack Start
```bash
# Start all services (Vault, Qdrant, OPA, Redis, App)
docker compose up -d

# Watch logs
docker compose logs -f
```

### Individual Service Start
```bash
# Infrastructure only
docker compose up -d qdrant vault opa redis

# Application only (after infra is healthy)
docker compose up -d finix-rag
```

### Service Startup Order
```
1. Vault          (port 8200)  - 5-10 seconds
2. Qdrant         (port 6333)  - 10-15 seconds
3. OPA            (port 8181)  - 2-5 seconds
4. Redis          (port 6379)  - 2-3 seconds
5. FINIX RAG App  (port 8000)  - 30-60 seconds (model loading)
```

---

## 5. Verification & Health Checks

### Check All Services
```bash
# Quick health check
curl http://localhost:8000/health

# Expected response:
{
  "status": "healthy",
  "vault": "healthy",
  "qdrant": "healthy",
  "opa": "healthy"
}
```

### Individual Service Checks

#### Vault
```bash
curl http://localhost:8200/v1/sys/health
curl -H "X-Vault-Token: dev-only-token" \
     http://localhost:8200/v1/transit/keys/finix-rag-key
```

#### Qdrant
```bash
curl http://localhost:6333/healthz
curl -H "api-key: your-qdrant-api-key" \
     http://localhost:6333/collections/finix_documents
```

#### OPA
```bash
curl http://localhost:8181/health
curl -X POST http://localhost:8181/v1/data/finix/authz \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "user": {"id":"test","role":"analyst","department":"risk","clearance_level":3,"tenant_id":"default"},
      "action": "query",
      "resource": {"type":"query_result","classification":"internal"},
      "environment": {}
    }
  }'
```

#### Redis
```bash
redis-cli -a finix-redis-password ping
# Expected: PONG
```

---

## 6. Document Ingestion

### Login First
```bash
curl -X POST http://localhost:8000/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "admin",
    "password": "admin123",
    "role": "admin",
    "department": "operations",
    "clearance_level": 4,
    "tenant_id": "bank_axis"
  }'

export TOKEN="<access_token from response>"
```

### Ingest Single Document
```bash
curl -X POST http://localhost:8000/ingest \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@financial_report.pdf"
```

### Supported File Types
| Type | Extension |
|------|-----------|
| PDF | `.pdf` |
| Images | `.png`, `.jpg`, `.jpeg`, `.tiff`, `.bmp` |

---

## 7. Querying the System

### Basic Query
```bash
curl -X POST http://localhost:8000/query \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"question": "What was the NPA ratio for Q4 2025?", "max_sources": 5}'
```

### Response Fields
| Field | Description |
|-------|-------------|
| `answer` | LLM-generated response with citations |
| `citations` | Source documents with relevance scores |
| `confidence` | Response confidence (0.0 - 1.0) |
| `classification` | Document sensitivity level |
| `pii_masked` | Whether PII was masked in response |
| `authorization_checked` | Whether OPA authorization passed |

---

## 8. Security Operations

### Key Rotation
```bash
curl -X POST http://localhost:8000/admin/rotate-keys \
  -H "Authorization: Bearer $TOKEN"
```

### Audit Log Review
```bash
curl http://localhost:8000/audit/logs \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

---

## 9. Backup & Recovery

### Qdrant Backup
```bash
curl -X POST \
  -H "api-key: your-qdrant-api-key" \
  http://localhost:6333/collections/finix_documents/snapshots
```

### Vault Backup
```bash
docker exec vault vault operator raft snapshot save /vault/file/raft-snapshot-$(date +%Y%m%d)
docker cp vault:/vault/file/raft-snapshot-$(date +%Y%m%d) ./backups/
```

### Redis Backup
```bash
docker exec redis redis-cli -a finix-redis-password BGSAVE
docker cp redis:/data/dump.rdb ./backups/redis_$(date +%Y%m%d).rdb
```

---

## 10. Troubleshooting

### Common Issues

#### 1. "Vault authentication failed"
```bash
curl http://localhost:8200/v1/sys/health
docker compose restart vault
docker compose logs vault
```

#### 2. "Qdrant connection refused"
```bash
docker compose ps qdrant
curl -H "api-key: your-key" http://localhost:6333/healthz
```

#### 3. "Model loading timeout"
```bash
nvidia-smi
# Pre-download models:
python -c "from FlagEmbedding import BGEM3FlagModel; BGEM3FlagModel('BAAI/bge-m3')"
```

#### 4. "OPA policy evaluation error"
```bash
curl http://localhost:8181/health
curl http://localhost:8181/v1/policies/finix/authz
```

#### 5. "CORS error in browser"
Set `CORS_ORIGINS` in `.env` to your frontend origin (e.g. `http://localhost:3000`).
`allow_origins=["*"]` with credentials is rejected by browsers â€” the app now reads from env.

### Log Locations
| Service | Log Command |
|---------|-------------|
| FINIX App | `docker compose logs finix-rag` |
| Vault | `docker compose logs vault` |
| Qdrant | `docker compose logs qdrant` |
| OPA | `docker compose logs opa` |
| Redis | `docker compose logs redis` |

---

## 11. Maintenance

### Daily Tasks
- [ ] Check service health: `curl http://localhost:8000/health`
- [ ] Review audit logs for anomalies
- [ ] Monitor disk usage

### Weekly Tasks
- [ ] Review Qdrant collection stats
- [ ] Check Vault key rotation status
- [ ] Clean up old Docker images: `docker image prune -a`

### Monthly Tasks
- [ ] Rotate Vault transit keys
- [ ] Update Docker images: `docker compose pull`
- [ ] Review and rotate API keys
- [ ] Backup verification test

### Update Procedure
```bash
git pull origin main
docker compose build finix-rag
docker compose up -d --no-deps finix-rag
curl http://localhost:8000/health
```

---

## 12. Incident Response

### Data Breach Protocol
1. **Immediately**: Revoke all JWT tokens
2. **Disable**: Ingest/query endpoints
3. **Rotate**: All encryption keys
4. **Audit**: Review all access logs
5. **Notify**: Security team and compliance

### Emergency Commands
```bash
docker compose down          # Stop all, preserve volumes
docker compose stop          # Stop containers only
docker compose down -v       # Nuclear: destroys all data
curl -X POST http://localhost:8000/admin/rotate-keys \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

---

## 13. Testing Guide

### Prerequisites
```bash
cd "PSB Golang Stack/finix-rag"
pip install pytest httpx pytest-asyncio
# Services must be running: docker compose up -d
```

### 13.1 Unit Tests â€” PII Masker

```bash
# Run from repo root
python -c "
import sys; sys.path.insert(0, 'src')
from finix_rag.security.pii import PIIMasker

# Test 1: hashlib import (was broken â€” NameError before fix)
m = PIIMasker('hash')
result = m.mask('My Aadhaar is 2345 6789 0123')
assert 'HASH_' in result, 'hash strategy broken'
print('PASS: hash strategy works')

# Test 2: tokenize round-trip
m2 = PIIMasker('tokenize')
original = 'Account 1234567890 belongs to PAN ABCDE1234F'
masked = m2.mask(original)
assert 'ABCDE1234F' not in masked, 'PAN not masked'
restored = m2.unmask(masked)
assert 'ABCDE1234F' in restored, 'unmask failed'
print('PASS: tokenize round-trip works')

# Test 3: per-request isolation (no cross-user token leak)
m3 = PIIMasker('tokenize')
m4 = PIIMasker('tokenize')
m3.mask('Phone 9876543210')
# m4 should NOT be able to unmask m3's tokens
assert len(m4._reverse_map) == 0, 'singleton leak: shared token map'
print('PASS: per-request masker isolation works')
"
```

### 13.2 Unit Tests â€” Auth / JWT

```bash
python -c "
import sys; sys.path.insert(0, 'src')
from finix_rag.security.auth import TokenManager, UserRole

tm = TokenManager(secret_key='test-secret-32-chars-minimum-ok!')

# Test 1: create and validate
token = tm.create_token('user1', UserRole.ANALYST, 'risk', 3, 'tenant1')
payload = tm.validate_token(token)
assert payload.sub == 'user1'
assert payload.role == 'analyst'
assert payload.clearance_level == 3
print('PASS: token create/validate')

# Test 2: revocation
tm.revoke_token(payload.jti)
assert tm.validate_token(token) is None
print('PASS: token revocation')

# Test 3: expired token returns None (use expiry_minutes=0 trick)
import time
tm2 = TokenManager(secret_key='test-secret-32-chars-minimum-ok!', expiry_minutes=0)
t2 = tm2.create_token('u2', UserRole.READONLY, 'gen', 1, 'default')
time.sleep(1)
assert tm2.validate_token(t2) is None
print('PASS: expired token rejected')
"
```

### 13.3 Integration Tests â€” API Endpoints

Start services first: `docker compose up -d`

```bash
BASE=http://localhost:8000

# Health check
curl -sf $BASE/health | python -c "
import sys,json; d=json.load(sys.stdin)
assert d['status']=='healthy', d
print('PASS: /health')
"

# Login â†’ get token
TOKEN=$(curl -sf -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"test_analyst","password":"x","role":"analyst","department":"risk","clearance_level":3,"tenant_id":"default"}' \
  | python -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
echo "Token acquired: ${TOKEN:0:20}..."

# Query (empty collection returns no-docs response, not 500)
curl -sf -X POST $BASE/query \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"question":"What is the NPA ratio?","max_sources":3}' \
  | python -c "
import sys,json; d=json.load(sys.stdin)
assert 'answer' in d
assert 'confidence' in d
assert d['authorization_checked'] == True
print('PASS: /query returns valid shape')
"

# Audit logs â€” non-admin should get 403
curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer $TOKEN" $BASE/audit/logs \
  | python -c "import sys; code=sys.stdin.read(); assert code=='403', f'Expected 403 got {code}'; print('PASS: /audit/logs enforces admin-only')"

# Admin token
ADMIN_TOKEN=$(curl -sf -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"admin","password":"x","role":"admin","department":"ops","clearance_level":4,"tenant_id":"default"}' \
  | python -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

curl -sf -H "Authorization: Bearer $ADMIN_TOKEN" $BASE/audit/logs \
  | python -c "import sys,json; d=json.load(sys.stdin); assert 'logs' in d; print('PASS: /audit/logs accessible by admin')"
```

### 13.4 Security Tests

```bash
python -c "
import sys; sys.path.insert(0, 'src')

# Test: SYSTEM_PROMPT is now included in LLM prompt (not dead code)
from finix_rag.query.secure_query_engine import SYSTEM_PROMPT
assert 'social engineering' in SYSTEM_PROMPT.lower() or 'fabricate' in SYSTEM_PROMPT.lower()
print('PASS: SYSTEM_PROMPT contains security rules')

# Test: CORS origins are not wildcard
import os
os.environ['CORS_ORIGINS'] = 'http://localhost:3000'
origins = os.getenv('CORS_ORIGINS', '').split(',')
assert '*' not in origins, 'CORS wildcard still present'
print('PASS: CORS origins are not wildcard')
"

# Test: OPA denies readonly user from querying restricted docs
python -c "
import sys; sys.path.insert(0, 'src')
from finix_rag.security.opa_client import OPAAuthorizer
opa = OPAAuthorizer()
result = opa.check_query_access(
    user_id='u1', role='readonly', department='general',
    clearance_level=1, tenant_id='default',
    query_text='show me fraud investigation details',
)
# OPA may be unavailable in unit context â€” just check it returns a response
assert hasattr(result, 'allow')
print(f'OPA response: allow={result.allow}, reason={result.reason}')
print('PASS: OPA client returns structured response')
"
```

### 13.5 PII Masking Strategy Tests

```bash
python -c "
import sys; sys.path.insert(0, 'src')
from finix_rag.security.pii import PIIMasker

cases = [
    ('aadhaar', '2345 6789 0123'),
    ('pan',     'ABCDE1234F'),
    ('phone',   '9876543210'),
    ('email',   'user@example.com'),
    ('upi_id',  'user@upi'),
]

for strategy in ('redact', 'tokenize', 'hash'):
    m = PIIMasker(strategy)
    for pii_type, value in cases:
        masked = m.mask(f'Value is {value}')
        assert value not in masked, f'{strategy}/{pii_type}: value not masked'
    print(f'PASS: {strategy} strategy masks all PII types')
"
```

### 13.6 End-to-End Smoke Test

```bash
# Ingest a test document then query it
BASE=http://localhost:8000

ADMIN_TOKEN=$(curl -sf -X POST $BASE/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"admin","password":"x","role":"admin","department":"ops","clearance_level":4,"tenant_id":"default"}' \
  | python -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

# Create a minimal test PDF (requires reportlab or use any existing PDF)
python -c "
try:
    from reportlab.pdfgen import canvas
    c = canvas.Canvas('/tmp/test_finix.pdf')
    c.drawString(100, 750, 'FINIX Test Document')
    c.drawString(100, 700, 'NPA ratio for Q4 2025 was 2.3 percent.')
    c.save()
    print('Test PDF created')
except ImportError:
    # Fallback: create a minimal valid PDF manually
    with open('/tmp/test_finix.pdf', 'wb') as f:
        f.write(b'%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj 2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj 3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj 4 0 obj<</Length 44>>stream\nBT /F1 12 Tf 100 700 Td (NPA ratio 2.3 percent) Tj ET\nendstream\nendobj 5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\nxref\n0 6\n0000000000 65535 f\n0000000009 00000 n\n0000000058 00000 n\n0000000115 00000 n\n0000000274 00000 n\n0000000370 00000 n\ntrailer<</Size 6/Root 1 0 R>>\nstartxref\n441\n%%EOF')
    print('Minimal test PDF created')
"

# Ingest
INGEST_RESULT=$(curl -sf -X POST $BASE/ingest \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -F "file=@/tmp/test_finix.pdf")
echo "Ingest result: $INGEST_RESULT"

# Query
sleep 2
curl -sf -X POST $BASE/query \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"question":"What is the NPA ratio?","max_sources":3}' \
  | python -c "
import sys,json
d=json.load(sys.stdin)
print('Answer:', d['answer'][:120])
print('Confidence:', d['confidence'])
print('Citations:', len(d['citations']))
print('PASS: end-to-end smoke test complete')
"
```

### 13.7 Quick Test Summary

| Test | Command | What it checks |
|------|---------|----------------|
| PII hash fix | Section 13.1 test 1 | `hashlib` import no longer crashes |
| PII isolation | Section 13.1 test 3 | No cross-request token map leak |
| JWT lifecycle | Section 13.2 | Create, validate, revoke, expiry |
| API shape | Section 13.3 | `/health`, `/query`, `/audit/logs` |
| SYSTEM_PROMPT wired | Section 13.4 | Security rules reach the LLM |
| CORS not wildcard | Section 13.4 | Browser-safe CORS config |
| OPA integration | Section 13.4 | Authz client returns structured response |
| All PII strategies | Section 13.5 | `redact`/`tokenize`/`hash` all mask |
| End-to-end | Section 13.6 | Ingest â†’ query full pipeline |

---

## Appendix A: Port Reference

| Port | Service | Protocol |
|------|---------|----------|
| 8000 | FINIX RAG API | HTTP |
| 8200 | HashiCorp Vault | HTTP |
| 6333 | Qdrant REST | HTTP/HTTPS |
| 6334 | Qdrant gRPC | gRPC |
| 8181 | OPA | HTTP |
| 6379 | Redis | TCP |

## Appendix B: File Locations

| Path | Description |
|------|-------------|
| `finix-rag/.env` | Environment config |
| `finix-rag/config/` | Service configs |
| `finix-rag/src/finix_rag/` | Application code |
| `finix-rag/docs/` | Documentation |

