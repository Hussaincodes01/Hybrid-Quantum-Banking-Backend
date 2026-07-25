"""FINIX RAG - end-to-end accuracy evaluation.

Ingests the gold corpus, runs each labelled question through the FULL secure query
pipeline (auth -> hybrid retrieval -> rerank -> Groq synthesis), and scores:

  * retrieval@1  : the correct document is the top citation
  * retrieval@k  : the correct document is cited at all
  * answer match : all expected keywords appear in the generated answer

Usage (from repo root, venv active, infra up):
    set PYTHONPATH=src
    python tests/accuracy/run_accuracy.py            # fresh run (resets collection)
    python tests/accuracy/run_accuracy.py --keep     # keep existing collection
"""
import os
import sys
import time
import json
import tempfile
import argparse
from pathlib import Path

# Make `finix_rag` importable whether or not PYTHONPATH is set.
_SRC = Path(__file__).resolve().parents[2] / "src"
if str(_SRC) not in sys.path:
    sys.path.insert(0, str(_SRC))
sys.path.insert(0, str(Path(__file__).resolve().parent))  # for gold_corpus

try:
    from dotenv import load_dotenv
    load_dotenv(Path(__file__).resolve().parents[2] / ".env")
except Exception:
    pass

import fitz  # PyMuPDF
from qdrant_client import QdrantClient

from gold_corpus import CORPUS, GOLD_QA
from finix_rag.ingestion.secure_ingestor import SecureIngestor
from finix_rag.query.secure_query_engine import SecureQueryEngine
from finix_rag.security.auth import TokenPayload, UserRole


