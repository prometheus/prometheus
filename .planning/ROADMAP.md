# Roadmap: Histogram ST WAL Records

**Created:** 2026-03-02
**Phases:** 4
**Scope:** tsdb/record/ only (record layer)

## Phase 1: Struct and Type Definitions

**Goal:** All types and structs exist so encoding/decoding can reference them.

**Requirements:** STRUCT-01, STRUCT-02, TYPE-01..05

**Plans:** 1/1 plans complete

Plans:
- [x] 01-01-PLAN.md -- Add ST fields to histogram structs and declare V2 record type constants

**Files:** `tsdb/record/record.go`

**Verification:** Code compiles. `go vet ./tsdb/record/...` passes.

---

## Phase 2: V2 Encoders

**Goal:** Encoder can produce V2 histogram records with ST when EnableSTStorage is true.

**Requirements:** ENC-01..05

**Plans:** 2/2 plans complete

Plans:
- [x] 02-01-PLAN.md -- Int-histogram V2 encoders (HistogramSamples + CustomBucketsHistogramSamples dispatch and V2 methods)
- [x] 02-02-PLAN.md -- Float-histogram V2 encoders (FloatHistogramSamples + CustomBucketsFloatHistogramSamples dispatch and V2 methods)

**Files:** `tsdb/record/record.go`

**Verification:** Code compiles. Encoder produces valid V2 byte sequences.

---

## Phase 3: V2 Decoders

**Goal:** Decoder reads both V1 and V2 histogram records correctly.

**Requirements:** DEC-01..04

**Plans:** 2/2 plans complete

Plans:
- [x] 03-01-PLAN.md -- Int-histogram V2 decoder (histogramSamplesV2 + HistogramSamples dispatch)
- [x] 03-02-PLAN.md -- Float-histogram V2 decoder (floatHistogramSamplesV2 + FloatHistogramSamples dispatch)

**Files:** `tsdb/record/record.go`

**Verification:** Code compiles. Decoder correctly round-trips V2 records.

---

## Phase 4: Tests

**Goal:** Full test coverage for V2 histogram encoding, backward compat verified.

**Requirements:** TEST-01..07

**Plans:** 2 plans

Plans:
- [ ] 04-01-PLAN.md -- V2 histogram round-trip tests (all ST scenarios, custom-bucket, gauge, backward compat)
- [ ] 04-02-PLAN.md -- V2 histogram type recognition assertions in TestRecord_Type

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
*Last updated: 2026-03-03 after Phase 4 planning complete*
