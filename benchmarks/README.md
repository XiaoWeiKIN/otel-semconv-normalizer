# Historical benchmark evidence

This directory publishes redacted copies of three historical microbenchmark Runs, their protocols, measurement scripts, all retained samples, reports, source snapshots, compiler diagnostics, and two CPU profiles. These are historical measurements on Apple M4, darwin/arm64, Go 1.25.4 and pdata v1.55.0. They do not measure the current public commit or establish Collector throughput.

[Browse the Suite, Scenarios and Runs](BENCHMARKS.md). BR-001 passed, BR-002 was inconclusive, and BR-003 passed under their original decision rules. The inconclusive Run and exploratory evidence are retained.

## What was redacted

Module and package identifiers now use the public GitHub module. Workstation paths and hostname are replaced with generic values. Project-specific branding is removed. Private fixture-source references are explicitly labeled as redacted origins rather than assigned to a fictitious public repository. CPU profiles retain their native gzip/protobuf format; only string-table entries were redacted. Sample counts, sample values, timing and allocation rows, thresholds, summaries and outcomes are unchanged. The historical compiled binaries were already removed by the original scripts; their recorded digests remain historical identifiers.

The original sealed bundles remain unchanged in their source location. This public distribution is a derivative copy, not a resealed Run. Each original `EVIDENCE_MANIFEST.json`, source/harness identity and generated snapshot manifest is retained as historical provenance. Their digests describe the original bytes, so they do not authenticate redacted copies. Do not run `benchctl validate` on this derivative distribution and interpret it as an original seal check.

`PUBLICATION_MANIFEST.json` separately inventories every distributed file, includes the original file digests, identifies changed files, and records the local preservation checks. It does not certify the experimental design or create a new performance result.

## Verify the public files

From the repository root:

```bash
python3 benchmarks/verify_publication.py
```

The command checks the exact inventory, file sizes and SHA-256 values, and fails on missing, added, modified or symlinked files. It does not execute benchmark code.

## Read and reproduce the evidence

Use the linked `RESULT.md` files for conclusions and limits, and `SCENARIO.md` for the protocol of record. Frozen source snapshots are historical evidence subsets, not complete standalone repositories. Their generated-file digests still identify the original source; use the public distribution verifier for their redacted bytes.

The active scripts run against the checkout from which they are invoked. The historical Runs used macOS; the selective script also invokes `sysctl`. On a suitable macOS host, write new output outside the historical Run directories:

```bash
python3 benchmarks/suites/b-001_span-projection/scripts/run.py /tmp/projection-new-run
python3 benchmarks/suites/b-001_span-projection/scripts/run_selective.py /tmp/selective-new-run
```

Use a fresh output directory for each new execution. Changing the public checkout, environment or compiler creates a new measurement; it does not reproduce the original byte identity. The scripts take several minutes. Their predeclared historical results are not updated by this publication.
