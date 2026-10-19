---
schema_version: "1.1"
metadata_schema: "1"
artifact_type: benchmark-result
id: "BR-003"
suite_id: "B-001"
scenario_id: "BS-002"
title: "Complete noise rerun of selective migration"
status: "sealed"
outcome: "passed"
subject_revision: "sha256:d1254df33748ef2d72c1cee5362c135ed986d91c64c495f76638b594bf1973cc"
harness_revision: "sha256:3e733a7a386137ede594cc8a26c090c2a09a1d5e1956537facb962447cf08535"
supersedes: ["BR-002"]
manifest: "EVIDENCE_MANIFEST.json"
created: "2026-09-16T05:03:37Z"
updated: "2026-09-16T05:06:04Z"
author: "Codex"
owner: "otel-span-classifier maintainers"
completed: "2026-09-16T05:06:04Z"
executed_by: "Codex"
---

> Public redacted copy of historical evidence. Measurements and outcome are unchanged. Original digests identify the pre-redaction source; verify these distributed bytes with the [public distribution verifier](../../../../verify_publication.py). This is not a new measurement or resealed Run.


# Complete noise rerun of selective migration

## Summary

Completed eight paired process repetitions under BS-002. Outcome: passed. All fixture allocation counts and bytes are unchanged. All predeclared latency and noise-margin gates passed.

## Revisions and Environment

Frontmatter and artifacts/source-manifest.json bind source and harness bytes from the uncommitted worktree. Frozen control is the BR-001 candidate, not the original unoptimized implementation. Apple M4, darwin/arm64, Go 1.25.4, GOMAXPROCS=1; pinned pdata v1.55.0. Workstation background load is uncontrolled. No deviation from BS-002. Binary digest retained after cleanup.

## Procedure and Commands

Executed `python3 benchmarks/suites/b-001_span-projection/scripts/run_selective.py benchmarks/suites/b-001_span-projection/runs/br-003_selective-sparse-repeat/artifacts`. Correctness and compilation preceded retained 100ms/case warmup, followed by eight alternating Prior/Candidate processes per variant at 200ms/case. Optimized compiler and objdump diagnostics followed timing. Summary is reproducible with scripts/summarize_selective.py in the snapshot. All samples retained; no exclusions.

## Decision Rule

Protocol-case geometric mean is 0.748084 against <=0.95; its conservative noise band is 0.042430. Cases whose margin is not resolved: none. Per-case thresholds, MAD bands, allocation decisions and sample counts are in artifacts/summary.json. Outcome is passed under the unmodified protocol. Stop at this passing run.

## Raw Observations

Each value is the median of eight process samples; ± is median absolute deviation (MAD), not a confidence interval. Mixed is one full sweep of 20 fixture Spans, not a single Span. Dense cases contain 32/64/128 legacy-style underscore HTTP header keys, including missing-context diagnostics.

