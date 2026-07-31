# ONNX Model Integration Audit — FINIX Backend

**Date:** 2026-07-28
**Scope:** `PSB Golang Stack/backend/models/` (two ONNX models + label JSONs) vs. the existing Go inference path in `internal/domain/transaction/`.
**Verdict:** ⚠️ **Not integratable as-is.** Both models have hard contract mismatches with the current code, and one critical input (the exact feature list used at training time) is missing from the repo. The models load fine as files, but wiring them correctly requires the fixes below.

---

## 1. What you dropped in

| File | Size | Producer | Opset | Type |
|---|---|---|---|---|
| `transaction_risk_model.onnx` | 227 KB | OnnxMLTools | `ai.onnx.ml` v1 | `TreeEnsembleClassifier` (one node) |
| `MuleAccountDetection.onnx` | 56 KB | pytorch | `ai.onnx` v18 | Graph Neural Net (Gemm/BatchNorm/Relu/Scatter) |
| `transaction_risk_labels.json` | — | — | — | `{"0":"Legitimate Transaction","1":"Fraudulent Transaction"}` |
| `mule_labels.json` | — | — | — | `{"0":"Normal Account","1":"Mule Account"}` |

Both files are valid ONNX protobuf and parse cleanly. The runtime library (`github.com/yalue/onnxruntime_go v1.31.0`) is already in `go.mod`.

---

## 2. Exact tensor contracts (read from the model graphs)

### transaction_risk_model.onnx
```
INPUT   input          float32  [N, 32]      ← 32 features per transaction
OUTPUT  label          int64    [N]          ← predicted class (0/1)
OUTPUT  probabilities  float32  [N, 2]       ← [P(legit), P(fraud)]
```
Binary classifier. A tree ensemble exported from sklearn/LightGBM-style tooling.

### MuleAccountDetection.onnx
```
INPUT   node_features  float32  [num_nodes, 20]   ← 20 features per account/node
INPUT   edge_index     int64    [2, num_edges]    ← COO edge list (graph structure)
OUTPUT  logits         float32  [num_nodes, 2]    ← raw logits, need softmax → [P(normal), P(mule)]
```
This is a **graph** model: it scores every node in a payment graph at once, and it needs the edge list as a second input. It is architecturally the ML counterpart to your hand-written `internal/domain/fraud/gnn.go`.

---

## 3. Compatibility findings

### transaction_risk_model.onnx vs. `onnx_predictor.go` + `risk_engine.go`

