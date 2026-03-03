# Phase 4: Tests - Research

**Researched:** 2026-03-03
**Domain:** Go test patterns for WAL record encode/decode round-trips
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Test organization**
- Add V2 histogram round-trip tests as new sections within the existing `TestRecord_EncodeDecode` function, matching how V2 float samples were added inline (lines 91-134).
- `TestRecord_Type` gets new assertions for V2 histogram types (extend existing function, do not create a new one).
- Schema validation and corrupted record tests already loop over `enableSTStorage: true/false` so they exercise V2 paths. No changes needed there.

**ST scenario coverage**
- Mirror all 4 float sample ST patterns for histograms (no ST, constant ST, varying/delta ST, same-ST-across-samples).
- Each scenario tests both int-histogram and float-histogram V2.
- Each scenario includes custom-bucket variants.
- Encoder uses `EnableSTStorage: true` for all V2 tests.

**Backward compatibility**
- Encode histograms with V1 encoder (default, no EnableSTStorage), decode, verify ST=0 on all decoded samples.
- This confirms V1 records remain readable and the zero-value ST is backward-compatible.

**TestRecord_Type additions**
- Encode histograms with `EnableSTStorage: true`, verify `dec.Type()` returns `HistogramSamplesV2` and `CustomBucketsHistogramSamplesV2`.
- Same for float histogram V2 types.
- Uses the existing `histograms` test data slice already defined in `TestRecord_Type`.

### Claude's Discretion
- Exact histogram test data values (can reuse existing `histograms` slice from line 166).
- Whether to use subtests (`t.Run`) for each ST scenario within the monolithic test.
- Exact ST values in test data.

### Deferred Ideas (OUT OF SCOPE)
None. Discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| TEST-01 | Round-trip encode/decode for histogram V2 with no ST | Use `histograms` slice (line 166), V2 encoder (EnableSTStorage: true), all ST fields zero; decode and require.Equal |
| TEST-02 | Round-trip encode/decode for histogram V2 with constant ST | Same data with identical non-zero ST on all samples; sameST marker path |
| TEST-03 | Round-trip encode/decode for histogram V2 with varying ST | Different non-zero ST per sample; explicitST marker path |
| TEST-04 | Round-trip encode/decode for float histogram V2 (same ST scenarios) | Derive floatHistograms from histograms via h.H.ToFloat(nil); same 4 scenarios |
| TEST-05 | Round-trip encode/decode for custom buckets variants V2 | Third histogram in slice (schema=-53, CustomValues set) naturally splits into custom-buckets path; encode with CustomBucketsHistogramSamplesV2 / CustomBucketsFloatHistogramSamplesV2 |
| TEST-06 | V1 records still decode correctly (backward compat test) | V1 encoder (zero-value Encoder{}), encode histograms, decode, assert ST==0 on all results |
| TEST-07 | Type() correctly identifies new record types | enc with EnableSTStorage:true; assert dec.Type returns HistogramSamplesV2, FloatHistogramSamplesV2, CustomBucketsHistogramSamplesV2, CustomBucketsFloatHistogramSamplesV2 |
</phase_requirements>

## Summary

This phase is entirely within `tsdb/record/record_test.go`. The implementation work (encoders and decoders for all four V2 histogram record types) is complete in Phases 1-3. Phase 4 only adds tests — no production code changes.

The test patterns are already fully established in the file. The V2 float-sample tests (lines 91-134) are the direct template for V2 histogram tests. The existing V1 histogram round-trip block (lines 166-274) is the source of reusable test data and shows exactly how custom-bucket splitting works.

There are no unknown APIs, no new dependencies, and no architectural decisions left. Every encoder and decoder method being tested already exists and compiles.

**Primary recommendation:** Extend `TestRecord_EncodeDecode` with four ST-scenario blocks (no-ST, constant-ST, delta-ST, same-ST) for int-histograms + float-histograms + custom-buckets, then extend `TestRecord_Type` with V2 type assertions. Treat the existing V2 sample tests as the line-by-line model.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `testing` | stdlib | Go test framework | Required |
| `github.com/stretchr/testify/require` | already imported | Assertion helpers | Already used throughout the file |

No new imports are needed. The test file already imports everything required: `testing`, `require`, `histogram`, `labels`, `promslog`, `testutil`, `rand`.

### Supporting

