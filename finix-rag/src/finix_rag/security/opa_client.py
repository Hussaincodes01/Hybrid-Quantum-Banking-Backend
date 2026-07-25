"""OPA (Open Policy Agent) Client for centralized authorization.

Enforces RBAC + ABAC policies for document access control in the RAG system.
"""
import os
import logging
from typing import Optional, Any

import httpx
from pydantic import BaseModel

logger = logging.getLogger(__name__)


class AuthorizationRequest(BaseModel):
    """Input for OPA policy evaluation."""
    user_id: str
    role: str
    department: str
    clearance_level: int
    tenant_id: str
    action: str  # "read", "write", "query", "ingest", "admin"
    resource_type: str  # "document", "collection", "query_result", "system"
    resource_id: Optional[str] = None
    resource_classification: Optional[str] = None  # "public", "confidential", "restricted"
    time_of_day: Optional[str] = None
    ip_address: Optional[str] = None


class AuthorizationResponse(BaseModel):
    """OPA decision response."""
    allow: bool
    reason: str = ""
    conditions: dict = {}


class OPAAuthorizer:
    """Client for Open Policy Agent authorization decisions."""

    def __init__(
        self,
        opa_url: Optional[str] = None,
        policy_path: Optional[str] = None,
    ):
        self.opa_url = opa_url or os.getenv("OPA_URL", "http://127.0.0.1:8181")
        self.policy_path = policy_path or os.getenv(
            "OPA_POLICY_PATH", "/v1/data/finix/authz"
        )
        self._client = httpx.Client(timeout=5.0)

    def authorize(self, request: AuthorizationRequest) -> AuthorizationResponse:
        """Evaluate authorization request against OPA policies.

        Args:
            request: The authorization request with user/resource/action context.

        Returns:
            AuthorizationResponse with allow/deny decision and reasoning.
        """
        input_data = {
            "input": {
                "user": {
                    "id": request.user_id,
                    "role": request.role,
                    "department": request.department,
                    "clearance_level": request.clearance_level,
                    "tenant_id": request.tenant_id,
                },
                "action": request.action,
                "resource": {
                    "type": request.resource_type,
                    "id": request.resource_id,
                    "classification": request.resource_classification,
                },
                "environment": {
                    "time_of_day": request.time_of_day,
                    "ip_address": request.ip_address,
                },
            }
        }

        try:
            url = f"{self.opa_url}{self.policy_path}"
            response = self._client.post(url, json=input_data)
            response.raise_for_status()
            result = response.json().get("result", {})

            allow = result.get("allow", False)
            reason = result.get("reason", "No reason provided")
            conditions = result.get("conditions", {})

            logger.info(
                "OPA decision: user=%s action=%s resource=%s allow=%s",
                request.user_id, request.action, request.resource_type, allow,
            )
            return AuthorizationResponse(
                allow=allow, reason=reason, conditions=conditions
            )

        except httpx.HTTPError as e:
            logger.error("OPA request failed: %s", e)
            # Fail-closed: deny on OPA failure
            return AuthorizationResponse(
                allow=False, reason=f"OPA unavailable: {e}"
            )

    def check_query_access(
        self,
        user_id: str,
        role: str,
        department: str,
        clearance_level: int,
        tenant_id: str,
        query_text: str,
        ip_address: Optional[str] = None,
    ) -> AuthorizationResponse:
        """Convenience method for query authorization check."""
        # Classify query sensitivity
        classification = self._classify_query(query_text)

        return self.authorize(
            AuthorizationRequest(
                user_id=user_id,
                role=role,
                department=department,
                clearance_level=clearance_level,
                tenant_id=tenant_id,
                action="query",
                resource_type="query_result",
                resource_classification=classification,
                ip_address=ip_address,
            )
        )

    def check_document_access(
        self,
        user_id: str,
        role: str,
        department: str,
        clearance_level: int,
        tenant_id: str,
        document_id: str,
        action: str = "read",
    ) -> AuthorizationResponse:
        """Check if user can access a specific document."""
        return self.authorize(
            AuthorizationRequest(
                user_id=user_id,
                role=role,
                department=department,
                clearance_level=clearance_level,
                tenant_id=tenant_id,
                action=action,
                resource_type="document",
                resource_id=document_id,
            )
        )

    def check_ingestion_access(
        self,
        user_id: str,
        role: str,
        department: str,
        clearance_level: int,
        tenant_id: str,
    ) -> AuthorizationResponse:
        """Check if user can ingest documents."""
        return self.authorize(
            AuthorizationRequest(
                user_id=user_id,
                role=role,
                department=department,
                clearance_level=clearance_level,
                tenant_id=tenant_id,
                action="ingest",
                resource_type="collection",
            )
        )

    def _classify_query(self, query_text: str) -> str:
        """Classify query sensitivity level."""
        sensitive_keywords = [
            "salary", "account balance", "credit score", "loan amount",
            "password", "otp", "secret", "pin", "cvv", "ssn",
        ]
        restricted_keywords = [
            "internal audit", "compliance violation", "fraud",
            "money laundering", "suspicious activity", "investigation",
        ]

        query_lower = query_text.lower()
        for keyword in restricted_keywords:
            if keyword in query_lower:
                return "restricted"
        for keyword in sensitive_keywords:
            if keyword in query_lower:
                return "confidential"
        return "public"

    def close(self):
        """Close HTTP client."""
        self._client.close()
