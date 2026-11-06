# UNIFUSION — Release readiness and execution checklist (2026-10-25)

Authority: engineering planning only; NOT COSL, REG, Q, or HUMAN_FINAL approval. Source baseline: CI run #41 on `0f48cca180607f9e1be83b56d63e3daa539fa33d` passed unit tests and vet. Existing implementation remains complete where previously verified. Preserve current typed Go/COSL/REG/FIELD path.

## Ready-to-execute gates

| Gate | Preparation / exact action | Evidence required to close | Current status |
| --- | --- | --- | --- |
| 1. PR #30 integration | Review diff, branch rules and mergeability; retain verified source baseline; merge only after all required checks/reviews and explicit release decision | Merge SHA on `governed-demand-v1`, passing checks | NOT MERGED |
| 2. HUMAN_FINAL operator | Existing FIELD UI already shows pending evidence, risk, relationships, approve/reject; explicit reason and error handling added; verify backend decision contract, reason persistence, stale-tip handling and non-authoritative projection | Focused regression, HTTP integration test, decision receipt with unchanged authority rules | PARTIAL; UI regression CI PASS |
| 3. Sustained multi-wave closure | Use existing GovernedCycle, SAW/BRIDGEGAP, RECAPTURE and provider selection; record successive causally related waves, no duplicate work, stop conditions, resource measurements | Full reproducible multi-wave evidence with kin links and COSL tip | PROOF PENDING |
| 4. AtGOV disposition | Obtain current `field status` and canonical tip; classify by evidence hash, required capability, kin predecessor, provider coverage and HUMAN_FINAL decision | Dated canonical census and safe disposition, no batch approval | PHONE SNAPSHOT PENDING; 1,121 historical |
| 5. PROGRAM execution | Trace existing proposal→AtGOV→REG acceptance→Q authorization→adapter→execution result→RECAPTURE; verify source SHA and failure signal | Governed test and lineage receipt, no self-authorization | E2E PROOF PENDING |
| 6. FIELD projection integrity | Compare `field doctor`, ledger tip, `field.json`, `field.html`, `projection.current.json` SHA-256 receipt, accepted deliverables | Matching hashes and deterministic rebuild on target host | LIVE PROOF PENDING |
| 7. VERITEX/PIVOTAI services | Inventory existing phone services, WVC/SHEEN, API health, startup and adapters; preserve separation from canonical Go authority | Dated health/status outputs, source/port/process map | OUTSIDE REPO; LIVE PROOF PENDING |
| 8. Pilot / investor proof | Regenerate build-only tracked source manifest; compile CI logs, demo script, operator screenshots, evidence lineage, scope/limitations, deployment guide | Reproducible bundle with hashes, verified demo, no invented mathematics | PACKAGING PENDING |

## Prepared evidence capture (no authority mutation)

On the existing Termux repository, collect `git rev-parse HEAD`, `git status --short`, `go run ./cmd/field doctor` and the existing `field status` command if available. Use `scripts/verify_termux_live.sh` only after checking device resources and Gemma variables; it runs serialized Go tests and vet. Do not execute unbounded model workloads on a memory-constrained phone. Record output timestamps, branch SHA and COSL tip. Avoid deleting evidence, bulk accepting AtGOV, or initiating any gated action.

## Release controls

- GitHub CI verifies checked-in Go code, not live Termux, local Gemma, or external Python services.
- The latest verified CI baseline is run #41. Later commits require fresh verification before being classified PASS.
- Existing capabilities are not reopened solely because an older gap matrix lists them incomplete.
- HUMAN_FINAL and Q authority remain explicit; Q cannot precede REG.
- SELF_BUILDING != SELF_AUTHORIZING. No parallel engine or ledger.

## Execution order

Inspect PR merge restrictions and operator backend first. Close focused operator tests and CI. Prepare deterministic multi-wave proof fixtures and acceptance criteria. Obtain current phone census before interpreting AtGOV. Validate live integrity and external services. Only then publish release evidence and seek release authorization.
