"""FINIX RAG - Main API Server.

FastAPI application with JWT auth, rate limiting, and secure endpoints.
"""
import os
import logging
from pathlib import Path
from typing import Optional
from contextlib import asynccontextmanager

# Load .env for local/non-container runs (docker-compose injects env directly, so this
# is a no-op there). Must run before the config values below are read.
try:
    from dotenv import load_dotenv
    load_dotenv()
except Exception:
    pass

from fastapi import FastAPI, Depends, HTTPException, UploadFile, File, Request
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
import structlog

from finix_rag.security.auth import TokenManager, TokenPayload, UserRole
from finix_rag.security.opa_client import OPAAuthorizer
from finix_rag.ingestion.secure_ingestor import SecureIngestor
from finix_rag.query.secure_query_engine import SecureQueryEngine, QueryResponse

# Configure structured logging
structlog.configure(
    processors=[
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.JSONRenderer(),
    ],
)
logger = structlog.get_logger()

# ============================================================
# Application State
# ============================================================

token_manager: Optional[TokenManager] = None
ingestor: Optional[SecureIngestor] = None
query_engine: Optional[SecureQueryEngine] = None
authorizer: Optional[OPAAuthorizer] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Initialize services on startup, cleanup on shutdown."""
    global token_manager, ingestor, query_engine, authorizer

    logger.info("Starting FINIX RAG services...")

    token_manager = TokenManager()
    authorizer = OPAAuthorizer()
    qdrant_api_key = os.getenv("QDRANT_API_KEY")
    ingestor = SecureIngestor(
        qdrant_url=f"http://{os.getenv('QDRANT_HOST', 'localhost')}:{os.getenv('QDRANT_PORT', '6333')}",
        collection_name=os.getenv("QDRANT_COLLECTION", "finix_documents"),
        api_key=qdrant_api_key,
        enable_pii_masking=os.getenv("ENABLE_PII_MASKING", "true") == "true",
    )
    query_engine = SecureQueryEngine(
        qdrant_url=f"http://{os.getenv('QDRANT_HOST', 'localhost')}:{os.getenv('QDRANT_PORT', '6333')}",
        collection_name=os.getenv("QDRANT_COLLECTION", "finix_documents"),
        api_key=qdrant_api_key,
        groq_api_key=os.getenv("GROQ_API_KEY"),
        enable_pii_masking=os.getenv("ENABLE_PII_MASKING", "true") == "true",
    )

    logger.info("All services initialized")
    yield

    logger.info("Shutting down FINIX RAG...")
    authorizer.close()


# ============================================================
# FastAPI App
# ============================================================

app = FastAPI(
    title="FINIX RAG - Secure Banking RAG API",
    description="Retrieval-Augmented Generation with cryptographic security for banking",
    version="1.0.0",
    lifespan=lifespan,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=os.getenv("CORS_ORIGINS", "http://localhost:3000").split(","),
    allow_credentials=True,
    allow_methods=["GET", "POST"],
    allow_headers=["Authorization", "Content-Type"],
)

security = HTTPBearer()


# ============================================================
# Request/Response Models
# ============================================================

class LoginRequest(BaseModel):
    user_id: str
    password: str
    role: UserRole = UserRole.READONLY
    department: str = "general"
    clearance_level: int = 1
    tenant_id: str = "default"


class TokenResponse(BaseModel):
    access_token: str
    token_type: str = "bearer"
    expires_in: int


class QueryRequest(BaseModel):
    question: str = Field(..., min_length=1, max_length=2000)
    max_sources: int = Field(default=5, ge=1, le=20)


class QueryResultResponse(BaseModel):
    answer: str
    citations: list[dict]
    confidence: float
    classification: str
    pii_masked: bool
    authorization_checked: bool
    retrieval_stats: dict


class IngestResponse(BaseModel):
    doc_id: str
    title: str
    chunks_ingested: int
    classification: str
    integrity_hash: str
    ocr_confidence: float
    tables_extracted: int
    pii_masked: bool


class HealthResponse(BaseModel):
    status: str
    vault: str
    qdrant: str
    opa: str


# ============================================================
# Auth Dependency
# ============================================================

async def get_current_user(
    credentials: HTTPAuthorizationCredentials = Depends(security),
) -> TokenPayload:
    """Validate JWT token and return user payload."""
    if token_manager is None:
        raise HTTPException(status_code=503, detail="Service not initialized")

    payload = token_manager.validate_token(credentials.credentials)
    if payload is None:
        raise HTTPException(status_code=401, detail="Invalid or expired token")
    return payload


# ============================================================
# Health Check
# ============================================================

@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Check health of all dependent services."""
    vault_status = "healthy" if ingestor and ingestor.vault and ingestor.vault.is_healthy else "unavailable"
    qdrant_status = "healthy"  # Would check Qdrant health
    opa_status = "healthy" if authorizer else "unavailable"

    return HealthResponse(
        status="healthy",
        vault=vault_status,
        qdrant=qdrant_status,
        opa=opa_status,
    )


# ============================================================
# Authentication
# ============================================================

