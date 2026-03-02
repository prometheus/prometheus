# Roadmap: Histogram ST WAL Records

**Created:** 2026-03-02
**Phases:** 4
**Scope:** tsdb/record/ only (record layer)

## Phase 1: Struct and Type Definitions

**Goal:** All types and structs exist so encoding/decoding can reference them.

**Requirements:** STRUCT-01, STRUCT-02, TYPE-01..05

**Plans:** 1 plan

Plans:
- [ ] 01-01-PLAN.md -- Add ST fields to histogram structs and declare V2 record type constants

**Files:** `tsdb/record/record.go`

**Verification:** Code compiles. `go vet ./tsdb/record/...` passes.

---

## Phase 2: V2 Encoders

**Goal:** Encoder can produce V2 histogram records with ST when EnableSTStorage is true.

**Requirements:** ENC-01..05

**Tasks:**
1. Add `histogramSamplesV2` private method (mirrors `samplesV2` pattern): first sample writes ref+T+ST+histogram, subsequent samples write dRef+dT+STmarker+[dST]+histogram
2. Add `floatHistogramSamplesV2` private method (same pattern for float histograms)
3. Add `customBucketsHistogramSamplesV2` private method
4. Add `customBucketsFloatHistogramSamplesV2` private method
5. Update `HistogramSamples()` to gate on `EnableSTStorage`, dispatch to V2
6. Update `FloatHistogramSamples()` to gate on `EnableSTStorage`, dispatch to V2
7. Update `CustomBucketsHistogramSamples()` to gate on `EnableSTStorage`, dispatch to V2
8. Update `CustomBucketsFloatHistogramSamples()` to gate on `EnableSTStorage`, dispatch to V2

**Files:** `tsdb/record/record.go`

**Verification:** Code compiles. Encoder produces valid V2 byte sequences.

---

## Phase 3: V2 Decoders

**Goal:** Decoder reads both V1 and V2 histogram records correctly.

**Requirements:** DEC-01..04

**Tasks:**
1. Add `histogramSamplesV2` private decoder method: reads first sample ref+T+ST+histogram, subsequent samples read dRef+dT+STmarker+[dST]+histogram
2. Add `floatHistogramSamplesV2` private decoder method
3. Update `Decoder.HistogramSamples()` to switch on record type: V1 types use existing logic, V2 types use new method
4. Update `Decoder.FloatHistogramSamples()` similarly
5. V1 path unchanged (ST defaults to zero value)

**Files:** `tsdb/record/record.go`

**Verification:** Code compiles. Decoder correctly round-trips V2 records.

---

## Phase 4: Tests

**Goal:** Full test coverage for V2 histogram encoding, backward compat verified.

**Requirements:** TEST-01..07

**Tasks:**
1. Add histogram V2 test cases to existing test structure (no ST, constant ST, varying ST, chained ST)
2. Add float histogram V2 test cases (same ST scenarios)
3. Add custom buckets V2 test cases (both int and float)
4. Add backward compat test: V1-encoded records still decode with ST=0
5. Add Type() recognition test for new record types
6. Run full test suite: `go test ./tsdb/record/...`

**Files:** `tsdb/record/record_test.go`

**Verification:** `go test ./tsdb/record/... -count=1` passes. All new test cases green.

---

## Phase Dependencies

```
Phase 1 (types/structs)
  └─> Phase 2 (encoders)
  └─> Phase 3 (decoders)  [depends on Phase 2 for round-trip verification]
        └─> Phase 4 (tests) [depends on Phase 2 + Phase 3]
```

---
*Created: 2026-03-02*
*Last updated: 2026-03-02 after Phase 1 planning*
