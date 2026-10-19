---
schema_version: "1.1"
metadata_schema: "1"
artifact_type: benchmark-result
id: "BR-002"
suite_id: "B-001"
scenario_id: "BS-002"
title: "Selective migration and bounded sparse projection"
status: "sealed"
outcome: "inconclusive"
subject_revision: "sha256:d1254df33748ef2d72c1cee5362c135ed986d91c64c495f76638b594bf1973cc"
harness_revision: "sha256:3e733a7a386137ede594cc8a26c090c2a09a1d5e1956537facb962447cf08535"
supersedes: []
manifest: "EVIDENCE_MANIFEST.json"
created: "2026-09-16T05:00:21Z"
updated: "2026-09-16T05:03:37Z"
author: "Codex"
owner: "otel-span-classifier maintainers"
completed: "2026-09-16T05:03:37Z"
executed_by: "Codex"
---

> Public redacted copy of historical evidence. Measurements and outcome are unchanged. Original digests identify the pre-redaction source; verify these distributed bytes with the [public distribution verifier](../../../../verify_publication.py). This is not a new measurement or resealed Run.


# Selective migration and bounded sparse projection

## Summary

Completed eight paired process repetitions under BS-002. Outcome: inconclusive. All fixture allocation counts and bytes are unchanged. Median latency thresholds were met, but dispersion prevents the predeclared no-regression claim. This is not a passing acceptance result.

## Revisions and Environment

Frontmatter and artifacts/source-manifest.json bind source and harness bytes from the uncommitted worktree. Frozen control is the BR-001 candidate, not the original unoptimized implementation. Apple M4, darwin/arm64, Go 1.25.4, GOMAXPROCS=1; pinned pdata v1.55.0. Workstation background load is uncontrolled. No deviation from BS-002. Binary digest retained after cleanup.

## Procedure and Commands

Executed `python3 benchmarks/suites/b-001_span-projection/scripts/run_selective.py benchmarks/suites/b-001_span-projection/runs/br-002_selective-sparse/artifacts`. Correctness and compilation preceded retained 100ms/case warmup, followed by eight alternating Prior/Candidate processes per variant at 200ms/case. Optimized compiler and objdump diagnostics followed timing. Summary is reproducible with scripts/summarize_selective.py in the snapshot. All samples retained; no exclusions.

## Decision Rule

Protocol-case geometric mean is 0.785313 against <=0.95; its conservative noise band is 0.708463. Cases whose margin is not resolved: Unchanged8, Unchanged32, Unchanged64, CanonicalHTTP8, HTTP, DB, RPC, MissingContext, Invalid, Sparse1Of32, Sparse2Of32, Dense32, Sparse2Of64, Dense64, Sparse1Of128, Dense128. Per-case thresholds, MAD bands, allocation decisions and sample counts are in artifacts/summary.json. Outcome is inconclusive under the unmodified protocol. One complete fresh noise rerun is permitted by the scenario; no samples from this run may be discarded or substituted.

## Raw Observations

Each value is the median of eight process samples; ± is median absolute deviation (MAD), not a confidence interval. Mixed is one full sweep of 20 fixture Spans, not a single Span. Dense cases contain 32/64/128 legacy-style underscore HTTP header keys, including missing-context diagnostics.

