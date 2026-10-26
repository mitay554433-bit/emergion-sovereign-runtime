# UNIFUSION — Complete source custody and portability

The **complete current code** is the Git tree at an exact commit SHA. Git is the versioned source ledger; `CODE_STATE.current.json` is a derived index, not a replacement for source bytes. Do not paste duplicate copies of every file into a separate canonical ledger: that creates divergence.

## Exact reproducible source package

From a checkout of the desired commit:

```sh
python3 scripts/export_portable_source.py --output-dir exports
```

The tool archives **every tracked file from the exact committed HEAD**, including code, contracts, scripts and documentation, into `UNIFUSION_SOURCE_<sha>.zip`. It embeds `SOURCE_LEDGER.json` with each full path, byte count, SHA-256 and Unix mode; alongside is the ZIP SHA-256. Export reads `git archive HEAD`, so uncommitted files cannot silently contaminate the baseline. It does not mutate COSL/REG or require a live runtime.

To inspect or reconstruct later, retrieve the ZIP by commit SHA, verify the ZIP checksum and each member hash against the embedded ledger, then extract `source/`. `git clone` and `git checkout <sha>` also recover the complete committed source and history.

## After any code inspection/change

1. Record exact repo, branch, commit and changed file paths.
2. Update the actual source in Git and commit; **Git retains the previous version and complete bytes**.
3. Generate a new source snapshot/export and retain its hash as a dated build artifact. Never label the old snapshot current after a code change.
4. When a source file is outside Git (Termux Python/VERITEX, codex, mirror, external adapter), first inventory the path and capture its exact bytes/version into an authorized source archive. Never assume it was included in the Go repository.
5. Document build/dependency versions, startup instructions and host-specific requirements separately. A source archive alone is not a deployed or operational system.
6. For a different host, rebuild from the verified source, provision local model/runtime dependencies, restore governed state from verified evidence export, then run acceptance tests. Never fabricate or rewrite COSL/REG state.

## Current limitation

The portable exporter is implemented in the repository, but the ZIP has not yet been produced and verified on a checked-out host. External phone code and runtime state are not automatically captured. The portable Go repository is therefore a **source custody mechanism**, not proof that UNIFUSION can already run unchanged on every operating system.
