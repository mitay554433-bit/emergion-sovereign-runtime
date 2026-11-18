#!/usr/bin/env python3
"""Maintain an inspectable source twin after each Git commit, without runtime access.

Captures exact committed bytes through git archive; records hashes and component
grouping. It is a derived engineering artifact, not COSL/REG authority.
"""
import hashlib
import io
import json
import pathlib
import subprocess
import tarfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = ROOT / "UNIFUSION_TWIN.current.json"

def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)

def component(path):
    if path.startswith("cmd/"): return "entrypoints"
    if path.startswith("internal/"):
        return "internal/" + path.split("/")[1]
    if path.startswith("pkg/"): return "public_api"
    if path.startswith("docs/"): return "contracts_docs"
    if path.startswith("scripts/"): return "tooling"
    if path.startswith(".github/"): return "ci"
    return "repository_other"

def build():
    head = git("rev-parse", "HEAD").decode().strip()
    archive = git("archive", "--format=tar", "HEAD")
    records = []
    with tarfile.open(fileobj=io.BytesIO(archive)) as t:
        for member in t:
            if member.isfile():
                data = t.extractfile(member).read()
                records.append({
                    "path": member.name,
                    "component": component(member.name),
                    "sha256": hashlib.sha256(data).hexdigest(),
                    "bytes": len(data),
                    "git_blob": git("rev-parse", "HEAD:" + member.name).decode().strip(),
                })
    records.sort(key=lambda x: x["path"])
    groups = {}
    for item in records:
        groups.setdefault(item["component"], []).append(item["path"])
    return {
        "schema": "UNIFUSION_INSPECTABLE_TWIN_v1",
        "authority": "DERIVED_SOURCE_INDEX_NOT_COSL_REG",
        "source_repository": "mitay554433-bit/emergion-sovereign-runtime",
        "git_head": head,
        "file_count": len(records),
        "components": groups,
        "files": records,
        "external_phone_sources": "NOT_CAPTURED",
        "retrieval": "git show <git_head>:<path> or GitHub blob by git_blob",
    }

def main():
    data = build()
    OUT.write_text(json.dumps(data, indent=2) + "\n")
    print(f"UNIFUSION twin: {data['file_count']} source files at {data['git_head']}")

if __name__ == "__main__":
    main()
