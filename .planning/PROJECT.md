# Add Start Time (ST) to Histogram WAL Records

## What This Is

Extend the Prometheus TSDB WAL record encoding layer to support per-sample start time (ST) for histogram types. This mirrors the existing ST support already implemented for float samples (RefSample / SamplesV2).

## Core Value

Histogram samples carry accurate start time metadata through the WAL, enabling correct staleness and reset detection downstream.

## Requirements

### Validated

- RefSample already has ST field with V1/V2 encoding (SamplesV2, type 11)

### Active

- [ ] Add ST field to RefHistogramSample struct
- [ ] Add ST field to RefFloatHistogramSample struct
- [ ] New V2 record types for all four histogram variants
- [ ] V2 encoder/decoder using same ST marker scheme as samplesV2
- [ ] Backward-compatible: V1 decoder still works, EnableSTStorage gates V2

### Out of Scope

- WAL replay changes. Record layer only.
- Remote write integration. Separate follow-up.
- Head append path changes. Separate follow-up.
- Any non-record-layer changes.

## Context

- `tsdb/record/record.go` is the primary file
- Existing pattern: `Encoder.Samples()` checks `EnableSTStorage` to pick V1 vs V2
- V2 encoding uses marker bytes: `noST` (0), `sameST` (1), `explicitST` (2)
- Four histogram encoder functions exist: `HistogramSamples`, `CustomBucketsHistogramSamples`, `FloatHistogramSamples`, `CustomBucketsFloatHistogramSamples`
- Each has a corresponding decoder in `Decoder.HistogramSamples` and `Decoder.FloatHistogramSamples`
- Current histogram encoding: `[type(1)] [baseRef(8)] [baseTime(8)] [dRef(varint) dTime(varint) histogramPayload]...`

## Constraints

- **Backward compat**: Old decoders must still read V1 records. New decoders must read both V1 and V2.
- **Gating**: V2 encoding gated behind `EnableSTStorage` flag, same as float samples.
- **Pattern consistency**: Must follow the exact same ST marker scheme used in samplesV2.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| New record types (V2) rather than in-place changes | Mirrors Samples/SamplesV2 pattern, clean backward compat | -- Pending |
| All four histogram variants get V2 | Consistent coverage, avoids partial support | -- Pending |
| Both RefHistogramSample and RefFloatHistogramSample get ST field | All four record types need the struct field to encode ST | -- Pending |

---
*Last updated: 2026-03-02 after initial project setup*
