"""JWT Authentication and Token Management for FINIX RAG API."""
import os
import time
import uuid
import logging
from typing import Optional
from datetime import datetime, timedelta, timezone
from enum import Enum

from jose import JWTError, jwt
from pydantic import BaseModel

logger = logging.getLogger(__name__)


class UserRole(str, Enum):
    """Banking system roles."""
    ADMIN = "admin"
    ANALYST = "analyst"
    AUDITOR = "auditor"
    TELLER = "teller"
    READONLY = "readonly"


class TokenPayload(BaseModel):
    sub: str
    role: UserRole
    department: str
    clearance_level: int = 1
    tenant_id: str = "default"
    jti: str = ""
    exp: int = 0
    iat: int = 0


class TokenManager:
    """Create, validate, and revoke JWT tokens."""

    def __init__(
        self,
        secret_key: Optional[str] = None,
        algorithm: str = "HS256",
        expiry_minutes: int = 30,
    ):
        self.secret_key = secret_key if secret_key is not None else os.getenv(
            "API_SECRET_KEY", "change-this-to-a-strong-random-key"
        )
        self.algorithm = algorithm if algorithm is not None else os.getenv("JWT_ALGORITHM", "HS256")
        expiry_env = os.getenv("JWT_EXPIRY_MINUTES", "30")
        self.expiry_minutes = expiry_minutes if expiry_minutes is not None else int(expiry_env)
        self._revoked_tokens: set[str] = set()

    def create_token(
        self,
        user_id: str,
        role: UserRole = UserRole.READONLY,
        department: str = "general",
        clearance_level: int = 1,
        tenant_id: str = "default",
        custom_claims: Optional[dict] = None,
    ) -> str:
        """Create a signed JWT token."""
        now = int(time.time())
        jti = str(uuid.uuid4())
        payload = {
            "sub": user_id,
            "role": role.value,
            "department": department,
            "clearance_level": clearance_level,
            "tenant_id": tenant_id,
            "jti": jti,
            "iat": now,
            "exp": now + (self.expiry_minutes * 60),
        }
        if custom_claims:
            payload.update(custom_claims)
        token = jwt.encode(payload, self.secret_key, algorithm=self.algorithm)
        logger.info("Token created for user=%s role=%s jti=%s", user_id, role, jti)
        return token

    def validate_token(self, token: str) -> Optional[TokenPayload]:
        """Validate token and return payload, or None if invalid."""
        try:
            payload = jwt.decode(
                token, self.secret_key, algorithms=[self.algorithm]
            )
            if payload.get("jti") in self._revoked_tokens:
                logger.warning("Revoked token used: jti=%s", payload.get("jti"))
                return None
            return TokenPayload(**payload)
        except JWTError as e:
            logger.warning("Invalid token: %s", e)
            return None

    def revoke_token(self, jti: str):
        """Add token JTI to revocation list."""
        self._revoked_tokens.add(jti)
        logger.info("Token revoked: jti=%s", jti)

    def refresh_token(self, token: str) -> Optional[str]:
        """Issue a new token if the current one is valid."""
        payload = self.validate_token(token)
        if payload is None:
            return None
        self.revoke_token(payload.jti)
        return self.create_token(
            user_id=payload.sub,
            role=UserRole(payload.role),
            department=payload.department,
            clearance_level=payload.clearance_level,
            tenant_id=payload.tenant_id,
        )


class JWTAuthenticator:
    """FastAPI dependency for JWT authentication."""

    def __init__(self, token_manager: TokenManager):
        self.token_manager = token_manager

    def __call__(self, token: str) -> Optional[TokenPayload]:
        """Validate Bearer token from request."""
        if not token:
            return None
        # Strip "Bearer " prefix if present
        if token.startswith("Bearer "):
            token = token[7:]
        return self.token_manager.validate_token(token)
