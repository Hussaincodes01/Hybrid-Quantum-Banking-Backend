@echo off
REM ============================================
REM FINIX RAG - Quick Setup Script (Windows)
REM ============================================

echo.
echo ========================================
echo   FINIX RAG - Banking RAG System Setup
echo ========================================
echo.

REM Step 1: Check prerequisites
echo [1/7] Checking prerequisites...
where python >nul 2>&1
if %errorlevel% neq 0 (
    echo ERROR: Python not found. Install Python 3.11+ first.
    pause
    exit /b 1
)
echo OK - Python found

where docker >nul 2>&1
if %errorlevel% neq 0 (
    echo WARNING: Docker not found. Infrastructure services won't start.
    echo You can still use the Python API locally.
)
echo OK - Prerequisites checked

REM Step 2: Create virtual environment
echo.
echo [2/7] Creating Python virtual environment...
if not exist venv (
    python -m venv venv
    echo Virtual environment created
) else (
    echo Virtual environment already exists
)

REM Step 3: Activate and install dependencies
echo.
echo [3/7] Installing Python dependencies...
call venv\Scripts\activate.bat
pip install -r requirements.txt
echo Dependencies installed

REM Step 4: Create .env if not exists
echo.
echo [4/7] Setting up environment...
if not exist .env (
    copy .env.example .env
    echo Created .env from template
    echo IMPORTANT: Edit .env with your API keys before starting!
) else (
    echo .env already exists
)

REM Step 5: Download models from HuggingFace
echo.
echo [5/7] Downloading BGE-M3 models from HuggingFace...
echo This will download ~2.8 GB of model weights.
echo This may take 10-30 minutes depending on your internet speed.
echo.
python scripts\download_models.py
if %errorlevel% neq 0 (
    echo WARNING: Model download failed. You can retry later with:
    echo   python scripts\download_models.py
)

REM Step 6: Create required directories
echo.
echo [6/7] Creating directories...
if not exist config\tls mkdir config\tls
if not exist uploads mkdir uploads
if not exist backups mkdir backups
echo Directories created

REM Step 7: Start services (if Docker available)
echo.
echo [7/7] Starting services...
where docker >nul 2>&1
if %errorlevel% equ 0 (
    docker compose up -d
    echo Services started
) else (
    echo Docker not available. Start infrastructure manually.
    echo See docs\RUNBOOK.md for instructions.
)

echo.
echo ========================================
echo   Setup Complete!
echo ========================================
echo.
echo Services:
echo   - FINIX RAG API:  http://localhost:8000
echo   - Vault:          http://localhost:8200
echo   - Qdrant:         http://localhost:6333
echo   - OPA:            http://localhost:8181
echo   - Redis:          localhost:6379
echo.
echo Quick test:
echo   curl http://localhost:8000/health
echo.
echo See docs\RUNBOOK.md for detailed operations guide.
echo.
pause
