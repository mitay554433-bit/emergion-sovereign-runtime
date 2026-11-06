# UNIFUSION inspectable twin — current source map

**Role:** a durable, inspectable engineering twin of the last observed UNIFUSION code and component topology. It is NOT a second executing runtime, second COSL/REG ledger, or claim of live synchronization.

## How to use the twin without re-running the phone

For code committed to GitHub, inspect the exact branch/commit and read any source file directly. Git stores the **full file bytes** and all historical revisions. `docs/UNIFUSION_CURRENT_CODE_LEDGER.md` is the entry point. `scripts/snapshot_code_state.py` can record hashes and changed files; `scripts/export_portable_source.py` can archive the complete committed tree. Neither is required for reading code that is already committed.

**Rule after each code inspection or edit:** record the exact path, source commit/hash, what it does, its dependencies and relevant test/command. On edit, commit the changed bytes, update the affected entry and keep the old Git revision. If a source was inspected but not saved in Git or an attached artifact, do **not** claim its complete code is retained.

## Current twin map (source-backed)

| Component | Inspectable code location | Role |
| --- | --- | --- |
| Entry/CLI/daemon/Android | `cmd/field/`, `cmd/fieldd/`, `cmd/fieldandroid/` | Starts and exposes the canonical Go runtime |
| Core and travelling EmergION | `internal/core/`, `internal/emerger/`, `internal/runtime/` | Bounded emergence, kin identity, lifecycle and recapture |
| Canonical event chain | `internal/cosl/` | COSL serialization and integrity |
| Evidence objects | `internal/store/` | Content-addressed source preservation |
| Governance/REG | `internal/gov/`, `internal/reg/` | HUMAN_FINAL and acceptance rules |
| Execution/Q/recapture | `internal/runtime/governed_execution.go`, `internal/runtime/execution_signal.go`, `pkg/fieldapi/` | Authorized actions and evidence-bound results |
| Reasoner and adapters | `internal/reason/`, `internal/adapters/` | Bounded Gemma inference and external capability adapters |
| FIELD projection and drawers | `internal/proj/`, `pkg/fieldapi/` | Derived operator UI, deliverables and projection receipts |
| Recurrence / provider selection | `internal/runtime/realization_*.go` | Target difference, comparison, capability providers |
| Analytics | `internal/analytics/` | Descriptive measurements, not authority |
| Mathematical contracts and terminology | `docs/MATHEMATICAL_CONTRACT_INVENTORY_2026-10-24.md`, `internal/axiom/` | Versioned definitions, implementation evidence and gaps |
| Verification and source export | `.github/workflows/go-verification.yml`, `scripts/` | CI, phone checks and exact code custody |

## External twin coverage (not yet captured as complete source)

| Component | Previously reported host path | Status |
| --- | --- | --- |
| VERITEX V12 | `/storage/emulated/0/VERITEX/` | Historical path; exact current bytes not in this Git tree |
| PIVOTAI Python | Reported `127.0.0.1:8089` | Historical service; exact current bytes not in this Git tree |
| UNIFUSION Python / WVC | `~/UNIFUSION/`, `~/UNIFUSION/WVC/events.wvc.jsonl` | Historical path; source/ledger not yet captured |
| SHEEN receipts | `~/UNIFUSION/SHEEN/runtime_receipts.jsonl` | Historical path; not a code source |
| Phone FIELD mirror | `~/UNIFUSION/FIELD/current_field.json` | Historical projection; not canonical authority |
| Local Gemma model and runtime state | Termux host | Binary/model and canonical evidence require separate authorized capture |

## Change rule

On a future conversation, start with this twin map and the latest Git SHA. Retrieve only changed files and their dependencies. If a new source file, codex or component is shown, preserve its exact bytes in the appropriate source repository or accessible artifact and add a map entry. For external code, record source path, host, version, and capture status; never invent content from historical descriptions.

**Done condition:** every in-scope source file has a recoverable exact version; every component has a map entry and build/run instructions; external source capture gaps are explicit. Until then this is a partial, durable engineering twin, not a full executable replica.
