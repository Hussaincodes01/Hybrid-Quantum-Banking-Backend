"""Local cryptographic primitives for defense-in-depth.

Provides AES-256-GCM, RSA-OAEP, and HMAC-SHA256 as fallback/acceleration
when Vault Transit is unavailable, or for client-side pre-encryption before
sending data to Vault.
"""
import os
import secrets
import hashlib
import logging
from typing import Optional

from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.asymmetric import rsa, padding
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.backends import default_backend

logger = logging.getLogger(__name__)


class AES256GCM:
    """AES-256-GCM authenticated encryption."""

    def __init__(self, key: Optional[bytes] = None):
        if key is None:
            key = AESGCM.generate_key(bit_length=256)
        if len(key) != 32:
            raise ValueError("AES-256-GCM requires a 32-byte key")
        self._key = key
        self._aesgcm = AESGCM(self._key)

    def encrypt(self, plaintext: bytes, associated_data: Optional[bytes] = None) -> bytes:
        """Encrypt with a random 96-bit nonce."""
        nonce = secrets.token_bytes(12)
        ciphertext = self._aesgcm.encrypt(nonce, plaintext, associated_data)
        return nonce + ciphertext

    def decrypt(self, data: bytes, associated_data: Optional[bytes] = None) -> bytes:
        """Decrypt; first 12 bytes are the nonce."""
        nonce = data[:12]
        ciphertext = data[12:]
        return self._aesgcm.decrypt(nonce, ciphertext, associated_data)

    @classmethod
    def from_password(cls, password: str, salt: Optional[bytes] = None) -> "AES256GCM":
        """Derive a 256-bit key from a password using HKDF-SHA256."""
        if salt is None:
            salt = secrets.token_bytes(16)
        hkdf = HKDF(
            algorithm=hashes.SHA256(),
            length=32,
            salt=salt,
            info=b"finix-rag-aes256gcm",
            backend=default_backend(),
        )
        key = hkdf.derive(password.encode())
        instance = cls(key)
        return instance

    @property
    def key(self) -> bytes:
        return self._key


class RSAEncryptor:
    """RSA-OAEP encryption for key wrapping and small payloads."""

    def __init__(self, private_key_pem: Optional[bytes] = None):
        if private_key_pem:
            self._private_key = serialization.load_pem_private_key(
                private_key_pem, password=None, backend=default_backend()
            )
        else:
            self._private_key = rsa.generate_private_key(
                public_exponent=65537,
                key_size=4096,
                backend=default_backend(),
            )
        self._public_key = self._private_key.public_key()

    def encrypt(self, plaintext: bytes) -> bytes:
        """RSA-OAEP encrypt (max 446 bytes for 4096-bit key)."""
        return self._public_key.encrypt(
            plaintext,
            padding.OAEP(
                mgf=padding.MGF1(algorithm=hashes.SHA256()),
                algorithm=hashes.SHA256(),
                label=None,
            ),
        )

    def decrypt(self, ciphertext: bytes) -> bytes:
        """RSA-OAEP decrypt."""
        return self._private_key.decrypt(
            ciphertext,
            padding.OAEP(
                mgf=padding.MGF1(algorithm=hashes.SHA256()),
                algorithm=hashes.SHA256(),
                label=None,
            ),
        )

    def export_public_key(self) -> bytes:
        """Export public key in PEM format."""
        return self._public_key.public_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PublicFormat.SubjectPublicKeyInfo,
        )

    def export_private_key(self) -> bytes:
        """Export private key in PEM format (protect!)."""
        return self._private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        )


class HMACValidator:
    """HMAC-SHA256 for data integrity verification.

    The key MUST be stable across processes: the ingestor signs each chunk and the
    query engine verifies it later, in a different process. A random per-instance key
    (the old default) made verification always fail, so every retrieved chunk was
    dropped as "failed integrity". When no key is given we derive a deterministic one
    from INTEGRITY_HMAC_KEY / API_SECRET_KEY so all instances agree.
    """

    def __init__(self, key: Optional[bytes] = None):
        if key is None:
            secret = (
                os.getenv("INTEGRITY_HMAC_KEY")
                or os.getenv("API_SECRET_KEY")
                or "finix-rag-default-integrity-key-change-me"
            )
            key = hashlib.sha256(f"finix-integrity:{secret}".encode()).digest()
        self._key = key

    def sign(self, data: bytes) -> bytes:
        """Create HMAC-SHA256 signature (real HMAC, not a prefix-MAC)."""
        import hmac as _hmac
        return _hmac.new(self._key, data, hashlib.sha256).digest()

    def verify(self, data: bytes, signature: bytes) -> bool:
        """Constant-time verify of HMAC signature."""
        expected = self.sign(data)
        return secrets.compare_digest(expected, signature)

    def sign_hex(self, data: bytes) -> str:
        """Create hex-encoded HMAC signature."""
        return self.sign(data).hex()

    def verify_hex(self, data: bytes, signature_hex: str) -> bool:
        """Verify hex-encoded HMAC signature."""
        return self.verify(data, bytes.fromhex(signature_hex))
