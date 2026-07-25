"""Hybrid Retriever - BM25 + Dense Vector Search with Query-Aware Fusion.

Combines keyword-based BM25 retrieval with BGE-M3 dense embeddings
for maximum recall on financial document queries. Supports adaptive weight
tuning based on query characteristics (numeric factoid vs. semantic).
"""
import logging
import os
from typing import Optional
from dataclasses import dataclass

import numpy as np
from qdrant_client import QdrantClient
from qdrant_client.models import (
    Filter,
    FieldCondition,
    MatchValue,
    VectorParams,
    Distance,
)
from llama_index.core import VectorStoreIndex, Settings
from llama_index.core.schema import NodeWithScore
from llama_index.vector_stores.qdrant import QdrantVectorStore
from llama_index.retrievers.bm25 import BM25Retriever

from finix_rag.embedding_config import ensure_embed_model

logger = logging.getLogger(__name__)

@dataclass
class RetrievalResult:
    """Combined retrieval result."""
    nodes: list[NodeWithScore]
    bm25_scores: dict[str, float]
    vector_scores: dict[str, float]
    combined_scores: dict[str, float]


class HybridRetriever:
    """Hybrid BM25 + Dense Vector retrieval with query-aware score fusion.

    Uses BGE-M3 for dense embeddings. Implements Reciprocal Rank Fusion (RRF)
    with adaptive weights based on query type:
    - Numeric/factoid queries: boost BM25 (exact keyword match is critical)
    - Semantic/conceptual queries: boost dense vectors
    - Mixed queries: balanced fusion
    """

    def __init__(
        self,
        qdrant_url: str = "http://localhost:6333",
        collection_name: str = "finix_documents",
        api_key: Optional[str] = None,
        bm25_weight: float = 0.35,
        vector_weight: float = 0.65,
        top_k: int = 20,
        rrf_k: int = 60,
    ):
        self.bm25_weight_default = bm25_weight
        self.vector_weight_default = vector_weight
        self.top_k = top_k
        self.rrf_k = rrf_k
        self.api_key = api_key or os.getenv("QDRANT_API_KEY")

        # Ensure the BGE-M3 embedding is configured (query path must not depend on
        # the ingestor having run first, else LlamaIndex falls back to OpenAI).
        ensure_embed_model()

        # Qdrant client
        self.qdrant_client = QdrantClient(url=qdrant_url, api_key=self.api_key)

        # Vector store index for dense retrieval
        self.vector_store = QdrantVectorStore(
            url=qdrant_url, collection_name=collection_name, api_key=self.api_key
        )
        self.index = VectorStoreIndex.from_vector_store(self.vector_store)

        # BM25 retriever from index (with fallback)
        try:
            self.bm25_retriever = BM25Retriever.from_defaults(
                index=self.index,
                similarity_top_k=self.top_k,
            )
            self._bm25_available = True
        except Exception as e:
            logger.warning("BM25Retriever unavailable, falling back to dense-only: %s", e)
            self._bm25_available = False

        logger.info(
            "HybridRetriever initialized: collection=%s top_k=%d bm25_w=%.2f vec_w=%.2f",
            collection_name, top_k, bm25_weight, vector_weight,
        )

    def retrieve(
        self,
        query: str,
        tenant_filter: Optional[str] = None,
        classification_filter: Optional[str] = None,
        bm25_boost: bool = False,
    ) -> RetrievalResult:
        """Hybrid retrieval with BM25 + Dense + RRF fusion.

        Args:
            query: User query string.
            tenant_filter: Filter by tenant ID for multi-tenancy.
            classification_filter: Filter by document classification.
            bm25_boost: If True, increase BM25 weight (for numeric/factoid queries).

        Returns:
            RetrievalResult with fused and ranked nodes.
        """
        # Adapt weights based on query characteristics
        if bm25_boost:
            # Numeric queries rely heavily on exact keyword match
            bm25_w = 0.55
            vector_w = 0.45
        else:
            bm25_w = self.bm25_weight_default
            vector_w = self.vector_weight_default

        # Build Qdrant filter if needed
        qdrant_filter = self._build_filter(tenant_filter, classification_filter)

        # Dense vector retrieval
        dense_retriever = self.index.as_retriever(similarity_top_k=self.top_k)
        dense_nodes = dense_retriever.retrieve(query)

        # BM25 retrieval (with fallback if unavailable)
        bm25_nodes = []
        if self._bm25_available:
            try:
                bm25_nodes = self.bm25_retriever.retrieve(query)
            except Exception as e:
                logger.warning("BM25 retrieval failed, skipping: %s", e)
                self._bm25_available = False

        # Score maps
        bm25_scores = {n.node.node_id: n.score for n in bm25_nodes}
        vector_scores = {n.node.node_id: n.score for n in dense_nodes}

        # Reciprocal Rank Fusion with adapted weights
        combined_scores = self._rrf_fusion(
            bm25_scores, vector_scores,
            bm25_weight=bm25_w, vector_weight=vector_w,
        )

        # Merge and sort all unique nodes
        all_nodes = {}
        for node in dense_nodes + bm25_nodes:
            if node.node.node_id not in all_nodes:
                all_nodes[node.node.node_id] = node

        # Sort by combined score
        sorted_ids = sorted(
            combined_scores.keys(),
            key=lambda x: combined_scores[x],
            reverse=True,
        )[:self.top_k]

        fused_nodes = [all_nodes[nid] for nid in sorted_ids if nid in all_nodes]

        logger.info(
            "Retrieved %d nodes (dense=%d, bm25=%d, fused=%d, bm25_w=%.2f)",
            len(fused_nodes), len(dense_nodes), len(bm25_nodes),
            len(fused_nodes), bm25_w,
        )

        return RetrievalResult(
            nodes=fused_nodes,
            bm25_scores=bm25_scores,
            vector_scores=vector_scores,
            combined_scores=combined_scores,
        )

    def _rrf_fusion(
        self,
        scores_a: dict[str, float],
        scores_b: dict[str, float],
        bm25_weight: float = None,
        vector_weight: float = None,
    ) -> dict[str, float]:
        """Reciprocal Rank Fusion of two score dictionaries.

        RRF score = sum( weight / (k + rank) ) for each list.
        Supports dynamic weights per call for query-adaptive fusion.
        """
        if bm25_weight is None:
            bm25_weight = self.bm25_weight_default
        if vector_weight is None:
            vector_weight = self.vector_weight_default

        combined = {}

        # Rank-based scoring for BM25
        sorted_a = sorted(scores_a.keys(), key=lambda x: scores_a[x], reverse=True)
        for rank, doc_id in enumerate(sorted_a):
            combined[doc_id] = combined.get(doc_id, 0) + (
                bm25_weight / (self.rrf_k + rank + 1)
            )

        # Rank-based scoring for dense
        sorted_b = sorted(scores_b.keys(), key=lambda x: scores_b[x], reverse=True)
        for rank, doc_id in enumerate(sorted_b):
            combined[doc_id] = combined.get(doc_id, 0) + (
                vector_weight / (self.rrf_k + rank + 1)
            )

        return combined

    def _build_filter(
        self,
        tenant_filter: Optional[str],
        classification_filter: Optional[str],
    ) -> Optional[Filter]:
        """Build Qdrant metadata filter."""
        conditions = []
        if tenant_filter:
            conditions.append(
                FieldCondition(key="tenant_id", match=MatchValue(value=tenant_filter))
            )
        if classification_filter:
            conditions.append(
                FieldCondition(
                    key="classification",
                    match=MatchValue(value=classification_filter),
                )
            )
        if conditions:
            return Filter(must=conditions)
        return None

    def search_with_filters(
        self,
        query: str,
        allowed_doc_ids: Optional[list[str]] = None,
        max_classification: str = "internal",
    ) -> RetrievalResult:
        """Filtered retrieval based on authorization decisions."""
        # Classification hierarchy
        classification_levels = {"public": 0, "internal": 1, "confidential": 2, "restricted": 3}
        max_level = classification_levels.get(max_classification, 1)

        # Filter nodes by allowed classifications
        result = self.retrieve(query)

        filtered_nodes = []
        for node in result.nodes:
            node_classification = node.metadata.get("classification", "internal")
            node_level = classification_levels.get(node_classification, 1)
            if node_level <= max_level:
                if allowed_doc_ids is None or node.metadata.get("doc_id") in allowed_doc_ids:
                    filtered_nodes.append(node)

        result.nodes = filtered_nodes
        return result
