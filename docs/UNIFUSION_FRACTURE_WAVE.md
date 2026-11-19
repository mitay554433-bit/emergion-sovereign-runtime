# UNIFUSION fracture wave (TORQUE as FRACTURE)

TARGET PRESSURE -> GOVERNED REALITY -> SATISFIED / GAP -> probe -> stoppage ->
smallest structural addition -> same probe again -> recompute target.

A fracture probe is a generated, deliberately failing test. Its first truthful
stoppage is boundary evidence. Nobody preselects the fix. Every probe is a
candidate: HUMAN_FINAL accepts it, a fresh Q authorizes each run, and it never
runs automatically.

## One-time setup (dedicated clean probe worktree)

Run once from the repository (for example `~/AXM/emergion-sovereign-runtime`).
It leaves your own worktree and its untracked files untouched:

    git worktree add --detach ../esr-probes HEAD
    export FIELD_PROBE_WORKTREE="$(cd ../esr-probes && pwd)"

After a structural addition lands on your branch, sync the probe worktree:

    git -C "$FIELD_PROBE_WORKTREE" checkout -q --detach "$(git rev-parse HEAD)"

`field` refuses to propose or run a probe if the probe worktree is missing or
not at the current HEAD. It never moves, cleans or deletes anything itself.

## Operator loop

    field fracture-next                          # proposal only
    field decide <probe-id> APPROVE "why"        # HUMAN_FINAL
    field authorize <probe-id> GITHUB PROGRAM "why"   # fresh Q, every run
    field execute <probe-id> GITHUB PROGRAM      # runs in the probe worktree
    field decide <result-id> APPROVE "why"       # accept the stoppage evidence
    field fracture-next                          # next status / next round

`fracture-next` prints one status per active boundary:

| status | meaning |
| --- | --- |
| PROPOSED_PROBE | a probe was admitted to GOV (level catalog, prepare or enabled) |
| AWAITING_HUMAN_FINAL | a probe, result or recompute waits at GOV |
| AWAITING_Q_AND_RUN | the accepted probe has not run yet |
| RERUN_SAME_PROBE_AFTER_STRUCTURAL_ADDITION | it fractured; add the smallest structure, sync, rerun with a fresh Q |
| NO_NEW_STOPPAGE | two identical stoppages in a row; nothing changed |
| MAX_ROUNDS | `-max-rounds` reached (default 3) |
| RECOMPUTE_PROPOSED / TARGET_UNCHANGED | ladder exhausted; target recomputed against REG (probes excluded) |
| SATISFIED / NO_GENUINE_REMAINDER | the active comparison has no genuine gap |

Probe ladder: `catalog` (an adapter expresses each recipe token) -> `prepare`
(PrepareExecution derives the action) -> `enabled` (an enabled adapter would
execute it). A deeper probe is generated only from the boundary record and the
previous probe's accepted PASS evidence. A passing probe is reverse-applied so
the probe worktree stays clean; a failing one is rolled back by the executor.
Probe records and their results never count as reality for a target.
