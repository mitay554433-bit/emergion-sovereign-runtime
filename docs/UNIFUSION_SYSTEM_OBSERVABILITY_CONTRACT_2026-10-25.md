# UNIFUSION — System observability and continuity contract (2026-10-25)

**Purpose:** Maintain one durable, source-linked inventory of *all* systems that participate in UNIFUSION, whether in the canonical Go repository, Android/Termux, Python/VERITEX/PIVOTAI, or external providers. Make the travelling EmergION's evidence and authority path inspectable while live waves run. This is a read-only engineering contract, NOT a new runtime, ledger, mirror, scheduler, registry, or authority.

## Rules

1. **Canonical authority remains COSL → GOV/HUMAN_FINAL → REG → FIELD; Q follows REG.** No inventory, drawer, snapshot, codex, mirror or external adapter may confer acceptance or execution authority.
2. Every system/component has a stable identifier, responsible source location, runtime host, current evidence location, version or source hash, health verification method, and last verified timestamp. Unknown fields are recorded as UNKNOWN, not guessed.
3. Record **implementation state**, **last verified state**, and **live state** separately. A passing GitHub CI run cannot certify phone-local services. A historical phone status is not live telemetry.
4. All projection/mirror entries must declare their **source ledger**, **canonical tip/hash**, **projection timestamp**, **projection receipt/hash**, and **staleness/validation status**. A stale mirror is displayed as stale; never silently promoted to canonical.
5. Every travelling EmergION observation must retain **EmergION ID**, predecessor/kin lineage, source evidence hash, current state, capability and provider context, WVC/RECOIL result, decision/REG acceptance identity where present, exact Q/action authorization where applicable, execution result/recapture successor and associated FIELD tip. If absent, say UNKNOWN. No inferred authority.
6. Every math/terminology/codex claim must link to a **versioned source definition** and, if described as implemented, a **code symbol and test**. Distinguish mathematical specification, predicate/contract implementation, and numeric algorithm implementation. Do not assert TORQUE, fracture or adaptive rational-probabilistic equations are executable solely because described in prose.
7. Updates are **append-safe and non-destructive**: preserve prior evidence and dated snapshots, never rewrite COSL/REG/WVC to make an inventory agree, never bulk-authorize AtGOV. Record source disagreement explicitly.
8. Before changing a cross-system contract, inspect consumers and affected ledgers/mirrors; update source, tests, projections and this inventory together where applicable. If an external component is inaccessible, mark LIVE_UNVERIFIED and prepare a verification instruction.
9. Read-only reporting is permitted. SEND, TRANSFER, DEPLOY, PROGRAM, HUMAN_FINAL and other consequential actions remain governed by existing mechanisms.

## System inventory (source-grounded; verify live independently)

| ID | System / surface | Source or canonical location | Authority / relationship | Current evidence and next check |
| --- | --- | --- | --- | --- |
| GO-CORE | Typed Go runtime / EmergER / EmergION | `internal/core`, `internal/emerger`, `internal/runtime`, `cmd/field` | Implements bounded recurrence; not self-authorizing | GitHub CI #42 PASS at `0e0a20b`; current branch verification separate |
| COSL | Canonical event chain | `internal/cosl`, phone `.field/field.cosl` | Canonical event integrity | Last reported tip historical; run doctor, compare tip |
| REG-GOV-Q | REG / HUMAN_FINAL / action authorization | `internal/reg`, `internal/gov`, runtime Q checks | Only established acceptance/authorization path | Tests in Go CI; live decision receipts pending |
| STORE | Evidence object store | `internal/store`, phone `.field/o/` | Content-addressed source evidence | Check referenced objects, hashes, orphans non-destructively |
| KIN | Travelling EmergION lineage | `internal/core`, `internal/runtime` | Evidence-bound predecessor/successor | Validate chain through admission, execution, recapture |
| WVC-RECOIL | Observation and continuity | `internal/runtime`; reported `~/UNIFUSION/WVC/events.wvc.jsonl` | Observation/evidence, not independent REG authority | Reconcile Go evidence with Python ledger identities; no assumed equivalence |
| FIELD-PROJ | FIELD HTML/JSON/operator drawers | `internal/proj`, `pkg/fieldapi`; reported `outputs/` | Read-only derived projection | Check `projection.current.json` and both SHA-256 hashes against tip |
| SAW | SAW / BRIDGEGAP / provider population | `internal/runtime`, `internal/proj` | Governed capability continuation | Existing tests; sustained live causal proof pending |
| GEMMA | Local inference / bounded reasoner | `internal/adapters`, `internal/reason`, Termux model | Proposal only; no authority | `scripts/verify_termux_live.sh` when resources permit |
| FIELD-SERVER | `fieldd` and delivery serving | `cmd/fieldd`, `pkg/fieldapi` | Serve accepted verified artifacts | Health and served artifact receipts on host |
| VERITEX | VERITEX V12 scheduler/host processes | Reported `/storage/emulated/0/VERITEX` | External orchestration, not canonical authority | Live source/process/startup census needed |
| PIVOTAI | Python PIVOTAI service | Reported loopback `127.0.0.1:8089` | External adapter/service | Live health and identity needed |
| UNIFUSION-PY | Python UNIFUSION API/workers | Reported `~/UNIFUSION`, `127.0.0.1:8097` | Noncanonical service boundary | Live health, worker and WVC map needed |
| SHEEN | Execution receipt ledger | Reported `~/UNIFUSION/SHEEN/runtime_receipts.jsonl` | Receipt evidence, not REG | Hash/identity and recapture cross-reference |
| MIRRORS | Derived phone projections / backups | Reported `~/UNIFUSION/FIELD/current_field.json` and device backups | Derived/stored copies only | Record source tip, freshness and restoration evidence |
| CODEX-MATH | Codex, MXPD/2, COSL contract, math definitions | `docs/MATHEMATICAL_CONTRACT_INVENTORY_2026-10-24.md`, `docs/UNIFUSION_LIVING_IMPLEMENTATION_2026-10-08.md`, `internal/axiom` | Versioned semantics, not automatic code | Link each claimed equation to source + implementation + test or mark SPEC_ONLY |
| EXTERNAL | Communications, payments, CRM, partners and other providers | Not established by current Go tree | Bounded providers, no peer authority | Enumerate actual integrations and explicit authorization class |

