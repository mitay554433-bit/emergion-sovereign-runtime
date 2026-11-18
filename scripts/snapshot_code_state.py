#!/usr/bin/env python3
"""Read-only snapshot of current tracked UNIFUSION code and contracts.

Regenerate after source changes. Git history is the append-only history;
this file is a derived, non-authoritative index, never COSL/REG.
"""
import datetime
import hashlib
import json
import pathlib
import subprocess

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = ROOT / "CODE_STATE.current.json"

def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)

def main():
    head = git("rev-parse", "HEAD").decode().strip()
    branch = git("branch", "--show-current").decode().strip()
    tracked = [p.decode("utf-8", "surrogateescape") for p in git("ls-files", "-z").split(b"\0") if p]
    files = []
    for name in tracked:
        path = ROOT / name
        if not path.is_file() or path.resolve() == OUT.resolve():
            continue
        data = path.read_bytes()
        files.append({"path": name, "sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data)})
    status = git("status", "--porcelain", "--untracked-files=no").decode("utf-8", "replace").strip()
    record = {
        "schema": "UNIFUSION_CODE_STATE_v1",
        "authority": "READ_ONLY_GIT_SOURCE_INDEX_NOT_COSL_REG",
        "captured_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "git_head": head,
        "branch": branch,
        "tracked_tree_dirty": bool(status),
        "tracked_tree_status": status.splitlines(),
        "file_count": len(files),
        "files": files,
        "external_systems": "NOT_VERIFIED_BY_THIS_SOURCE_SNAPSHOT",
    }
    OUT.write_text(json.dumps(record, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"{OUT.name}: {len(files)} tracked files, HEAD {head}, dirty={bool(status)}")

if __name__ == "__main__":
    main()
