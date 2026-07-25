# FINIX RAG — GPU Setup & Test Runbook

The authoritative, verified guide to get FINIX RAG running **on your NVIDIA GPU** and
to **measure its accuracy**. Written for Windows 11 + PowerShell (the dev box), but the
commands map 1:1 to Linux.

> **Where this fits:** FINIX RAG is a self-contained Python service. Your **PSB Golang
> stack** talks to it over its REST API (`POST /auth/login`, `POST /ingest`,
> `POST /query`) — nothing in the Go stack changes. Run this as a sidecar/microservice
> next to the Go app and call it over HTTP.

---

## 0. What was broken (and is now fixed)

If you're picking this up after the repair pass, here's what changed and why:

| Area | Problem | Fix |
|------|---------|-----|
| **torch** | `torch 2.6.0+cpu` mixed with `torchvision/torchaudio +cu121` → `RuntimeError: operator torchvision::nms does not exist`; the app couldn't even import. Also CPU-only, so no GPU. | Reinstall a clean **CUDA** torch (`2.6.0+cu124`); drop the unused torchvision/torchaudio. |
| **Vault** | Ran in *production* mode, booted **sealed & uninitialized** → encryption silently off. | `docker-compose` now runs Vault in **dev mode**; the client **auto-enables the transit engine**. |
| **Healthchecks** | Qdrant/OPA images have no `curl`/`wget`, so they showed **"unhealthy"** and blocked the app container. | Removed the impossible exec healthchecks; dependents use `service_started`. |
| **Ingestion** | Hard-wired through PaddleOCR (which conflicts with the embedding stack), so even text PDFs failed. | Added a **native PyMuPDF text path**; OCR is now a fallback for scanned/image files only. |
| **Integrity** | HMAC hashed the *whole document* but verified per *chunk* → every multi-chunk doc was dropped as "failed integrity". | **Per-chunk** hashing with directly-built nodes (no double-chunking). |

---

## 1. Prerequisites

| Need | This box | Notes |
|------|----------|-------|
| GPU | NVIDIA RTX 2050, **4 GB** | Any CUDA GPU works. 4 GB is tight — see §8. |
| NVIDIA driver | CUDA 12.x+ runtime | `nvidia-smi` must succeed. |
| Docker Desktop | running | Hosts Qdrant / Vault / OPA / Redis. |
| Python | **3.11** (or 3.12) | The repo's `venv\` is 3.11 — fine. |
| Groq API key | in `.env` | LLM synthesis. |

> **Why the app runs in a local venv, not the app container:** Docker Desktop on Windows
> does not pass the NVIDIA GPU into Linux containers without extra WSL2 setup. For GPU,
> run **infra in Docker + the app in the venv**. The `finix-rag` container remains the
> CPU-only fallback for pure-Docker deployments.

---

## 2. One-time setup

```powershell
# from d:\finix-rag
python -m venv venv                 # if not already present
.\venv\Scripts\Activate.ps1
python -m pip install -U pip setuptools wheel

# Core deps (CPU torch is pulled first; we replace it with CUDA torch next)
pip install -r requirements.txt

# --- GPU: replace torch with the CUDA build (this is what puts you on the GPU) ---
pip uninstall -y torch torchvision torchaudio
pip install "torch==2.6.0" --index-url https://download.pytorch.org/whl/cu124
```

**Verify the GPU is visible to torch:**

```powershell
python -c "import torch; print('cuda:', torch.cuda.is_available(), '|', torch.cuda.get_device_name(0))"
# Expect: cuda: True | NVIDIA GeForce RTX 2050
```

If it prints `cuda: False`, you're still on a CPU wheel — re-run the two GPU lines above.

---

## 3. Configure `.env`

`.env` is already populated. The values that matter for GPU + accuracy:

```ini
GROQ_API_KEY=gsk_...                 # required
GROQ_MODEL=llama-3.3-70b-versatile
EMBEDDING_DEVICE=auto                # -> cuda when a GPU is present
RERANKER_DEVICE=cpu                  # keep on CPU on a 4 GB card (see §8)
QDRANT_API_KEY=...                   # must match docker-compose
VAULT_TOKEN=dev-only-token
```

---

## 4. Start the infrastructure

```powershell
docker compose up -d qdrant vault opa redis
docker compose ps        # vault + redis = healthy; qdrant + opa = up (no healthcheck)
```

**Health probes (all should answer):**

```powershell
curl http://127.0.0.1:6333/readyz                                   # Qdrant -> "all shards are ready"
(curl http://127.0.0.1:8200/v1/sys/health | ConvertFrom-Json).sealed # Vault  -> False
curl http://127.0.0.1:8181/health                                    # OPA    -> {}
docker exec finix-rag-redis-1 redis-cli -a finix-redis-password ping # Redis  -> PONG
```

---

## 5. Download the models (~2.8 GB, one time)

```powershell
python scripts/download_models.py
# BGE-M3 embedding (~2.27 GB) + BGE-reranker-v2-m3 (~0.57 GB) -> HuggingFace cache
```

(They also download automatically on first ingest/query, but doing it explicitly makes
the first request fast and confirms connectivity.)

---

## 6. Run the API

```powershell
$env:PYTHONPATH = "src"
uvicorn finix_rag.api:app --host 0.0.0.0 --port 8000
```

On startup you should see `device=cuda` in the embedding log line, and
`nvidia-smi` will show python holding ~2.2 GB of VRAM.

**Smoke test (new terminal):**

