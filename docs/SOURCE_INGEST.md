# SOURCE ingest

`field ingest` records a sorted-upload manifest as SOURCE records.

```
field ingest -dry-run sources/ingest-manifest.jsonl
field ingest sources/ingest-manifest.jsonl
```

- One manifest line (`path, sha256, bytes, mime, topic, title, source_kind "upload", captured_at, related`) becomes one EmergION with `source_kind=SOURCE_UPLOAD`.
- It uses the normal governed candidate path (`CaptureBytes`): evidence preserved, RECOIL, WVC, PROTECTOR, then the record waits AT_GOV. Nothing is decided, approved or REG-accepted. HUMAN_FINAL stays with the human (A01, A02, A03).
- The preserved evidence is a deterministic `SOURCE_UPLOAD/1` descriptor of the line. The upload file itself is referenced by sha256 and is never executed or copied.
- REL keys: `source_sha256`, `source_path`, `source_title`, `source_topic`, `source_mime`, `source_bytes`, `source_captured_at`, `source_related` (stored only; related ids are not EmergION ids, so no KIN link is claimed). The title is the Archonym.
- Idempotent by sha256: a second run adds nothing. Repeated sha256 lines in one manifest count once.
- A bad line (invalid JSON, bad sha256, unsafe path, source_kind not upload, bad captured_at) is skipped, counted and reported with its line number; the run continues.
- `-root DIR` (optional): files found under DIR are hashed read-only; a mismatch makes that line bad, a missing file is only counted.
- Stop fieldd before a real run and start it again after, so the Living Universe reads the new records.