None. This is a pure test extension of an existing file.

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Inline sections in `TestRecord_EncodeDecode` | Separate top-level test functions | Locked decision: inline, matching existing V2 sample pattern |
| `t.Run` subtests for ST scenarios | Flat inline blocks | Claude's discretion — subtests improve failure isolation, planner should decide |

## Architecture Patterns

### Pattern 1: V2 Histogram Round-Trip (noST case)

**What:** Encode with V2 encoder, decode, assert equality.
**When to use:** Every scenario block.

```go
// Source: record_test.go lines 91-102 (V2 sample pattern)
enc = Encoder{EnableSTStorage: true}
histogramsNoST := []RefHistogramSample{
    {Ref: 56, T: 1234, H: histograms[0].H},
    {Ref: 42, T: 5678, H: histograms[1].H},
}
histSamples, customBucketsHistograms := enc.HistogramSamples(histogramsNoST, nil)
require.Equal(t, HistogramSamplesV2, dec.Type(histSamples))
decHistograms, err := dec.HistogramSamples(histSamples, nil)
require.NoError(t, err)
require.Equal(t, histogramsNoST, decHistograms)
```

### Pattern 2: Custom Buckets Split-and-Encode

**What:** `HistogramSamples()` returns `(mainRecord, customBucketsSlice)`. Custom-bucket samples go through a separate encoder call.
**When to use:** Every block that includes the custom-bucket histogram (schema=-53).

```go
// Source: record_test.go lines 222-229
histSamples, customBucketsHistograms := enc.HistogramSamples(histograms, nil)
customBucketsHistSamples := enc.CustomBucketsHistogramSamples(customBucketsHistograms, nil)
decHistograms, err := dec.HistogramSamples(histSamples, nil)
require.NoError(t, err)
decCustomBuckets, err := dec.HistogramSamples(customBucketsHistSamples, nil)
require.NoError(t, err)
decHistograms = append(decHistograms, decCustomBuckets...)
require.Equal(t, histograms, decHistograms)
```

### Pattern 3: Float Histogram Derivation

**What:** Derive float histograms from int histograms via `h.H.ToFloat(nil)`.
**When to use:** Float histogram V2 tests (TEST-04).

```go
// Source: record_test.go lines 231-238
floatHistograms := make([]RefFloatHistogramSample, len(histogramsWithST))
for i, h := range histogramsWithST {
    floatHistograms[i] = RefFloatHistogramSample{
        Ref: h.Ref,
        T:   h.T,
        ST:  h.ST,
        FH:  h.H.ToFloat(nil),
    }
}
```

### Pattern 4: Backward Compat Assertion

**What:** Encode with V1 encoder (zero-value `Encoder{}`), decode, verify `ST == 0` on every result.

```go
// V1 encoder: no EnableSTStorage
encV1 := Encoder{}
histSamples, customBucketsHistograms := encV1.HistogramSamples(histograms, nil)
customBucketsHistSamples := encV1.CustomBucketsHistogramSamples(customBucketsHistograms, nil)
decHistograms, err := dec.HistogramSamples(histSamples, nil)
require.NoError(t, err)
decCustomBuckets, err := dec.HistogramSamples(customBucketsHistSamples, nil)
require.NoError(t, err)
all := append(decHistograms, decCustomBuckets...)
for _, h := range all {
    require.Equal(t, int64(0), h.ST, "V1 records must decode with ST=0")
}
```

### Pattern 5: TestRecord_Type V2 Assertions

**What:** After the existing V1 histogram type assertions (line 605), add V2 type checks.

```go
// V2 type assertions — append after existing histogram type block
enc = Encoder{EnableSTStorage: true}
hists, customBucketsHistograms := enc.HistogramSamples(histograms, nil)
recordType = dec.Type(hists)
require.Equal(t, HistogramSamplesV2, recordType)
customBucketsHists := enc.CustomBucketsHistogramSamples(customBucketsHistograms, nil)
recordType = dec.Type(customBucketsHists)
require.Equal(t, CustomBucketsHistogramSamplesV2, recordType)

// Float histogram V2 types
floatHistogramsLocal := make([]RefFloatHistogramSample, len(histograms))
for i, h := range histograms {
    floatHistogramsLocal[i] = RefFloatHistogramSample{Ref: h.Ref, T: h.T, FH: h.H.ToFloat(nil)}
}
floatHists, customBucketsFloatHistograms := enc.FloatHistogramSamples(floatHistogramsLocal, nil)
recordType = dec.Type(floatHists)
require.Equal(t, FloatHistogramSamplesV2, recordType)
customBucketsFloatHists := enc.CustomBucketsFloatHistogramSamples(customBucketsFloatHistograms, nil)
recordType = dec.Type(customBucketsFloatHists)
require.Equal(t, CustomBucketsFloatHistogramSamplesV2, recordType)
```

