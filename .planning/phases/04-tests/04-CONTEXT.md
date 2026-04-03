# Phase 4: Tests - Context

**Gathered:** 2026-03-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Full test coverage for V2 histogram encoding/decoding. Round-trip tests for all ST scenarios, backward compat verified, Type() recognition for new types. All in `tsdb/record/record_test.go`.

</domain>

<decisions>
## Implementation Decisions

### Test organization
- Add V2 histogram round-trip tests as new sections within the existing `TestRecord_EncodeDecode` function, matching how V2 float samples were added inline (lines 91-134)
- `TestRecord_Type` gets new assertions for V2 histogram types (extend existing function, don't create new one)
- Schema validation and corrupted record tests already loop over `enableSTStorage: true/false` so they exercise V2 paths. No changes needed there.

### ST scenario coverage
- Mirror all 4 float sample ST patterns for histograms (no ST, constant ST, varying/delta ST, same-ST-across-samples)
- Each scenario tests both int-histogram and float-histogram V2
- Each scenario includes custom-bucket variants
- Encoder uses `EnableSTStorage: true` for all V2 tests

### Backward compatibility
- Encode histograms with V1 encoder (default, no EnableSTStorage), decode, verify ST=0 on all decoded samples
- This confirms V1 records remain readable and the zero-value ST is backward-compatible

### TestRecord_Type additions
- Encode histograms with `EnableSTStorage: true`, verify `dec.Type()` returns `HistogramSamplesV2` and `CustomBucketsHistogramSamplesV2`
- Same for float histogram V2 types
- Uses the existing `histograms` test data slice already defined in `TestRecord_Type`

### Claude's Discretion
- Exact histogram test data values (can reuse existing `histograms` slice from line 166)
- Whether to use subtests (`t.Run`) for each ST scenario within the monolithic test
- Exact ST values in test data

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `histograms` slice at line 166: 3 samples (standard schema=1, negative buckets, custom buckets schema=-53). Already filters into standard + custom-bucket groups.
- `floatHistograms` construction at line 231: converts from int histograms via `h.H.ToFloat(nil)`. Same pattern for V2 float tests.
- Float sample ST test patterns at lines 79-134: direct template for histogram ST scenarios.
- `enc.HistogramSamples()` returns `(histSamples, customBucketsHistograms)` pattern already used at line 222.

### Established Patterns
- Round-trip: `enc.HistogramSamples(input, nil)` -> `dec.HistogramSamples(encoded, nil)` -> `require.Equal(t, input, decoded)`
- Custom buckets: filter return from `HistogramSamples()`, encode separately with `CustomBucketsHistogramSamples()`, decode, append, compare
- Gauge variant: mutate `CounterResetHint` on same data, re-encode, round-trip again (lines 248-274)
- `Encoder{EnableSTStorage: true}` creates V2 encoder (line 91)

### Integration Points
- `TestRecord_Type` at line 536: add V2 type assertions after existing V1 histogram assertions (line 605)
- `TestRecord_EncodeDecode` at line 275: add V2 histogram sections before the closing brace

</code_context>

<specifics>
## Specific Ideas

- The existing V1 histogram test data includes a custom-bucket histogram (schema=-53 with CustomValues). When testing V2, the same data naturally exercises the custom-bucket filtering and separate encoding path.
- The gauge variant re-test (lines 248-274) should also be done for V2, since gauge histograms encode differently (no counter reset hint assumptions).

</specifics>

<deferred>
## Deferred Ideas

None. Discussion stayed within phase scope.

</deferred>

---

*Phase: 04-tests*
*Context gathered: 2026-03-02*
