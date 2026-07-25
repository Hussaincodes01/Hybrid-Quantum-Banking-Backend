#!/bin/bash
# ============================================
# FINIX RAG - Quick Setup Script (Linux/Mac)
# ============================================

set -e

echo ""
echo "========================================"
echo "  FINIX RAG - Banking RAG System Setup"
echo "========================================"
echo ""

# Step 1: Check prerequisites
echo "[1/6] Checking prerequisites..."
command -v docker >/dev/null 2>&1 || { echo "ERROR: Docker not found"; exit 1; }
command -v docker-compose >/dev/null 2>&1 || command -v docker compose >/dev/null 2>&1 || { echo "ERROR: Docker Compose not found"; exit 1; }
echo "OK - Docker found"

# Step 2: Create .env if not exists
echo ""
echo "[2/6] Setting up environment..."
if [ ! -f .env ]; then
    cp .env.example .env
    echo "Created .env from template"
    echo "IMPORTANT: Edit .env with your API keys before starting!"
else
    echo ".env already exists"
fi

# Step 3: Create required directories
echo ""
echo "[3/6] Creating directories..."
mkdir -p config/tls uploads backups
echo "Directories created"

# Step 4: Pull Docker images
echo ""
echo "[4/6] Pulling Docker images..."
docker compose pull 2>/dev/null || docker-compose pull

# Step 5: Build application image
echo ""
echo "[5/6] Building FINIX RAG application..."
docker compose build finix-rag 2>/dev/null || docker-compose build finix-rag

# Step 6: Start services
echo ""
echo "[6/6] Starting services..."
docker compose up -d 2>/dev/null || docker-compose up -d

echo ""
echo "========================================"
echo "  Setup Complete!"
echo "========================================"
echo ""
echo "Services starting:"
echo "  - Vault:        http://localhost:8200"
echo "  - Qdrant:       http://localhost:6333"
echo "  - OPA:          http://localhost:8181"
echo "  - Redis:        localhost:6379"
echo "  - FINIX RAG:    http://localhost:8000"
echo ""
echo "Wait 60 seconds, then test:"
echo "  curl http://localhost:8000/health"
echo ""
echo "See docs/RUNBOOK.md for operations guide."
