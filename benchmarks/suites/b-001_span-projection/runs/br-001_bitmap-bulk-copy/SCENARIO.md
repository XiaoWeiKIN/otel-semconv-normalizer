---
schema_version: "1.1"
metadata_schema: "1"
artifact_type: benchmark-scenario
id: "BS-001"
suite_id: "B-001"
title: "Bitmap dispatch and bulk copy against frozen projection"
status: "active"
author: "Codex"
owner: "otel-span-classifier maintainers"
created: "2026-09-16"
updated: "2026-09-16"
---

> Public redacted copy of historical evidence. Measurements and outcome are unchanged. Original digests identify the pre-redaction source; verify these distributed bytes with the [public distribution verifier](../../../../verify_publication.py). This is not a new measurement or resealed Run.


# Bitmap dispatch and bulk copy against frozen projection

## Question and Hypothesis

H1: combining the existing bitmap negative filter, protocol work mask, and bulk Map copy removes unnecessary per-key projection and unrelated protocol scans. Prediction: median candidate/control <=0.70 on unchanged 8/32/64 attribute cases, with fewer allocations. H2: equivalent slow/mixed output costs do not regress: each ratio <=1.10 and slow geometric mean <=1.0; bytes and allocations do not increase. Violating these thresholds falsifies the respective prediction. Prior exploratory same-worktree data (5 repetitions, uncontrolled Go scheduling) showed 1.07/3.97/9.07 us for unchanged 8/32/64 attributes; these observations motivated this fresh comparison, not acceptance evidence.

## Subject, Control, and Variants

Freeze ProjectSpan, its semantic/context helpers and protocol handlers in test-only prefixed functions before editing. Baseline and Candidate compile in one binary. Content SHA-256 manifests bind the dirty-worktree implementation and harness; no git commit is implied. Keep ABI, output map ownership, values/types, status and issue ordering unchanged.

## Variables

Controlled: identical spans/options, output lifetime/sinks, toolchain, GOMAXPROCS=1, dependency versions and binary. Intentionally changed: combined algorithm (bulk copy and dispatch); do not attribute timing solely to bitmap. Responses: ns/op, B/op, allocs/op; confounders: workstation background load and thermal drift.

## Dataset and Traffic Model

Synthetic unchanged custom string attributes at 8/32/64 and canonical standard keys at 8; legacy HTTP URL redaction, DB namespace, RPC methods, missing-context and malformed cases; one fixed equal-weight mixed operation. No production representativeness claimed. Single-thread hot-cache workload, no network.

## Environment and Isolation

Local Apple M4 darwin/arm64, Go toolchain recorded by go version; pinned module dependencies. GOMAXPROCS=1 and serial runs. No race/profiling flags for timed comparisons. Background activity is uncontrolled and handled through paired repetition and dispersion, not selective exclusions.

## Procedure and Commands

Before measurements run go test ./semconvweave (including deterministic differential corpus and ownership checks). Compile go test -c ./semconvweave to the Run artifacts. One unreported warmup per variant retained in warmup.txt. Execute 8 process repetitions at 200ms per benchmark, alternating Baseline-then-Candidate and Candidate-then-Baseline; patterns ^BenchmarkProjectSpanBaseline$ and ^BenchmarkProjectSpanCandidate$. Each process repetition contributes one sample per case. Use -test.run=^$ -test.benchmem -test.benchtime=200ms. Save all stdout/stderr. Global sinks and result comparisons prevent dead-code/empty-work measurements. Capture optimized compiler escape output and optional CPU profile separately. Restore no runtime settings outside the child environment.

## Metrics and Correctness Checks

Per-case median and MAD, candidate/control ratios, allocs/op and B/op; geometric mean for slow cases. Differential tests compare complete attributes/status/issues/Complete and source preservation for all schema states and kinds. Check nested ownership by mutation. No exclusions; invalid fixture/setup runs are retained and marked errored. Compile diagnostics and source snapshots corroborate the paths; do not infer causality from timing alone.

## Decision Rule

Pass iff all correctness tests pass; unchanged cases meet H1; every other case ratio <=1.10 and their geometric mean <=1.0; no case increases bytes or allocations. At least 8 samples per variant. If a threshold is within 3 times the sum of relative MADs, report inconclusive and allow exactly one complete 8-repeat rerun as a new Run; never discard samples. Stop when gates pass. Failed runs remain sealed; a substantive design correction uses a new Run under unchanged protocol.

## Evidence Requirements

Sealed artifacts: source/harness SHA-256 manifest and snapshots, Go/OS/CPU environment, exact runner script, raw outputs, per-case summary, correctness logs, compiler diagnostics, optional profiles and analysis. All numerical claims link to raw outputs. Record rejected/failed experiments without editing protocol.

## Safety, Cleanup, and Recovery

Local synthetic-only execution. Stop on correctness failures or build errors; retain logs. No services, production data, credentials or external publication. Temporary binaries can be excluded from the sealed bundle after saving hashes and tool outputs; source snapshot allows reproduction.

## Boundaries and Extrapolation

This is microbenchmark evidence for current library operations, including a fresh independently owned destination Map. It does not prove zero-copy operation, Collector throughput, p99, production memory pressure, other Go/CPU versions, or query-sanitizer performance.
