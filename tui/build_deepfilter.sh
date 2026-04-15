#!/usr/bin/env bash
# Clones and builds DeepFilterNet's C library (libdeep_filter.so).
# Run this once before `go build` when setting up the project.
# Requires: cargo, rustc, git, git-lfs

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC_DIR="$SCRIPT_DIR/deep-filter-net"
OUT_DIR="$SCRIPT_DIR/internal"

# Clone if not present
if [ ! -d "$SRC_DIR/.git" ]; then
    echo "Cloning DeepFilterNet..."
    git clone --depth=1 https://github.com/Rikorose/DeepFilterNet "$SRC_DIR"
fi

# Make sure the model file is present (may require git-lfs)
MODEL_FILE="$SRC_DIR/models/DeepFilterNet3_onnx.tar.gz"
if [ ! -s "$MODEL_FILE" ]; then
    echo "Model file not found or empty: $MODEL_FILE"
    echo "Trying git lfs pull..."
    (cd "$SRC_DIR" && git lfs pull --include "models/DeepFilterNet3_onnx.tar.gz") || {
        echo ""
        echo "ERROR: Could not retrieve model file."
        echo "Install git-lfs and re-run, or download manually:"
        echo "  https://github.com/Rikorose/DeepFilterNet/raw/main/models/DeepFilterNet3_onnx.tar.gz"
        echo "  -> $MODEL_FILE"
        exit 1
    }
fi

echo "Building libdf.so (this takes a few minutes on first run)..."
(
    cd "$SRC_DIR/libDF"
    # Patch capi.rs to support empty path → embedded default model (DFN3)
    if ! grep -q "model_path.is_empty" src/capi.rs; then
        perl -i -pe 's/DfParams::new\(PathBuf::from\(model_path\)\)\.expect\("Could not load model from path"\);/if model_path.is_empty() { DfParams::default() } else { DfParams::new(PathBuf::from(model_path)).expect("Could not load model from path") };/' src/capi.rs
    fi
    cargo build --release --lib --features capi,tract
)

if [ -n "${CARGO_BUILD_TARGET:-}" ]; then
    LIB="$SRC_DIR/target/${CARGO_BUILD_TARGET}/release/libdf.a"
else
    LIB="$SRC_DIR/target/release/libdf.a"
fi
if [ ! -f "$LIB" ]; then
    echo "ERROR: Expected library not found at $LIB"
    exit 1
fi

cp "$LIB" "$OUT_DIR/"
echo "Done. Copied $LIB -> $OUT_DIR/libdf.a"
echo "The model is statically linked — go build now produces a single self-contained binary."
