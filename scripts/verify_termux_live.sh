#!/usr/bin/env bash
# Serial live verification for memory-constrained Android/Termux.
# Go's default package parallelism can launch multiple 1.6 GB Gemma processes.
set -euo pipefail
cd "$(dirname "$0")/.."
: "${GEMMA_BIN:?Set GEMMA_BIN to the local llama-cli executable}"
: "${GEMMA_MODEL:?Set GEMMA_MODEL to the local Gemma GGUF model}"
if [[ ! -x "$GEMMA_BIN" || ! -r "$GEMMA_MODEL" ]]; then
  echo "Gemma executable/model missing or unreadable" >&2
  exit 1
fi
# Keep the full suite and real inference tests; serialize packages rather
# than disabling the live tests or weakening source-grounding.
export GOMAXPROCS="${GOMAXPROCS:-2}"
export GEMMA_TIMEOUT_SECONDS="${GEMMA_TIMEOUT_SECONDS:-300}"
echo "Running full Go suite with one test package at a time (live Gemma enabled)"
go test -p 1 -parallel 1 -count=1 -timeout 40m ./...
go vet -p 1 ./...
echo "PASS: full Go suite and vet"
