# UNIFUSION — Current code ledger

This is the durable index for inspecting the **last observed source code** without needing continuous access to the running phone. Git commits are the version history. `scripts/snapshot_code_state.py` produces `CODE_STATE.current.json` with the exact Git HEAD, tracked file paths, byte counts, SHA-256 hashes, dirty-state warning and observation timestamp. The JSON snapshot is a derived source index, **not** COSL, REG, HUMAN_FINAL or Q authority.

## Recorded source baseline

- Repository: `mitay554433-bit/emergion-sovereign-runtime`
- Branch: `evidence-provider-selection-20261025`
- Baseline before snapshot tool: `e4dd154979f905cba96ea96e8298ac9522e88a05`
- Snapshot tool added in `a196e50b573c470392948b61cd7d6a91985b1373`.
- On October 25, GitHub compare reported **45 commits ahead** of `governed-demand-v1`, none behind, before adding the snapshot tool.
- Latest previously verified CI run: #42 at `0e0a20be69ecdaa47835fbfb3886c58ba5fb3b72`. Do not apply that PASS label to later commits without their own run.

## Code and contracts to inspect

| Subject | Existing source of truth |
| --- | --- |
| Typed Go runtime and travelling EmergION | `internal/runtime/`, `internal/core/`, `internal/emerger/` |
| Source/evidence persistence | `internal/store/`, `internal/cosl/` |
| Governance, REG, Q | `internal/gov/`, `internal/reg/`, `internal/runtime/governed_execution.go` |
| RECOIL/WVC and continuation | `internal/runtime/`, `pkg/fieldapi/` |
| FIELD, drawers, mirrors and projection receipts | `internal/proj/`, `pkg/fieldapi/` |
| Gemma and adapters | `internal/reason/`, `internal/adapters/` |
| Mathematical/terminology definitions | `docs/MATHEMATICAL_CONTRACT_INVENTORY_2026-10-24.md`, `docs/UNIFUSION_LIVING_IMPLEMENTATION_2026-10-08.md`, `internal/axiom/` |
| External inventory and trace contract | `docs/UNIFUSION_SYSTEM_OBSERVABILITY_CONTRACT_2026-10-25.md` |
| Local host verification | `scripts/verify_termux_live.sh` |

## Refresh procedure

After a code change, run `python3 scripts/snapshot_code_state.py` from the repository root and inspect `CODE_STATE.current.json`. For a clean, reproducible committed snapshot, first commit the source change, then generate the snapshot and commit the JSON as a *derived* inventory. The snapshot includes its own generator file but excludes itself to avoid circular hashes. Its `git_head` records the source commit before committing the snapshot; later commits must be compared, not silently treated as identical.

For current remote source, use the exact GitHub commit and file blob hashes. For phone-local code, capture its local Git HEAD and source snapshot when access is available; record divergence rather than overwriting it. The snapshot does **not** certify running processes, external ledgers or phone mirrors.

## Continuity rule

At the start of a future work session, read this ledger, inspect Git HEAD and compare any new commits; only inspect changed files plus their dependents. Record new verified facts, preserving prior dated states. Never infer live authority or synchronization from code inventory alone.
