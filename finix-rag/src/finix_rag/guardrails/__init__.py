"""NeMo Guardrails Integration for FINIX RAG.

Provides safety rails for banking domain AI interactions:
- Input validation (jailbreak, off-topic, sensitive data)
- Output validation (PII, financial advice, system info)
- Hallucination detection
- Compliance with RBI guidelines
"""
import os
import re
import logging
from typing import Optional
from dataclasses import dataclass
from pathlib import Path

logger = logging.getLogger(__name__)

# Lazy import to avoid startup overhead
_rails_app = None
_rails_initialized = False


@dataclass
class GuardrailResult:
    """Result from guardrails check."""
    passed: bool
    message: str
    intent: str
    confidence: float
    details: dict = None

    def __post_init__(self):
        if self.details is None:
            self.details = {}


def _get_config_path() -> str:
    """Get the path to the guardrails configuration."""
    # Look for config in the project root
    project_root = Path(__file__).parent.parent.parent.parent
    config_path = project_root / "config" / "guardrails"
    if config_path.exists():
        return str(config_path)
    
    # Fallback: look relative to this file
    alt_path = Path(__file__).parent.parent.parent / "config" / "guardrails"
    if alt_path.exists():
        return str(alt_path)
    
    logger.warning("Guardrails config not found, using default")
    return ""


def _init_rails():
    """Initialize NeMo Guardrails with banking domain configuration."""
    global _rails_app, _rails_initialized
    
    if _rails_initialized:
        return
    
    try:
        from nemoguardrails import RailsConfig, LLMRails
        
        config_path = _get_config_path()
        if not config_path:
            logger.warning("NeMo Guardrails config not found, using programmatic guardrails")
            _rails_initialized = True
            return
        
        # Load configuration from YAML
        config = RailsConfig.from_path(config_path)
        
        # Set Groq API key from environment
        os.environ.setdefault("OPENAI_API_KEY", os.getenv("GROQ_API_KEY", ""))
        
        _rails_app = LLMRails(config)
        _rails_initialized = True
        logger.info("NeMo Guardrails initialized successfully")
        
    except ImportError:
        # Check if we're in production - if so, fail loud unless explicitly allowed
        if os.getenv("FINIX_ENV", "").lower() in ("prod", "production"):
            if os.getenv("FINIX_ALLOW_GUARDRAILS_FALLBACK", "").lower() not in ("true", "1", "yes"):
                raise RuntimeError(
                    "nemoguardrails not installed but required in production. "
                    "Install with 'pip install nemoguardrails' or set FINIX_ALLOW_GUARDRAILS_FALLBACK=true to use programmatic fallback."
                )
        logger.warning("nemoguardrails not installed, using programmatic guardrails (set FINIX_ALLOW_GUARDRAILS_FALLBACK=true to acknowledge)")
        _rails_initialized = True
    except Exception as e:
        logger.error("Failed to initialize NeMo Guardrails: %s", e)
        _rails_initialized = True


# ── Programmatic Guardrails (Fallback when NeMo is not available) ────────────

# Patterns that indicate jailbreak attempts
_JAILBREAK_PATTERNS = [
    re.compile(r"ignore\s+(?:all\s+)?(?:previous|prior|above)\s+instructions?", re.IGNORECASE),
    re.compile(r"you\s+are\s+now\s+(?:DAN|a\s+hacker|unrestricted)", re.IGNORECASE),
    re.compile(r"pretend\s+you\s+(?:are|have)\s+no\s+restrictions?", re.IGNORECASE),
    re.compile(r"bypass\s+(?:your\s+)?(?:safety|security)\s+filters?", re.IGNORECASE),
    re.compile(r"act\s+as\s+if\s+you\s+(?:have|are)\s+no\s+(?:restrictions?|rules?)", re.IGNORECASE),
    re.compile(r"do\s+not\s+follow\s+(?:any|your)\s+(?:rules?|guidelines?)", re.IGNORECASE),
    re.compile(r"disregard\s+(?:all|any)\s+(?:previous|prior)\s+(?:instructions?|rules?)", re.IGNORECASE),
    re.compile(r"now\s+you\s+(?:are|will)\s+(?:be|act)", re.IGNORECASE),
    re.compile(r"enter\s+(?:developer|debug|admin)\s+mode", re.IGNORECASE),
    re.compile(r"output\s+(?:your|the)\s+(?:system|initial)\s+prompt", re.IGNORECASE),
]

