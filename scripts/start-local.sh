#!/bin/bash
# =============================================================================
# FINIX PRODUCTION - LOCAL DEVELOPMENT STARTUP SCRIPT
# =============================================================================
# This script sets up the complete local development environment
# Usage: ./scripts/start-local.sh
# =============================================================================

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${PROJECT_ROOT}/.env"

echo -e "${BLUE}═══════════════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}   FINIX SECURE BANKING AI PLATFORM - LOCAL DEVELOPMENT STARTUP       ${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════════════${NC}"
echo

# -----------------------------------------------------------------------------
# Check prerequisites
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[1/7] Checking prerequisites...${NC}"

check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo -e "${RED}  ✗ $1 not found. Please install $1.${NC}"
        exit 1
    fi
    echo -e "${GREEN}  ✓ $1 found${NC}"
}

check_command docker
check_command docker-compose
check_command go
check_command python3
check_command openssl

# -----------------------------------------------------------------------------
# Generate .env if not exists
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[2/7] Setting up environment...${NC}"

if [[ ! -f "${ENV_FILE}" ]]; then
    echo -e "  Creating .env from .env.example..."
    cp "${PROJECT_ROOT}/.env.example" "${ENV_FILE}"

    # Generate secure defaults for development
    FINIX_JWT_SECRET=$(openssl rand -base64 48)
    FINIX_INTERNAL_TOKEN=$(openssl rand -base64 32)
    FINIX_AADHAAR_HASH_KEY=$(openssl rand -base64 32)
    FINIX_DATA_ENCRYPTION_KEY=$(openssl rand -base64 32)
    VAULT_TOKEN=$(openssl rand -base64 32)
    PGPASSWORD=$(openssl rand -base64 24 | tr -d '/+=' | cut -c1-32)

    # Update .env with generated values
    sed -i "s/^FINIX_JWT_SECRET=.*/FINIX_JWT_SECRET=${FINIX_JWT_SECRET}/" "${ENV_FILE}"
    sed -i "s/^FINIX_INTERNAL_TOKEN=.*/FINIX_INTERNAL_TOKEN=${FINIX_INTERNAL_TOKEN}/" "${ENV_FILE}"
    sed -i "s/^FINIX_AADHAAR_HASH_KEY=.*/FINIX_AADHAAR_HASH_KEY=${FINIX_AADHAAR_HASH_KEY}/" "${ENV_FILE}"
    sed -i "s/^FINIX_DATA_ENCRYPTION_KEY=.*/FINIX_DATA_ENCRYPTION_KEY=${FINIX_DATA_ENCRYPTION_KEY}/" "${ENV_FILE}"
    sed -i "s/^VAULT_TOKEN=.*/VAULT_TOKEN=${VAULT_TOKEN}/" "${ENV_FILE}"
    sed -i "s/^PGPASSWORD=.*/PGPASSWORD=${PGPASSWORD}/" "${ENV_FILE}"
    sed -i "s/^FINIX_REVOCATION_FAIL_CLOSED=.*/FINIX_REVOCATION_FAIL_CLOSED=false/" "${ENV_FILE}"
    sed -i "s/^FINIX_PIN_ENFORCE=.*/FINIX_PIN_ENFORCE=false/" "${ENV_FILE}"
    sed -i "s/^CERT_PINS=.*/CERT_PINS=/" "${ENV_FILE}"
    sed -i "s/^AI_PROVIDER=.*/AI_PROVIDER=local/" "${ENV_FILE}"

    echo -e "${GREEN}  ✓ .env created with secure development defaults${NC}"
    echo -e "${YELLOW}  ⚠ IMPORTANT: Add your GROQ_API_KEY to .env for AI features${NC}"
else
    echo -e "${GREEN}  ✓ .env already exists${NC}"
fi

# Source the .env
set -a
source "${ENV_FILE}"
set +a

# -----------------------------------------------------------------------------
# Create required directories
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[3/7] Creating required directories...${NC}"
mkdir -p "${PROJECT_ROOT}/finix-rag/uploads"
mkdir -p "${PROJECT_ROOT}/backend/migrations"
mkdir -p "${PROJECT_ROOT}/config/vault"
mkdir -p "${PROJECT_ROOT}/config/opa"
mkdir -p "${PROJECT_ROOT}/config/grafana/dashboards"
mkdir -p "${PROJECT_ROOT}/config/grafana/datasources"
mkdir -p "${PROJECT_ROOT}/config/prometheus"
echo -e "${GREEN}  ✓ Directories created${NC}"

# -----------------------------------------------------------------------------
# Start infrastructure with Docker Compose
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[4/7] Starting infrastructure services...${NC}"
cd "${PROJECT_ROOT}"
docker-compose up -d postgres redis qdrant vault opa prometheus grafana

# Wait for services to be healthy
echo -e "  Waiting for services to be healthy..."
sleep 5

check_health() {
    local service=$1
    local url=$2
    local max_attempts=30
    local attempt=1

    while [[ $attempt -le $max_attempts ]]; do
        if curl -sf "${url}" > /dev/null 2>&1; then
            echo -e "${GREEN}  ✓ ${service} is healthy${NC}"
            return 0
        fi
        echo -e "  Waiting for ${service}... (${attempt}/${max_attempts})"
        sleep 2
        ((attempt++))
    done

    echo -e "${RED}  ✗ ${service} failed to become healthy${NC}"
    return 1
}

