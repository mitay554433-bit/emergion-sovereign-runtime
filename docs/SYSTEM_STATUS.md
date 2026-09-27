# EmergION Sovereign Runtime — Current System State

**State date:** 2026-09-27

**Branch:** `governed-demand-v1`

**Verified HEAD before this documentation finalization:** `90d3bb4` — `test: preserve governed cycle composition evidence`.

This document records the current implementation boundary. It deliberately separates verified operation from capabilities that are designed but not yet independently verified.

## 1. Build and test state

The live Termux tree reported the following on 2026-09-27:

```text
go test ./...
PASS
```

The reported package results were passing for the existing `cmd`, `internal`, and `pkg/fieldapi` packages.

The repository's previously recorded status also reports:

```text
go vet ./...
PASS
```

No claim is made here that a new test run was executed by GitHub after this documentation commit. The phone remains the authoritative live build environment for final local verification.

## 2. Current source state observed on the phone

The live working tree reported:

```text
branch: governed-demand-v1
HEAD:   90d3bb4
remote: origin/governed-demand-v1
```

At the time of inspection, the local tree also contained existing uncommitted test/proof artifacts:

```text
M  internal/reason/gemma_cli_test.go
M  pkg/fieldapi/api_test.go
?? LIVE_PROOF_SAVEPOINT_2026-09-26/
?? LIVE_PROOF_TMP/
?? field-live
?? t.go
```

These are not silently classified as canonical architecture. Their disposition remains a live-tree housekeeping/build decision.

## 3. Canonical architecture status

The implemented architecture is:

```text
SOURCE
→ EmergER
→ EmergION
→ RECOIL/WVC
→ GOV/HUMAN_FINAL
→ REG
→ FIELD
```

The following boundaries are implemented:

- canonical EmergION identity `<IDN, STA, MEM, REL, CAP, VAL, EVO>`;
- bounded semantic operators;
- deterministic admission validation;
- COSL hash-chain persistence;
- content-addressed compressed evidence;
- GOV/HUMAN_FINAL decision handling;
- separate REG acceptance receipt;
- deterministic FIELD rebuild;
- JSON/HTML projections;
- projection freshness receipt;
- local Gemma Reasoner integration;
- governed action derivation;
- HUMAN_FINAL action authorization;
- governed execution and execution-result recapture;
- PRM → SAAB → CPSL → SAW → LIB projection chain;
- CPU/FIELD analytics;
- native `pkg/fieldapi` embedding seam.

## 4. AI operational state

The current verified environment has a real local Gemma path:

```text
llama-cli
  /data/data/com.termux/files/usr/bin/llama-cli

Gemma model
  /data/data/com.termux/files/home/models/gemma-2-2b-it.Q4_K_M.gguf
```

The runtime's existing `field doctor` path has previously reported the local runtime, COSL chain, evidence store, Gemma binary, and Gemma model ready.

AI is bounded by the existing `Reasoner` and MXPD/structural validation path. The model does not receive GOV or REG authority.

## 5. Governed state and execution

The existing runtime verifies that an execution target is REG-accepted before `PrepareExecution` can produce an execution request. The request is derived from accepted EmergION capabilities/facets. fileciteturn8file0

The existing authorization path records:

```text
EmergIONID
Adapter
Action
Authority = HUMAN_FINAL
Authorized
Reason
Timestamp
```

The existing execution path supports:

```text
LOCAL_GEMMA
GITHUB / PROGRAM
```

and binds execution results back to the original request before governed recapture. fileciteturn8file0

## 6. Capability catalog

The existing action catalog derives capabilities from accepted EmergION capability declarations and metadata facets. Current families include:

```text
PROGRAM_FORGE
PRODUCT_STORE
CUSTOMERS_SALES
COMMUNICATIONS
PAYMENTS_FINANCE
GRANT_FUNDING
PATENT_IP
MA_PARTNERSHIPS
DOCS_PROJECTION
ANALYTICS_FORECAST
```

The existing catalog marks `PROGRAM`, `SEND`, `TRANSFER`, `DEPLOY`, `CONTRACT`, and `ACQUIRE` as HUMAN_FINAL actions. The GitHub `PROGRAM` path is explicitly available through the existing catalog when its conditions are met. fileciteturn9file0