# Patterns that indicate requests for system information
_SYSTEM_INFO_PATTERNS = [
    re.compile(r"(?:what|show|tell|give)\s+(?:is|are|me)\s+(?:the\s+)?(?:api|secret|password|key|token)", re.IGNORECASE),
    re.compile(r"(?:how|what)\s+(?:does|is)\s+(?:the\s+)?(?:authentication|auth|login)\s+work", re.IGNORECASE),
    re.compile(r"(?:what|which)\s+(?:database|db|vector\s*store)\s+(?:do|are)\s+you\s+use", re.IGNORECASE),
    re.compile(r"(?:show|display|reveal)\s+(?:me\s+)?(?:the\s+)?source\s+code", re.IGNORECASE),
    re.compile(r"(?:what|where)\s+(?:is|are)\s+(?:your|the)\s+(?:host|server|endpoint)", re.IGNORECASE),
    re.compile(r"(?:how|what)\s+(?:do\s+you|does\s+the)\s+(?:store|save|process)", re.IGNORECASE),
]

# Patterns that indicate investment advice requests
_FINANCIAL_ADVICE_PATTERNS = [
    re.compile(r"(?:should|could|would)\s+(?:I|we)\s+(?:invest|buy|sell|hold)", re.IGNORECASE),
    re.compile(r"(?:which|what)\s+.*?(?:best|good|better)\s+(?:investment|mutual\s+fund|stock|etf)", re.IGNORECASE),
    re.compile(r"(?:which|what)\s+(?:is\s+the\s+)?(?:best|good|better)\s+(?:investment|mutual\s+fund|stock|etf)", re.IGNORECASE),
    re.compile(r"(?:how|what)\s+(?:should|could|would)\s+(?:I|we)\s+(?:diversify|allocate|distribute)", re.IGNORECASE),
    re.compile(r"(?:give|provide|tell)\s+(?:me\s+)?(?:investment|financial|portfolio)\s+advice", re.IGNORECASE),
    re.compile(r"(?:recommend|suggest)\s+(?:a\s+)?(?:stock|fund|investment|strategy)", re.IGNORECASE),
    re.compile(r"(?:mutual\s+fund|stock|etf|investment)\s+.*?(?:best|good|better|worth)", re.IGNORECASE),
]

# Patterns that indicate PII requests
_PII_REQUEST_PATTERNS = [
    re.compile(r"(?:show|display|reveal|give|what)\s+(?:is|are|me)?\s*(?:the\s+)?(?:customer|user|account)\s+(?:data|details|info|number)", re.IGNORECASE),
    re.compile(r"(?:what|show)\s+(?:is|are)\s+(?:the\s+)?(?:account|pan|aadhaar|phone|email)\s+(?:number|id|details)?", re.IGNORECASE),
    re.compile(r"(?:give|show|display)\s+(?:me\s+)?(?:the\s+)?(?:pan|aadhaar|account)\s+(?:details|number|info)", re.IGNORECASE),
    re.compile(r"(?:show|display)\s+(?:me\s+)?(?:transaction|payment)\s+history\s+(?:for|of)\s+(?:user|account|customer)", re.IGNORECASE),
    re.compile(r"(?:give|provide)\s+(?:me\s+)?(?:the\s+)?(?:ssn|social\s+security|tax\s+id)", re.IGNORECASE),
    re.compile(r"(?:what|which)\s+(?:is|are)\s+.*?(?:account|pan|aadhaar|phone)\s+(?:number|id|details)", re.IGNORECASE),
]

# Off-topic patterns (banking domain)
_OFF_TOPIC_PATTERNS = [
    re.compile(r"(?:what|how|tell|explain)\s+(?:is|are|about)\s+(?:the\s+)?(?:weather|temperature|forecast)", re.IGNORECASE),
    re.compile(r"(?:tell|say|share)\s+(?:me\s+)?(?:a\s+)?(?:joke|story|poem)", re.IGNORECASE),
    re.compile(r"(?:who|what)\s+(?:won|scored|played)\s+.*?(?:match|game|race)", re.IGNORECASE),
    re.compile(r"(?:write|create|generate)\s+(?:a\s+)?(?:essay|article|blog\s+post)", re.IGNORECASE),
    re.compile(r"(?:translate|convert)\s+(?:this|it|the)\s+(?:to|into)\s+(?:spanish|french|german|hindi)", re.IGNORECASE),
    re.compile(r"(?:who|what)\s+(?:is\s+the\s+)?(?:president|prime\s+minister|ceo)\s+(?:of|for)", re.IGNORECASE),
]


