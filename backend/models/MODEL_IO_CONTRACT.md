# FINIX Model I/O Contract

**Status:** Authoritative. This document is the source of truth for how the two
in-scope ONNX models are wired into the Go risk engine. Code comments in
`internal/domain/transaction/{risk_engine,heuristic}.go` and
`internal/domain/fraud/gnn.go` reference this file by name.

**Scope:** exactly two models —
`backend/models/transaction_risk_model.onnx` (transaction risk) and
`backend/models/MuleAccountDetection.onnx` (mule-account GNN). Every other
entry in `model_registry.json` is out of scope.

**Date:** 2026-07-28

<!-- SECTIONS-APPENDED-BELOW -->

---

## 1. The two models and how they chain

FINIX runs a **two-stage** fraud pipeline. Stage 1 scores accounts on the payment
graph; its output becomes one input feature to Stage 2, which scores an individual
transaction.

```
accounts + successful transfers
   └─(Stage 1)─ MuleAccountDetection.onnx  ──► per-node logits [n,2]
                    │ softmax per row, take column 1
                    ▼
              RecipientGNNScore ∈ [0,1]  (per account)
                    │  looked up for the transaction's recipient
                    ▼
   transaction features + RecipientGNNScore
   └─(Stage 2)─ transaction_risk_model.onnx ──► probabilities [1,2]
                    │ P(fraud) = column 1 → low/medium/high assessment
                    ▼
              Transaction risk decision
```

Both stages **fail safe**: if the ONNX runtime or a model is unavailable, Stage 1
falls back to the hand-written graph in `internal/domain/fraud/gnn.go` and Stage 2
falls back to the heuristic in `internal/domain/transaction/heuristic.go`. Neither
stage ever emits a *guessed* ML score.

---

## 2. Verified tensor contracts

Read directly from the model graphs (see `ONNX_INTEGRATION_AUDIT.md §2`).

### transaction_risk_model.onnx (Stage 2)

```
INPUT   input          float32  [N, 32]   ← 32 encoded features per transaction
OUTPUT  label          int64    [N]       ← argmax class (0/1)  — index 0 in output list
OUTPUT  probabilities  float32  [N, 2]    ← [P(legit), P(fraud)] — index 1
```

- Producer OnnxMLTools, opset `ai.onnx.ml` v1, `TreeEnsembleClassifier`.
- Binary classifier. `P(fraud) = probabilities[1]`.
- **Select outputs by NAME (`probabilities`), not position** — `label` (int64) is
  the first output and a positional read would type-assert the wrong tensor.
  (`internal/domain/transaction/onnx_predictor.go` discovers the index by name.)

### MuleAccountDetection.onnx (Stage 1)

```
INPUTS
  node_features  float32  [num_nodes, 20]   ← 20 encoded features per account
  edge_index     int64    [2, num_edges]    ← COO edge list: row 0 senders, row 1 receivers
OUTPUT
  logits         float32  [num_nodes, 2]    ← RAW logits (NOT probabilities)
```

- Producer pytorch (GraphSAGE), opset `ai.onnx` v18.
- Multi-input, whole-graph model: it scores every node in one call.
- **Output is raw logits** — apply `fraud.Softmax` per row and take column 1 for
  `P(mule)`. Tensor names are matched by string, so a re-export that reorders I/O
  does not silently break inference (`NewMuleONNXPredictor` verifies the three
  names and disables itself on mismatch).

Label maps: `transaction_risk_labels.json` `{"0":"Legitimate Transaction","1":"Fraudulent Transaction"}`,
`mule_labels.json` `{"0":"Normal Account","1":"Mule Account"}`.

---

## 3. Graph assembly (Stage 1 input construction)

`fraud.AssembleGraph` turns accounts + successful transfers into the two tensors.
Rules (must match the training-time graph construction — see the Mule GNN Dataset
Spec v2.0 §10 "Graph Construction Rules"):

- Node = account; edge = **one directed** edge sender → receiver per **successful**
  transfer. Pass only settled transfers; failed/reversed ones must not create edges.