## Mandatory live inspection record (one record per system; read-only)

```text
system_id:
observed_at_utc:
host:
source_path_or_url:
source_commit_or_version:
canonical_ledger_tip_if_applicable:
source_evidence_hash:
projection_receipt_hash_if_applicable:
current_health: PASS | FAIL | UNKNOWN
implementation_status: IMPLEMENTED | PARTIAL | SPEC_ONLY | UNKNOWN
verification_status: CI_VERIFIED | LIVE_VERIFIED | HISTORICAL | UNVERIFIED
last_verified_at_utc:
dependent_system_ids:
mismatch_or_gap:
next_non_destructive_check:
```

Do not invent fields or silently use one system's timestamp for another. Store inspection records as dated evidence files or verified exports, not as edits to canonical ledgers.

## Travelling EmergION trace requirements

```text
emergion_id → source_sha256 → predecessor_kin_id(s)
→ current_state / capability / provider population
→ RECOIL/WVC observation and difference
→ HUMAN_FINAL decision (when required) → REG acceptance identity
→ Q authorization identity (for execution, after REG)
→ adapter/action/result evidence → RECAPTURE successor_id
→ projection tip and verified deliverable (if any)
```

A missing link is an explicit gap, not an invented connection. Distinguish a pending candidate from an approved/accepted/executable one.

## Mathematics and terminology discipline

- `MXPD/2`: compact machine semantic interchange; not a second authority.
- `KIN`: evidence-bound lineage; do not substitute provider adjacency for COMPOSITION_KIN.
- `Difference`: typed target-state primitive; test round-trip rather than assuming numerical scoring.
- `pivot.Observe`: invariant predicate with divergence evidence, **not** a numeric probability score.
- `REALIZATION`: four-valued comparison; non-unresolved requires supported Difference and accepted realizations.
- `TORQUE`, fracture, adaptive rational-probabilistic bridge: preserve distinct source definitions and do not assert executable equations without source, symbol and test.

## Inspection order / done condition

1. Collect exact repository HEAD, CI run, `field doctor` and source tip.
2. Enumerate phone services, ledgers, evidence stores, mirrors, backups, codex source documents and adapters **without changing them**.
3. Match references and hashes from source→COSL→REG→FIELD, then execution→SHEEN/WVC→RECAPTURE→travelling EmergION.
4. Validate projection receipts, current operator drawers and noncanonical mirror freshness.
5. Publish dated, read-only system records and explicit gaps; only mark a system LIVE_VERIFIED when observed on its host.
6. Re-run tests for changed contracts; obtain HUMAN_FINAL where required before consequential action.

**DONE means** every known component has an inventory entry, a source, an explicit last verification state, dependency mapping and no concealed integrity mismatch. UNKNOWN remains open; it does not mean FAIL or PASS.
