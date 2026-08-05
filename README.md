# EmergION Sovereign Runtime

A separate local-first implementation of:

```text
EmergER → EmergION candidate → RECOIL → WVC → GOV/HUMAN_FINAL → REG → FIELD
```

`EmergOX` is an alias for the live FIELD embodiment. It is not a layer, file, folder, server, or authority.

## What is working

- one native Go runtime using only the standard library,
- no ChatGPT dependency,
- no HTTP server,
- local Gemma reasoning through `llama-cli`,
- one GOV-ready EmergION per unique source,
- one compressed evidence object per unique source,
- one compact hash-chained COSL event stream,
- automatic Dropzone capture and clearing,
- explicit HUMAN_FINAL decisions,
- REG acceptance only after approval,
- deterministic FIELD reconstruction,
- automatic JSON and HTML FIELD projections,
- CPU and FIELD analytics,
- a native embedding API under `pkg/fieldapi`,
- cross-platform native builds.

The Gemma adapter is incorporated. A compatible Gemma GGUF model and `llama-cli` executable are not bundled because the model is large and platform-specific. The runtime automatically searches common local paths and `PATH`; `field doctor` fails closed until both are found.

## Minimal persistence

```text
.field/field.cosl     compact append-only semantic and governance events
.field/o/<sha>.gz     one compressed lossless source object per unique hash
dropzone/             transient intake; cleared after verified capture
outputs/              rebuildable FIELD projections
```

No SOURCE, KIN, EmergER, RECOIL, or WVC stage files are persisted.

## Local Gemma discovery

The runtime first checks `GEMMA_BIN` and `GEMMA_MODEL`, then searches `PATH` and common local model directories. Explicit configuration remains available:

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
field open
```

`field open` is the direct product entry point. It rebuilds `outputs/field.html`
from the canonical event stream and opens the living registry. On Termux it uses
`termux-open`; Linux, macOS, and Windows use their native opener.

New registry entities include a UTC integration timestamp, reasoner provenance,
logical prompts, monetization hypotheses, build-graph metadata, and the governed
dodecahedron topology. The heuristic fallback labels unresolved assumptions
instead of copying source code into the visible summary.

Files placed in the Dropzone are analyzed by local Gemma, reduced to one EmergION candidate, verified through RECOIL/WVC, persisted at GOV, and removed from the Dropzone. The runtime then waits for HUMAN_FINAL.

```text
field decide E-... APPROVE "reason"
```

Approval creates a GOV receipt and a separate REG acceptance receipt. FIELD projections are refreshed automatically.

## Independence

Termux is only one possible current host. The core does not import or depend on Termux. Native binaries are built for Linux AMD64, Linux ARM64, and Windows AMD64. `pkg/fieldapi` allows a future Android, desktop, or embedded native shell to use the same runtime without a CLI or server.

A truly Termux-free Android deployment still requires a native Android shell and a platform build of the local inference engine. That packaging is not yet completed.
