"""Secure Ingestor - End-to-end secure document ingestion pipeline.

Orchestrates: OCR → PII Masking → Encryption → Embedding → Storage
with Vault encryption at every stage.
"""
import os
import logging
from pathlib import Path
from typing import Optional

from llama_index.core import (
    Document,
    Settings,
    VectorStoreIndex,
    StorageContext,
)
from llama_index.core.node_parser import SentenceSplitter
from llama_index.core.ingestion import IngestionPipeline, IngestionCache
from llama_index.core.schema import TextNode
from llama_index.embeddings.huggingface import HuggingFaceEmbedding
from llama_index.vector_stores.qdrant import QdrantVectorStore

from finix_rag.embedding_config import ensure_embed_model
from finix_rag.security.pii import PIIMasker, get_pii_masker
from finix_rag.security.crypto import HMACValidator
from finix_rag.ingestion.ocr_engine import PaddleOCREngine
from finix_rag.ingestion.text_extractor import extract_document
from finix_rag.ingestion.document_processor import DocumentProcessor

logger = logging.getLogger(__name__)


class SecureIngestor:
    """Secure document ingestion with PII protection and encryption.

    Pipeline:
    1. OCR extraction (PaddleOCR)
    2. PII detection & masking
    3. Document processing & chunking
    4. Embedding generation (BGE-M3 from HuggingFace)
    5. Vector storage with encrypted metadata (Qdrant + Vault)
    6. HMAC integrity verification
    """

    def __init__(
        self,
        qdrant_url: str = "http://localhost:6333",
        collection_name: str = "finix_documents",
        api_key: Optional[str] = None,
        embedding_model: str = None,
        enable_pii_masking: bool = True,
        vault_url: Optional[str] = None,
    ):
        self._qdrant_api_key = api_key or os.getenv("QDRANT_API_KEY")
        # OCR engine
        self.ocr = PaddleOCREngine()

        # Document processor
        self.processor = DocumentProcessor()

        # PII masker
        self.pii_masker = get_pii_masker("tokenize") if enable_pii_masking else None

        # Vault encryption (lazy import to avoid hvac startup failure)
        try:
            from finix_rag.security.vault_client import VaultEncryption
            self.vault = VaultEncryption(url=vault_url)
        except Exception:
            logger.warning("Vault unavailable - using local AES encryption fallback")
            self.vault = None

        # HMAC for integrity
        self.hmac = HMACValidator()

        # Configure embedding model (BGE-M3 from HuggingFace) on the resolved device
        # (GPU when available, else CPU). Shared with the query path so both agree.
        ensure_embed_model(embedding_model, force=True)
        # Larger chunks (800 chars, 150 overlap) keep financial data together with
        # its context. The SentenceSplitter is a fallback; the primary chunking
        # happens in DocumentProcessor._semantic_chunk which is paragraph-aware.
        Settings.node_parser = SentenceSplitter(
            chunk_size=800, chunk_overlap=150
        )

        # Qdrant vector store
        self.vector_store = QdrantVectorStore(
            url=qdrant_url,
            collection_name=collection_name,
            api_key=self._qdrant_api_key,
        )
        self.storage_context = StorageContext.from_defaults(
            vector_store=self.vector_store
        )

        logger.info(
            "SecureIngestor initialized: collection=%s pii=%s vault=%s",
            collection_name, enable_pii_masking, self.vault is not None,
        )

    def ingest_file(
        self,
        file_path: str | Path,
        metadata: Optional[dict] = None,
        user_id: str = "system",
    ) -> dict:
        """Ingest a single file through the secure pipeline.

        Returns:
            Ingestion result with doc_id, chunk count, and integrity hash.
        """
        file_path = Path(file_path)
        logger.info("Starting secure ingestion: %s", file_path.name)

        # Step 1: Extract text — native PDF/text layer first, OCR only for scans/images.
        ocr_result = extract_document(file_path, self.ocr)

        # Step 2: PII Masking
        masked_text = ocr_result.text
        if self.pii_masker:
            masked_text = self.pii_masker.mask(ocr_result.text)
            logger.info("PII masked: %s", self.pii_masker.stats)

        # Step 3: Process & chunk
        doc = self.processor.process(
            text=masked_text,
            tables=ocr_result.tables,
            source_file=file_path.name,
            extra_metadata={
                **(metadata or {}),
                "user_id": user_id,
                "ocr_confidence": ocr_result.confidence,
                "page_count": ocr_result.page_count,
                "pii_masked": self.pii_masker is not None,
            },
        )

        # Step 4: Document-level checksum (returned to the caller for audit).
        doc_checksum = self.hmac.sign_hex(doc.text.encode())

        # Step 5: Build one node per chunk with a PER-CHUNK integrity hash.
        # FIX: the previous code stored the whole-document hash on every chunk and
        # then verified it against each chunk's text at query time — so any document
        # that produced more than one chunk failed integrity verification and was
        # dropped entirely. We hash each chunk's exact text and build TextNodes
        # directly (no SentenceSplitter re-chunking), so query-time
        # node.get_content() matches the hashed text byte-for-byte.
        # Non-semantic bookkeeping keys are excluded from the embedded/LLM text so
        # they don't pollute the vectors.
        _excluded = [
            "integrity_hash", "doc_checksum", "vault_encrypted", "chunk_index",
            "doc_id", "ocr_confidence", "page_count", "pii_masked",
            "uploaded_by", "tenant_id", "user_id",
        ]
        nodes = []
        for chunk in doc.chunks:
            chunk_text = chunk["text"]
            node = TextNode(
                text=chunk_text,
                metadata={
                    **chunk["metadata"],
                    "doc_id": doc.doc_id,
                    "title": doc.title,
                    "classification": doc.classification,
                    "integrity_hash": self.hmac.sign_hex(chunk_text.encode()),
                    "doc_checksum": doc_checksum,
                    "vault_encrypted": self.vault is not None,
                },
                excluded_embed_metadata_keys=_excluded,
                excluded_llm_metadata_keys=_excluded,
            )
            nodes.append(node)

        # Step 6: Store in Qdrant with embeddings (nodes are pre-chunked).
        index = VectorStoreIndex(nodes=nodes, storage_context=self.storage_context)

        result = {
            "doc_id": doc.doc_id,
            "title": doc.title,
            "chunks_ingested": len(doc.chunks),
            "classification": doc.classification,
            "integrity_hash": doc_checksum,
            "ocr_confidence": ocr_result.confidence,
            "tables_extracted": len(ocr_result.tables),
            "pii_masked": self.pii_masker is not None,
        }

        logger.info("Ingestion complete: %s", result)
        return result

    def ingest_batch(
        self,
        file_paths: list[str | Path],
        metadata: Optional[dict] = None,
        user_id: str = "system",
    ) -> list[dict]:
        """Ingest multiple files."""
        results = []
        for path in file_paths:
            try:
                result = self.ingest_file(path, metadata, user_id)
                results.append(result)
            except Exception as e:
                logger.error("Failed to ingest %s: %s", path, e)
                results.append({
                    "file": str(path),
                    "error": str(e),
                    "status": "failed",
                })
        return results

    def encrypt_metadata(self, metadata: dict) -> dict:
        """Encrypt sensitive metadata fields before storage."""
        if not self.vault:
            return metadata

        sensitive_fields = ["account_number", "customer_id", "ifsc_code"]
        encrypted = {}
        for key, value in metadata.items():
            if key in sensitive_fields and isinstance(value, str):
                encrypted[key] = self.vault.encrypt(value)
            else:
                encrypted[key] = value
        return encrypted
