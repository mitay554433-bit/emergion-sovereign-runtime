# Architecture

## Identity

```text
EmergER → EmergION → EmergOX/FIELD
```

- **EmergER** is the transient semantic emergence process.
- **EmergION** is the sovereign persistent semantic entity:
  `<IDN, STA, MEM, REL, CAP, VAL, EVO>`.
- **EmergOX** is an alias for the live FIELD embodiment formed by REG-accepted EmergIONs.
- **HUMAN_FINAL** governs reality.

Files, binaries, event streams, compressed objects, models, projections, plugins, and adapters are embodiments. They are never identity or authority.

## Legal operators

```text
OBS CMP RLT VLD COM DIF PRJ EVL
```

## Admission

```text
SOURCE
→ transient OBS/CMP/RLT/DIF/COM/VLD/EVL
→ one EmergION candidate
→ RECOIL
→ WVC
→ GOV/HUMAN_FINAL
→ REG
→ LIB/PROJ
→ FIELD
```

The runtime persists only the completed GOV-ready EmergION. Intermediate stages exist in memory and disappear after validation.

## Dodecahedral metadata projection

Optional metadata is carried inside `EVO`, preserving the canonical
`<IDN, STA, MEM, REL, CAP, VAL, EVO>` identity and the existing COSL/REG lineage.
It timestamps capture, records the logical prompt schema and AI integration state,
and may hold source-supported build-graph and monetization attributes. Capability
classification is bounded to twelve faces: FIELD Command, Emergence Capture,
Program Forge, Product/Store, Customers/Sales, Communications, Payments/Finance,
Grant/Funding, Patent/IP, M&A/Partnerships, Documentation/Projection, and
Analytics/Forecasting. The Evolution Engine governs those faces; it is not a
thirteenth face. Metadata remains a GOV candidate until HUMAN_FINAL and REG.
Before persistence, the runtime bounds and validates facets, graph nodes, graph
edges and monetization text. REG reconstruction also verifies that every
acceptance receipt names the exact HUMAN_FINAL approval event.

## Persistence embodiment

The implementation uses:

```text
one COSL event stream
+ one compressed evidence object per unique source
+ rebuildable projections
```

Candidate, decision, and REG events are the only durable semantic transitions. Source objects are deduplicated by SHA-256 and gzip-compressed. The Dropzone is deleted after verified capture. Orphan evidence is removed by verification.

## Reasoning

Gemma is a bounded CAP embodiment through the `Reasoner` interface. It may analyze, compare, relate, draft, and simulate. It cannot decide at GOV or accept at REG.

```text
local source
→ Gemma analysis
→ deterministic structural validation
→ EmergION candidate
```

The default reasoner is Gemma. The heuristic reasoner exists only as an explicit diagnostic and test fallback.

## Living FIELD

The local event loop is not a server. It observes the Dropzone, performs local reasoning, persists one candidate, clears transient input, updates projections, and waits at GOV.

```text
FIELD observes
→ EmergER activates
→ candidate emerges
→ HUMAN_FINAL governs
→ REG accepts
→ FIELD changes
```

Every new EmergION carries visible integration metadata: UTC capture time,
reasoner identity and version, the governing prompt schema, logical prompts,
monetization hypotheses, build nodes and edges, and the bounded dodecahedron
facets. These remain evidence-backed hypotheses until HUMAN_FINAL approval and
REG acceptance.

The existing HTML projection presents this metadata as a readable registry.
`field open` regenerates it and invokes the platform file opener; it does not
introduce a server or external runtime dependency.

## External capability boundary

Programs, patents, mergers and acquisitions, websites, stores, email, payments, customers, products, and sales will be added as bounded CAP adapters. Read, analysis, drafting, simulation, and testing may be automated. Sending, payment movement, contracts, acquisitions, deployment, and REG acceptance remain gated.

## Independence path

1. Native Go core — implemented.
2. Local Gemma CLI adapter — implemented.
3. Native embedding API — implemented.
4. Desktop/native shell — not yet implemented.
5. Android shell with embedded inference engine — not yet implemented.
6. External business adapters — not yet implemented.
