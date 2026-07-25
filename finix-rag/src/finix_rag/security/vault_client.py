"""HashiCorp Vault Integration - Transit Engine for Encryption-as-a-Service.

Uses Vault's Transit secrets engine so the application never holds raw keys.
All encrypt/decrypt calls go through Vault over TLS.
"""
import os
import base64
import logging
from typing import Optional
from tenacity import retry, stop_after_attempt, wait_exponential

import hvac
from cryptography.fernet import Fernet

logger = logging.getLogger(__name__)


class VaultEncryption:
    """Vault Transit engine client for AES-256-GCM encryption."""

    def __init__(
        self,
        url: Optional[str] = None,
        token: Optional[str] = None,
        key_name: Optional[str] = None,
        mount_point: str = "transit",
    ):
        self.url = url or os.getenv("VAULT_ADDR", "http://127.0.0.1:8200")
        self.token = token or os.getenv("VAULT_TOKEN", "dev-only-token")
        self.key_name = key_name or os.getenv("VAULT_TRANSIT_KEY", "finix-rag-key")
        self.mount_point = mount_point
        self._client: Optional[hvac.Client] = None
        self._connect()

    def _connect(self):
        """Establish connection to Vault server."""
        try:
            self._client = hvac.Client(url=self.url, token=self.token)
            if self._client.is_authenticated():
                logger.info("Connected to Vault at %s", self.url)
                self._ensure_key_exists()
            else:
                logger.error("Vault authentication failed")
                raise ConnectionError("Vault authentication failed")
        except Exception as e:
            logger.error("Failed to connect to Vault: %s", e)
            raise

    def _ensure_transit_mounted(self):
        """Enable the transit secrets engine if it isn't mounted yet.

        Vault -dev does NOT enable transit by default, so without this the very first
        create_or_update_key() call 404s and the app silently drops to no-Vault mode.
        """
        try:
            mounts = self._client.sys.list_mounted_secrets_engines()
            if f"{self.mount_point}/" in mounts:
                return
        except Exception as e:
            logger.debug("Could not list secrets engines (continuing): %s", e)
        try:
            self._client.sys.enable_secrets_engine(
                backend_type="transit", path=self.mount_point
            )
            logger.info("Enabled transit secrets engine at '%s/'", self.mount_point)
        except Exception as e:
            # 400 "path is already in use" is fine (race / already enabled).
            logger.debug("enable transit engine: %s", e)

    def _ensure_key_exists(self):
        """Create transit key if it doesn't exist."""
        self._ensure_transit_mounted()
        try:
            self._client.secrets.transit.read_key(
                name=self.key_name, mount_point=self.mount_point
            )
            logger.info("Transit key '%s' already exists", self.key_name)
        except Exception:
            logger.info("Creating transit key '%s'", self.key_name)
            # hvac 2.x exposes create_key (not create_or_update_key).
            self._client.secrets.transit.create_key(
                name=self.key_name,
                mount_point=self.mount_point,
                key_type="aes256-gcm96",
                auto_rotate_period="720h",  # 30 days
                exportable=False,
            )

    @retry(stop=stop_after_attempt(3), wait=wait_exponential(min=1, max=10))
    def encrypt(self, plaintext: str) -> str:
        """Encrypt plaintext using Vault Transit engine.

        Args:
            plaintext: The string to encrypt.

        Returns:
            Vault-encoded ciphertext string (starts with 'vault:v1:').
        """
        encoded = base64.b64encode(plaintext.encode("utf-8")).decode("utf-8")
        response = self._client.secrets.transit.encrypt_data(
            name=self.key_name,
            plaintext=encoded,
            mount_point=self.mount_point,
        )
        ciphertext = response["data"]["ciphertext"]
        logger.debug("Encrypted data with key '%s'", self.key_name)
        return ciphertext

    @retry(stop=stop_after_attempt(3), wait=wait_exponential(min=1, max=10))
    def decrypt(self, ciphertext: str) -> str:
        """Decrypt ciphertext using Vault Transit engine.

        Args:
            ciphertext: Vault-encoded ciphertext to decrypt.

        Returns:
            Decrypted plaintext string.
        """
        response = self._client.secrets.transit.decrypt_data(
            name=self.key_name,
            ciphertext=ciphertext,
            mount_point=self.mount_point,
        )
        plaintext = base64.b64decode(response["data"]["plaintext"]).decode("utf-8")
        logger.debug("Decrypted data with key '%s'", self.key_name)
        return plaintext

    def encrypt_batch(self, plaintexts: list[str]) -> list[str]:
        """Encrypt multiple plaintexts in a single Vault call."""
        batch = [
            {"plaintext": base64.b64encode(p.encode()).decode()}
            for p in plaintexts
        ]
        response = self._client.secrets.transit.encrypt_data(
            name=self.key_name,
            batch_input=batch,
            mount_point=self.mount_point,
        )
        return [item["ciphertext"] for item in response["data"]["batch_results"]]

    def decrypt_batch(self, ciphertexts: list[str]) -> list[str]:
        """Decrypt multiple ciphertexts in a single Vault call."""
        batch = [{"ciphertext": ct} for ct in ciphertexts]
        response = self._client.secrets.transit.decrypt_data(
            name=self.key_name,
            batch_input=batch,
            mount_point=self.mount_point,
        )
        return [
            base64.b64decode(item["plaintext"]).decode("utf-8")
            for item in response["data"]["batch_results"]
        ]

    def rotate_key(self):
        """Rotate the transit encryption key."""
        self._client.secrets.transit.rotate_key(
            name=self.key_name, mount_point=self.mount_point
        )
        logger.info("Rotated key '%s'", self.key_name)

    def get_store_token(self, data: str) -> str:
        """Wrap sensitive data for secure transport (response wrapping)."""
        return self._client.secrets.transit.encrypt_data(
            name=self.key_name,
            plaintext=base64.b64encode(data.encode()).decode(),
            type="aes256-gcm96",
            mount_point=self.mount_point,
        )["data"]["ciphertext"]

    @property
    def is_healthy(self) -> bool:
        """Check Vault connection health."""
        try:
            return self._client.is_authenticated()
        except Exception:
            return False
