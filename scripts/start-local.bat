@echo off
REM =============================================================================
REM FINIX PRODUCTION - WINDOWS LOCAL DEVELOPMENT STARTUP SCRIPT
REM =============================================================================
REM This script sets up the complete local development environment on Windows
REM Usage: scripts\start-local.bat
REM =============================================================================

setlocal enabledelayedexpansion

echo ═══════════════════════════════════════════════════════════════════════
echo    FINIX SECURE BANKING AI PLATFORM - LOCAL DEVELOPMENT STARTUP
echo ═══════════════════════════════════════════════════════════════════════

REM -----------------------------------------------------------------------------
REM Check prerequisites
REM -----------------------------------------------------------------------------
echo [1/7] Checking prerequisites...

where docker >nul 2>&1
if %errorlevel% neq 0 (
    echo ✗ docker not found. Please install Docker Desktop.
    exit /b 1
) else (
    echo ✓ docker found
)

where docker-compose >nul 2>&1
if %errorlevel% neq 0 (
    echo ✗ docker-compose not found. Please install Docker Compose.
    exit /b 1
) else (
    echo ✓ docker-compose found
)

where go >nul 2>&1
if %errorlevel% neq 0 (
    echo ✗ go not found. Please install Go 1.21+.
    exit /b 1
) else (
    echo ✓ go found
)

where python >nul 2>&1
if %errorlevel% neq 0 (
    echo ✗ python not found. Please install Python 3.11+.
    exit /b 1
) else (
    echo ✓ python found
)

where openssl >nul 2>&1
if %errorlevel% neq 0 (
    echo ✗ openssl not found. Please install OpenSSL (included with Git for Windows).
    exit /b 1
) else (
    echo ✓ openssl found
)

REM -----------------------------------------------------------------------------
REM Setup environment
REM -----------------------------------------------------------------------------
echo [2/7] Setting up environment...

set PROJECT_ROOT=%~dp0..
set ENV_FILE=%PROJECT_ROOT%\.env
set ENV_EXAMPLE=%PROJECT_ROOT%\.env.example