| Case | Prior ns/op ± MAD | Candidate ns/op ± MAD | Ratio | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|
| Unchanged8 | 321.55 ± 1.60 | 294.20 ± 2.00 | 0.915 | 544 → 544 | 12 → 12 |
| Unchanged32 | 988.20 ± 7.00 | 892.15 ± 3.15 | 0.903 | 2016 → 2016 | 36 → 36 |
| Unchanged64 | 1906.50 ± 22.00 | 1709.00 ± 9.00 | 0.896 | 3808 → 3808 | 68 → 68 |
| CanonicalHTTP8 | 698.45 ± 4.40 | 489.70 ± 2.85 | 0.701 | 544 → 544 | 12 → 12 |
| HTTP | 1547.00 ± 15.50 | 1496.50 ± 7.50 | 0.967 | 872 → 872 | 21 → 21 |
| DB | 1127.00 ± 3.50 | 816.75 ± 12.45 | 0.725 | 536 → 536 | 12 → 12 |
| RPC | 1471.50 ± 11.00 | 1111.50 ± 5.00 | 0.755 | 608 → 608 | 17 → 17 |
| MissingContext | 817.90 ± 2.70 | 565.70 ± 2.55 | 0.692 | 656 → 656 | 11 → 11 |
| Invalid | 985.10 ± 7.60 | 630.15 ± 6.20 | 0.640 | 816 → 816 | 14 → 14 |
| Sparse1Of32 | 2805.00 ± 29.50 | 1048.00 ± 10.00 | 0.374 | 2016 → 2016 | 36 → 36 |
| Sparse2Of32 | 3032.00 ± 14.00 | 1219.50 ± 13.00 | 0.402 | 2016 → 2016 | 36 → 36 |
| Dense32 | 3855.50 ± 12.50 | 3463.50 ± 32.00 | 0.898 | 6960 → 6960 | 138 → 138 |
| Sparse1Of64 | 6937.50 ± 36.00 | 1951.00 ± 14.50 | 0.281 | 3808 → 3808 | 68 → 68 |
| Sparse2Of64 | 7376.50 ± 44.00 | 2218.00 ± 5.00 | 0.301 | 3808 → 3808 | 68 → 68 |
| Dense64 | 7534.00 ± 125.50 | 6968.00 ± 114.50 | 0.925 | 14384 → 14384 | 267 → 267 |
| Sparse1Of128 | 20208.00 ± 36.50 | 3793.50 ± 4.50 | 0.188 | 7520 → 7520 | 132 → 132 |
| Sparse2Of128 | 20966.50 ± 71.00 | 4231.50 ± 21.00 | 0.202 | 7520 → 7520 | 132 → 132 |
| Dense128 | 14988.00 ± 154.00 | 13493.00 ± 122.50 | 0.900 | 29360 → 29360 | 524 → 524 |
| CanonicalDB8 | 929.65 ± 16.25 | 567.00 ± 2.60 | 0.610 | 528 → 528 | 12 → 12 |
| CanonicalRPC8 | 1079.00 ± 7.00 | 664.80 ± 6.15 | 0.616 | 528 → 528 | 12 → 12 |
| Mixed | 103865.00 ± 1949.50 | 49200.50 ± 273.50 | 0.474 | 88848 → 88848 | 1628 → 1628 |

## Interpretation

H1/H2/H3 timing predictions supported under the stated gates. The unchanged allocation prediction is supported in every fixture. Source snapshots show operation-level work bits, reuse of HTTP identity and RPC system, and at most two attribute decisions before bulk copy and output-owned value moves. Compiler diagnostics show hasHTTP/rpcSystem inlining; objdump retains the projection and copy paths. These corroborate executed work and ownership mechanics, not an exact CPU cost breakdown. The comparison changes dispatch, context reuse and sparse construction together; do not apportion speedup between them.

## Contradictions and Supersession

No differential correctness mismatch. Two preliminary exploratory trials are retained separately: trial-01 reserved fixed extra capacity and increased sparse B/op; it was rejected. Trial-02 computed capacity from decisions and removed that allocation-byte regression. Their single-sample timings are not formal evidence. BR-002 completed but was inconclusive because of high timing dispersion. This is the single complete noise rerun allowed by BS-002; source and harness identities are identical. BR-002 remains sealed and retained; the two runs are not pooled and no samples were excluded.

## Boundaries and Extrapolation

Synthetic local hot-cache library microbenchmarks with independently owned output Maps. No production throughput, end-to-end Collector latency, p99, Linux, other Go/CPU versions or sanitizer-cost claim. Query fixtures use already-sanitized text. The pre-copy sparse path can copy values later suppressed; unchanged allocation observations are fixture-specific, not a guarantee for every possible input. Exact attributes/types/status/ordered issues and nested ownership are checked independently of performance.

## Handoff

Repository maintainers may use this bundle to inspect the implementation and reproduce BS-002. It satisfies this scenario on the measured revision. Existing APIs and SemConv v1.41.0 output semantics are preserved. No deploy or Collector integration is implied.

## Artifacts

source-manifest.json and snapshot/** bind implementation, controls, tests, fixtures and runner/summary scripts. go-version.txt, go-env.txt, os.txt and cpu.txt identify environment; correctness.txt and build.txt prove setup; warmup.txt and repeat-*.txt contain raw observations; summary.json contains calculations; compiler.txt and objdump.txt contain optimized diagnostics; binary-sha256.txt identifies the deleted executable. exploratory/** retains initial trials and verification logs. spec-audit.json and spec-evidence.json record activated requirement coverage; post-run-load.txt records a non-invasive workstation load observation. No external artifacts or production inputs.