| # | Severity | Finding |
|---|---|---|
| **A-1** | 🔴 Blocker | **Feature count: 9 sent vs 32 required.** `signalToFeatures()` in `heuristic.go` emits 9 floats; the model's `input` is `[N,32]`. ORT will reject the tensor shape. |
| **A-2** | 🔴 Blocker | **Output arity: code expects 3 classes, model emits 2.** `onnx_predictor.go` errors on `len(outputData) < 3` and `assessmentFromProbs` indexes `probs[0..2]` (low/med/high). The model is binary `[legit, fraud]`. This will always fail or panic. |
| **A-3** | 🔴 Blocker | **Wrong output tensor read.** Code does `outputs[0].(*ort.TensorFloat32)`. But `outputs[0]` is `label` (**int64**), and `probabilities` is `outputs[1]`. The type assertion fails → inference "fails" → silent fallback to heuristic. Select outputs **by name**, not index. |
| **A-4** | 🟠 High | **Feature order/scaling unknown.** There is no `ml/` training script in the repo (CLAUDE.md references it, but it doesn't exist on disk). The 32 features' names, order, and any normalization/standardization applied at training time are unknown. Feeding features in the wrong order produces confident-but-wrong scores — worse than no model. |
| **A-5** | 🟡 Med | **Default path is wrong.** `config.go` and `risk_engine.go` default to `../../models/security/fraud_label/v1/model.onnx`. Actual file is `backend/models/transaction_risk_model.onnx`. Nothing loads without an explicit `FINIX_RISK_MODEL_PATH`. |
| **A-6** | 🟡 Med | **Build tag gating.** ONNX only compiles under `-tags onnx`; the default build uses `model_stub.go` (`IsAvailable()==false`) and never touches the model. Fine as a safety default, but means "it builds and passes tests" tells you nothing about the model path working. |

### MuleAccountDetection.onnx

| # | Severity | Finding |
|---|---|---|
| **B-1** | 🔴 Blocker | **No consumer exists.** No Go file references it. It is not wired to anything. |
| **B-2** | 🔴 Blocker | **Interface can't express it.** `ModelPredictor.Predict(features []float32)` is single-input, single-dtype. This model needs **two** inputs, one of them **int64** (`edge_index`). You cannot call it through the current interface. |
| **B-3** | 🟠 High | **It's a whole-graph model, not per-transaction.** It scores all nodes given the graph topology. To use it you must materialize a node-feature matrix + edge list from your account/transaction data — a different data-prep pipeline than the transaction risk model. It overlaps with the existing heuristic `fraud/gnn.go`; decide whether it replaces or augments it. |
| **B-4** | 🟠 High | **20-feature node spec unknown** (same root cause as A-4). |
| **B-5** | 🟡 Med | **Logits, not probabilities.** Output is raw logits; you must apply softmax in Go before thresholding. |

### Runtime / ops (both)

| # | Severity | Finding |
|---|---|---|
| **C-1** | 🟠 High | **Native library required.** `yalue/onnxruntime_go` is a cgo wrapper over the ONNX Runtime shared library. You must ship `onnxruntime.dll` (Windows) / `.so` (Linux) and call `ort.SetSharedLibraryPath(...)` before `InitializeEnvironment()`. The current code never sets the path, so on any machine without ORT auto-discoverable it silently disables the predictor. |
| **C-2** | 🟡 Med | **`ai.onnx.ml` opset.** The tree model uses ML ops (`TreeEnsembleClassifier`). Standard ORT builds include these, but a trimmed/mobile build may not. Verify your ORT build has the ML operator set. |
| **C-3** | 🟡 Med | **API drift risk.** `NewTensorFromFloat32Bytes` and the `outputs[0].(*ort.TensorFloat32)` shape in `onnx_predictor.go` should be re-checked against v1.31.0's actual API when you enable the `onnx` tag; this file has likely never been compiled against the pinned version. |

---

## 4. The one thing that blocks everything: the feature spec

Both A-4 and B-4 are the real gate. An ONNX model is just a function `f(tensor)→tensor`; it carries **no reliable description of what each input column means**. Getting 32 (or 20) numbers in the exact order, units, and scaling the model was trained on is mandatory — otherwise inference "works" and returns garbage.

**You must obtain, from whoever trained these models:**
1. The ordered list of the 32 transaction features and the 20 mule-node features (column names).
2. Any preprocessing: standardization (mean/std), min-max scaling, log transforms, categorical encodings.
3. For the mule model: how `edge_index` is constructed (directed? both directions? self-loops? node indexing scheme).
4. The decision threshold used in training/eval (fraud rarely uses 0.5).

If the training code lives elsewhere (a `ml/` folder, a notebook, a separate repo), that's the source of truth. Without it, integration is guesswork.

---

## 5. Integration plan (once the feature spec is in hand)

### Phase 0 — Runtime prerequisites
- Vendor the ONNX Runtime shared lib for each target OS; add `ort.SetSharedLibraryPath()` driven by an env var (`FINIX_ONNXRUNTIME_LIB`).
- Fix the default model paths in `config.go` / `risk_engine.go` to `backend/models/*.onnx`, or (better) make both paths explicit config with no silent default.

### Phase 1 — Transaction risk model (lower effort, high value)
1. Rewrite `signalToFeatures` to emit the **32** features in the trained order (needs the spec). Keep it in one well-documented function — it is the contract.
2. In `onnx_predictor.go`: select outputs **by name** (`probabilities`), handle `[N,2]`, drop the `< 3` check.
3. Rewrite `assessmentFromProbs` for **binary** output: `pFraud = probs[1]`; map to low/med/high via calibrated thresholds (put thresholds in `config` params, not literals).
4. Add a golden-vector test: a handful of `(features → expected probability)` pairs exported from the training environment, asserted in Go so a bad wiring fails CI. This is the single most important safeguard.
5. Keep the heuristic as the fallback (already the design) — good.

### Phase 2 — Mule GNN model (larger effort)
1. Extend the abstraction beyond `Predict([]float32)`. Introduce a `GraphPredictor` interface: `PredictGraph(nodeFeatures [][]float32, edges [][2]int64) ([][]float32, error)` (multi-input, returns per-node logits).
2. Build a graph-assembly step that turns your account/payment data into `node_features [n,20]` + `edge_index [2,e]`, with a stable node-index map so you can read a specific account's score back out.
3. Apply softmax to `logits`; threshold `P(mule)`.
4. Decide the relationship with `fraud/gnn.go`: run the ONNX GNN as the scorer and keep the hand-written graph as fallback, or replace. Wire the mule score into the same risk assessment that already consumes `RecipientGNNScore`.

### Phase 3 — Ops & safety
- Log model version + which path (ONNX vs heuristic) served each decision (audit requirement — you already thread `ModelVersion` through `Assessment`; populate it from the model, currently unset).
- Add a startup self-test that runs one canned inference and disables ONNX (falling back to heuristic) if the output shape/range is wrong — fail safe, not fail open.
- Monitor score distribution drift.

---

## 6. Bottom line

- **Files are valid and the runtime dep is present** — nothing wrong with the models themselves.
- **transaction_risk_model.onnx**: integratable with moderate code changes (fix feature count 9→32, binary output handling, output-by-name, path) **once the 32-feature spec is provided.**
- **MuleAccountDetection.onnx**: needs a new multi-input graph interface and a graph-assembly pipeline; it does not fit the current single-vector predictor at all. Larger effort.
- **Hard dependency:** the training feature specification for both models. This is the gating item — please source it before I write the wiring, so we build against the real contract rather than a guess.

*File references: `internal/domain/transaction/{risk_engine,onnx_predictor,model_stub,model,heuristic}.go`, `internal/config/config.go`, `internal/domain/fraud/gnn.go`, `backend/models/*`.*

---

## 7. v2 reconciliation — structural integration built (2026-07-29)

Since the original audit, the structural integration for **both** in-scope models
has been built, and two **dataset specification** docs were added to this folder.
This section reconciles the findings above against that new state.

### 7.1 What changed in code

| Original finding | Status now | Where |
|---|---|---|
| A-2 output arity (3 vs 2) | ✅ Fixed | `assessmentFromProbs` handles binary `[P(legit),P(fraud)]`; `< 3` check dropped. |
| A-3 wrong output tensor | ✅ Fixed | `onnx_predictor.go` selects `probabilities` **by name**, not index 0. |
| A-5 wrong default path | ✅ Fixed | `config.go` + `.env.example` default to `../../models/transaction_risk_model.onnx`. |
| B-1 no mule consumer | ✅ Fixed (seam) | `fraud/graph_predictor.go` (`AssembleGraph`, `ScoreMuleAccounts`) + `mule_onnx.go`/`mule_stub.go`. |
| B-2 interface can't express multi-input | ✅ Fixed | New `GraphPredictor` interface (`node_features` + `edge_index` → logits); separate from the single-vector `ModelPredictor`. |
| B-3 whole-graph pipeline | ✅ Fixed (seam) | `AssembleGraph` materialises `[n,20]` + directed `[2,e]` with stable node index. |
| B-5 logits not probabilities | ✅ Fixed | `ScoreMuleAccounts` applies `fraud.Softmax` per row, takes column 1. |
| C-1 native lib path | ✅ Fixed | Both predictors gate `ort.SetSharedLibraryPath` on `FINIX_ONNXRUNTIME_LIB` before `InitializeEnvironment`. |
| A-1 feature count 9 vs 32 | 🔴 **Still open** | `signalToFeatures` still emits 9 (safe width mismatch → heuristic). Blocked on preprocessing. |
| A-4 / B-4 feature spec unknown | 🔴 **Still open** | See §7.2 — the new docs are a schema, not the preprocessing. |
| C-2 / C-3 opset / API drift | 🟡 Unverified | No Go toolchain in this environment; must be checked under `-tags onnx` on a real machine. |

Config now also exposes `FINIX_MULE_MODEL_PATH` (default
`../../models/MuleAccountDetection.onnx`) via `config.ModelsConfig`.

### 7.2 The two new dataset specs do NOT close A-4 / B-4

`FINIX_Transaction_Risk_Dataset_Specification_v3.0_FINAL_Formatted.docx` and
`FINIX_Mule_GNN_Dataset_Specification_v2.0_FINAL.docx` are welcome context — they
confirm the two-stage design (the transaction spec's §12 explicitly lists
`RecipientGNNScore` as an external input **generated by the Mule GNN**), the binary
targets (`is_fraud`, `is_mule_account`), and the directed-edge graph construction
rule (mule spec §10). **But they are a dataset schema, not the training-time
preprocessing.** They enumerate ~40 raw transaction fields + 10 engineered features
and ~13 node fields + 11 computed graph features — far more than the 32 / 20 the
models actually take — and they do **not** specify:

1. **Which** features made the final 32-col / 20-col vectors, or their **order**.
2. **Categorical encodings** (e.g. `payment_channel`, `account_type`, `occupation`).
3. **Scaler statistics** (mean/std or min/max) applied before training.
4. The **decision threshold** (the transaction spec lists `RiskScore` as an
   engineered feature but gives no operating point).

So the gating item from §4 stands: the ordered column layout + encoders + scaler
stats (the `preprocess.joblib` artifact) are still required before
`signalToFeatures` (32) and `encodeMuleNode` (20) can be wired. The code keeps
both encoders as explicit stubs and **fails safe** rather than guessing.

### 7.3 Verification status

All v2 code was written **without a Go toolchain or network in the authoring
environment** — it is **unverified by execution**. Before relying on it:

- `make test` (default, no `-tags onnx`) — expected green; exercises fallbacks.
- `go build -tags onnx ./...` + `go test -tags onnx ./internal/domain/fraud/ ./internal/domain/transaction/`
  with the native ONNX Runtime present — expected to **compile**, with the mule
  golden test RED until `muleGoldenVectors` is populated.

### 7.4 Bottom line (v2)

The structural seam is complete and safe: both models are loadable, the mule
graph→softmax→score pipeline and the binary transaction path are wired, config and
build-tag fallbacks are correct. Exactly **two encoder functions and one golden
test remain intentionally RED**, gated solely on the training-time preprocessing
artifact. No ML score reaches a user until that artifact lands and the golden tests
pass.
