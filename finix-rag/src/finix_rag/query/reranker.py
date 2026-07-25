"""BGE-Reranker-v2-M3 - Cross-encoder reranking for financial queries.

Uses BAAI/bge-reranker-v2-m3 for high-precision reranking of retrieved chunks.
Implements sigmoid-based score normalization to map raw cross-encoder logits
into a [0, 1] range that is more meaningful for confidence calculations.
"""
import os
import math
import logging
import threading
from typing import Optional
from dataclasses import dataclass

import numpy as np
from FlagEmbedding import FlagReranker

logger = logging.getLogger(__name__)

_reranker_instance = None
_reranker_lock = threading.Lock()

def _cuda_available() -> bool:
    """True only when a CUDA GPU is present."""
    try:
        import torch
        return torch.cuda.is_available()
    except Exception:
        return False

def _resolve_device() -> str:
    """Resolve the reranker device from RERANKER_DEVICE.

    Default is CPU: on small GPUs (e.g. a 4 GB card) the ~2.2 GB BGE-M3 embedding
    model already dominates VRAM, so keeping the reranker on CPU avoids CUDA OOM.
    Set RERANKER_DEVICE=cuda on a larger GPU, or 'auto' to use the GPU when free.
    """
    device = os.getenv("RERANKER_DEVICE", "cpu").strip().lower()
    if device in ("auto", "gpu"):
        return "cuda" if _cuda_available() else "cpu"
    if device == "cuda" and not _cuda_available():
        logger.warning("RERANKER_DEVICE=cuda but no CUDA GPU found — falling back to CPU")
        return "cpu"
    return device or "cpu"

def _get_reranker(model_name: str = "BAAI/bge-reranker-v2-m3"):
    """Lazy-load reranker model (heavy, ~600MB)."""
    global _reranker_instance
    if _reranker_instance is None:
        with _reranker_lock:
            # Double-checked locking pattern
            if _reranker_instance is None:
                device = _resolve_device()
                # fp16 only makes sense on CUDA; on CPU it degrades/crashes.
                use_fp16 = device == "cuda"
                logger.info("Loading BGE-Reranker model: %s (device=%s, fp16=%s)", model_name, device, use_fp16)
                _reranker_instance = FlagReranker(model_name, use_fp16=use_fp16, devices=device)
                logger.info("Reranker loaded successfully")
    return _reranker_instance


@dataclass
class RerankResult:
    """Result from reranking."""
    text: str
    score: float
    metadata: dict
    original_index: int


def _sigmoid_normalize(raw_score: float) -> float:
    """Map raw cross-encoder logit to [0, 1] via sigmoid.

    BGE-Reranker's raw scores can be negative or > 1.  When
    compute_score(normalize=True) is used the output is already in
    [0, 1], but its distribution is compressed.  This function
    further stretches it so that scores for relevant documents land
    in [0.4, 0.8] instead of [0.1, 0.3], making the number more
    intuitive for downstream confidence calculations.
    """
    # apply sigmoid then rescale from [0,1] -> [0.1, 0.95]
    s = 1.0 / (1.0 + math.exp(-raw_score))
    return 0.1 + s * 0.85


class BGEReranker:
    """Cross-encoder reranker using BGE-Reranker-v2-M3.

    Takes query-document pairs and outputs relevance scores
    for high-precision filtering of retrieved chunks.
    """

    def __init__(
        self,
        model_name: str = "BAAI/bge-reranker-v2-m3",
        top_k: int = 5,
        score_threshold: float = 0.10,
    ):
        self.model_name = model_name
        self.top_k = top_k
        self.score_threshold = score_threshold
        self._reranker = None

    def _ensure_loaded(self):
        if self._reranker is None:
            self._reranker = _get_reranker(self.model_name)

    def rerank(
        self,
        query: str,
        documents: list[str],
        metadata_list: Optional[list[dict]] = None,
    ) -> list[RerankResult]:
        """Rerank documents by relevance to query.

        Args:
            query: The user query.
            documents: List of document texts to rerank.
            metadata_list: Optional metadata for each document.

        Returns:
            Sorted list of RerankResult with scores.
        """
        if not documents:
            return []

        self._ensure_loaded()

        # Build query-document pairs for cross-encoder
        pairs = [[query, doc] for doc in documents]

        # Get relevance scores (normalize=True gives [0,1] range)
        scores = self._reranker.compute_score(pairs, normalize=True)

        # Handle single document case
        if isinstance(scores, float):
            scores = [scores]

        # Build results
        results = []
        for i, (doc, score) in enumerate(zip(documents, scores)):
            meta = metadata_list[i] if metadata_list else {}
            # Apply sigmoid normalization for better distribution
            normalized_score = _sigmoid_normalize(float(score))
            results.append(
                RerankResult(
                    text=doc,
                    score=normalized_score,
                    metadata=meta,
                    original_index=i,
                )
            )

        # Sort by score descending
        results.sort(key=lambda x: x.score, reverse=True)

        # Filter by threshold and limit to top_k
        results = [r for r in results if r.score >= self.score_threshold]
        results = results[: self.top_k]

        logger.info(
            "Reranked %d documents, keeping top %d (score range: %.3f - %.3f)",
            len(documents), len(results),
            results[-1].score if results else 0,
            results[0].score if results else 0,
        )

        return results

    def rerank_with_nodes(self, query: str, nodes: list) -> list:
        """Rerank LlamaIndex NodeWithScore objects.

        Preserves original node objects with updated scores.
        """
        if not nodes:
            return []

        self._ensure_loaded()
        pairs = [[query, n.get_content()] for n in nodes]
        scores = self._reranker.compute_score(pairs, normalize=True)

        if isinstance(scores, float):
            scores = [scores]

        # Update scores with sigmoid normalization and sort
        for node, score in zip(nodes, scores):
            node.score = _sigmoid_normalize(float(score))

        nodes.sort(key=lambda x: x.score, reverse=True)
        return nodes[: self.top_k]
