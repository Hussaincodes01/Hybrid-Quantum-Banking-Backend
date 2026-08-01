"""Backend bridge: the /v1/rag/* contract the Go backend calls.

The Go service (backend/internal/infra/ai/rag_client.go) talks to three
endpoints — /v1/rag/chat, /v1/rag/sms and /v1/rag/tool/risk — authenticated with
a shared X-Internal-Token header. The public API in api.py exposes /query,
/ingest and friends behind a per-user JWT instead, so nothing the Go client
asked for existed and every call 404'd whenever AI_PROVIDER=remote.

This module supplies exactly that service-to-service surface and maps it onto
the existing secure query engine. It is deliberately a thin adapter:

  * auth is the shared internal token (constant-time compare), because the
    caller is a trusted internal service, not an end user;
  * request/response field names match the Go structs verbatim, so the two
    sides stay in sync;
  * chat delegates to the same SecureQueryEngine used by /query, keeping the
    PII / authorization / audit pipeline intact.
"""
from __future__ import annotations

import hmac
import os
import re
from typing import Any, Callable, Optional

import structlog
from fastapi import APIRouter, Depends, Header, HTTPException
from pydantic import BaseModel, Field

logger = structlog.get_logger()

router = APIRouter(prefix="/v1/rag", tags=["backend-bridge"])

DISCLAIMER = (
    "Disclaimer: This is AI-generated guidance. Please consult a certified "
    "financial advisor before making financial decisions."
)


# ============================================================
# Auth: shared internal token
# ============================================================

def _expected_internal_token() -> str:
    return (os.getenv("FINIX_INTERNAL_TOKEN") or "").strip()


async def require_internal_token(
    x_internal_token: Optional[str] = Header(default=None, alias="X-Internal-Token"),
) -> None:
    """Authenticate the calling backend service.

    Fails closed: if FINIX_INTERNAL_TOKEN is unset the bridge refuses every
    request rather than silently accepting anonymous callers.
    """
    expected = _expected_internal_token()
    if not expected:
        raise HTTPException(
            status_code=503,
            detail="internal bridge disabled: FINIX_INTERNAL_TOKEN is not configured",
        )
    supplied = (x_internal_token or "").strip()
    if not supplied or not hmac.compare_digest(supplied, expected):
        raise HTTPException(status_code=401, detail="invalid internal token")


# ============================================================
# Wire format — mirrors the Go structs field for field
# ============================================================

class ChatMessage(BaseModel):
    role: str = ""
    content: str = ""


class ChatRequest(BaseModel):
    query: str = Field(..., min_length=1, max_length=4000)
    history: list[ChatMessage] = Field(default_factory=list)
    user_id: str = ""
    device_fp: str = ""
    scope: str = ""          # wealth | fraud | general
    max_sources: int = Field(default=5, ge=1, le=20)


class Citation(BaseModel):
    okf_id: str = ""
    title: str = ""
    section: str = ""
    relevance: float = 0.0
    document_url: str = ""


class ChatResponse(BaseModel):
    answer: str
    citations: list[Citation] = Field(default_factory=list)
    confidence: float = 0.0
    tools_used: list[str] = Field(default_factory=list)
    disclaimer: str = DISCLAIMER


class ScanSMSRequest(BaseModel):
    sender: str = ""
    message: str = Field(default="", max_length=4000)


class ScanSMSResponse(BaseModel):
    is_phishing: bool
    confidence: float
    risk_level: str          # low | medium | high
    reasons: list[str] = Field(default_factory=list)
    recommended_action: str


class RiskScoreRequest(BaseModel):
    features: dict[str, float] = Field(default_factory=dict)


class RiskScoreResponse(BaseModel):
    risk_score: float        # 0..100
    risk_level: str          # low | medium | high
    explanation: str
    top_factors: list[str] = Field(default_factory=list)


# ============================================================
# Engine accessor (injected by api.py so lifespan state is respected)
# ============================================================

_engine_provider: Callable[[], Any] = lambda: None


def set_engine_provider(provider: Callable[[], Any]) -> None:
    """Register how the bridge reaches the live SecureQueryEngine."""
    global _engine_provider
    _engine_provider = provider


def _citations_from(raw: Any) -> list[Citation]:
    out: list[Citation] = []
    for c in raw or []:
        if isinstance(c, dict):
            out.append(
                Citation(
                    okf_id=str(c.get("okf_id") or c.get("doc_id") or ""),
                    title=str(c.get("title") or ""),
                    section=str(c.get("section") or ""),
                    relevance=float(c.get("relevance") or c.get("score") or 0.0),
                    document_url=str(c.get("document_url") or c.get("url") or ""),
                )
            )
    return out


# ============================================================
# Endpoints
# ============================================================

@router.post("/chat", response_model=ChatResponse, dependencies=[Depends(require_internal_token)])
async def rag_chat(request: ChatRequest) -> ChatResponse:
    """Answer a user question through the secure RAG pipeline."""
    engine = _engine_provider()
    if engine is None:
        raise HTTPException(status_code=503, detail="query engine not initialised")

    # The engine expects a user principal; the backend has already
    # authenticated the end user, so pass its identity through for audit.
    try:
        from finix_rag.security.auth import TokenPayload, UserRole

        principal = TokenPayload(
            sub=request.user_id or "finix-backend",
            role=UserRole.ANALYST if hasattr(UserRole, "ANALYST") else None,
            department="general",
            clearance_level=1,
            tenant_id="default",
        )
    except Exception:  # pragma: no cover - principal shape varies by build
        principal = None

    try:
        result = engine.query(
            question=request.query,
            user=principal,
            ip_address="internal",
            max_sources=request.max_sources,
        )
    except Exception as exc:
        logger.error("rag_chat_failed", error=str(exc), user_id=request.user_id)
        raise HTTPException(status_code=502, detail=f"query engine error: {exc}") from exc

    return ChatResponse(
        answer=getattr(result, "answer", "") or "",
        citations=_citations_from(getattr(result, "citations", [])),
        confidence=float(getattr(result, "confidence", 0.0) or 0.0),
        tools_used=["retrieval", "rerank", "synthesis"],
        disclaimer=DISCLAIMER,
    )