def _check_jailbreak(query: str) -> GuardrailResult:
    """Check for jailbreak attempts."""
    for pattern in _JAILBREAK_PATTERNS:
        if pattern.search(query):
            return GuardrailResult(
                passed=False,
                message="Jailbreak attempt detected. This request violates our security policies.",
                intent="jailbreak",
                confidence=0.95,
                details={"pattern": pattern.pattern}
            )
    return GuardrailResult(passed=True, message="OK", intent="safe", confidence=0.0)


def _check_system_info(query: str) -> GuardrailResult:
    """Check for system information requests."""
    for pattern in _SYSTEM_INFO_PATTERNS:
        if pattern.search(query):
            return GuardrailResult(
                passed=False,
                message="I cannot discuss internal system details, security configurations, or technical architecture. This information is confidential.",
                intent="system_info",
                confidence=0.90,
                details={"pattern": pattern.pattern}
            )
    return GuardrailResult(passed=True, message="OK", intent="safe", confidence=0.0)


def _check_financial_advice(query: str) -> GuardrailResult:
    """Check for investment advice requests."""
    for pattern in _FINANCIAL_ADVICE_PATTERNS:
        if pattern.search(query):
            return GuardrailResult(
                passed=False,
                message="I cannot provide investment advice or financial recommendations. I can only share factual information about banking regulations and FINIX platform features.",
                intent="financial_advice",
                confidence=0.85,
                details={"pattern": pattern.pattern}
            )
    return GuardrailResult(passed=True, message="OK", intent="safe", confidence=0.0)


def _check_pii_request(query: str) -> GuardrailResult:
    """Check for PII data requests."""
    for pattern in _PII_REQUEST_PATTERNS:
        if pattern.search(query):
            return GuardrailResult(
                passed=False,
                message="I cannot share or discuss personal identifiable information (PII), account details, or customer data. This is protected under our privacy policy.",
                intent="pii_request",
                confidence=0.90,
                details={"pattern": pattern.pattern}
            )
    return GuardrailResult(passed=True, message="OK", intent="safe", confidence=0.0)


def _check_off_topic(query: str) -> GuardrailResult:
    """Check for off-topic queries."""
    for pattern in _OFF_TOPIC_PATTERNS:
        if pattern.search(query):
            return GuardrailResult(
                passed=False,
                message="I can only help with questions about banking, finance, and the FINIX platform. Please ask about relevant topics.",
                intent="off_topic",
                confidence=0.80,
                details={"pattern": pattern.pattern}
            )
    return GuardrailResult(passed=True, message="OK", intent="safe", confidence=0.0)


# ── Output Validation ──────────────────────────────────────────────────────────

