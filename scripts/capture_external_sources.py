#!/usr/bin/env python3
"""Capture *available* external and local source trees into an inspectable twin.

Read-only to source roots. Explicit paths only; never automatically capture
private runtime state, credentials, models, evidence ledgers or secrets.
Copies permitted source files to an export folder and hashes exact bytes.
"""
import argparse
import datetime
import hashlib
import json
import os
import pathlib
import shutil

EXCLUDE_DIRS = {".git", ".field", "__pycache__", ".venv", "node_modules", "secrets", "credentials", "private", "backups"}
EXCLUDE_SUFFIXES = {".key", ".pem", ".p12", ".pfx", ".db", ".sqlite", ".gguf", ".bin"}
EXCLUDE_NAMES = {".env", "events.wvc.jsonl", "runtime_receipts.jsonl", "field.cosl"}
SOURCE_SUFFIXES = {".go", ".py", ".sh", ".js", ".ts", ".tsx", ".jsx", ".json", ".yaml", ".yml", ".toml", ".md", ".html", ".css", ".sql", ".proto"}
MAX_BYTES = 5_000_000

def allowed(path):
    return path.name not in EXCLUDE_NAMES and path.suffix.lower() in SOURCE_SUFFIXES and path.suffix.lower() not in EXCLUDE_SUFFIXES and path.stat().st_size <= MAX_BYTES

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--source", action="append", required=True, help="Explicitly authorized source tree (repeatable)")
    p.add_argument("--output", required=True, help="Export directory outside source trees")
    args = p.parse_args()
    out = pathlib.Path(args.output).expanduser().resolve()
    roots = [pathlib.Path(s).expanduser().resolve() for s in args.source]
    if any(out == root or out.is_relative_to(root) for root in roots):
        p.error("output must be outside all source trees")
    out.mkdir(parents=True, exist_ok=True)
    records, gaps = [], []
    for index, root in enumerate(roots):
        if not root.is_dir():
            gaps.append({"source": str(root), "status": "NOT_ACCESSIBLE"})
            continue
        for folder, dirs, files in os.walk(root, followlinks=False):
            dirs[:] = sorted(d for d in dirs if d not in EXCLUDE_DIRS and not (pathlib.Path(folder)/d).is_symlink())
            for name in sorted(files):
                path = pathlib.Path(folder)/name
                if path.is_symlink() or not path.is_file() or not allowed(path):
                    continue
                rel = path.relative_to(root)
                target = out / "sources" / f"root_{index}" / rel
                target.parent.mkdir(parents=True, exist_ok=True)
                try:
                    data = path.read_bytes()
                    target.write_bytes(data)
                except OSError as exc:
                    gaps.append({"source": str(path), "status": "READ_ERROR", "detail": str(exc)})
                    continue
                records.append({"source_root": str(root), "path": str(rel), "copy": str(target.relative_to(out)), "sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data)})
    result = {"schema": "UNIFUSION_EXTERNAL_SOURCE_CAPTURE_v1", "authority": "READ_ONLY_SOURCE_COPY_NOT_COSL_REG", "captured_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(), "file_count": len(records), "files": records, "gaps": gaps, "excluded": "runtime ledgers, secrets, credentials, binaries, models, and large files"}
    (out / "EXTERNAL_SOURCE_LEDGER.json").write_text(json.dumps(result, indent=2) + "\n")
    print(f"Captured {len(records)} source files; {len(gaps)} inaccessible/read errors; ledger at {out}")

if __name__ == "__main__":
    main()
