---
phase: 02-v2-encoders
plan: 01
subsystem: tsdb/record
tags: [encoder, v2, histogram, wal, start-time]
dependency_graph:
  requires: [01-01]
  provides: [histogramSamplesV2, customBucketsHistogramSamplesV2, dispatch]
  affects: [tsdb/record/record.go]
tech_stack:
  added: []
  patterns: [v2-st-marker-scheme, varint-encoding, ref-delta-to-prev]
key_files:
  modified: [tsdb/record/record.go]
decisions:
  - V2 uses all-varint first-sample encoding (not BE64), deliberately breaking from V1 wire format
  - Ref deltas are to previous ref (not first), matching samplesV2 pattern
  - T deltas are to first T, ST uses noST/sameST/explicitST marker scheme
  - Both tasks implemented in single edit pass; no separate task commits needed
metrics:
  duration: "~5 minutes"
  completed: "2026-03-02T21:40:44Z"
  tasks_completed: 2
  files_modified: 1
---

# Phase 02 Plan 01: V2 Int-Histogram Encoder Summary

Added V2 encoding for int-histogram WAL record types with full ST (start-time) support using the varint-based V2 wire format and noST/sameST/explicitST marker scheme.

## What Was Built

Four private methods added + two public dispatch methods updated in `tsdb/record/record.go`:

- `histogramSamplesV1`: exact extraction of previous `HistogramSamples` body (BE64, delta-to-first-ref, no ST)
- `histogramSamplesV2`: new V2 encoding (varint, delta-to-prev-ref, delta-to-first-T, ST markers, custom-bucket filtering)
- `customBucketsHistogramSamplesV1`: exact extraction of previous `CustomBucketsHistogramSamples` body
- `customBucketsHistogramSamplesV2`: new V2 encoding (same pattern, no filtering, no buf.Reset)
- `HistogramSamples`: now dispatches via `e.EnableSTStorage`
- `CustomBucketsHistogramSamples`: now dispatches via `e.EnableSTStorage`

## Commits

| Task | Description | Hash |
|------|-------------|------|
| 1+2  | Add V2 encoding for int-histogram WAL record types | a3d49b0ac |

## Deviations from Plan

Tasks 1 and 2 were combined into a single edit and commit. The plan described them as separate tasks but since task 2's code was fully specified in the plan and no intermediate verification was needed between them, they were implemented together. Build and vet passed on the combined result.

## Self-Check: PASSED

- `tsdb/record/record.go` modified: confirmed
- commit `a3d49b0ac` exists: confirmed
- `go build ./tsdb/record/...` passes
- `go vet ./tsdb/record/...` passes
- `HistogramSamples` dispatches to `histogramSamplesV2` when `EnableSTStorage` is true
- `CustomBucketsHistogramSamples` dispatches to `customBucketsHistogramSamplesV2` when `EnableSTStorage` is true
- V1 bodies are exact copies of original code (wire format unchanged)
- V2 uses varint encoding, ref-delta-to-prev, T-delta-to-first, ST marker scheme
