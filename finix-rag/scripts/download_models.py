"""
FINIX RAG - Download BGE-M3 Models from HuggingFace

This script downloads the BGE-M3 embedding model and BGE-Reranker-v2-M3
reranking model from HuggingFace for use in the FINIX RAG system.

Models:
- BAAI/bge-m3: Multi-Functionality, Multi-Linguality, Multi-Granularity embeddings
- BAAI/bge-reranker-v2-m3: Cross-encoder reranking model

Usage:
    python scripts/download_models.py
"""
import os
import sys
import logging
from pathlib import Path

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


def download_embedding_model():
    """Download BGE-M3 embedding model from HuggingFace."""
    from sentence_transformers import SentenceTransformer

    model_name = "BAAI/bge-m3"
    cache_dir = os.path.join(os.path.expanduser("~"), ".cache", "finix-rag", "models")

    logger.info("Downloading embedding model: %s", model_name)
    logger.info("Cache directory: %s", cache_dir)

    model = SentenceTransformer(model_name, cache_folder=cache_dir)

    # Test the model
    test_sentences = [
        "What is the NPA ratio of the bank?",
        "Explain the KYC compliance requirements.",
    ]
    embeddings = model.encode(test_sentences)
    logger.info(
        "Model downloaded successfully. Embedding dimension: %d",
        embeddings.shape[1],
    )
    return model


def download_reranker_model():
    """Download BGE-Reranker-v2-M3 from HuggingFace."""
    from FlagEmbedding import FlagReranker

    model_name = "BAAI/bge-reranker-v2-m3"

    logger.info("Downloading reranker model: %s", model_name)

    reranker = FlagReranker(model_name, use_fp16=False)

    # Test the model
    test_pairs = [
        ["What is NPA?", "Non-Performing Assets are loans that are in default."],
        ["What is NPA?", "The weather is nice today."],
    ]
    scores = reranker.compute_score(test_pairs, normalize=True)
    logger.info("Reranker downloaded successfully. Test scores: %s", scores)
    return reranker


def main():
    """Download all required models."""
    logger.info("=" * 60)
    logger.info("FINIX RAG - Model Download Script")
    logger.info("=" * 60)

    try:
        # Download embedding model
        logger.info("\n[1/2] Downloading BGE-M3 Embedding Model (~2.27 GB)...")
        embedding_model = download_embedding_model()
        logger.info("✓ Embedding model ready")

        # Download reranker model
        logger.info("\n[2/2] Downloading BGE-Reranker-v2-M3 (~568 MB)...")
        reranker_model = download_reranker_model()
        logger.info("✓ Reranker model ready")

        logger.info("\n" + "=" * 60)
        logger.info("All models downloaded successfully!")
        logger.info("=" * 60)
        logger.info("\nModels cached in: ~/.cache/finix-rag/models/")
        logger.info("\nNext steps:")
        logger.info("1. Copy .env.example to .env")
        logger.info("2. Edit .env with your configuration")
        logger.info("3. Run: docker compose up -d")
        logger.info("4. Test: curl http://localhost:8000/health")

        return 0

    except Exception as e:
        logger.error("Model download failed: %s", e)
        logger.error("\nTroubleshooting:")
        logger.error("1. Check internet connection")
        logger.error("2. Ensure you have ~3 GB free disk space")
        logger.error("3. Try: pip install --upgrade sentence-transformers FlagEmbedding")
        return 1


if __name__ == "__main__":
    sys.exit(main())
