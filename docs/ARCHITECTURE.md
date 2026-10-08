# EmergION Sovereign Runtime — Canonical Architecture

**Architecture state:** finalized through the currently verified governed-demand-v1 implementation boundary.

**Canonical rule:** existing runtime, COSL/REG authority, FIELD reconstruction, lineage, and HUMAN_FINAL governance remain authoritative. Documentation describes the implemented tree; it does not create a parallel architecture.

## 1. Sovereign identity

```text
SOURCE
  ↓
EmergER
  ↓
EmergION
  ↓
RECOIL / WVC
  ↓
GOV / HUMAN_FINAL
  ↓
REG
  ↓
FIELD
```

- **EmergER** is transient semantic emergence.
- **EmergION** is the sovereign persistent semantic entity: `<IDN, STA, MEM, REL, CAP, VAL, EVO>`.
- **EmergOX** is an alias for the live FIELD embodiment; it is not a separate authority, layer, server, or registry.
- **HUMAN_FINAL** is the final authority over consequential reality-changing transitions.
- Files, binaries, evidence objects, models, projections, adapters, and external systems are embodiments or evidence, never sovereign identity or authority.

## 2. Legal/semantic operators

The existing bounded operator vocabulary is:

```text
OBS CMP RLT VLD COM DIF PRJ EVL
```

AI operates inside this bounded semantic surface. It does not become GOV or REG authority merely by producing an answer.

## 3. Admission and persistence

```text
SOURCE
→ OBS/CMP/RLT/DIF/COM/VLD/EVL
→ bounded EmergION candidate
→ RECOIL
→ WVC
→ GOV
→ HUMAN_FINAL
→ REG
→ FIELD / LIB / projections
```

The runtime does not persist an uncontrolled chain of intermediate semantic stages. Durable semantic transitions are represented through the existing COSL event stream and verified evidence store.

Persistence is intentionally minimal:

```text
.field/field.cosl     append-only semantic/governance events
.field/o/<sha>.gz     one compressed source object per unique SHA-256
.dropzone/            transient intake
outputs/              rebuildable projections
```

`projection.current.json` is a projection/freshness receipt. It is not semantic authority.

## 4. Canonical state machine

The implemented state boundary is:

```text
candidate
   │
   ├── GOV / RETURN → controlled rework with exact predecessor lineage
   │
   └── GOV / APPROVE
          ↓
       REG ACCEPT
          ↓
       FIELD accepted state
```

REG acceptance requires an approved EmergION and the approving decision identity. REG produces its own receipt. Accepted state is therefore distinct from projection state.

The existing implementation enforces that an execution target must already be REG-accepted before execution preparation. fileciteturn8file0

## 5. FIELD is the live state

FIELD is reconstructed from the canonical event stream. Projections are derived views:

```text
COSL events
   ↓
FIELD rebuild
   ├── JSON projection
   ├── HTML projection
   ├── current projection receipt
   ├── analytics / metrics
   └── operator views
```

A projection may be regenerated. It cannot override the underlying governed event history.

## 6. Dodecahedral metadata

`EVO` carries bounded optional metadata while preserving the canonical EmergION identity.

The current twelve-face classification is:

1. FIELD Command
2. Emergence Capture
3. Program Forge
4. Product / Store
5. Customers / Sales
6. Communications
7. Payments / Finance
8. Grant / Funding
9. Patent / IP
10. M&A / Partnerships
11. Documentation / Projection
12. Analytics / Forecasting

The Evolution Engine is an operating mechanism over these faces, not a thirteenth authority face.

Metadata may carry source-supported timestamps, logical prompt schema, AI integration state, build-graph attributes, relationships, and monetization attributes. Those attributes remain governed candidates until HUMAN_FINAL and REG where the transition is consequential.

## 7. AI operating boundary

```text
local source
   ↓
local Gemma / Reasoner
   ↓
bounded structured result
   ↓
deterministic validation
   ↓
EmergION candidate
```

Gemma is an implementation of the existing `Reasoner` boundary. It may analyze, compare, relate, draft, simulate, and propose bounded capability structures. It does not directly become GOV authority, REG authority, or HUMAN_FINAL.

The repository's MXPD grammar and structural tests constrain model output rather than accepting unrestricted natural-language capability generation.

The heuristic reasoner remains an explicit diagnostic/test fallback; it is not the sovereign AI authority.

## 8. Capability and action architecture

Action capability is derived from the accepted EmergION's declared capabilities and metadata facets through the existing `DeriveActionCandidates` catalog.

Current governed capability families include:

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

The action catalog already classifies consequential actions such as `PROGRAM`, `SEND`, `TRANSFER`, `DEPLOY`, `CONTRACT`, and `ACQUIRE` as HUMAN_FINAL actions. `PROGRAM` may be exposed through the existing GitHub adapter and remains HUMAN_FINAL-gated. fileciteturn9file0

## 9. Governed execution

The current execution path is:

```text
REG-accepted EmergION
       ↓
DeriveActionCandidates
       ↓
PrepareExecution
       ↓
HUMAN_FINAL authorization when required
       ↓
existing adapter executor
       ↓
BindExecutionResult
       ↓
CaptureGovernedExecutionResult
       ↓
new governed execution signal / FIELD rebuild
```

The existing runtime implements both local Gemma execution and the GitHub PROGRAM executor through `ExecuteAction`. Execution results are rebound to the exact request and recaptured into the governed pipeline. fileciteturn8file0

