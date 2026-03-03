# Phase 3: V2 Decoders - Context

**Gathered:** 2026-03-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Add V2 decoder methods for all four histogram record types. Decoder.HistogramSamples() and Decoder.FloatHistogramSamples() must accept both V1 and V2 record types. V1 records decode with ST=0 (backward compat). No encoder changes.

</domain>

<decisions>
## Implementation Decisions

### V2 wire format (locked from Phase 2)
- First sample: varint(ref) + varint(T) + varint(ST) + histogram payload
- Subsequent samples: varint(dRef from prev) + varint(dT from first) + STmarker(1 byte) + [varint(dST from first)] + histogram payload
- Decoder must mirror this exactly

### Dispatch pattern
- Decoder.HistogramSamples() already reads the type byte and switches on it
- Add V2 types to the switch: HistogramSamplesV2 and CustomBucketsHistogramSamplesV2 dispatch to a new histogramSamplesV2 private method
- Same for FloatHistogramSamples(): add FloatHistogramSamplesV2 and CustomBucketsFloatHistogramSamplesV2
- V1 path unchanged (ST defaults to zero value in the struct)

### Ref reconstruction
- Delta to previous ref: `ref = int64(prev.Ref) + dec.Varint64()` (matching samplesV2 decoder)
- First sample: `ref = dec.Varint64()` (absolute, no delta)

### ST reconstruction
- First sample: `ST = dec.Varint64()` (absolute)
- Subsequent: read marker byte, then noST (ST=0), sameST (ST=prev.ST), explicitST (ST=firstST+delta)
- Exact same logic as samplesV2 decoder

### Schema validation
- V2 decoder must preserve the existing schema validation from V1: IsKnownSchema check, ReduceResolution for high-resolution histograms
- Same warn-and-skip behavior for unknown schemas

### Backward compatibility
- V1 records decode with ST=0 (zero value, no change to V1 path)
- New decoders accept both V1 and V2 type bytes

### Claude's Discretion
- Whether to extract V1 decoder bodies into private methods (like encoders did) or leave inline
- Exact error message wording for V2 decode failures

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `samplesV2` decoder at line 384: direct pattern to mirror
- `DecodeHistogram` function: called after ref/T/ST decoding, unchanged
- `DecodeFloatHistogram` function: same for float variants
- Existing `Decoder.HistogramSamples()` at line 529: add V2 dispatch here
- Existing `Decoder.FloatHistogramSamples()`: add V2 dispatch here

### Established Patterns
- `Decoder.Samples()` at line 320: the V1/V2 dispatch pattern (switch on type byte)
- V1 histogram decoder reads Be64 for baseRef/baseTime, then varint deltas
- Schema validation + ReduceResolution logic must be preserved in V2 path

### Integration Points
- Phase 4 tests will round-trip encode/decode to verify correctness
- No caller changes needed: public method signatures unchanged

</code_context>

<specifics>
## Specific Ideas

- The samplesV2 decoder tracks `firstT` and `firstST` as running state outside the loop. V2 histogram decoders need the same.
- The "skipped custom-bucket" edge case in the encoder means the decoder's `prev` ref tracks the last decoded sample, not the last encoded one. This is fine because the decoder only sees non-skipped samples in its byte stream.

</specifics>

<deferred>
## Deferred Ideas

None. Discussion stayed within phase scope.

</deferred>

---

*Phase: 03-v2-decoders*
*Context gathered: 2026-03-02*
