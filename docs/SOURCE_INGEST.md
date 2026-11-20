# SOURCE ingest

`field ingest` records a sorted-upload manifest as SOURCE records.

```
field ingest -dry-run sources/ingest-manifest.jsonl
field ingest sources/ingest-manifest.jsonl
```

- One manifest line (`path, sha256, bytes, mime, topic, title, source_kind "upload", captured_at, related`) becomes one EmergION with `source_kind=SOURCE_UPLOAD`.
- It uses the normal governed candidate path (`CaptureBytes`): evidence preserved, RECOIL, WVC, PROTECTOR, then the record waits AT_GOV. Nothing is decided, approved or REG-accepted. HUMAN_FINAL stays with the human (A01, A02, A03).
- The preserved evidence is a deterministic `SOURCE_UPLOAD/1` descriptor of the line. The upload file itself is referenced by sha256 and is never executed or copied.
- REL keys: `source_sha256`, `source_path`, `source_title`, `source_topic`, `source_mime`, `source_bytes`, `source_captured_at`, `source_related` (stored only; related ids are not EmergION ids, so no KIN link is claimed). The title is kept as `source_title` and in the summary; ingest claims no Archonym, because an Archonym is a sovereign name that only a HUMAN_FINAL accept may claim.
- Idempotent by sha256: a second run adds nothing. Repeated sha256 lines in one manifest count once.
- A bad line (invalid JSON, bad sha256, unsafe path, source_kind not upload, bad captured_at) is skipped, counted and reported with its line number; the run continues.
- `-root DIR` (optional): files found under DIR are hashed read-only; a mismatch makes that line bad, a missing file is only counted.
- Stop fieldd before a real run and start it again after, so the Living Universe reads the new records.

## Archonym conflicts (fix 2026-11-20)

The 2b54f72 ingest set each record's Archonym to its title, and the accept paths wrote decision + REG receipts without checking Archonym uniqueness, which only REG replay enforced. Accepting two uploads with the same title (the manifest has 45 repeated titles, e.g. three "BDC Innovation Funding Package") put a REG receipt in the ledger that replay refused, so every read (`/field.json`, `/projection.json`) failed.

- Accept is now strict before anything is written: `field decide ... APPROVE`, `/decide` and `/decide-batch-human` refuse a record whose Archonym already belongs to another REG-accepted or HUMAN_FINAL RETURNED EmergION (same Kin continuation through Supersedes stays allowed). HOLD, REJECT and RETURN are unaffected.
- Replay no longer fails on such a receipt that is already in the ledger. The ledger is never rewritten. The refused record stays Approved, not Accepted. The owner keeps the Archonym, and the conflict is reported as `archonym_conflicts` in `/projection.json` and `ArchonymConflicts` in `field.json` (present only when non-empty).
- Waiting records whose Archonym matches an accepted one have always replayed fine; they stay AtGOV, are shown by ID and summary, and are listed in `archonym_waiting_conflicts` in `/projection.json` (an APPROVE of them is refused; HOLD or REJECT work).

## SOURCE topic facets (display grouping, 2026-11-20)

Accepted uploads carry no REG facet, so the Living Universe showed them all as UNASSIGNED. `/projection.json` now derives a display grouping on every read; nothing is written and `.field` evidence is untouched.

- A REG-accepted record with `source_kind=SOURCE_UPLOAD`, no REG facet and a safe `source_topic` gets `derived_facets: ["SOURCE_TOPIC:<topic>"]` and `facet_source: "source_topic"`. Its `facets` (REG) stay empty. A record with any REG facet is never given a derived one.
- `derived_facet_groups` lists the topic groups (`facet`, readable `label` such as "Inventions & IP", `topic`, `facet_source`, `members`), largest first. `facet_order` and `facet_groups` remain REG facets only.
- `unassigned` now lists only records with neither a REG facet nor a derived grouping. `reg_unassigned` keeps the strict list (every accepted record with no REG facet).
- `source_ref_groups` lists accepted SOURCE records that name the same manifest `related` id. Related ids are not EmergION ids, so this is a shared reference, never KIN. The page uses it only as weak springs to hidden hubs.
- The page treats a derived facet like a facet attractor, so topics cluster through the same emergent spring layout (no preset geometry). Facet tiles name topics readably and mark them "SOURCE topic"; the detail panel says "(SOURCE topic, not a REG facet)". Canvas text still appears only with Labels on.
- No HUMAN_FINAL is needed for display. GOV, A01/A02 and the Archonym rules are unchanged.
