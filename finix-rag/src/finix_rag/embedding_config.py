"""Shared embedding-model configuration for FINIX RAG.

Both the ingestor and the query-time retriever need LlamaIndex's global
``Settings.embed_model`` to be the BGE-M3 HuggingFace embedding on the right device.
Centralising it here means the query path no longer depends on the ingestor having
run first (otherwise LlamaIndex falls back to its OpenAI default and errors without
an OpenAI key).
"""
import os
import logging

logger = logging.getLogger(__name__)


def resolve_embedding_device() -> str:
    """Resolve EMBEDDING_DEVICE to a concrete torch device string.

    'auto'/'cuda'/'gpu'/'' -> CUDA when a GPU is present, else CPU.
    Anything else (e.g. 'cpu') is honored as-is.
    """
    device = os.getenv("EMBEDDING_DEVICE", "").strip().lower()
    if device in ("", "cuda", "gpu", "auto"):
        try:
            import torch
            return "cuda" if torch.cuda.is_available() else "cpu"
        except Exception:
            return "cpu"
    return device


def ensure_embed_model(model_name: str | None = None, force: bool = False):
    """Ensure Settings.embed_model is the BGE-M3 HuggingFace embedding.

    Idempotent: if a HuggingFaceEmbedding is already configured it is left in place
    unless ``force=True``. Returns the configured embed model.
    """
    from llama_index.core import Settings
    from llama_index.embeddings.huggingface import HuggingFaceEmbedding

    # Read the PRIVATE backing field, not Settings.embed_model — the property getter
    # lazily resolves LlamaIndex's OpenAI default (and raises without an OpenAI key).
    existing = getattr(Settings, "_embed_model", None)
    if not force and isinstance(existing, HuggingFaceEmbedding):
        return existing

    name = model_name or os.getenv("EMBEDDING_MODEL", "BAAI/bge-m3")
    device = resolve_embedding_device()
    logger.info("Configuring embedding model: %s on device=%s", name, device)
    Settings.embed_model = HuggingFaceEmbedding(model_name=name, device=device)
    return Settings.embed_model
