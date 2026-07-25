# FINIX RAG - OPA Authorization Policies
# RBAC + ABAC for banking document access control
#
# Roles: admin, analyst, auditor, teller, readonly
# Classifications: public, internal, confidential, restricted
# Actions: read, write, query, ingest, admin

package finix.authz

default allow = false

# ============================================================
# Role-Based Access Control (RBAC)
# ============================================================

# Role clearance levels
role_clearance := {
    "admin": 4,
    "analyst": 3,
    "auditor": 3,
    "teller": 2,
    "readonly": 1,
}

# ============================================================
# Query Authorization Rules
# ============================================================

# Admin can query anything
allow {
    input.user.role == "admin"
    input.action == "query"
}

# Analyst can query public, internal, and confidential documents
allow {
    input.user.role == "analyst"
    input.action == "query"
    input.resource.classification == "public"
}

allow {
    input.user.role == "analyst"
    input.action == "query"
    input.resource.classification == "internal"
}

allow {
    input.user.role == "analyst"
    input.action == "query"
    input.resource.classification == "confidential"
    input.user.clearance_level >= 3
}

# Auditor can query all classifications for compliance review
allow {
    input.user.role == "auditor"
    input.action == "query"
    input.user.clearance_level >= 3
}

# Teller can query public and internal documents only
allow {
    input.user.role == "teller"
    input.action == "query"
    input.resource.classification == "public"
}

allow {
    input.user.role == "teller"
    input.action == "query"
    input.resource.classification == "internal"
}

# Readonly can only query public documents
allow {
    input.user.role == "readonly"
    input.action == "query"
    input.resource.classification == "public"
}

# ============================================================
# Document Read Authorization
# ============================================================

# All authenticated users can read public documents
allow {
    input.action == "read"
    input.resource.classification == "public"
    _valid_role
}

# Internal documents: teller, analyst, auditor, admin
allow {
    input.action == "read"
    input.resource.classification == "internal"
    _role_in(["teller", "analyst", "auditor", "admin"])
}

# Confidential documents: analyst, auditor, admin (with clearance)
allow {
    input.action == "read"
    input.resource.classification == "confidential"
    input.user.clearance_level >= 3
    _role_in(["analyst", "auditor", "admin"])
}

# Restricted documents: admin only with clearance
allow {
    input.action == "read"
    input.resource.classification == "restricted"
    input.user.clearance_level >= 4
    input.user.role == "admin"
}

# ============================================================
# Document Ingestion Authorization
# ============================================================

# Only admin and analyst can ingest documents
allow {
    input.action == "ingest"
    _role_in(["admin", "analyst"])
}

# ============================================================
# Write/Admin Authorization
# ============================================================

# Only admin can write or perform admin actions
allow {
    input.action == "write"
    input.user.role == "admin"
}

allow {
    input.action == "admin"
    input.user.role == "admin"
}

# ============================================================
# ABAC - Time-Based Access Control
# ============================================================

# Restricted queries only allowed during business hours (9 AM - 6 PM IST)
deny_restricted_hours {
    input.resource.classification == "restricted"
    _is_outside_business_hours
}

# Deny restricted access outside business hours
allow {
    input.action == "query"
    input.resource.classification == "restricted"
    not _is_outside_business_hours
    input.user.role == "admin"
}

# ============================================================
# Tenant Isolation (Multi-Tenancy)
# ============================================================

# Users can only access documents within their tenant
allow {
    input.user.tenant_id == input.resource.tenant_id
    _valid_role
    input.action == "read"
}

# Admin can access cross-tenant with explicit flag
allow {
    input.user.role == "admin"
    input.action == "read"
    input.resource.cross_tenant_access == true
}

# ============================================================
# Helper Rules
# ============================================================

_valid_role {
    role_clearance[input.user.role]
}

_role_in(roles) {
    roles[_] == input.user.role
}

_is_outside_business_hours {
    # Simplified: flag set by application layer
    input.environment.time_of_day == "outside_business_hours"
}

# ============================================================
# Deny Rules (override any allow)
# ============================================================

# Always deny if user is locked out
deny {
    input.user.account_locked == true
}

# Always deny if IP is in blocked range
deny {
    _ip_blocked
}

_ip_blocked {
    startswith(input.environment.ip_address, "0.0.0.")
}
