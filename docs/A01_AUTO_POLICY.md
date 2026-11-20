# A01 amendment: governed auto-accept (A01-AUTO-1) and the nonstop worker

**Approved:** 2026-11-20 by Michael (owner) as HUMAN_FINAL, in chat, answering
"How much should UNIFUSION decide on its own?": *"Build it and change A01 so
low-risk records auto-accept."*

## A01 as amended

> **A01 — HUMAN_FINAL authority cannot be bypassed.** *Amended 2026-11-20
> (A01-AUTO-1, approved by Michael):* the low-risk record classes named in
> `policy/auto-accept.json` may be decided by the AUTO_POLICY decision source,
> and only while a HUMAN_FINAL-approved, REG-accepted switch has that exact
> policy (version and sha256) ON. Every auto decision is its own decision
> receipt (and REG receipt for APPROVE) with authority `AUTO_POLICY`, never
> HUMAN_FINAL, and can be undone with one HUMAN_FINAL press. Everything else
> still needs HUMAN_FINAL.

A02 (self-building does not imply self-authorizing) is unchanged: the worker
only proposes; the policy that decides was itself authorized by HUMAN_FINAL and
is switched on and off only by HUMAN_FINAL.

## The policy

File: `policy/auto-accept.json` (embedded in the binaries). Version
`A01-AUTO-1`, sha256 `18d8739c7fca808fb4ad773b05f1a4299d5981f563e6588261e2e0102f7c037e` at this commit. Any edit to the file changes the
hash; the old switch then shows "stale" and the policy stops deciding until a
new HUMAN_FINAL switch-on names the new hash.

| Class | Auto decision | Rule (all must hold) |
|---|---|---|
| `SOURCE_UPLOAD_VERIFIED` | APPROVE | `source_kind` SOURCE_UPLOAD from `governed_source_upload`; 64-hex `source_sha256`; the preserved descriptor's SHA256/PATH/TITLE/BYTES lines equal the record and the descriptor hash equals the record's source hash; if the file is present under `UNIFUSION_SOURCES_ROOT`, its sha256 must match; no Archonym; CAP exactly OBS; risk L; RECOIL+WVC passed; no facets, Supersedes, build graph, monetization or authority REL keys; no other record shares its sha256. |
| `EXACT_DUPLICATE` | HOLD | Another record already carries the same upload sha256 or preserved source hash and is decided, or is AtGOV with a smaller IDN. |
| `GENERATED_PROVIDER_COMBINATION` | HOLD | Generated provider-combination / trajectory record (`source_name = provider_population`). Never accepted. |
| `SOURCE_KIN_LINK_ACCEPTED_ENDS` | APPROVE | Worker-derived topic/kin link whose two distinct ends are both REG-accepted SOURCE_UPLOAD records at decision time; descriptor FROM/TO match; same structural guard as above. |

Never auto-decided: capabilities other than OBS; projections, facets, build
graphs, monetization; anything that changes authority (Supersedes,
COMPOSITION_KIN, kin_emergion, capability providers, targets, requirements);
wire keys; the GOV gate; the policy itself (AUTO_POLICY_SWITCH records); money
or external actions (action authorizations stay HUMAN_FINAL); Archonym-claiming
records; worker SOURCE_INSIGHT proposals (summaries, combinations, gaps).

## How it is enforced

- `gov.Decide` still accepts only HUMAN_FINAL. The policy writes through
  `gov.DecideAuto`, which can only APPROVE or HOLD an AtGOV record.
- Replay (`internal/field/rebuild.go`) re-checks every `AUTO_POLICY` receipt:
  the switch must be ON at that point in the ledger, the reason must name the
  switch's policy version and sha256, and the classifier must give the same
  decision and class. A forged or out-of-policy auto receipt makes replay fail
  closed.
- Receipt reason format: `AUTO_POLICY A01-AUTO-1 sha256=<hash> class=<CLASS> switch=<switch IDN>: <why>`.
- Switch: `AUTO_POLICY_SWITCH` record (REL `policy_state`, `policy_version`,
  `policy_sha256`), captured and approved in one HUMAN_FINAL press with a
  reason. Default OFF (no switch record = OFF).
- Undo: `REVERT` (new decision, HUMAN_FINAL only) moves an AUTO_POLICY
  REG-accepted record to HELD; an AUTO_POLICY HOLD is undone with the existing
  HUMAN_FINAL RESUME. Both are append-only. A HUMAN_FINAL accept can never be
  reverted through this path.
- Archonym strictness is unchanged; the ledger (`.field`) is only appended to.

Compatibility: a binary older than this change refuses to replay a ledger that
contains AUTO_POLICY or REVERT receipts ("decision requires HUMAN_FINAL").
Once the policy has decided anything, run this build or newer.

## Nonstop worker

`internal/worker`, run by fieldd's background loop (and `field work`). It
walks REG-accepted SOURCE_UPLOAD records that have no insight yet, gives the
reasoner the title, topic, mime and tags plus up to 8 related accepted sources
(shared tags first, then same topic), and writes:

- one `SOURCE_INSIGHT` per source (summary, combination, gaps) — waits AtGOV
  for one HUMAN_FINAL batch press (batch filter class `SOURCE_INSIGHT`);
- at most 3 `SOURCE_KIN_LINK` records per source, each unordered pair once,
  only to peers the reasoner picked from that short list — low-risk, so the
  policy accepts them when it is ON.

No Cartesian generation. Bounded batches (3 sources per step locally, 20 with
an online model), a pause between steps, a resumable checkpoint, and the
ledger as the source of truth for what is done (a lost checkpoint never
regenerates). A source whose reasoning fails 3 times is skipped; a reasoner
that cannot run at all (no model, endpoint down) pauses the worker without
blaming any source. Control and checkpoint live in `<state>-work/`
(`.field-work/` on the phone), outside the ledger. Default OFF.

## Reasoner switch

- `local` (default): `llama-completion` (or `GEMMA_BIN`), all cores
  (`GEMMA_THREADS` overrides), short 4-line prompt, `-no-cnv`, 120 tokens
  (`UNIFUSION_WORKER_TOKENS`), 10-minute limit (`UNIFUSION_WORKER_TIMEOUT_SECONDS`).
- `online`: an OpenAI-compatible endpoint: `UNIFUSION_REASONER_URL` (base,
  e.g. `https://host/v1`), `UNIFUSION_REASONER_MODEL`, key from
  `UNIFUSION_REASONER_KEY` or the file named by `UNIFUSION_REASONER_KEY_FILE`.
  The key is never logged, stored or returned. Only titles, topics, mime types
  and tags leave the phone, never file bytes.
- `auto`: online when configured and reachable (probe cached 60 s), else
  local; an online failure falls back to local for that call.

## Controls

/living shows an "Auto · A01-AUTO-1" panel: policy ON/OFF, worker ON/OFF,
reasoner mode, auto-accepted, auto-held, AtGOV to decide, and a status line.
Each button is a double-press (tap, then tap again within 4 s). The policy
press also asks for the HUMAN_FINAL reason. An auto-accepted record shows
"Undo auto-accept" in its detail panel.

Routes: `GET /auto/status`; bearer-token `POST /auto/policy`
`{state:on|off, reason}`, `POST /auto/worker` `{on, mode}`, `POST /auto/undo`
`{emergion_id, reason}`.
CLI: `field policy auto-accept status|on|off -reason R`, `field work [-once]`,
`field undo-auto <id> <reason>`.
