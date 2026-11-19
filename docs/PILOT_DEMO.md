# Pilot demo and footprint method

This page is for showing the runtime to a prospect without touching any real
state. It is documentation, not authority.

## Throwaway walkthrough

```sh
bash scripts/pilot-demo.sh "$PWD"
```

The script clones the checkout into a fresh directory under `$TMPDIR`, builds
`field`, and runs:

1. `field init` and `field doctor` on throwaway state (doctor is shown, not
   enforced; its Gemma check may be false on a machine without a model).
2. Two `capture` calls on synthetic samples, each followed by a HUMAN_FINAL
   `decide ... APPROVE` on the throwaway state.
3. `field verify`, saving `tip_hash` and `events` as a receipt.
4. Tamper cases, each on its own copy of the throwaway state:
   - one byte flipped in a stored evidence object: verify fails and names
     the IDN and hash;
   - a stored evidence object deleted: verify fails;
   - two stored evidence objects swapped: verify fails and names the hash;
   - the last ledger event deleted: verify still passes without a receipt,
     and fails with `-expect-tip` / `-expect-count` from the saved receipt,
     naming expected versus actual.

It refuses to run inside an `AXM` path, inside the source repository, or on
an existing target directory, and it removes the throwaway tree on success
unless `KEEP=1`. It uses the heuristic reasoner, so it needs no model and no
network.

The same tamper cases are pinned as Go tests in
`internal/runtime/stress_test.go` and `internal/runtime/done_when_test.go`.

## Footprint method

What to record, on the target phone, from the repository root:

- binary sizes: `du -h` on `field` and `fieldd` built from the same commit;
- evidence size: `du -sh .field`;
- one read-only `field verify` of the real state: wall time, and peak
  resident memory from `VmHWM` in `/proc/<pid>/status` sampled every 50 ms;
- `MemAvailable` from `/proc/meminfo` before and after;
- battery percentage and temperature from `termux-battery-status` before and
  after (needs the Termux:API app; skip with a timeout if it does not answer).

`field verify` is read-only for the ledger and evidence; it does not decide
or accept anything.

### First phone measurement (2026-11-19, operator Termux, reported in chat)

| Quantity | Value |
|---|---|
| `field` binary | 9.9 MB |
| `fieldd` binary | 14 MB |
| `.field` evidence directory | 29 MB |
| `field verify` on real state | PASS, 1.6–2.5 s (two runs) |
| Peak resident memory during verify | 81–91 MB (two runs) |
| `MemAvailable` during the runs | about 1.42–1.67 GB |
| Battery | not measured (Termux:API did not answer) |

Not yet measured: memory, storage and battery over one full fracture wave,
and battery at all. Do not make a battery claim until it is measured.

### Scale timing (opt-in test)

`UNIFUSION_STRESS=1 go test ./internal/runtime -run TestVerifyScaleGrowth -v`
builds 200 and 2,000 REG-accepted records in temporary state and logs verify
and FIELD rebuild times at 1x and 10x. On the development box (not the phone)
on 2026-11-19 it logged verify 7.2 ms vs 80.6 ms (ratio 11.2) and FIELD
rebuild 1.9 ms vs 25.9 ms (ratio 13.6). Building the 2,000-record fixture
takes over a minute on that box, so expect several minutes on a phone.
