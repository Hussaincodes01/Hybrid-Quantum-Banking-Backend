"""PII Detection and Masking for banking data protection.

Detects and masks sensitive information before embedding/storage to prevent
data leakage through vector embeddings.
"""
import re
import hashlib
import secrets
import logging
from typing import Optional
from dataclasses import dataclass, field

logger = logging.getLogger(__name__)


@dataclass
class PIIMatch:
    """A detected PII entity."""
    pii_type: str
    value: str
    start: int
    end: int
    token: str = ""


class PIIPatterns:
    """Regex patterns for Indian banking PII detection."""

    PATTERNS = {
        "aadhaar": re.compile(
            r"\b[2-9]\d{3}\s?\d{4}\s?\d{4}\b"  # 12-digit Aadhaar
        ),
        "pan": re.compile(
            r"\b[A-Z]{5}\d{4}[A-Z]\b"  # PAN card
        ),
        "ifsc": re.compile(
            r"\b[A-Z]{4}0[A-Z0-9]{6}\b"  # IFSC code
        ),
        "account_number": re.compile(
            r"\b(?:acct|account|a/c)[\s#:]*\d{9,18}\b", re.IGNORECASE
        ),
        "upi_id": re.compile(
            r"\b[\w.\-]+@[\w]+\b"  # UPI IDs
        ),
        "phone": re.compile(
            r"(?<!\d)[6-9]\d{9}(?!\d)"  # Indian phone numbers
        ),
        "email": re.compile(
            r"\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Z|a-z]{2,}\b"
        ),
        "ifsc_code": re.compile(
            r"\b[A-Z]{4}0[A-Z0-9]{6}\b"
        ),
        "gst_number": re.compile(
            r"\b\d{2}[A-Z]{5}\d{4}[A-Z]\d[Z][A-Z\d]\b"
        ),
        "transaction_id": re.compile(
            r"\b(?:UTR|REF|TXN)[\s#:]*[A-Z0-9]{12,22}\b", re.IGNORECASE
        ),
        "date_of_birth": re.compile(
            r"\b(?:DOB|dob|date.of.birth)[\s:=]*(\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4})\b",
            re.IGNORECASE,
        ),
        "credit_card": re.compile(
            r"\b(?:\d[ -]*?){13,19}\b"
        ),
        "cvv": re.compile(
            r"\b[Cc][Vv][Vv][\s:=]*\d{3,4}\b"
        ),
    }


class PIIMasker:
    """Detect and mask/tokenize PII in text before storage or embedding."""

    def __init__(self, strategy: str = "tokenize"):
        """
        Args:
            strategy: 'redact' replaces with type labels,
                      'tokenize' replaces with consistent fake tokens,
                      'hash' replaces with salted hashes.
        """
        self.strategy = strategy
        self.patterns = PIIPatterns.PATTERNS
        self._token_map: dict[str, str] = {}  # original -> token
        self._reverse_map: dict[str, str] = {}  # token -> original

    def detect(self, text: str) -> list[PIIMatch]:
        """Detect all PII entities in text."""
        matches = []
        for pii_type, pattern in self.patterns.items():
            for match in pattern.finditer(text):
                value = match.group()
                # Deduplicate overlapping matches
                if not any(
                    m.start <= match.start() and m.end >= match.end()
                    for m in matches
                ):
                    token = self._generate_token(pii_type, value)
                    matches.append(
                        PIIMatch(
                            pii_type=pii_type,
                            value=value,
                            start=match.start(),
                            end=match.end(),
                            token=token,
                        )
                    )
        return sorted(matches, key=lambda m: m.start)

    def mask(self, text: str) -> str:
        """Mask all PII in text using configured strategy."""
        matches = self.detect(text)
        if not matches:
            return text

        result = []
        last_end = 0
        for match in matches:
            result.append(text[last_end : match.start])
            if self.strategy == "redact":
                result.append(f"[{match.pii_type.upper()}]")
            elif self.strategy == "tokenize":
                result.append(match.token)
                self._token_map[match.value] = match.token
                self._reverse_map[match.token] = match.value
            elif self.strategy == "hash":
                salted = hashlib.sha256(
                    (secrets.token_hex(8) + match.value).encode()
                ).hexdigest()[:12]
                result.append(f"HASH_{match.pii_type}_{salted}")
            last_end = match.end
        result.append(text[last_end:])

        logger.debug("Masked %d PII entities", len(matches))
        return "".join(result)

    def unmask(self, masked_text: str) -> str:
        """Restore original PII from tokenized text."""
        if self.strategy != "tokenize":
            raise ValueError("Unmasking only supported for 'tokenize' strategy")
        result = masked_text
        for token, original in self._reverse_map.items():
            result = result.replace(token, original)
        return result

    def _generate_token(self, pii_type: str, value: str) -> str:
        """Generate a deterministic token for a PII value."""
        if value in self._token_map:
            return self._token_map[value]
        prefix = pii_type[:3].upper()
        rand_part = secrets.token_hex(4).upper()
        return f"[{prefix}_{rand_part}]"

    def is_pii_free(self, text: str) -> bool:
        """Check if text contains any PII."""
        return len(self.detect(text)) == 0

    @property
    def stats(self) -> dict:
        """Return masking statistics."""
        return {
            "total_masked": len(self._token_map),
            "strategy": self.strategy,
            "token_count": len(self._reverse_map),
        }


def get_pii_masker(strategy: str = "tokenize") -> PIIMasker:
    """Create a fresh per-request PII masker (no shared token map)."""
    return PIIMasker(strategy=strategy)