def make_pdf(path: Path, body: str) -> None:
    """Render text into a real PDF with a proper text layer.

    Each paragraph is placed as its own block with a vertical gap so the blank lines
    survive extraction — otherwise a single insert_textbox reflows everything into one
    block and the doc collapses to a single chunk.
    """
    doc = fitz.open()
    page = doc.new_page()
    y = 60.0
    for para in body.split("\n\n"):
        para = para.strip()
        if not para:
            continue
        rect = fitz.Rect(50, y, 545, 800)
        leftover = page.insert_textbox(rect, para, fontsize=11, fontname="helv", align=0)
        # insert_textbox returns the y of the last baseline used; advance past it + gap.
        used = (rect.y1 - leftover) if leftover and leftover > 0 else (y + 16 * (1 + len(para) // 75))
        y = used + 26  # blank-line gap between paragraphs
    doc.save(str(path))
    doc.close()


def qdrant_env():
    host = os.getenv("QDRANT_HOST", "127.0.0.1")
    port = os.getenv("QDRANT_PORT", "6333")
    return f"http://{host}:{port}", os.getenv("QDRANT_API_KEY"), os.getenv("QDRANT_COLLECTION", "finix_documents")


def banner(title: str) -> None:
    print("\n" + "=" * 74)
    print(title)
    print("=" * 74)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--keep", action="store_true", help="do not reset the collection")
    args = ap.parse_args()

    url, api_key, collection = qdrant_env()

    banner("FINIX RAG - Accuracy Evaluation")
    try:
        import torch
        dev = "GPU (" + torch.cuda.get_device_name(0) + ")" if torch.cuda.is_available() else "CPU"
    except Exception:
        dev = "unknown"
    print(f"Embedding device : {dev}")
    print(f"Reranker device  : {os.getenv('RERANKER_DEVICE', 'cpu')}")
    print(f"LLM model        : {os.getenv('GROQ_MODEL', 'llama-3.3-70b-versatile')}")
    print(f"Qdrant           : {url}  collection={collection}")

    # --- Reset collection for a deterministic run -------------------------------
    if not args.keep:
        client = QdrantClient(url=url, api_key=api_key)
        try:
            client.delete_collection(collection)
            print(f"Reset collection '{collection}'.")
        except Exception:
            pass

    # --- Ingest the gold corpus -------------------------------------------------
    banner("Ingesting gold corpus")
    ingestor = SecureIngestor(qdrant_url=url, collection_name=collection, api_key=api_key,
                              enable_pii_masking=os.getenv("ENABLE_PII_MASKING", "true") == "true")
    t0 = time.time()
    with tempfile.TemporaryDirectory() as tmp:
        for doc in CORPUS:
            p = Path(tmp) / doc["filename"]
            make_pdf(p, doc["body"])
            res = ingestor.ingest_file(p, user_id="eval-admin", metadata={"tenant_id": "default"})
            print(f"  + {doc['filename']:36s} chunks={res['chunks_ingested']:<2d} "
                  f"class={res['classification']:<12s} conf={res['ocr_confidence']:.2f}")
    print(f"Ingestion took {time.time() - t0:.1f}s")

    # --- Build the query engine -------------------------------------------------
    qe = SecureQueryEngine(qdrant_url=url, collection_name=collection, api_key=api_key,
                           groq_api_key=os.getenv("GROQ_API_KEY"),
                           enable_pii_masking=os.getenv("ENABLE_PII_MASKING", "true") == "true")

    now = int(time.time())
    admin = TokenPayload(sub="eval-admin", role=UserRole.ADMIN, department="ops",
                         clearance_level=4, tenant_id="default", jti="eval", iat=now, exp=now + 3600)

    # --- Run the gold Q&A -------------------------------------------------------
    banner("Running gold Q&A")
    rows, rec1, reck, ans_ok, confs = [], 0, 0, 0, []
    for i, item in enumerate(GOLD_QA, 1):
        t = time.time()
        try:
            resp = qe.query(item["q"], user=admin, max_sources=5)
        except Exception as e:
            rows.append((i, item["q"], False, False, False, 0.0, f"ERROR: {e}"))
            continue
        ans = (resp.answer or "").lower()
        kw_ok = all(k.lower() in ans for k in item["keywords"])
        cites = [c.get("source", "") for c in resp.citations]
        top_ok = bool(cites) and item["hint"].lower() in cites[0].lower()
        any_ok = any(item["hint"].lower() in c.lower() for c in cites)
        rec1 += top_ok
        reck += any_ok
        ans_ok += kw_ok
        confs.append(resp.confidence)
        rows.append((i, item["q"], kw_ok, top_ok, any_ok, resp.confidence,
                     resp.answer.replace("\n", " ")[:90], time.time() - t))

    # --- Report -----------------------------------------------------------------
    banner("Per-question results")
    print(f"{'#':>2}  {'ans':>3} {'r@1':>3} {'r@k':>3} {'conf':>5}  question")
    for r in rows:
        i, q = r[0], r[1]
        if len(r) == 7:  # error row
            print(f"{i:>2}  {'ERR':>3}                {r[6]}")
            continue
        _, _, kw_ok, top_ok, any_ok, conf, ans, dt = r
        print(f"{i:>2}  {'  Y' if kw_ok else '  .':>3} {'  Y' if top_ok else '  .':>3} "
              f"{'  Y' if any_ok else '  .':>3} {conf:5.2f}  {q}")
        print(f"        -> {ans}")

    n = len(GOLD_QA)
    banner("Summary")
    print(f"Answer accuracy (keywords present) : {ans_ok}/{n} = {ans_ok/n:5.1%}")
    print(f"Retrieval@1  (correct doc is top)  : {rec1}/{n} = {rec1/n:5.1%}")
    print(f"Retrieval@k  (correct doc cited)   : {reck}/{n} = {reck/n:5.1%}")
    print(f"Mean confidence                    : {sum(confs)/len(confs):.3f}" if confs else "n/a")

    report = {
        "answer_accuracy": ans_ok / n, "retrieval_at_1": rec1 / n,
        "retrieval_at_k": reck / n, "n": n, "device": dev,
    }
    out = Path(__file__).resolve().parent / "last_report.json"
    out.write_text(json.dumps(report, indent=2))
    print(f"\nJSON report written to {out}")

    # Fail the run on regressions so this doubles as a CI gate.
    ok = report["answer_accuracy"] >= 0.7 and report["retrieval_at_1"] >= 0.8
    print("\nRESULT:", "PASS" if ok else "FAIL (below threshold)")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