if not exist "%ENV_FILE%" (
    echo Creating .env from .env.example...
    copy "%ENV_EXAMPLE%" "%ENV_FILE%" >nul

    REM Generate secure defaults for development using PowerShell
    powershell -Command "
        $bytes = New-Object byte[] 48; (New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes); $jwt = [Convert]::ToBase64String($bytes)
        $bytes = New-Object byte[] 32; (New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes); $internal = [Convert]::ToBase64String($bytes)
        $bytes = New-Object byte[] 32; (New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes); $aadhaar = [Convert]::ToBase64String($bytes)
        $bytes = New-Object byte[] 32; (New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes); $data = [Convert]::ToBase64String($bytes)
        $bytes = New-Object byte[] 32; (New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes); $vault = [Convert]::ToBase64String($bytes)
        $bytes = New-Object byte[] 24; (New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($bytes); $pg = [Convert]::ToBase64String($bytes)
        $pg = $pg -replace '[/+=]', '' -replace '.{32}.*','$&'
        
        (Get-Content '%ENV_FILE%') -replace '^FINIX_JWT_SECRET=.*', \"FINIX_JWT_SECRET=$jwt\" -replace '^FINIX_INTERNAL_TOKEN=.*', \"FINIX_INTERNAL_TOKEN=$internal\" -replace '^FINIX_AADHAAR_HASH_KEY=.*', \"FINIX_AADHAAR_HASH_KEY=$aadhaar\" -replace '^FINIX_DATA_ENCRYPTION_KEY=.*', \"FINIX_DATA_ENCRYPTION_KEY=$data\" -replace '^VAULT_TOKEN=.*', \"VAULT_TOKEN=$vault\" -replace '^PGPASSWORD=.*', \"PGPASSWORD=$pg\" -replace '^FINIX_REVOCATION_FAIL_CLOSED=.*', 'FINIX_REVOCATION_FAIL_CLOSED=false' -replace '^FINIX_PIN_ENFORCE=.*', 'FINIX_PIN_ENFORCE=false' -replace '^CERT_PINS=.*', 'CERT_PINS=' -replace '^AI_PROVIDER=.*', 'AI_PROVIDER=local' | Set-Content '%ENV_FILE%'
    "
    echo ✓ .env created with secure development defaults
    echo ⚠ IMPORTANT: Add your GROQ_API_KEY to .env for AI features
) else (
    echo ✓ .env already exists
)

REM -----------------------------------------------------------------------------
REM Create required directories
REM -----------------------------------------------------------------------------
echo [3/7] Creating required directories...
if not exist "%PROJECT_ROOT%\finix-rag\uploads" mkdir "%PROJECT_ROOT%\finix-rag\uploads"
if not exist "%PROJECT_ROOT%\backend\migrations" mkdir "%PROJECT_ROOT%\backend\migrations"
if not exist "%PROJECT_ROOT%\config\vault" mkdir "%PROJECT_ROOT%\config\vault"
if not exist "%PROJECT_ROOT%\config\opa" mkdir "%PROJECT_ROOT%\config\opa"
if not exist "%PROJECT_ROOT%\config\grafana\dashboards" mkdir "%PROJECT_ROOT%\config\grafana\dashboards"
if not exist "%PROJECT_ROOT%\config\grafana\datasources" mkdir "%PROJECT_ROOT%\config\grafana\datasources"
if not exist "%PROJECT_ROOT%\config\prometheus" mkdir "%PROJECT_ROOT%\config\prometheus"
echo ✓ Directories created

REM -----------------------------------------------------------------------------
REM Start infrastructure with Docker Compose
REM -----------------------------------------------------------------------------
echo [4/7] Starting infrastructure services...
cd /d "%PROJECT_ROOT%"
docker-compose up -d postgres redis qdrant vault opa prometheus grafana

echo Waiting for services to be healthy...
timeout /t 10 /nobreak >nul

REM -----------------------------------------------------------------------------
REM Initialize Vault
REM -----------------------------------------------------------------------------
echo [5/7] Initializing Vault...
docker exec finix-vault vault status >nul 2>&1
if %errorlevel% neq 0 (
    echo Enabling Vault transit engine...
    docker exec finix-vault vault secrets enable transit
    docker exec finix-vault vault write -f transit/keys/finix-pii type=aes256-gcm96
    docker exec finix-vault vault write -f transit/keys/finix-data type=aes256-gcm96
    echo ✓ Vault initialized
) else (
    echo ✓ Vault already initialized
)

REM -----------------------------------------------------------------------------
REM Initialize OPA policies
REM -----------------------------------------------------------------------------
echo [6/7] Checking OPA policies...
if not exist "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego" (
    echo ⚠ OPA policy not found, creating default...
    type nul > "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo package finix.authz >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo. >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo default allow = false >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo. >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo allow { >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo     input.user.role == "admin" >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo } >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo. >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo allow { >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo     input.user.role == "analyst" >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo     input.action in {"ingest", "query"} >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo } >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo. >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo allow { >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo     input.user.role == "user" >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo     input.action == "query" >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo } >> "%PROJECT_ROOT%\finix-rag\config\opa-policy.rego"
    echo ✓ Default OPA policy created
) else (
    echo ✓ OPA policy found
)

REM -----------------------------------------------------------------------------
REM Build and verify Go backend
REM -----------------------------------------------------------------------------
echo [7/7] Building Go backend...
cd /d "%PROJECT_ROOT%\backend"
go build -o finix-server.exe ./cmd/server >nul 2>&1
if %errorlevel% equ 0 (
    echo ✓ Go backend builds successfully
    del finix-server.exe
) else (
    echo ✗ Go backend build failed
    exit /b 1
)

REM Verify Python RAG syntax
echo Verifying Python RAG...
cd /d "%PROJECT_ROOT%\finix-rag"
python -m py_compile src/finix_rag/api.py src/finix_rag/query/secure_query_engine.py src/finix_rag/ingestion/secure_ingestor.py >nul 2>&1
if %errorlevel% equ 0 (
    echo ✓ Python RAG syntax OK
) else (
    echo ✗ Python RAG syntax error
    exit /b 1
)

REM -----------------------------------------------------------------------------
REM Summary
REM -----------------------------------------------------------------------------
echo.
echo ═══════════════════════════════════════════════════════════════════════
echo ✓ LOCAL DEVELOPMENT ENVIRONMENT READY!
echo ═══════════════════════════════════════════════════════════════════════
echo.
echo Services running:
echo   - PostgreSQL:     localhost:5432 (db: finix, user: finix)
echo   - Redis:          localhost:6379
echo   - Qdrant:         localhost:6333 (gRPC: 6334)
echo   - Vault:          localhost:8200 (check .env for token)
echo   - OPA:            localhost:8181
echo   - Prometheus:     localhost:9090
echo   - Grafana:        localhost:3000 (admin/admin)
echo.
echo To start the backend:
echo   cd %PROJECT_ROOT%\backend && go run ./cmd/server
echo.
echo To start the Python RAG service:
echo   cd %PROJECT_ROOT%\finix-rag && python -m venv venv && venv\Scripts\activate && pip install -r requirements.txt && uvicorn finix_rag.api:app --reload --port 8000
echo.
echo To start everything with Docker Compose:
echo   cd %PROJECT_ROOT% && docker-compose up -d backend finix-rag
echo.
echo ⚠ REMINDER: Add GROQ_API_KEY to .env for AI features!
echo ═══════════════════════════════════════════════════════════════════════