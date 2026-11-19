# Documentation census — 2026-11-19

Head reviewed: `governed-demand-v1` at `8e25ef9` (PR #54).

This census says which repository documents describe the current tree, which
are dated historical records, and which are superseded. It is documentation,
not authority: COSL/REG and HUMAN_FINAL stay authoritative, and FIELD and
every file here are projections (A06, A09). No document was deleted; dated
records keep their original text as evidence.

Classes: **current** (describes the tree at the head above), **current,
needs refresh** (still the right home but missing recent merges),
**historical** (a dated record; read as of its date), **stale** (contents no
longer match the tree; do not cite for current state).

| Document | Class | Note |
|---|---|---|
| `README.md` | current | Operation, fieldd bearer-token setup (PR #53), persistence layout. |
| `docs/ARCHITECTURE.md` | current | Canonical architecture and invariants. Which canon to cite first, and which axiom numbering (A01–A10 vs MXPD/2 A0–A20), is still an open HUMAN_FINAL question. |
| `docs/UNIFUSION_FRACTURE_WAVE.md` | current | TORQUE as FRACTURE and the bounded wave (PRs #49, #50). |
| `docs/SYSTEM_STATUS.md` | current, needs refresh | Sections 1–18 are the 2026-10-08 state; sections 19–21 add 2026-11-14 evidence. Section 22 (added with this census) lists merges #39–#54 by commit. |
| `docs/PILOT_DEMO.md` | current | Throwaway pilot walkthrough and the footprint method (added with this census). |
| `docs/UNIFUSION_SOURCE_CUSTODY_AND_PORTABILITY.md` | current | Undated method: the Git tree at a commit is the source of record. |
| `docs/UNIFUSION_INSPECTABLE_TWIN.md` | historical | Source map method; component table is still broadly right but predates #39–#54. |
| `docs/UNIFUSION_CURRENT_CODE_LEDGER.md` | stale | Records baseline branch `evidence-provider-selection-20261025`; use `git log` at the head instead. |
| `docs/UNIFUSION_LIVING_IMPLEMENTATION_2026-10-08.md` | historical | Dated record. |
| `docs/MATHEMATICAL_CONTRACT_INVENTORY_2026-10-24.md` | historical | Dated inventory; quantities stay informational and do not feed `gov.Decide`. |
| `docs/UNIFUSION_FULL_SYSTEM_RECONCILIATION_2026-10-24.md` | historical | Dated reconciliation. |
| `docs/UNIFUSION_RELEASE_READINESS_2026-10-25.md` | historical | Dated planning checklist; not a release record. |
| `docs/UNIFUSION_STATUS_RECONCILIATION_2026-10-25.md` | historical | Dated live counts as reported 2026-10-25. |
| `docs/UNIFUSION_SYSTEM_OBSERVABILITY_CONTRACT_2026-10-25.md` | historical | Dated contract; its authority rules match the current canon. |
| `REPOSITORY_MANIFEST.json` | stale | Generated 2026-08-05 for 29 files; the tree is far larger. Its hashes are not current. Regenerate with `scripts/generate_manifest.py` before citing it, or ignore it. |

## What changed in the tree since the dated records

See `docs/SYSTEM_STATUS.md` section 22 for the merge list. In short:
coverage requires at least one source fact; Section 8 Difference →
Requirement; exact Q referential integrity; genuine-remainder BRIDGEGAP;
TORQUE as FRACTURE and the bounded fracture wave; verify localization,
batch isolation and optional tip/count receipt; human contradiction marks;
source-hash admission at GOV; fieldd bearer token and loopback default; and
the Skyhook restart regression test.
