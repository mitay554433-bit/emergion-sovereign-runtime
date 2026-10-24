#!/usr/bin/env python3
"""Generate a reproducible SHA-256 inventory of tracked repository files.

This is a build artifact, never COSL/REG authority. Run from repository root.
"""
import argparse
import hashlib
import json
import pathlib
import subprocess

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", default="REPOSITORY_MANIFEST.current.json")
    args = parser.parse_args()
    root = pathlib.Path(__file__).resolve().parent.parent
    paths = subprocess.check_output(["git", "ls-files", "-z"], cwd=root).split(b"\0")
    files = []
    output = pathlib.Path(args.output)
    if not output.is_absolute():
        output = root / output
    for raw in paths:
        if not raw:
            continue
        name = raw.decode("utf-8", "surrogateescape")
        path = root / name
        if not path.is_file() or path.resolve() == output.resolve():
            continue
        data = path.read_bytes()
        files.append({
            "path": name,
            "bytes": len(data),
            "sha256": hashlib.sha256(data).hexdigest(),
        })
    result = {
        "schema": "UNIFUSION_TRACKED_SOURCE_MANIFEST_v1",
        "authority": "BUILD_INVENTORY_ONLY_NOT_COSL_REG",
        "file_count": len(files),
        "source_files": files,
    }
    output.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"{output}: {len(files)} tracked files")

if __name__ == "__main__":
    main()
