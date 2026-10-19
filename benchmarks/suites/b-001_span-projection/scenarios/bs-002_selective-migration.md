---
schema_version: "1.1"
metadata_schema: "1"
artifact_type: benchmark-scenario
id: "BS-002"
suite_id: "B-001"
title: "Selective migration work and sparse map projection"
status: "active"
author: "Codex"
owner: "otel-span-classifier maintainers"
created: "2026-09-16"
updated: "2026-09-16"
---

# Selective migration work and sparse map projection

## Question and Hypothesis

H1: selective contextual work and reuse of HTTP/RPC facts reduce needless scans. CanonicalHTTP8 ratio <=0.80 and geometric mean of HTTP/DB/RPC/MissingContext/Invalid <=0.95 against the BR-001 candidate. H2: sparse projection at 32/64/128 attributes with one or two legacy fields avoids quadratic destination searches: each ratio <=0.75. H3: all other cases <=1.10, with no allocation-count increase; existing cases have no B/op increase, sparse cases allow <=15% more B/op for bounded destination headroom. A reliable threshold violation falsifies its prediction. Prior evidence is BR-001 only; no selective/sparse candidate timings have been observed. Exploratory trials before formal runs will be retained separately and disclosed. Claims apply to the combined change, not independent attribution.

## Subject, Control, and Variants

Control is the frozen BR-001 candidate, preserved as priorProjectSpan with frozen baseline protocol helpers. Candidate is current ProjectSpan. Both compile in one binary. The original pre-BR-001 baseline remains unchanged as an additional correctness oracle. Content digests and snapshots identify dirty worktree subject and harness. Public API, all values/types, ordered issues, status and independent nested ownership stay fixed.

## Variables

Same inputs, sinks, options, weave, compiler, module dependencies and single-thread process environment. Change only the combined dispatcher/context/output algorithm. Background workstation activity is uncontrolled; alternating process order and MAD expose drift.

## Dataset and Traffic Model

Existing 9 fixtures plus sparse 32/64/128-attribute spans (one http.method or two http.method/db.operation keys, remaining custom strings), dense legacy 32/64/128 attributes, canonical DB/RPC, and a fixed mixed sweep. Supplemental sparse nested values, current-target collisions and malformed cases validate correctness and ownership. Synthetic hot-cache single-thread inputs, not a production distribution.

## Environment and Isolation

Local Apple M4, darwin/arm64; record Go version and pinned module state. GOMAXPROCS=1 for child processes. No profiling/race flags in measured binary. No CPU frequency control or machine exclusivity claimed.

## Procedure and Commands

Run python3 benchmarks/suites/b-001_span-projection/scripts/run_selective.py <run>/artifacts. First run correctness tests, compile one binary, retain 100ms/case warmup. Alternate Prior then Selective and reverse for 8 paired independent process repetitions at 200ms/case. Benchmark names BenchmarkSelectiveProjectionPrior and BenchmarkSelectiveProjectionCandidate. Fixtures and global sinks keep work live. Collect optimized compiler diagnostics separately. Exploratory runs may select implementations but are not formal acceptance samples; save them with final evidence. The script snapshots subject/harness and computes content digests before timing.

## Metrics and Correctness Checks

Per-case median ns/op and MAD, candidate/control ratios, B/op and allocs/op. Geometric mean of five protocol/error cases. All raw samples retained; no exclusions. Differential corpus across all schema states, kinds, attribute orders, malformed values, aliases, target collisions, options; exact Issues ordering and StatusDescription/Complete. Mutation checks for nested maps/slices/bytes. Source and optimized compiler evidence corroborate path selection and ownership, without claiming exact CPU attribution.

## Decision Rule

Pass requires correctness, all H1/H2/H3 thresholds and 8 samples per variant. For timing gates require the absolute distance from the threshold to exceed 3 times the sum of relative MADs; otherwise inconclusive. Permit exactly one complete fresh run for noise, retain original. Failed runs stay recorded; implementation corrections use a new run under unchanged protocol. Stop at first passing formal run. Do not relax thresholds after observations.

## Evidence Requirements

Immutable source/harness snapshots and SHA-256 identities; full raw repetition and warmup output; correctness/build/Go/OS logs; summary generator and summary; optimized compiler output; exploratory results including rejected alternatives. Final result separates observations, interpretation and limitations.

## Safety, Cleanup, and Recovery

Only local synthetic spans. Stop on correctness/build failure. No network services or production data. Temporary test binaries may be removed after hashing; snapshots retain reproducibility. Preserve all previously sealed runs.

## Boundaries and Extrapolation

Library microbenchmarks only. No zero-allocation, Collector throughput, production p99, sanitizer cost, Linux or alternate-Go performance claim. No new streaming API, pool, unsafe access, cross-span cache or project-specific semantic fallback.