### Anti-Patterns to Avoid

- **Mutating the shared `histograms` slice before V2 tests:** The existing V1 test at line 248 mutates `histograms[i].H.CounterResetHint = histogram.GaugeType`. V2 test blocks added after line 274 will see already-mutated data. Either use fresh data or take a copy before adding V2 test blocks.
- **Testing custom-bucket type without non-custom samples:** If all 3 histograms in the slice use custom buckets, `HistogramSamples()` resets its buffer and returns empty. The test data has 2 standard + 1 custom, which is fine.
- **Forgetting ST in the float histogram copy:** The `ToFloat(nil)` pattern from line 232 does not copy ST, because the existing test data has no ST. For V2 float tests with ST, the conversion loop must also set `ST: h.ST`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Histogram payload encoding | Custom byte packing | `EncodeHistogram` / `DecodeHistogram` (already in record.go) | These handle all schema variants including custom buckets |
| Test data construction | Novel histogram structs | Reuse `histograms` slice from line 166, set ST field | Already covers standard, negative-buckets, and custom-bucket cases |

## Common Pitfalls

### Pitfall 1: Gauge mutation aliasing

**What goes wrong:** The gauge test block (lines 248-274) mutates `histograms[i].H.CounterResetHint` in-place. Any V2 int-histogram test blocks appended after line 274 will use gauge-type histograms, not counter-type.

**Why it happens:** Go slices of structs with pointer fields (`*histogram.Histogram`) share the underlying `Histogram` objects when assigned by value. Mutation through `histograms[i].H.CounterResetHint` affects the shared object.

**How to avoid:** Define V2 test data slices fresh (using `RefHistogramSample{Ref: ..., T: ..., ST: ..., H: histograms[i].H}` where `histograms[i]` refers to the original line-166 slice — but that is already mutated). Safest: define V2 test slices before line 248, or build fresh `histogram.Histogram` values inline for V2 blocks, or reset `CounterResetHint` before the V2 blocks.

**Warning signs:** Tests pass with `CounterResetHint = histogram.GaugeType` in the decoded histograms when you expected `histogram.UnknownCounterReset`.

### Pitfall 2: Custom-bucket histogram in the first position

**What goes wrong:** `histogramSamplesV2` writes the first histogram unconditionally (full varint, no marker). If `histograms[0]` is a custom-bucket histogram, it gets encoded into the main record (not split out), then the decoder returns it via `HistogramSamples()`. But subsequent custom-bucket items are split. This creates an inconsistency in what `HistogramSamples()` returns vs what `CustomBucketsHistogramSamples()` returns.

**Why it happens:** The V2 encoder only checks `h.H.UsesCustomBuckets()` for items at index `i >= 1`. Index 0 is always written to the main buffer.

**How to avoid:** In test data, place the custom-bucket histogram (schema=-53) last in the slice — matching the existing test data at line 166 (index 2). The standard histograms are at indices 0 and 1.

**Warning signs:** `dec.HistogramSamples(histSamples, nil)` returns more items than expected, or `customBucketsHistograms` slice returned by encoder has fewer items than expected.

### Pitfall 3: Backward compat test must use a separate V1 encoder

**What goes wrong:** Re-using the `enc` variable that was set to `Encoder{EnableSTStorage: true}` earlier in the function will encode V2 records, not V1.

**How to avoid:** Declare a local `encV1 := Encoder{}` for the backward compat block (TEST-06).

### Pitfall 4: ST=0 is the noST case, not "zero start time"

**What goes wrong:** A test that sets ST=0 on all samples and decodes expects to get ST=0 back. That works. But it is NOT testing the noST encoder branch for samples after the first — for subsequent samples, ST=0 encodes as `noST` marker, and decodes leaving ST at the zero value of the struct. The round-trip produces ST=0, which matches. No problem.

**Why it matters:** The noST scenario (TEST-01) does work correctly when ST=0 on all samples. Just be aware the encoder writes `noST` byte (not an explicit varint) for subsequent samples, so the test implicitly exercises that path without needing special assertions.

### Pitfall 5: `dec.Type()` call requires the V2 encoder to be set