check_health "PostgreSQL" "http://localhost:5432" || true  # pg doesn't have HTTP health
check_health "Redis" "http://localhost:6379" || true  # redis doesn't have HTTP health
check_health "Qdrant" "http://localhost:6333/health"
check_health "Vault" "http://localhost:8200/v1/sys/health"
check_health "OPA" "http://localhost:8181/health"
check_health "Prometheus" "http://localhost:9090/-/healthy"
check_health "Grafana" "http://localhost:3000/api/health"

# -----------------------------------------------------------------------------
# Initialize Vault
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[5/7] Initializing Vault...${NC}"
docker exec finix-vault vault status > /dev/null 2>&1 || {
    echo -e "  Enabling Vault transit engine..."
    docker exec finix-vault vault secrets enable transit
    docker exec finix-vault vault write -f transit/keys/finix-pii type=aes256-gcm96
    docker exec finix-vault vault write -f transit/keys/finix-data type=aes256-gcm96
    echo -e "${GREEN}  ✓ Vault initialized${NC}"
}

# -----------------------------------------------------------------------------
# Initialize OPA policies
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[6/7] Checking OPA policies...${NC}"
if [[ -f "${PROJECT_ROOT}/finix-rag/config/opa-policy.rego" ]]; then
    echo -e "${GREEN}  ✓ OPA policy found${NC}"
else
    echo -e "${YELLOW}  ⚠ OPA policy not found, creating default...${NC}"
    mkdir -p "${PROJECT_ROOT}/finix-rag/config"
    cat > "${PROJECT_ROOT}/finix-rag/config/opa-policy.rego" << 'EOF'
package finix.authz

default allow = false

# Admin users can do anything
allow {
    input.user.role == "admin"
}

# Analysts can ingest and query
allow {
    input.user.role == "analyst"
    input.action in {"ingest", "query"}
}

# Regular users can only query
allow {
    input.user.role == "user"
    input.action == "query"
}

# Ingestion requires admin or analyst
allow {
    input.action == "ingest"
    input.user.role in {"admin", "analyst"}
}

# Query requires any authenticated role
allow {
    input.action == "query"
    input.user.role in {"admin", "analyst", "user"}
}
EOF
    echo -e "${GREEN}  ✓ Default OPA policy created${NC}"
fi

# -----------------------------------------------------------------------------
# Build and verify Go backend
# -----------------------------------------------------------------------------
echo -e "${YELLOW}[7/7] Building Go backend...${NC}"
cd "${PROJECT_ROOT}/backend"
if go build -o /tmp/finix-server ./cmd/server 2>/dev/null; then
    echo -e "${GREEN}  ✓ Go backend builds successfully${NC}"
else
    echo -e "${RED}  ✗ Go backend build failed${NC}"
    exit 1
fi

# Verify Python RAG syntax
echo -e "${YELLOW}     Verifying Python RAG...${NC}"
cd "${PROJECT_ROOT}/finix-rag"
if python3 -m py_compile src/finix_rag/api.py src/finix_rag/query/secure_query_engine.py src/finix_rag/ingestion/secure_ingestor.py 2>/dev/null; then
    echo -e "${GREEN}  ✓ Python RAG syntax OK${NC}"
else
    echo -e "${RED}  ✗ Python RAG syntax error${NC}"
    exit 1
fi

# -----------------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------------
echo
echo -e "${BLUE}═══════════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}   ✓ LOCAL DEVELOPMENT ENVIRONMENT READY!${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════════════${NC}"
echo
echo -e "${BLUE}Services running:${NC}"
echo -e "  • PostgreSQL:     ${GREEN}localhost:5432${NC} (db: finix, user: finix)"
echo -e "  • Redis:          ${GREEN}localhost:6379${NC}"
echo -e "  • Qdrant:         ${GREEN}localhost:6333${NC} (gRPC: 6334)"
echo -e "  • Vault:          ${GREEN}localhost:8200${NC} (token: ${VAULT_TOKEN})"
echo -e "  • OPA:            ${GREEN}localhost:8181${NC}"
echo -e "  • Prometheus:     ${GREEN}localhost:9090${NC}"
echo -e "  • Grafana:        ${GREEN}localhost:3000${NC} (admin/admin)"
echo
echo -e "${BLUE}To start the backend:${NC}"
echo -e "  cd ${PROJECT_ROOT}/backend && go run ./cmd/server"
echo
echo -e "${BLUE}To start the Python RAG service:${NC}"
echo -e "  cd ${PROJECT_ROOT}/finix-rag && python3 -m venv venv && source venv/bin/activate && pip install -r requirements.txt && uvicorn finix_rag.api:app --reload --port 8000"
echo
echo -e "${BLUE}To start everything with Docker Compose:${NC}"
echo -e "  cd ${PROJECT_ROOT} && docker-compose up -d backend finix-rag"
echo
echo -e "${YELLOW}⚠ REMINDER: Add GROQ_API_KEY to .env for AI features!${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════════════${NC}"