- **No self-loops** (`from == to` dropped).
- **No synthetic reverse edges** — the graph is directed.
- Transfers referencing an unknown account are dropped (can't be indexed).
- Node order is deterministic (`sort.Strings` on account ID) so the same input
  always yields the same row layout — required for reproducible scores and the
  golden test. `AssembledGraph.Order`/`IndexOf` map row index ↔ account ID so a
  specific account's score can be read back out.

`edge_index` is laid out row-major as `[senders..., receivers...]` (length
`2 * num_edges`), matching the `[2, num_edges]` tensor.

---

## 4. THE GATING DEPENDENCY: training-time preprocessing (still outstanding)

An ONNX model is a function `f(tensor) → tensor`; it carries **no reliable
description of what each input column means**. The two dataset specifications now
in this folder —

- `FINIX_Transaction_Risk_Dataset_Specification_v3.0_FINAL_Formatted.docx`
- `FINIX_Mule_GNN_Dataset_Specification_v2.0_FINAL.docx`

— enumerate the **candidate raw fields and engineered features**, but they are a
*dataset schema*, **not** the preprocessing artifact. They do **not** pin down:

1. The exact **ordered** 32-column (transaction) and 20-column (node) vectors —
   which engineered features are included and in what position.
2. **Categorical encodings** (e.g. how `payment_channel`, `account_type`,
   `occupation` map to numbers — one-hot? ordinal? target?).
3. **Scaler statistics** (mean/std or min/max) applied at training time.
4. The **decision threshold** used at training/eval (fraud rarely uses 0.5).

Until those land (as `preprocess.joblib` + a handful of exported golden vectors),
the two encoder functions stay stubs and both stages fall back:

| Encoder | File | State |
|---|---|---|
| 32-col transaction vector | `transaction/heuristic.go:signalToFeatures` | emits 9 raw signals + `NOTE(32)` — width mismatch → heuristic fallback |
| 20-col node vector | `fraud/graph_predictor.go:encodeMuleNode` | returns `errMuleEncodingUnset` → graph fallback |

**Do NOT guess these orderings.** Wire them from the supplied preprocessing and
lock each with a golden-vector test (`fraud/mule_golden_test.go` exists and is RED
under `-tags onnx` until `muleGoldenVectors` is populated).

---

## 5. Binary-probability handling (Stage 2)

`transaction/assessmentFromProbs` consumes `[P(legit), P(fraud)]`:
`pFraud = probabilities[1]`; `score = pFraud * 100`; then
`pFraud ≥ 0.70 → high`, `≥ 0.40 → medium`, else `low`. Those thresholds
(`fraudProbMediumThreshold` / `fraudProbHighThreshold`) are **placeholders**
pending the trained operating point and should move to auditable config once known.

---

## 6. Build & runtime wiring

- **Build tags.** ONNX bindings compile only under `-tags onnx`
  (`transaction/onnx_predictor.go`, `fraud/mule_onnx.go`). The default build uses
  the `//go:build !onnx` stubs (`model_stub.go`, `mule_stub.go`), so `make test`
  needs no native runtime and exercises the fallbacks. "It builds and passes tests"
  therefore says nothing about the model path — verify under `-tags onnx` too.
- **Native library.** `github.com/yalue/onnxruntime_go` is a cgo wrapper; ship
  `onnxruntime.dll`/`.so` and point `FINIX_ONNXRUNTIME_LIB` at it before
  `InitializeEnvironment()`. If unset and not auto-discoverable, the predictor
  disables itself (safe).
- **Config.** `FINIX_RISK_MODEL_PATH` (default `../../models/transaction_risk_model.onnx`)
  and `FINIX_MULE_MODEL_PATH` (default `../../models/MuleAccountDetection.onnx`),
  wired via `config.ModelsConfig`.

---

## 7. Integration checklist

Structural seam — **done** (both stages loadable, graph assembly + softmax + scoring wired, config + build tags + fallbacks):

- [x] `GraphPredictor` interface + `MuleONNXPredictor` (onnx) + stub (!onnx).
- [x] `AssembleGraph` (deterministic order, directed COO, edge rules).
- [x] `ScoreMuleAccounts` (fail-safe, per-row softmax, `P(mule)=col 1`).
- [x] `transaction` output-by-name + binary `[P(legit),P(fraud)]` handling.
- [x] Default model paths fixed (A-5); `FINIX_MULE_MODEL_PATH` added.
- [x] `mule_golden_test.go` scaffold (RED under `-tags onnx` until vectors supplied).

Blocked on the supplied preprocessing artifact — **pending**:

- [ ] Implement `encodeMuleNode` (20 cols) from preprocessing; populate `muleGoldenVectors`.
- [ ] Implement `signalToFeatures` (32 cols) from preprocessing; add its golden test.
- [ ] Replace placeholder decision thresholds with the trained operating point.
- [ ] Wire `ScoreMuleAccounts` output into the live `RecipientGNNScore` fed to Stage 2.
- [ ] Build + inference smoke test under `-tags onnx` with the native runtime.

> **Note:** the mule scoring seam is fully built but is **NOT** wired into the live
> request path yet — `encodeMuleNode` is a stub, so `ScoreMuleAccounts` returns no
> scores and the live path still uses the hand-written `fraud/gnn.go`. Wiring the
> ONNX score into `RecipientGNNScore` is the final step, gated on the encoder.