@app.post("/auth/login", response_model=TokenResponse)
async def login(request: LoginRequest):
    """Authenticate user and issue JWT token."""
    # In production, validate against identity provider
    # For demo, accept any credentials and issue token
    token = token_manager.create_token(
        user_id=request.user_id,
        role=request.role,
        department=request.department,
        clearance_level=request.clearance_level,
        tenant_id=request.tenant_id,
    )
    return TokenResponse(
        access_token=token,
        expires_in=token_manager.expiry_minutes * 60,
    )


@app.post("/auth/refresh", response_model=TokenResponse)
async def refresh_token(user: TokenPayload = Depends(get_current_user)):
    """Refresh an existing token."""
    # Get the original token from the request (would need to pass it)
    token = token_manager.create_token(
        user_id=user.sub,
        role=UserRole(user.role),
        department=user.department,
        clearance_level=user.clearance_level,
        tenant_id=user.tenant_id,
    )
    return TokenResponse(
        access_token=token,
        expires_in=token_manager.expiry_minutes * 60,
    )


# ============================================================
# Document Ingestion
# ============================================================

@app.post("/ingest", response_model=IngestResponse)
async def ingest_document(
    file: UploadFile = File(...),
    user: TokenPayload = Depends(get_current_user),
):
    """Ingest a document through the secure pipeline.

    Requires: admin or analyst role.
    Pipeline: OCR → PII Masking → Chunking → Embedding → Encrypted Storage
    """
    # Authorization check
    auth = authorizer.check_ingestion_access(
        user_id=user.sub,
        role=user.role,
        department=user.department,
        clearance_level=user.clearance_level,
        tenant_id=user.tenant_id,
    )
    if not auth.allow:
        raise HTTPException(status_code=403, detail=auth.reason)

    # Validate file type
    allowed_types = {".pdf", ".png", ".jpg", ".jpeg", ".tiff", ".bmp"}
    suffix = Path(file.filename).suffix.lower()
    if suffix not in allowed_types:
        raise HTTPException(
            status_code=400,
            detail=f"Unsupported file type: {suffix}. Allowed: {allowed_types}",
        )

    # Save uploaded file. UPLOAD_DIR defaults to the container path but is overridable
    # for local/non-container runs (e.g. Windows dev), where /app/uploads is invalid.
    upload_dir = Path(os.getenv("UPLOAD_DIR", "/app/uploads"))
    upload_dir.mkdir(parents=True, exist_ok=True)
    file_path = upload_dir / Path(file.filename).name

    content = await file.read()
    file_path.write_bytes(content)

    # Ingest
    try:
        result = ingestor.ingest_file(
            file_path=file_path,
            user_id=user.sub,
            metadata={"uploaded_by": user.sub, "tenant_id": user.tenant_id},
        )
        return IngestResponse(**result)
    except Exception as e:
        logger.error("Ingestion failed", error=str(e), user=user.sub)
        raise HTTPException(status_code=500, detail=f"Ingestion failed: {e}")
    finally:
        # Securely delete uploaded file
        if file_path.exists():
            file_path.unlink()


# ============================================================
# Query
# ============================================================

@app.post("/query", response_model=QueryResultResponse)
async def query_documents(
    request: QueryRequest,
    http_request: Request,
    user: TokenPayload = Depends(get_current_user),
):
    """Query the RAG system with full security checks.

    Pipeline: Auth → PII Protection → Retrieval → Reranking → Synthesis → Audit
    """
    client_ip = http_request.client.host if http_request.client else "unknown"

    try:
        result = query_engine.query(
            question=request.question,
            user=user,
            ip_address=client_ip,
            max_sources=request.max_sources,
        )
        return QueryResultResponse(
            answer=result.answer,
            citations=result.citations,
            confidence=result.confidence,
            classification=result.classification,
            pii_masked=result.pii_masked,
            authorization_checked=result.authorization_checked,
            retrieval_stats=result.retrieval_stats,
        )
    except Exception as e:
        logger.error("Query failed", error=str(e), user=user.sub)
        raise HTTPException(status_code=500, detail=f"Query failed: {e}")


# ============================================================
# Audit
# ============================================================

@app.get("/audit/logs")
async def get_audit_logs(
    user: TokenPayload = Depends(get_current_user),
):
    """Get audit logs. Admin only."""
    if user.role != "admin":
        raise HTTPException(status_code=403, detail="Admin access required")

    return {"logs": query_engine.get_audit_log() if query_engine else []}


# ============================================================
# Admin
# ============================================================

@app.post("/admin/rotate-keys")
async def rotate_encryption_keys(
    user: TokenPayload = Depends(get_current_user),
):
    """Rotate Vault encryption keys. Admin only."""
    if user.role != "admin":
        raise HTTPException(status_code=403, detail="Admin access required")

    try:
        if ingestor and ingestor.vault:
            ingestor.vault.rotate_key()
            return {"status": "success", "message": "Keys rotated successfully"}
        return {"status": "skipped", "message": "Vault not available"}
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Key rotation failed: {e}")


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)