**What goes wrong:** `TestRecord_Type` has `var enc Encoder` at line 537 (V1 encoder). After adding V2 type assertions, the pattern in the function reassigns `enc = Encoder{EnableSTStorage: true}` partway through (line 549). The histogram V2 block must come after that reassignment, OR must use a locally declared V2 encoder.

**How to avoid:** Add V2 histogram type assertions after line 605 and use the `enc` that was already set to `EnableSTStorage: true` at line 549.

## Code Examples

### ST scenario test data (INT histogram)

```go
// Source: mirrors lines 104-134 (float sample V2 ST scenarios)

// Reusable base histogram pointers (indices 0 and 1 are standard, index 2 is custom)
// Define these before the gauge mutation at line 248.
h0 := histograms[0].H  // standard, schema=1
h1 := histograms[1].H  // standard with negatives, schema=1
hCB := histograms[2].H // custom buckets, schema=-53

// No ST (TEST-01 for int histograms)
histsNoST := []RefHistogramSample{
    {Ref: 56, T: 1234, H: h0},
    {Ref: 42, T: 5678, H: h1},
}

// Constant ST (TEST-02 for int histograms)
histsConstST := []RefHistogramSample{
    {Ref: 56, T: 1234, ST: 1000, H: h0},
    {Ref: 42, T: 5678, ST: 1000, H: h1},
}

// Varying ST — delta case (TEST-03 for int histograms)
histsDeltaST := []RefHistogramSample{
    {Ref: 56, T: 1234, ST: 1000, H: h0},
    {Ref: 42, T: 5678, ST: 1234, H: h1}, // ST[1] == T[0]
}

// All three samples including custom-bucket (TEST-05)
histsWithCB := []RefHistogramSample{
    {Ref: 56, T: 1234, ST: 1000, H: h0},
    {Ref: 42, T: 5678, ST: 1000, H: h1},
    {Ref: 67, T: 5678, ST: 1000, H: hCB},
}
```

### Run command

```bash
go test ./tsdb/record/ -run TestRecord_EncodeDecode -count=1 -v
go test ./tsdb/record/ -run TestRecord_Type -count=1 -v
go test ./tsdb/record/ -count=1
```

Full package test confirmed working: `ok github.com/prometheus/prometheus/tsdb/record 0.006s`

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| V1 histogram encoding (BE64 baseRef/baseTime) | V2 all-varint + ST marker scheme | Phase 2 of this project | V2 tests require `EnableSTStorage: true`, not just non-zero ST fields |
| No ST field on histogram structs | `ST, T int64` on `RefHistogramSample` and `RefFloatHistogramSample` | Phase 1 of this project | Existing test data can gain ST by setting the field |

## Open Questions

1. **Should V2 histogram blocks use `t.Run` subtests?**
   - What we know: The existing V2 sample blocks (lines 91-134) do NOT use subtests. The schema/corrupted tests DO use subtests.
   - What's unclear: Whether using `t.Run` is preferable for readability given the number of scenario blocks.
   - Recommendation: Left to planner. Subtests improve failure isolation. Inline (matching existing pattern) keeps the file consistent with the V2 sample section. Either is correct.

2. **Should the V2 histogram test blocks also test the gauge variant?**
   - What we know: The CONTEXT.md specifically calls out that the gauge variant re-test (lines 248-274) should also be done for V2, matching the existing pattern.
   - What's unclear: Whether this is captured in an explicit requirement.
   - Recommendation: Include it. The CONTEXT.md `<specifics>` section says "The gauge variant re-test (lines 248-274) should also be done for V2." It is not a separate TEST-XX requirement but is within scope.

## Validation Architecture

(Skipped: `nyquist_validation` not set in config.json.)

## Sources

### Primary (HIGH confidence)
- Direct code inspection: `/home/owilliams/src/grafana/prometheus/tsdb/record/record_test.go` — full test file read, all line numbers verified
- Direct code inspection: `/home/owilliams/src/grafana/prometheus/tsdb/record/record.go` — all encoder and decoder implementations read and verified
- Live test run: `go test ./tsdb/record/ -run TestRecord_EncodeDecode -count=1` — passes, 0.006s

### Secondary (MEDIUM confidence)
- Phase 3 implementation summaries in `.planning/phases/03-v2-decoders/` — confirmed all four decoder methods are complete

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — pure Go stdlib + testify, already imported
- Architecture: HIGH — patterns read directly from the file, line numbers verified
- Pitfalls: HIGH — identified from direct code analysis, not inference

**Research date:** 2026-03-03
**Valid until:** Until record.go changes (stable; 90 days reasonable)