```powershell
$base = "http://127.0.0.1:8000"
(curl "$base/health").Content            # {"status":"healthy","vault":"healthy",...}

# login -> token
$tok = (Invoke-RestMethod "$base/auth/login" -Method Post -ContentType application/json -Body (@{
  user_id="admin"; password="x"; role="admin"; department="ops"; clearance_level=4; tenant_id="default"
} | ConvertTo-Json)).access_token

# ingest a PDF
curl.exe -s -X POST "$base/ingest" -H "Authorization: Bearer $tok" -F "file=@some_report.pdf"

# query
Invoke-RestMethod "$base/query" -Method Post -H @{Authorization="Bearer $tok"} -ContentType application/json -Body (@{
  question="What was the gross NPA ratio?"; max_sources=5 } | ConvertTo-Json)
```

---

## 7. Accuracy test

A gold corpus of 5 banking documents + 12 labelled questions runs through the **full**
pipeline (auth → hybrid retrieval → rerank → Groq synthesis) and is scored.

```powershell
# infra up, venv active, models downloaded
$env:PYTHONPATH = "src"
python tests/accuracy/run_accuracy.py
```

It resets the collection, ingests the corpus, asks each question, and reports:

- **answer accuracy** — all expected keywords present in the generated answer
- **retrieval@1** — the correct document is the top citation
- **retrieval@k** — the correct document is cited at all

<!-- ACCURACY_RESULTS -->
**Measured on this box** (RTX 2050, embedding→GPU, reranker→CPU, `llama-3.3-70b-versatile`):

| Metric | Result |
|--------|--------|
| Answer accuracy (keywords present) | **12/12 = 100%** |
| Retrieval@1 (correct doc is top citation) | **12/12 = 100%** |
| Retrieval@k (correct doc cited) | **12/12 = 100%** |
| Mean confidence | 0.25 |
| Ingestion (5 docs, GPU embed) | ~2 s |

> Confidence is the mean **absolute** BGE-reranker score, which is deliberately
> conservative (a correct-but-not-lexically-identical passage often scores 0.1–0.3).
> It reflects reranker certainty, not answer correctness — the answers above are all
> correct. Judge quality by the accuracy metrics, not the raw confidence number.

Thresholds (the script exits non-zero below these, so it doubles as a CI gate):
`answer ≥ 70%`, `retrieval@1 ≥ 80%`.

To add your own cases, edit `tests/accuracy/gold_corpus.py` (`CORPUS` + `GOLD_QA`).

---

## 8. GPU memory notes (important on 4 GB)

BGE-M3 and BGE-reranker are each ~2.2 GB in fp32 — **both cannot sit on a 4 GB GPU at
once** (guaranteed CUDA OOM). The working split on this box:

- **BGE-M3 embedding → GPU** (`EMBEDDING_DEVICE=auto`) — this is the hot path, run on every
  chunk at ingest and every query.
- **BGE-reranker → CPU** (`RERANKER_DEVICE=cpu`) — only reranks ~20 candidates per query,
  so CPU latency is acceptable.

Symptoms of over-committing VRAM: `torch.cuda.OutOfMemoryError` on startup or first query.
Fixes, in order: (1) keep `RERANKER_DEVICE=cpu`; (2) close other GPU apps (Docker Desktop
alone eats ~1 GB — check `nvidia-smi`); (3) as a last resort set `EMBEDDING_DEVICE=cpu`.

On the production PSB GPU (≥ 12 GB) you can set `RERANKER_DEVICE=cuda` for full-GPU speed.

---

## 8b. Known limitations (read before production)

- **Hybrid retrieval currently runs dense-only.** `BM25Retriever.from_defaults(index=...)`
  can't build a keyword index over a Qdrant-backed store (the in-memory docstore is
  empty), so it logs `BM25Retriever unavailable, falling back to dense-only` and uses
  BGE-M3 dense retrieval alone. Accuracy is excellent (100% on the gold set) because
  BGE-M3 is a strong retriever, but if you need true BM25+dense fusion, wire BM25 over
  the actual nodes (e.g. a Qdrant sparse vector or a maintained docstore).
- **Integrity hashing is keyed off `API_SECRET_KEY`.** Re-ingest after rotating that key,
  or set a dedicated stable `INTEGRITY_HMAC_KEY`. Data ingested under a different key
  will fail verification and be dropped.
- **Vault runs in dev mode** (in-memory, single unseal key, known root token). Fine for
  dev; production needs a real initialized/unsealed, TLS-fronted Vault.
- **OCR (scanned PDFs/images) needs the separate `requirements-ocr.txt`**, which upgrades
  `huggingface-hub` past 1.0 — install it in its own venv if you need it. Text PDFs use
  the native path and need nothing extra.

## 9. Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `operator torchvision::nms does not exist` | mismatched torch/torchvision | `pip uninstall -y torch torchvision torchaudio` then install CUDA torch (§2) |
| `cuda: False` | CPU torch wheel | reinstall CUDA torch (§2) |
| `/health` shows `vault: unavailable` | Vault sealed / old prod-mode container | `docker compose up -d --force-recreate vault` (now dev mode) |
| `... failed integrity verification` | pre-fix ingest data in Qdrant | re-ingest (the harness resets the collection) |
| query 500 with `OPA unavailable` | OPA container down | `docker compose up -d opa`; check `curl :8181/health` |
| `... needs OCR ... PaddleOCR is not installed` | a *scanned* PDF/image was ingested | `pip install -r requirements-ocr.txt` (see its header re: huggingface-hub) |
| CUDA OOM | reranker on GPU on a small card | `RERANKER_DEVICE=cpu` (§8) |

---

## 10. Handy commands

```powershell
docker compose logs -f vault             # or qdrant / opa / redis
docker compose down                      # stop, keep data
docker compose down -v                   # stop, DESTROY all data (fresh start)
python tests/accuracy/run_accuracy.py --keep   # score without re-ingesting
```

See `docs/RUNBOOK.md` for extended operations (backup/recovery, key rotation, incident
response) — this file covers setup, GPU, and accuracy testing.
