---
phase: 02-v2-encoders
plan: 02
subsystem: tsdb/record
tags: [encoder, v2, float-histogram, wal, start-time]
dependency_graph:
  requires: [02-01]
  provides: [floatHistogramSamplesV2, customBucketsFloatHistogramSamplesV2, dispatch]
  affects: [tsdb/record/record.go]
tech_stack:
  added: []
  patterns: [v2-st-marker-scheme, varint-encoding, ref-delta-to-prev]
key_files:
  modified: [tsdb/record/record.go]
decisions:
  - V2 float-histogram encoder uses all-varint first-sample (not BE64), matching Plan 01 int-histogram V2 pattern
  - Ref deltas to previous ref, T deltas to first T, ST uses noST/sameST/explicitST marker scheme
  - Tasks 1 and 2 combined into single edit and commit (V2 code fully specified, no intermediate verification needed)
metrics:
  duration: "~5 minutes"
  completed: "2026-03-02"
  tasks_completed: 2
  files_modified: 1
---

# Phase 02 Plan 02: V2 Float-Histogram Encoder Summary

Added V2 encoding for float-histogram WAL record types with full ST (start-time) support using varint-based V2 wire format and noST/sameST/explicitST marker scheme. All four histogram encoder methods now have V1/V2 dispatch.

## What Was Built

Four private methods added + two public dispatch methods updated in `tsdb/record/record.go`:

- `floatHistogramSamplesV1`: exact extraction of previous `FloatHistogramSamples` body (BE64, delta-to-first-ref, no ST)
- `floatHistogramSamplesV2`: new V2 encoding (varint, delta-to-prev-ref, delta-to-first-T, ST markers, custom-bucket filtering)
- `customBucketsFloatHistogramSamplesV1`: exact extraction of previous `CustomBucketsFloatHistogramSamples` body
- `customBucketsFloatHistogramSamplesV2`: new V2 encoding (same pattern, no filtering, no buf.Reset)
- `FloatHistogramSamples`: now dispatches via `e.EnableSTStorage`
- `CustomBucketsFloatHistogramSamples`: now dispatches via `e.EnableSTStorage`

## Commits

| Task | Description | Hash |
|------|-------------|------|
| 1+2  | Add V2 encoding for float-histogram WAL record types | 116b10e2a |

## Deviations from Plan

Tasks 1 and 2 were combined into a single edit and commit. The V2 code was fully specified in the plan and no intermediate verification was needed between them. Build and vet passed on the combined result.

## Self-Check: PASSED

- `tsdb/record/record.go` modified: confirmed
- commit `116b10e2a` exists: confirmed
- `go build ./tsdb/record/...` passes
- `go vet ./tsdb/record/...` passes
- `FloatHistogramSamples` dispatches to `floatHistogramSamplesV2` when `EnableSTStorage` is true
- `CustomBucketsFloatHistogramSamples` dispatches to `customBucketsFloatHistogramSamplesV2` when `EnableSTStorage` is true
- V1 bodies are exact copies of original code (wire format unchanged)
- V2 uses varint encoding, ref-delta-to-prev, T-delta-to-first, ST marker scheme