# --- SMS phishing classification -------------------------------------------

_SUSPICIOUS_URL = re.compile(r"(https?://|www\.)\S+", re.I)
_URGENCY = re.compile(
    r"\b(urgent|immediately|within \d+ (min|hour)|act now|last warning|expire[sd]?|blocked|suspend)\b",
    re.I,
)
_CREDENTIAL_BAIT = re.compile(r"\b(otp|pin|cvv|password|upi\s*pin|kyc|aadhaar|net\s*banking)\b", re.I)
_MONEY_BAIT = re.compile(r"\b(lottery|prize|refund|cashback|reward|won|claim)\b", re.I)
_SHORTENER = re.compile(r"\b(bit\.ly|tinyurl|t\.co|rb\.gy|is\.gd|cutt\.ly)\b", re.I)


@router.post("/sms", response_model=ScanSMSResponse, dependencies=[Depends(require_internal_token)])
async def rag_scan_sms(request: ScanSMSRequest) -> ScanSMSResponse:
    """Classify an SMS as phishing / safe with explainable reasons.

    Rule-based and fully deterministic: every positive signal is reported back
    so the reason list is a real explanation rather than an opaque score.
    """
    text = request.message or ""
    reasons: list[str] = []
    score = 0.0

    if _CREDENTIAL_BAIT.search(text):
        score += 0.35
        reasons.append("Requests credentials (OTP/PIN/CVV/KYC) — banks never ask for these over SMS")
    if _URGENCY.search(text):
        score += 0.25
        reasons.append("Uses urgency or account-suspension pressure")
    if _SHORTENER.search(text):
        score += 0.20
        reasons.append("Contains a shortened link that hides its true destination")
    elif _SUSPICIOUS_URL.search(text):
        score += 0.10
        reasons.append("Contains an external link")
    if _MONEY_BAIT.search(text):
        score += 0.20
        reasons.append("Promises a prize, refund or cashback")

    sender = (request.sender or "").strip()
    # Indian bank senders are alphanumeric header IDs (e.g. VM-SBIINB), not
    # plain mobile numbers; a 10-digit sender claiming to be a bank is a flag.
    if re.fullmatch(r"\+?\d[\d\s-]{7,}", sender) and re.search(r"\b(bank|sbi|hdfc|icici|axis|upi)\b", text, re.I):
        score += 0.25
        reasons.append("Bank message sent from a personal mobile number, not an official sender ID")

    score = min(score, 1.0)
    if score >= 0.6:
        level, action = "high", "Do not act on this message. Delete it and report to your bank."
    elif score >= 0.3:
        level, action = "medium", "Treat with caution. Verify via the official bank app before acting."
    else:
        level, action = "low", "No phishing indicators detected."

    if not reasons:
        reasons.append("No known phishing indicators found")

    return ScanSMSResponse(
        is_phishing=score >= 0.6,
        confidence=round(score, 3),
        risk_level=level,
        reasons=reasons,
        recommended_action=action,
    )


# --- Transaction risk scoring ----------------------------------------------

# Weight per normalised feature. Kept explicit (not learned) so the score is
# auditable; the trained ONNX models live in the Go service.
_RISK_WEIGHTS: dict[str, float] = {
    "amount_vs_average": 12.0,
    "is_new_recipient": 15.0,
    "failed_pin_attempts": 5.0,
    "velocity_count_1hr": 2.0,
    "balance_impact": 10.0,
    "recipient_gnn_score": 20.0,
    "behaviour_drift": 20.0,
    "geo_velocity_flag": 20.0,
    "structuring_flag": 25.0,
}


@router.post("/tool/risk", response_model=RiskScoreResponse, dependencies=[Depends(require_internal_token)])
async def rag_risk_score(request: RiskScoreRequest) -> RiskScoreResponse:
    """Score transaction risk 0..100 from named features, with top factors."""
    contributions: dict[str, float] = {}
    score = 0.0

    for name, weight in _RISK_WEIGHTS.items():
        value = float(request.features.get(name, 0.0) or 0.0)
        if name == "amount_vs_average":
            value = max(0.0, value - 1.0)          # only above-average spend adds risk
        contribution = min(max(value, 0.0) * weight, weight * 1.5)
        if contribution > 0:
            contributions[name] = contribution
            score += contribution

    # Low session trust is risk-increasing, so it is scored inversely.
    trust = float(request.features.get("session_trust_score", 1.0) or 0.0)
    if trust < 0.7:
        contribution = (0.7 - max(trust, 0.0)) * 20.0
        contributions["session_trust_score"] = contribution
        score += contribution

    score = round(min(score, 100.0), 2)
    if score >= 70:
        level = "high"
    elif score >= 40:
        level = "medium"
    else:
        level = "low"

    top = sorted(contributions.items(), key=lambda kv: kv[1], reverse=True)[:3]
    top_factors = [f"{name} (+{value:.1f})" for name, value in top]

    return RiskScoreResponse(
        risk_score=score,
        risk_level=level,
        explanation=(
            f"Risk {score}/100 ({level}). "
            + ("Main drivers: " + ", ".join(top_factors) if top_factors else "No risk drivers present.")
        ),
        top_factors=top_factors,
    )
