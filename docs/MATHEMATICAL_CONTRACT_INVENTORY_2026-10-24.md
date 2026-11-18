# UNIFUSION mathematical contract inventory — 2026-10-24

Classification: IMPLEMENTATION-BOUND INVENTORY, not an invented unified equation or a claim that a mathematical research model has been proved.

## Evidence identity
For preserved source bytes `b`, `h = SHA256(b)`. Content-addressed source objects and COSL-linked evidence must be checked against exact bytes. A hash is a deterministic identity/integrity primitive, not a confidence score.

## State and authority predicates
Let `A(e)` denote REG acceptance of EmergION `e`, `G(e)` denote an explicit HUMAN_FINAL approval receipt, and `Q(e,a)` denote valid action authorization for action `a`. The code's authority obligations include:
- `A(e) => G(e)` through the existing decision and REG acceptance path.
- An execution target must satisfy `A(e)` before action preparation.
- For HUMAN_FINAL-gated actions, execution requires the applicable `Q(e,a)`, after REG acceptance.
- A projected FIELD row or AI inference never implies `A(e)` or `Q(e,a)`.

These are logical invariants, not statistical estimates. Check their enforcement in `internal/gov`, `internal/reg`, `internal/runtime/governed_execution.go`, `internal/cosl`, and tests.

## Realization comparison
`RealizationComparison{TargetThing,Left,Right,Result,Differences}` is defined in `internal/runtime/realization_comparison.go`.
- `Left != Right`, both nonempty and both REG-accepted.
- `Result ∈ {LEFT_PREFERRED, RIGHT_PREFERRED, EQUIVALENT, UNRESOLVED}`.
- `Result != UNRESOLVED => len(Differences) > 0`.
No numeric preference function, utility weighting, or probabilistic calibration is specified by this type. Do not assign scores without a separate governed, evidenced definition.

## Reciprocal observation
`pivot.Observe(name,forward,reciprocal,invariant,check)` invokes a supplied check. Passing check sets `Passed=true`; missing or failing check returns a divergence error with an EmergION that has not passed RECOIL/WVC or admission. See `internal/pivot/pivot.go`. This does not define a probabilistic agreement metric.

## Analytics
`analytics.Measure(st)` reports cardinalities of reconstructed governance states, event count, COSL tip hash, CPU count, goroutine count, and UTC measurement time. See `internal/analytics/analytics.go`. It does not measure autonomous success rate, confidence, coherence, or economic return.

## Mathematics requiring primary-source reconciliation
The following may be historically described but cannot be promoted into numerical runtime claims without exact source, domain, dimensional assumptions, numerical tests, and governance provenance:
- Adaptive Rational–Probabilistic Bridge / coherence evaluation (including any `C′` formula);
- TORQUE and boundary/fracture models;
- WVC mathematical definitions beyond the enforced runtime validation contract;
- economic scoring and optimization across product, sales, finance, and grant systems;
- probability or confidence calibration of Gemma outputs.

## Acceptance for any future numeric implementation
Record: exact equation and source; symbol definitions; input domain; units; boundary behavior; determinism and numerical tolerance; sample fixtures; adversarial cases; provenance; and the existing runtime call path. Do not create a parallel math authority or self-authorize a derived result.