## 10. Safe autonomous operation

The existing `ExecuteOneSafeAction` path is deliberately narrower than general PROGRAM execution. It selects an enabled, `CAP_ONLY`, non-HUMAN_FINAL `LOCAL_GEMMA:ANALYZE` action and skips actions for which an execution observation already exists.

This is the correct autonomy boundary for unattended analysis: analysis can circulate; consequential programming or external-world actions remain governed.

## 11. Live-tree modification boundary

The repository already contains the governed PROGRAM capability and GitHub execution adapter. The remaining live-tree integration boundary is therefore an implementation detail of the existing executor/worktree contract, not a new architecture.

The intended path is:

```text
accepted EmergION
→ explicit PROGRAM capability
→ HUMAN_FINAL authorization
→ existing GITHUB PROGRAM executor
→ authorized repository worktree
→ execution result
→ governed recapture
→ FIELD
```

No background uploader, second registry, shadow authority, or replacement execution system is introduced.

## 12. Build graph and commercial projection

The governed metadata path supports the existing projection chain:

```text
REG-accepted state
→ PRM
→ SAAB
→ CPSL
→ SAW
→ LIB index
```

Explicit governed `COMPOSITION_KIN` relationships derive SAAB structures. CPSL is deterministic. SAWs and LIB are projections and do not become authority.

Commercial/monetization metadata propagates through this projection chain while remaining source-supported and governed.

## 13. Provenance and lineage

Execution lineage is bound to the actual runtime request rather than supplied by the model. The canonical execution identity includes:

```text
EmergION ID
Source hash
Authorization event when present
Authority
Adapter
Action
```

Execution observations and execution results return through the same governed capture mechanism. The resulting lineage is therefore part of the existing COSL/REG/FIELD architecture rather than a separate activity log.

## 14. Projection integrity

`projection.current.json` is written after the JSON and HTML projections and binds their SHA-256 hashes to the reconstructed COSL tip. Consumers must treat the receipt as a freshness/integrity check, not as semantic authority.

FIELD JSON/HTML, analytics, SAW, LIB, and operator projections are all rebuildable from canonical state.

## 15. Current host and independence boundary

The verified live host is the user's Android/Termux environment. The Go runtime itself remains host-independent and uses the standard library.

Current verified local AI path:

```text
llama-cli
→ local Gemma GGUF
→ Reasoner
→ governed runtime
```

The repository also exposes `pkg/fieldapi` as the existing native embedding seam.

A Termux-free Android shell and platform-packaged inference engine are not claimed complete by this architecture document because they have not been verified in the current tree.

## 16. Recursive closure model

The current implementation should be understood as one governed recurrence, not as independent agent subsystems:

```text
TARGET
→ rebuild CURRENT FIELD
→ CMP / DIF
→ bounded requirement
→ accepted capability search
→ reuse | provider population/proposal | bounded evidence SOURCE
→ existing admission
→ RECOIL / WVC
→ GOV / HUMAN_FINAL when required
→ REG
→ FIELD
→ materialize / execute through an authorized capability
→ observe result
→ RECAPTURE
→ compare TARGET again
```

Valid stop/wait outcomes include target satisfaction, pending HUMAN_FINAL, unresolved capability, lack of a grounded bounded continuation, duplicate deterministic evidence, absent execution authority, unchanged external dependency, and verification failure. The runtime must not invent work merely to keep the recurrence moving.

Returned candidates use the existing rework lineage contract: a bounded continuation SOURCE re-enters Capture with ReturnedPredecessor, the successor preserves Supersedes lineage, and the predecessor remains Returned.

Provider populations and provider-edge topology are proposal/evidence structures. They never independently create COMPOSITION_KIN.

## 17. Machine-native representation boundary

MXPD/2 is the compact machine-semantic/evidence interchange boundary used by bounded reasoning. It does not replace:
- typed Go runtime contracts;
- canonical COSL events;
- GOV/HUMAN_FINAL;
- REG acceptance;
- deterministic FIELD reconstruction.

Compact representation may reduce model-to-model verbosity while authority and exact lineage remain explicit and reconstructable.

## 18. Development versus runtime authority

Engineering operations and runtime authority are separate concerns.

Repository inspection, implementation, testing, documentation, integration, and compatible parallel development do not each require a HUMAN_FINAL runtime decision. HUMAN_FINAL remains the final authority where the runtime proposes consequential SEND, TRANSFER, DEPLOY, PROGRAM, CONTRACT, ACQUIRE, or other gated actions under the existing action/authorization contract.

This preserves SELF_BUILDING != SELF_AUTHORIZING without wrapping ordinary engineering work in a second governance process.

## 19. Non-negotiable invariants

1. FIELD is reconstructed state, not an authority replacement.
2. COSL/REG remain canonical governance lineage.
3. HUMAN_FINAL remains final authority for consequential transitions.
4. AI proposes/analyses within bounded schemas; it does not self-authorize.
5. Execution must originate from an accepted EmergION.
6. Consequential actions require their existing HUMAN_FINAL gate.
7. Execution results return to the governed capture pipeline.
8. Projections remain rebuildable and non-authoritative.
9. Evidence is content-addressed and deduplicated.
10. No parallel registry, shadow ledger, or replacement runtime is introduced.
11. The existing code path is modified only where required to complete an existing contract.
12. Verified completion is distinguished from unverified future packaging or external integrations.