## 7. Autonomous-safe operation

The existing safe execution path is intentionally constrained to enabled, non-HUMAN_FINAL, `CAP_ONLY`, local Gemma `ANALYZE` work. It checks existing execution observations before repeating work.

This means the current runtime can perform bounded local analysis without converting that autonomy into unrestricted repository modification or external-world action.

## 8. Proven operational evidence

Previously recorded live verification establishes:

- a real repository source traversed the local Gemma path;
- source evidence was preserved and verified;
- weak extraction was delivered to GOV rather than silently promoted;
- HUMAN_FINAL RETURN caused controlled rework with exact predecessor lineage;
- RETURNED predecessor lineage was preserved while the accepted successor remained the accepted Kin root;
- the corrected live ledger rendered and verified without rewriting prior governed events;
- governed execution-result recapture is proven;
- CPU and FIELD metrics work;
- orphan evidence cleanup works;
- deterministic SAAB/CPSL/SAW projection works;
- commercial metadata propagates through the governed projection chain.

## 9. Projection state

The projection architecture is:

```text
COSL / reconstructed State
        ↓
FIELD
 ├── field.json
 ├── field.html
 ├── projection.current.json
 ├── analytics
 ├── SAAB
 ├── CPSL
 ├── SAW
 └── LIB indexes
```

These are projections, not alternate truth stores.

`projection.current.json` binds projection hashes to the current COSL tip and is therefore a freshness/integrity receipt.

## 10. Build and release state

Previously recorded release artifacts exist for:

```text
Linux AMD64
Linux ARM64
Windows AMD64
```

The native Go runtime remains standard-library based with no required third-party Go dependencies.

The repository manifest exists and records source/build hashes, but its generated timestamp and hash inventory predate the current live-tree documentation commit. It must therefore be regenerated by the existing runtime/build workflow before being treated as the final current manifest.

## 11. Android state

Verified now:

```text
Android device
→ Termux
→ native Go runtime
→ local llama-cli
→ local Gemma GGUF
```

Not verified as complete:

```text
Android native shell without Termux
platform-packaged inference engine without Termux
```

The existing `pkg/fieldapi` seam is the intended bridge for a future native shell. No new Android architecture is introduced here.

## 12. Live-tree modification state

The existing code already contains the governed PROGRAM route:

```text
accepted EmergION
→ PROGRAM capability derivation
→ HUMAN_FINAL authorization
→ GITHUB PROGRAM executor
→ execution result
→ governed recapture
```

The remaining live-tree work is therefore confined to completing/verifying the existing executor's worktree binding. This is not permission for unrestricted AI modification. It remains subordinate to the existing HUMAN_FINAL and execution-lineage gates.

## 13. What is finalized now

The following are treated as the current canonical architecture and operating contract:

- EmergER → EmergION → RECOIL/WVC → GOV/HUMAN_FINAL → REG → FIELD;
- FIELD as rebuildable live state;
- COSL/REG as canonical lineage and governance;
- HUMAN_FINAL as consequential authority;
- bounded local AI reasoning;
- dodecahedral metadata projection;
- governed capability/action catalog;
- governed execution request/result lineage;
- local Gemma safe-analysis circulation;
- PROGRAM/GitHub governed modification path;
- PRM → SAAB → CPSL → SAW → LIB projections;
- projection freshness and hash binding;
- content-addressed evidence;
- no parallel registry or shadow authority.

## 14. Explicitly not claimed complete

These remain outside the verified-complete boundary:

1. Termux-free native Android shell.
2. Platform-packaged Android inference engine.
3. Live external email/payment/CRM/store/grant/patent/M&A adapters without separate governed proof.
4. Unverified external-world execution.
5. Final regenerated repository manifest after the documentation commits.
6. Final cleanup/commit disposition of the untracked local proof/build artifacts listed above.

These are not architecture failures. They are explicitly separated verification boundaries.

## 15. Final operational invariant

```text
AI may observe, reason, structure, project, propose, and execute only
through an already-governed capability path.

HUMAN_FINAL authorizes consequential transitions.
REG records acceptance.
COSL preserves lineage.
FIELD reconstructs current state.
Projections describe state; they do not become state authority.
```
