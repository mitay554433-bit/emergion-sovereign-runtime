#!/usr/bin/env python3
"""Export exact committed UNIFUSION source and its verifiable file ledger.

Read-only. Uses git archive at HEAD, never uncommitted working files.
No COSL/REG authority, runtime evidence, or phone-private services included.
"""
import argparse
import datetime
import hashlib
import io
import json
import pathlib
import subprocess
import tarfile
import zipfile

ROOT = pathlib.Path(__file__).resolve().parent.parent

def git(*args):
    return subprocess.check_output(["git", *args], cwd=ROOT)

def main():
    p = argparse.ArgumentParser()
    p.add_argument("--output-dir", default="exports")
    args = p.parse_args()
    output = pathlib.Path(args.output_dir).resolve()
    output.mkdir(parents=True, exist_ok=True)
    head = git("rev-parse", "HEAD").decode().strip()
    archive = git("archive", "--format=tar", "HEAD")
    members = []
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:") as tar:
        for entry in tar:
            if entry.isfile():
                data = tar.extractfile(entry).read()
                members.append((entry.name, data, entry.mode))
    members.sort(key=lambda x: x[0])
    manifest = {
        "schema": "UNIFUSION_PORTABLE_SOURCE_v1",
        "authority": "SOURCE_EXPORT_ONLY_NOT_COSL_REG",
        "git_head": head,
        "exported_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "file_count": len(members),
        "files": [{"path": name, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest(), "mode": mode}
                  for name, data, mode in members],
        "runtime_and_external_data": "NOT_INCLUDED_REQUIRES_SEPARATE_GOVERNED_EXPORT",
    }
    base = "UNIFUSION_SOURCE_" + head[:12]
    target = output / (base + ".zip")
    with zipfile.ZipFile(target, "w", compression=zipfile.ZIP_DEFLATED) as z:
        for name, data, mode in members:
            info = zipfile.ZipInfo("source/" + name, date_time=(1980, 1, 1, 0, 0, 0))
            info.external_attr = (mode & 0xffff) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(info, data)
        z.writestr("SOURCE_LEDGER.json", json.dumps(manifest, indent=2) + "\n")
    digest = hashlib.sha256(target.read_bytes()).hexdigest()
    (output / (base + ".sha256")).write_text(digest + "  " + target.name + "\n")
    print(f"{target} files={len(members)} HEAD={head} sha256={digest}")

if __name__ == "__main__":
    main()
