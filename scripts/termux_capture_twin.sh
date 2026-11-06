#!/usr/bin/env bash
# One-time Termux capture into a private local bundle; no credentials uploaded.
# Invoke from a checkout of emergion-sovereign-runtime.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${HOME}/unifusion-twin-capture/${STAMP}"
mkdir -p "$OUT"
SOURCES=()
for path in "$HOME/UNIFUSION" "/storage/emulated/0/VERITEX" "$HOME/AXM/emergion-sovereign-runtime"; do
  if [ -d "$path" ]; then SOURCES+=(--source "$path"); fi
done
if [ "${#SOURCES[@]}" -eq 0 ]; then
  echo "No known source directories accessible. Nothing captured." >&2
  exit 1
fi
python3 "$ROOT/scripts/capture_external_sources.py" "${SOURCES[@]}" --output "$OUT"
echo "Capture stored at: $OUT"
echo "Review EXTERNAL_SOURCE_LEDGER.json and copied sources before sharing."
echo "Do NOT commit or upload the bundle until secrets and personal data are reviewed."
