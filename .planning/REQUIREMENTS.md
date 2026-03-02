# Requirements: Histogram ST WAL Records

**Defined:** 2026-03-02
**Core Value:** Histogram samples carry accurate start time through the WAL

## v1 Requirements

### Struct Changes

- [x] **STRUCT-01**: RefHistogramSample has ST field (int64) alongside T
- [x] **STRUCT-02**: RefFloatHistogramSample has ST field (int64) alongside T

### Record Types

- [x] **TYPE-01**: HistogramSamplesV2 record type constant added
- [x] **TYPE-02**: FloatHistogramSamplesV2 record type constant added
- [x] **TYPE-03**: CustomBucketsHistogramSamplesV2 record type constant added
- [x] **TYPE-04**: CustomBucketsFloatHistogramSamplesV2 record type constant added
- [x] **TYPE-05**: Type.String() returns correct names for new types

### Encoding

- [ ] **ENC-01**: Encoder.HistogramSamples() gates on EnableSTStorage, dispatches to V2 when enabled
- [ ] **ENC-02**: Encoder.FloatHistogramSamples() gates on EnableSTStorage, dispatches to V2 when enabled
- [ ] **ENC-03**: Encoder.CustomBucketsHistogramSamples() gates on EnableSTStorage, dispatches to V2 when enabled
- [ ] **ENC-04**: Encoder.CustomBucketsFloatHistogramSamples() gates on EnableSTStorage, dispatches to V2 when enabled
- [ ] **ENC-05**: V2 histogram encoding uses noST/sameST/explicitST marker scheme

### Decoding

- [ ] **DEC-01**: Decoder.HistogramSamples() accepts both V1 and V2 record types
- [ ] **DEC-02**: Decoder.FloatHistogramSamples() accepts both V1 and V2 record types
- [ ] **DEC-03**: V2 histogram decoding correctly reads ST marker bytes and reconstructs ST values
- [ ] **DEC-04**: V1 records decoded with ST=0 (backward compat)

### Testing

- [ ] **TEST-01**: Round-trip encode/decode for histogram V2 with no ST
- [ ] **TEST-02**: Round-trip encode/decode for histogram V2 with constant ST
- [ ] **TEST-03**: Round-trip encode/decode for histogram V2 with varying ST
- [ ] **TEST-04**: Round-trip encode/decode for float histogram V2 (same ST scenarios)
- [ ] **TEST-05**: Round-trip encode/decode for custom buckets variants V2
- [ ] **TEST-06**: V1 records still decode correctly (backward compat test)
- [ ] **TEST-07**: Type() correctly identifies new record types

## Out of Scope

| Feature | Reason |
|---------|--------|
| WAL replay changes | Record layer only per scope decision |
| Remote write integration | Separate follow-up |
| Head append path | Separate follow-up |
| Exemplar ST support | Not requested |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| STRUCT-01 | Phase 1 | Complete |
| STRUCT-02 | Phase 1 | Complete |
| TYPE-01 | Phase 1 | Complete |
| TYPE-02 | Phase 1 | Complete |
| TYPE-03 | Phase 1 | Complete |
| TYPE-04 | Phase 1 | Complete |
| TYPE-05 | Phase 1 | Complete |
| ENC-01 | Phase 2 | Pending |
| ENC-02 | Phase 2 | Pending |
| ENC-03 | Phase 2 | Pending |
| ENC-04 | Phase 2 | Pending |
| ENC-05 | Phase 2 | Pending |
| DEC-01 | Phase 3 | Pending |
| DEC-02 | Phase 3 | Pending |
| DEC-03 | Phase 3 | Pending |
| DEC-04 | Phase 3 | Pending |
| TEST-01 | Phase 4 | Pending |
| TEST-02 | Phase 4 | Pending |
| TEST-03 | Phase 4 | Pending |
| TEST-04 | Phase 4 | Pending |
| TEST-05 | Phase 4 | Pending |
| TEST-06 | Phase 4 | Pending |
| TEST-07 | Phase 4 | Pending |

**Coverage:**
- v1 requirements: 23 total
- Mapped to phases: 23
- Unmapped: 0

---
*Requirements defined: 2026-03-02*
*Last updated: 2026-03-02 after initial definition*
