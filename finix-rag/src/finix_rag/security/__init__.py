"""FINIX RAG - Core Security Module"""
from finix_rag.security.crypto import AES256GCM, RSAEncryptor, HMACValidator
from finix_rag.security.auth import JWTAuthenticator, TokenManager
from finix_rag.security.pii import PIIMasker
from finix_rag.security.opa_client import OPAAuthorizer

# ponytail: VaultEncryption imported lazily — hvac connects on import,
# which fails when Vault is not running. Import it only where needed.
def get_vault_encryption(*args, **kwargs):
    from finix_rag.security.vault_client import VaultEncryption
    return VaultEncryption(*args, **kwargs)

__all__ = [
    "AES256GCM",
    "RSAEncryptor",
    "HMACValidator",
    "JWTAuthenticator",
    "TokenManager",
    "PIIMasker",
    "OPAAuthorizer",
    "get_vault_encryption",
]
