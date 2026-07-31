#!/usr/bin/env bash
# setup_onnx_runtime.sh — download the ONNX Runtime shared library matching
# onnxruntime_go v1.31.0 (ORT_API_VERSION 26 -> onnxruntime 1.26.0) into
# backend/runtime/. Run once on the Linux host that will serve the models.
#
#   bash backend/scripts/setup_onnx_runtime.sh
#
# It does NOT install a C compiler (see backend/models/ONNX_RUNTIME_SETUP.md §1).
set -euo pipefail

VERSION="1.26.0"
TGZ="onnxruntime-linux-x64-${VERSION}.tgz"
URL="https://github.com/microsoft/onnxruntime/releases/download/v${VERSION}/${TGZ}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND="$(dirname "$SCRIPT_DIR")"
RUNTIME="$BACKEND/runtime"
TMP="$(mktemp -d)"

mkdir -p "$RUNTIME"
echo "Downloading $URL ..."
curl -fsSL "$URL" -o "$TMP/$TGZ"
echo "Extracting ..."
tar -xzf "$TMP/$TGZ" -C "$TMP"

SO="$(find "$TMP" -name 'libonnxruntime.so*' | head -1)"
[ -n "$SO" ] || { echo "libonnxruntime.so not found in archive" >&2; exit 1; }
cp "$SO" "$RUNTIME/libonnxruntime.so"

LIB="$RUNTIME/libonnxruntime.so"
echo ""
echo "ONNX Runtime ${VERSION} ready: $LIB"
echo ""
echo "Next: set the env var and build with the onnx tag:"
echo "  export CGO_ENABLED=1"
echo "  export FINIX_ONNXRUNTIME_LIB=$LIB"
echo "  go build -tags onnx ./..."
command -v gcc >/dev/null 2>&1 || echo "WARNING: gcc not found; install build-essential (see ONNX_RUNTIME_SETUP.md §1)."
rm -rf "$TMP"
