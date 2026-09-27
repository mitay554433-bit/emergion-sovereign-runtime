# EmergION Sovereign Runtime

A local-first governed runtime implementing the existing sovereign path:

```text
SOURCE
→ EmergER
→ EmergION
→ RECOIL / WVC
→ GOV / HUMAN_FINAL
→ REG
→ FIELD
```

`EmergOX` is an alias for the live FIELD embodiment. It is not a separate layer, file, folder, server, registry, or authority.

## Canonical documentation

- `docs/ARCHITECTURE.md` — canonical architecture and invariants.
- `docs/SYSTEM_STATUS.md` — current verified state, operational AI boundary, projections, build state, and explicit unverified boundaries.
- `REPOSITORY_MANIFEST.json` — repository/build manifest; regenerate it after source or documentation changes before treating its hashes as current.

## Current operating surface

The existing Go runtime provides:

- standard-library native runtime;
- no ChatGPT dependency;
- no required HTTP server;
- local Gemma reasoning through `llama-cli`;
- bounded structured AI output and deterministic validation;
- one GOV-ready EmergION per unique source;
- one compressed evidence object per unique source;
- hash-chained COSL events;
- governed Dropzone capture and clearing;
- explicit HUMAN_FINAL decisions;
- REG acceptance only after approval;
- deterministic FIELD reconstruction;
- JSON/HTML projections with a hash-bound current receipt;
- evidence proof and governance-state separation;
- CPU and FIELD analytics;
- PRM → SAAB → CPSL → SAW → LIB projections;
- governed execution-result recapture;
- native `pkg/fieldapi` embedding seam;
- native release builds for the existing supported targets.

## AI boundary

Gemma is an embodiment of the existing `Reasoner` interface. It may analyze, compare, relate, draft, simulate, and propose bounded capabilities. It cannot independently approve GOV state, accept at REG, or bypass HUMAN_FINAL.

The action catalog derives capabilities from accepted EmergION declarations and facets. Consequential actions such as `PROGRAM`, `SEND`, `TRANSFER`, `DEPLOY`, `CONTRACT`, and `ACQUIRE` remain HUMAN_FINAL-gated.

## Persistence

```text
.field/field.cosl     canonical append-only semantic/governance events
.field/o/<sha>.gz     compressed source evidence by SHA-256
dropzone/             transient intake
outputs/              rebuildable FIELD projections
```

No source, KIN, EmergER, RECOIL, or WVC stage files are required as semantic authority.

## Local Gemma discovery

The runtime checks explicit configuration first and then local executable/model paths:

```text
GEMMA_BIN=/path/to/llama-cli
GEMMA_MODEL=/path/to/gemma.gguf
GEMMA_MODEL_DIRS=/additional/model/directories
```

Optional tuning:

```text
GEMMA_THREADS=4
GEMMA_CONTEXT=4096
GEMMA_MAX_TOKENS=768
GEMMA_TIMEOUT_SECONDS=180
GEMMA_EXTRA_ARGS=
```

## Operation

```text
field init
field doctor
field run
```

The live Termux environment has verified a real local Gemma executable/model path and the full Go test suite has passed there. The phone remains the authoritative environment for final local build verification.

Approval remains explicit:

```text
field decide E-... APPROVE "reason"
```

Approval creates the governed decision/REG acceptance path and refreshes FIELD projections.

The current projection boundary is:

```text
outputs/projection.current.json
```

It binds the JSON and HTML projection hashes to the current COSL tip. It is a freshness/integrity receipt, not semantic authority.

## Governed modification

The existing runtime already contains the governed PROGRAM path:

```text
REG-accepted EmergION
→ PROGRAM capability
→ HUMAN_FINAL authorization
→ existing GITHUB PROGRAM executor
→ execution result
→ governed recapture
→ FIELD
```

Completing the executor's live worktree binding is therefore an implementation step inside the existing architecture, not a new AI or registry layer.

## Independence

Termux is the current verified host, not the runtime architecture. The Go core does not import or depend on Termux. `pkg/fieldapi` is the existing native embedding seam for future Android, desktop, or embedded shells.

A Termux-free Android shell and platform-packaged inference engine remain explicitly unverified and are not represented as complete capabilities.