# PII patterns that should not appear in responses
_PII_PATTERNS = [
    (re.compile(r"\b\d{4}[\s-]?\d{4}[\s-]?\d{4}\b"), "aadhaar"),  # 12-digit Aadhaar
    (re.compile(r"\b[A-Z]{5}\d{4}[A-Z]\b"), "pan"),  # PAN card
    (re.compile(r"\b(?:\d[ -]*?){13,19}\b"), "credit_card"),  # Credit card
    (re.compile(r"\b[6-9]\d{9}\b"), "phone"),  # Indian phone
    (re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b"), "email"),
]

# System information patterns
_SYSTEM_INFO_RESPONSE_PATTERNS = [
    re.compile(r"(?:api|secret|private)\s+(?:key|token|password)", re.IGNORECASE),
    re.compile(r"(?:postgresql|mysql|mongodb|qdrant|redis)\s+(?:at|on|port)", re.IGNORECASE),
    re.compile(r"(?:127\.0\.0\.1|localhost):\d+", re.IGNORECASE),
    re.compile(r"(?:password|pwd)\s*[=:]\s*\S+", re.IGNORECASE),
]


def _check_output_pii(response: str) -> GuardrailResult:
    """Check if response contains PII."""
    found_pii = []
    for pattern, pii_type in _PII_PATTERNS:
        if pattern.search(response):
            found_pii.append(pii_type)
    
    if found_pii:
        return GuardrailResult(
            passed=False,
            message="Response contains personal identifiable information (PII) which has been redacted.",
            intent="pii_leak",
            confidence=0.95,
            details={"pii_types": found_pii}
        )
    return GuardrailResult(passed=True, message="OK", intent="safe", confidence=0.0)


def _check_output_system_info(response: str) -> GuardrailResult:
    """Check if response contains system information."""
    for pattern in _SYSTEM_INFO_RESPONSE_PATTERNS:
        if pattern.search(response):
            return GuardrailResult(
                passed=False,
                message="Response contains internal system information which has been redacted.",
                intent="system_info_leak",
                confidence=0.90,
                details={"pattern": pattern.pattern}
            )
    return GuardrailResult(passed=True, message="OK", intent="safe", confidence=0.0)


def _check_output_financial_advice(response: str) -> GuardrailResult:
    """Check if response contains financial advice.

    Uses contextual patterns, not isolated words — words like 'lend', 'buy',
    'sell' appear in regulatory facts but don't constitute advice.  Advisory
    patterns address the USER directly ("you should buy", "I recommend").
    """
    # Only flag direct advice addressed to the user
    advisory_patterns = [
        re.compile(r"you\s+should\s+(?:invest|buy|sell|hold|diversify|allocate)", re.IGNORECASE),
        re.compile(r"you\s+could\s+(?:invest|buy|sell|hold|diversify|allocate)", re.IGNORECASE),
        re.compile(r"you\s+would\s+(?:benefit|gain|earn|save)", re.IGNORECASE),
        re.compile(r"i\s+recommend\s+(?:that\s+)?you", re.IGNORECASE),
        re.compile(r"i\s+suggest\s+(?:that\s+)?you", re.IGNORECASE),
        re.compile(r"my\s+(?:advice|recommendation|suggestion)\s+is", re.IGNORECASE),
        re.compile(r"best\s+(?:time|strategy|approach)\s+to\s+(?:invest|buy|sell)", re.IGNORECASE),
        re.compile(r"consider\s+(?:investing|buying|selling|diversifying)", re.IGNORECASE),
        re.compile(r"high\s+return\s+(?:guaranteed|promised|expected)", re.IGNORECASE),
    ]
    
    for pattern in advisory_patterns:
        if pattern.search(response):
            return GuardrailResult(
                passed=False,
                message="Response may contain financial advice. Please rephrase to be more factual.",
                intent="financial_advice_leak",
                confidence=0.70,
                details={"pattern": pattern.pattern}
            )
    return GuardrailResult(passed=True, message="OK", intent="safe", confidence=0.0)


# ── Main Guardrails Class ─────────────────────────────────────────────────────

class FINIXGuardrails:
    """Guardrails for FINIX banking RAG system.
    
    Provides input and output validation to ensure safe, compliant responses.
    """
    
    def __init__(self, use_nemo: bool = True):
        """
        Args:
            use_nemo: Try to use NeMo Guardrails; fall back to programmatic.
        """
        self.use_nemo = use_nemo
        if use_nemo:
            _init_rails()
        
        logger.info("FINIXGuardrails initialized (nemo=%s)", use_nemo)
    
    def validate_input(self, query: str) -> GuardrailResult:
        """Validate user input before processing.
        
        Runs all input guardrails in sequence. Returns first failure,
        or success if all checks pass.
        
        Args:
            query: User's query text.
            
        Returns:
            GuardrailResult with pass/fail and message.
        """
        # Run all input checks
        checks = [
            _check_jailbreak(query),
            _check_system_info(query),
            _check_financial_advice(query),
            _check_pii_request(query),
            _check_off_topic(query),
        ]
        
        # Return first failure
        for result in checks:
            if not result.passed:
                logger.warning(
                    "Input guardrail blocked: intent=%s conf=%.2f query=%s",
                    result.intent, result.confidence, query[:100],
                )
                return result
        
        return GuardrailResult(passed=True, message="OK", intent="safe", confidence=1.0)
    
    def validate_output(self, response: str, query: str = "") -> GuardrailResult:
        """Validate bot response before sending to user.
        
        Checks for PII, system information, and financial advice.
        
        Args:
            response: Bot's response text.
            query: Original user query (for context).
            
        Returns:
            GuardrailResult with pass/fail and sanitized response.
        """
        # Run all output checks
        checks = [
            _check_output_pii(response),
            _check_output_system_info(response),
            _check_output_financial_advice(response),
        ]
        
        # Return first failure
        for result in checks:
            if not result.passed:
                logger.warning(
                    "Output guardrail blocked: intent=%s conf=%.2f",
                    result.intent, result.confidence,
                )
                return result
        
        return GuardrailResult(passed=True, message=response, intent="safe", confidence=1.0)
    
    async def check_with_nemo(self, query: str) -> Optional[str]:
        """Use NeMo Guardrails for advanced checking (if available).
        
        Args:
            query: User's query text.
            
        Returns:
            Guardrailed response or None if NeMo is not available.
        """
        if not self.use_nemo or _rails_app is None:
            return None
        
        try:
            response = await _rails_app.generate_async(messages=[{"role": "user", "content": query}])
            return response.get("content", "")
        except Exception as e:
            logger.error("NeMo Guardrails error: %s", e)
            return None
