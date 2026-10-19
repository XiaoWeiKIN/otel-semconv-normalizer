---
schema_version: "1.1"
metadata_schema: "1"
artifact_type: benchmark-suite
id: "B-001"
title: "Span attribute projection performance"
status: "active"
author: "Codex"
owner: "otel-span-classifier maintainers"
created: "2026-09-16"
updated: "2026-09-16"
---

# Span attribute projection performance

## Purpose and Scope

Measure complete read-only Span attribute projection against a frozen same-binary control, including output materialization and issue reporting.

## Subject Under Test

semconvweave.WeaveID.ProjectSpan with pinned pdata; snapshot code and test harness are hashed in each Run.

## Consumers

Repository maintainers and future local regression checks; no service capacity claim.

## Ownership and Lifecycle

otel-span-classifier maintainers own the projection boundary. Retain sealed runs; retire when the API is removed.

## Non-goals

Production throughput, p99, SQL sanitizer implementations, Collector transport and storage are excluded.

## Safety and Data Policy

Synthetic data only; local CPU measurements, no external writes. Build binaries and profiles live under the run artifacts.