| Case | Prior ns/op ± MAD | Candidate ns/op ± MAD | Ratio | B/op | allocs/op |
|---|---:|---:|---:|---:|---:|
| Unchanged8 | 391.30 ± 26.10 | 341.55 ± 6.55 | 0.873 | 544 → 544 | 12 → 12 |
| Unchanged32 | 1178.50 ± 41.00 | 1062.00 ± 46.50 | 0.901 | 2016 → 2016 | 36 → 36 |
| Unchanged64 | 2257.50 ± 189.00 | 2008.50 ± 83.50 | 0.890 | 3808 → 3808 | 68 → 68 |
| CanonicalHTTP8 | 842.50 ± 63.55 | 582.60 ± 29.00 | 0.692 | 544 → 544 | 12 → 12 |
| HTTP | 1906.00 ± 109.50 | 2007.00 ± 170.50 | 1.053 | 872 → 872 | 21 → 21 |
| DB | 2110.50 ± 251.50 | 1578.50 ± 124.50 | 0.748 | 536 → 536 | 12 → 12 |
| RPC | 2325.00 ± 317.50 | 1839.50 ± 212.50 | 0.791 | 608 → 608 | 17 → 17 |
| MissingContext | 1171.50 ± 191.20 | 870.90 ± 111.25 | 0.743 | 656 → 656 | 11 → 11 |
| Invalid | 1347.00 ± 208.00 | 868.55 ± 124.10 | 0.645 | 816 → 816 | 14 → 14 |
| Sparse1Of32 | 3774.50 ± 646.50 | 1389.00 ± 155.00 | 0.368 | 2016 → 2016 | 36 → 36 |
| Sparse2Of32 | 4167.00 ± 764.50 | 1600.50 ± 155.00 | 0.384 | 2016 → 2016 | 36 → 36 |
| Dense32 | 5008.00 ± 590.50 | 4435.00 ± 379.00 | 0.886 | 6960 → 6960 | 138 → 138 |
| Sparse1Of64 | 8554.50 ± 784.50 | 2450.50 ± 119.00 | 0.286 | 3808 → 3808 | 68 → 68 |
| Sparse2Of64 | 9066.50 ± 892.00 | 2901.50 ± 210.50 | 0.320 | 3808 → 3808 | 68 → 68 |
| Dense64 | 9399.00 ± 857.00 | 8694.50 ± 772.00 | 0.925 | 14384 → 14384 | 267 → 267 |
| Sparse1Of128 | 24696.00 ± 1477.50 | 5144.00 ± 763.00 | 0.208 | 7520 → 7520 | 132 → 132 |
| Sparse2Of128 | 24998.50 ± 1677.00 | 5568.50 ± 325.50 | 0.223 | 7520 → 7520 | 132 → 132 |
| Dense128 | 17877.50 ± 879.00 | 17723.00 ± 1614.00 | 0.991 | 29360 → 29360 | 524 → 524 |
| CanonicalDB8 | 1103.50 ± 69.50 | 719.60 ± 46.55 | 0.652 | 528 → 528 | 12 → 12 |
| CanonicalRPC8 | 1304.00 ± 85.50 | 846.25 ± 21.60 | 0.649 | 528 → 528 | 12 → 12 |
| Mixed | 123166.00 ± 6448.00 | 60500.00 ± 3793.50 | 0.491 | 88848 → 88848 | 1628 → 1628 |

## Interpretation

H1/H2/H3 median predictions are directionally consistent, but their acceptance interpretation remains inconclusive at the declared dispersion margins. The unchanged allocation prediction is supported in every fixture. Source snapshots show operation-level work bits, reuse of HTTP identity and RPC system, and at most two attribute decisions before bulk copy and output-owned value moves. Compiler diagnostics show hasHTTP/rpcSystem inlining; objdump retains the projection and copy paths. These corroborate executed work and ownership mechanics, not an exact CPU cost breakdown. The comparison changes dispatch, context reuse and sparse construction together; do not apportion speedup between them.

## Contradictions and Supersession

No differential correctness mismatch. Two preliminary exploratory trials are retained separately: trial-01 reserved fixed extra capacity and increased sparse B/op; it was rejected. Trial-02 computed capacity from decisions and removed that allocation-byte regression. Their single-sample timings are not formal evidence. Formal run history remains immutable; any noise rerun uses a separate Run with the same subject and harness.

## Boundaries and Extrapolation

Synthetic local hot-cache library microbenchmarks with independently owned output Maps. No production throughput, end-to-end Collector latency, p99, Linux, other Go/CPU versions or sanitizer-cost claim. Query fixtures use already-sanitized text. The pre-copy sparse path can copy values later suppressed; unchanged allocation observations are fixture-specific, not a guarantee for every possible input. Exact attributes/types/status/ordered issues and nested ownership are checked independently of performance.

## Handoff

Repository maintainers may use this bundle to inspect the implementation and reproduce BS-002. It cannot satisfy a passed performance gate; retain the inconclusive result and use only the scenario-authorized fresh rerun. Existing APIs and SemConv v1.41.0 output semantics are preserved. No deploy or Collector integration is implied.

## Artifacts

source-manifest.json and snapshot/** bind implementation, controls, tests, fixtures and runner/summary scripts. go-version.txt, go-env.txt, os.txt and cpu.txt identify environment; correctness.txt and build.txt prove setup; warmup.txt and repeat-*.txt contain raw observations; summary.json contains calculations; compiler.txt and objdump.txt contain optimized diagnostics; binary-sha256.txt identifies the deleted executable. exploratory/** retains initial trials and verification logs. No external artifacts or production inputs.
