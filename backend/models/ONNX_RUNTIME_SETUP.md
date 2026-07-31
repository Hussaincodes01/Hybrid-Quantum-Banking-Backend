# Running the ONNX models (Blocker 3 — runtime setup)

By default the backend builds **without** ONNX and uses the heuristic fallback.
To actually execute the two trained models you build with `-tags onnx`, which
uses [`yalue/onnxruntime_go`](https://github.com/yalue/onnxruntime_go) — a **CGo**
binding. That means two host prerequisites: a **C compiler** and the
**ONNX Runtime shared library**. This is a one-time setup on whatever machine
runs the server.

> Version lock: this repo pins `onnxruntime_go v1.31.0`, whose C headers are
> `ORT_API_VERSION 26` → **you need ONNX Runtime `1.26.0`**. A mismatched runtime
> version will fail to load at startup.

---

## 1. Install a C compiler

**Windows** (pick one):
```powershell
winget install -e --id BrechtSanders.WinLibs.POSIX.UCRT.LLVM   # winlibs (gcc)
# or:  choco install mingw -y
# or:  scoop install gcc
```
Verify `gcc` is on PATH: `gcc --version`. (Missing gcc is the
`fatal error: windows.h: No such file or directory` you'll see otherwise.)

**Linux (Debian/Ubuntu):**
```bash
sudo apt-get update && sudo apt-get install -y build-essential
```

## 2. Get the ONNX Runtime 1.26.0 shared library

Use the helper script (downloads + extracts the correct version):
```powershell
# Windows
pwsh backend/scripts/setup_onnx_runtime.ps1
```
```bash
# Linux/macOS
bash backend/scripts/setup_onnx_runtime.sh
```
It drops the library at `backend/runtime/` and prints the exact
`FINIX_ONNXRUNTIME_LIB` value to export.

Manual alternative — download from the
[ONNX Runtime 1.26.0 release](https://github.com/microsoft/onnxruntime/releases/tag/v1.26.0):
- Windows: `onnxruntime-win-x64-1.26.0.zip` → `lib/onnxruntime.dll`
- Linux:   `onnxruntime-linux-x64-1.26.0.tgz` → `lib/libonnxruntime.so`

## 3. Environment variables

| Variable | Purpose | Default |
|---|---|---|
| `FINIX_ONNXRUNTIME_LIB` | **Required.** Absolute path to `onnxruntime.dll` / `libonnxruntime.so` | (none → ONNX disabled) |
| `FINIX_RISK_MODEL_PATH` | transaction model | `../../models/transaction_risk_model.onnx` |
| `FINIX_MULE_MODEL_PATH` | mule model | `../../models/MuleAccountDetection.onnx` |
| `FINIX_RISK_PREPROCESS_PATH` | txn scaler/encoders | derived: `transaction_preprocess.json` beside the model |
| `FINIX_COLD_START_MONTHS` | accounts younger than this skip ONNX | `6` (set `0` to always use ONNX) |

The model paths default to being run from `backend/cmd/server` (hence `../../models/…`).
The preprocess JSONs (`transaction_preprocess.json`, `mule_preprocess.json`) must sit
**next to** their `.onnx` files — they already do in `backend/models/`.

## 4. Build & run with ONNX

```powershell
# Windows (PowerShell)
$env:CGO_ENABLED = "1"
$env:FINIX_ONNXRUNTIME_LIB = "D:\FINIX_PRODUCTION\backend\runtime\onnxruntime.dll"
$env:FINIX_COLD_START_MONTHS = "0"   # so seeded/new accounts still hit the model
go build -tags onnx ./...
go run  -tags onnx ./cmd/server
```
```bash
# Linux
export CGO_ENABLED=1
export FINIX_ONNXRUNTIME_LIB=/path/to/backend/runtime/libonnxruntime.so
export FINIX_COLD_START_MONTHS=0
go build -tags onnx ./...
```

## 5. Verify it's live (not silently falling back)

On startup the logs should show **both**:
```
transaction preprocess artifact loaded  features=32
mule node encoder wired                 features=20
```
If instead you see `... artifact unavailable; ... gated off`, the JSON wasn't
found at the expected path (see §3).

Run the golden parity check (transaction model → P(fraud) from
`transaction_golden.json`):
```bash
go test -tags onnx ./internal/domain/transaction/ -run Golden -v
```

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `fatal error: windows.h: No such file` | No C compiler — do §1. |
| `Error loading ONNX shared library` / `InitializeEnvironment` fails | Wrong path or version — must be **1.26.0**; check `FINIX_ONNXRUNTIME_LIB`. |
| Startup log says `artifact unavailable` | `*_preprocess.json` not beside the `.onnx`; check §3 paths. |
| Model never fires, always heuristic | Account age < `FINIX_COLD_START_MONTHS`; set it to `0` to test. |
| `probabilities output ... not float32` | Wrong/re-exported model; the shipped `transaction_risk_model.onnx` exposes a named `probabilities` output. |

## What runs once this is set up

- **Transaction model** (`transaction_risk_model.onnx`): every transaction →
  `featureMaps` → `preprocess.BuildVector` (label-encode + StandardScale the 27
  scaled cols, 5 raw bool flags) → ONNX → P(fraud) → low/med/high (0.40 / 0.70).
- **Mule model** (`MuleAccountDetection.onnx`): on each transaction the account
  graph is reassembled, GNN scores P(mule) per account, and that feeds
  `RecipientGNNScore` into the transaction model (two-stage pipeline).
