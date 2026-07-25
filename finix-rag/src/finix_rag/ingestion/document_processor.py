"""Document Processor - Metadata extraction, cleaning, and standardization.

Transforms raw OCR output into clean, structured documents ready for embedding.
Uses larger chunk sizes suited to financial documents where numbers need context.
"""
import re
import hashlib
import logging
from datetime import datetime
from typing import Optional
from dataclasses import dataclass, field

logger = logging.getLogger(__name__)

@dataclass
class ProcessedDocument:
    """Cleaned and structured document ready for chunking."""
    doc_id: str
    title: str
    text: str
    metadata: dict
    tables: list[dict]
    chunks: list[dict] = field(default_factory=list)
    classification: str = "internal"
    created_at: str = ""
    checksum: str = ""


class DocumentProcessor:
    """Clean, normalize, and extract metadata from OCR'd financial documents.

    Chunking strategy: 800-char chunks with 150-char overlap. Financial documents
    contain dense numbers that need surrounding context to be meaningful. The
    larger chunk size ensures figures stay with their labels and explanations.
    Paragraph boundaries are still respected — we never split mid-paragraph.
    """

    def __init__(self, chunk_size: int = 800, chunk_overlap: int = 150):
        self.chunk_size = chunk_size
        self.chunk_overlap = chunk_overlap

    def process(
        self,
        text: str,
        tables: list[dict],
        source_file: str,
        extra_metadata: Optional[dict] = None,
    ) -> ProcessedDocument:
        """Full processing pipeline for an OCR'd document.

        Args:
            text: Raw OCR text.
            tables: Extracted table data.
            source_file: Original filename.
            extra_metadata: Additional metadata to attach.

        Returns:
            ProcessedDocument ready for ingestion.
        """
        # Clean text
        cleaned_text = self._clean_text(text)

        # Extract metadata
        metadata = self._extract_metadata(cleaned_text, source_file)
        if extra_metadata:
            metadata.update(extra_metadata)

        # Generate deterministic doc ID
        doc_id = self._generate_doc_id(source_file, cleaned_text)

        # Extract title
        title = self._extract_title(cleaned_text, source_file)

        # Classify document sensitivity
        classification = self._classify_document(cleaned_text)

        # Semantic chunking
        chunks = self._semantic_chunk(cleaned_text, metadata)

        # Compute checksum
        checksum = hashlib.sha256(cleaned_text.encode()).hexdigest()

        doc = ProcessedDocument(
            doc_id=doc_id,
            title=title,
            text=cleaned_text,
            metadata=metadata,
            tables=tables,
            chunks=chunks,
            classification=classification,
            created_at=datetime.utcnow().isoformat(),
            checksum=checksum,
        )

        logger.info(
            "Processed document: id=%s title='%s' chunks=%d classification=%s",
            doc_id, title[:50], len(chunks), classification,
        )
        return doc

    def _clean_text(self, text: str) -> str:
        """Clean OCR artifacts and normalize text.

        Whitespace is normalized WITHOUT collapsing newlines: paragraph breaks must
        survive so `_extract_title` can find the first line and `_semantic_chunk` can
        split on blank lines.
        """
        # Remove common OCR artifacts
        text = re.sub(r"[|]", " ", text)          # Pipe characters from table borders
        text = re.sub(r"[^\S\n]+", " ", text)      # Collapse spaces/tabs, keep newlines
        text = re.sub(r"[ \t]*\n[ \t]*", "\n", text)  # Trim spaces around newlines
        text = re.sub(r"\n{3,}", "\n\n", text)     # Normalize blank-line runs to one
        # Remove page headers/footers patterns
        text = re.sub(r"Page \d+ of \d+", "", text, flags=re.IGNORECASE)
        text = re.sub(r"Confidential\s*\|\s*Internal", "[CLASSIFIED]", text, flags=re.IGNORECASE)
        # Normalize dates
        text = re.sub(
            r"(\d{1,2})[/\-](\d{1,2})[/\-](\d{2,4})",
            r"\1-\2-\3",
            text,
        )
        return text.strip()

    def _extract_metadata(self, text: str, source_file: str) -> dict:
        """Extract structured metadata from document text."""
        metadata = {
            "source_file": source_file,
            "file_type": source_file.split(".")[-1].lower(),
        }

        # Extract document date if present
        date_patterns = [
            (r"(?:date|dated|as of)[:\s]*(\d{1,2}[-/]\d{1,2}[-/]\d{2,4})", "document_date"),
            (r"(\d{1,2}\s+(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\w*\s+\d{4})", "document_date"),
        ]
        for pattern, key in date_patterns:
            match = re.search(pattern, text, re.IGNORECASE)
            if match:
                metadata[key] = match.group(1)
                break

        # Extract institution/bank name
        bank_patterns = [
            r"(?:bank|institution)[:\s]*([A-Z][A-Za-z\s&]+(?:Bank|Ltd|Limited|Corp))",
            r"(Reserve Bank of India|RBI|SEBI|NSE|BSE)",
        ]
        for pattern in bank_patterns:
            match = re.search(pattern, text)
            if match:
                metadata["institution"] = match.group(1).strip()
                break

        # Detect document type
        doc_type = "unknown"
        type_keywords = {
            "bank_statement": ["statement", "account summary", "transaction history"],
            "rbi_circular": ["rbi", "reserve bank", "circular", "notification"],
            "sebi_filing": ["sebi", "annual return", "compliance"],
            "financial_report": ["balance sheet", "profit and loss", "annual report"],
            "invoice": ["invoice", "bill", "amount due"],
            "loan_agreement": ["loan agreement", "terms and conditions", "repayment"],
            "regulation": ["regulation", "guideline", "norm", "standard", "protocol"],
            "product_specification": ["specification", "product", "architecture", "design"],
        }
        text_lower = text.lower()
        for dtype, keywords in type_keywords.items():
            if any(kw in text_lower for kw in keywords):
                doc_type = dtype
                break
        metadata["document_type"] = doc_type

        return metadata

    def _generate_doc_id(self, source_file: str, text: str) -> str:
        """Generate deterministic document ID."""
        content = f"{source_file}:{text[:1000]}"
        return hashlib.sha256(content.encode()).hexdigest()[:16]

    def _extract_title(self, text: str, source_file: str) -> str:
        """Extract document title from first meaningful line."""
        lines = [l.strip() for l in text.split("\n") if l.strip()]
        if lines:
            # First non-trivial line as title
            for line in lines[:5]:
                if len(line) > 5 and len(line) < 200:
                    return line
        return source_file

    def _classify_document(self, text: str) -> str:
        """Classify document sensitivity level."""
        text_lower = text.lower()
        restricted_indicators = [
            "top secret", "confidential", "restricted", "insider",
            "investigation", "fraud", "compliance violation",
        ]
        confidential_indicators = [
            "confidential", "internal use only", "do not distribute",
            "salary", "compensation", "personal data", "kyc",
        ]

        for indicator in restricted_indicators:
            if indicator in text_lower:
                return "restricted"
        for indicator in confidential_indicators:
            if indicator in text_lower:
                return "confidential"
        return "internal"

    def _semantic_chunk(self, text: str, metadata: dict) -> list[dict]:
        """Chunk text by semantic boundaries (paragraphs, sections).

        Respects natural document structure rather than arbitrary character limits.
        Uses a larger default chunk size (800 chars) to keep financial data and
        its context together.

        Overlap strategy: when a chunk boundary is needed, we keep the last 150
        chars of the previous chunk as context for the next. This ensures numbers
        like "3.2 percent" don't get separated from "Gross NPA ratio stood at".
        """
        chunks = []

        # Split by paragraph breaks first
        paragraphs = re.split(r"\n\s*\n", text)
        current_chunk = ""
        for para in paragraphs:
            para = para.strip()
            if not para:
                continue

            # If adding this paragraph exceeds chunk size, save current and start new
            if current_chunk and len(current_chunk) + len(para) + 1 > self.chunk_size:
                chunks.append({
                    "text": current_chunk.strip(),
                    "metadata": {**metadata, "chunk_index": len(chunks)},
                })
                # Overlap: keep last portion
                if self.chunk_overlap > 0 and len(current_chunk) > self.chunk_overlap:
                    current_chunk = current_chunk[-self.chunk_overlap:] + " " + para
                else:
                    current_chunk = para
            else:
                current_chunk = current_chunk + "\n" + para if current_chunk else para

        # Don't forget the last chunk
        if current_chunk.strip():
            chunks.append({
                "text": current_chunk.strip(),
                "metadata": {**metadata, "chunk_index": len(chunks)},
            })

        # Handle edge case: text smaller than chunk_size
        if not chunks and text.strip():
            chunks.append({
                "text": text.strip(),
                "metadata": {**metadata, "chunk_index": 0},
            })

        return chunks
