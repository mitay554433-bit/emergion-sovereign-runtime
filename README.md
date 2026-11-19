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
- `docs/SYSTEM_STATUS.md` — verified state, operational AI boundary, projections, build state, and explicit unverified boundaries (section 22 lists merges through #54).
- `docs/DOCS_CENSUS_2026-11-19.md` — which documents are current, historical, or stale.
- `docs/PILOT_DEMO.md` — throwaway pilot walkthrough (`scripts/pilot-demo.sh`) and the footprint method.
- `REPOSITORY_MANIFEST.json` — stale (generated 2026-08-05 for 29 files); regenerate it before treating its hashes as current.

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
GEMMA_MAX_TOKENS=256
GEMMA_TIMEOUT_SECONDS=360
GEMMA_EXTRA_ARGS=
```

## Operation

The canonical pilot package is the existing `field` CLI plus the existing
`fieldd` daemon. Build both from the same repository revision:

```sh
mkdir -p dist
go build -o dist/field ./cmd/field
go build -o dist/fieldd ./cmd/fieldd
```

The CLI discovers an adjacent `fieldd` binary, uses the platform temporary
directory for its PID/socket files, and exposes the operator surface on
loopback by default:

```sh
./dist/field init
./dist/field doctor
./dist/field daemon start
./dist/field daemon status
```

Operator surface:

```text
http://127.0.0.1:8899/field.html
```

Stop the same governed daemon with:

```sh
./dist/field daemon stop
```

The operator HTTP listener binds to `127.0.0.1:8899` by default. Set
`FIELD_HTTP` to another loopback address when required, or set it to the
empty string to disable HTTP. Non-loopback binds are refused unless a
bearer token is already available.

Mutating HTTP routes (`POST /decide`, `POST /authorize`, and any other
non-GET route) require `Authorization: Bearer <token>`. The token is read
from `FIELDD_TOKEN`, or from a `0600` `fieldd.token` file in the state
directory (default `.field/fieldd.token`, created on first loopback
start). Read-only projection routes may be called without a token on
loopback. `field decide` and `field authorize` talk to the local store
directly and do not use these HTTP routes. `cmd/fieldandroid` also
embeds `pkg/fieldapi` in-process and does not call fieldd HTTP.

Phone / Termux setup (from the repository root):

```sh
mkdir -p dist
go build -o dist/field ./cmd/field
go build -o dist/fieldd ./cmd/fieldd
./dist/field init
./dist/field daemon start
ls -l .field/fieldd.token
# -rw-------  fieldd.token

# read-only (no token needed on loopback)
curl -sS http://127.0.0.1:8899/status

# mutating routes
TOKEN=$(cat .field/fieldd.token)
curl -sS -X POST http://127.0.0.1:8899/decide \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"emergion_id":"E-...","decision":"APPROVE","reason":"reason"}'
curl -sS -X POST http://127.0.0.1:8899/authorize \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"emergion_id":"E-...","adapter":"ADAPTER","action":"ACTION","reason":"reason"}'
```

To set the token yourself instead of using the generated file:

```sh
umask 077
openssl rand -hex 32 > .field/fieldd.token
export FIELDD_TOKEN=$(cat .field/fieldd.token)
./dist/field daemon start
```

A non-loopback bind (for example `FIELD_HTTP=0.0.0.0:8899`) will not
start unless `FIELDD_TOKEN` or `.field/fieldd.token` already exists.
`FIELD_SOCKET`, `FIELD_PID`, and `FIELDD_BIN` remain explicit overrides.

For a foreground living FIELD loop rather than the daemon/operator surface:

```text
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
