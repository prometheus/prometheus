# Phase 2: V2 Encoders - Context

**Gathered:** 2026-03-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Add V2 encoder methods for all four histogram record types, gated on EnableSTStorage. Each V2 method uses the ST marker scheme (noST/sameST/explicitST). Public encoder methods dispatch to V2 when EnableSTStorage is true. No decoder changes (Phase 3).

</domain>

<decisions>
## Implementation Decisions

### V2 wire format
- Use varint throughout for first sample (matching samplesV2 pattern), not BE64
- First sample: varint(ref) + varint(T) + varint(ST) + histogram payload
- Subsequent samples: varint(dRef) + varint(dT) + STmarker(1 byte) + [varint(dST)] + histogram payload
- This is a deliberate break from V1 histogram encoding which uses BE64 for base ref/time

### Ref delta style
- Delta to previous ref (matching samplesV2), not delta to first ref
- Each sample's ref is encoded as delta from the immediately preceding sample
- T deltas remain against first T (matching samplesV2 convention)

### ST marker scheme
- Reuse the existing noST/sameST/explicitST constants (already defined)
- First sample: ST encoded directly as varint (no marker needed)
- Subsequent samples: 1-byte marker, then optional varint delta to first ST
- Exact same logic as samplesV2's ST handling

### Custom bucket filtering
- V2 public methods keep the same filter-and-return API contract as V1
- HistogramSamples() with EnableSTStorage still filters out custom-bucket histograms and returns them
- FloatHistogramSamples() same pattern
- Caller encodes returned custom-bucket histograms with the corresponding CustomBuckets V2 method

### Method signatures and dispatch
- Public methods (HistogramSamples, FloatHistogramSamples, etc.) gain EnableSTStorage gate
- HistogramSamples() and FloatHistogramSamples() return ([]byte, []RefHistogramSample/RefFloatHistogramSample) in both V1 and V2
- CustomBuckets methods return []byte in both V1 and V2
- Private V2 methods use (*Encoder) receiver (matching samplesV2 pattern)

### Claude's Discretion
- Whether to extract a shared ST-encoding helper or inline the marker logic in each V2 method
- Exact comment wording on new methods
- Whether to change the Encoder struct comment to mention histogram V2

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `samplesV2` at line 836: direct pattern to mirror for the V2 histogram encoding loop
- `noST/sameST/explicitST` constants at line 825: reuse directly
- `EncodeHistogram` at line 989: called after ref/T/ST encoding, unchanged
- `EncodeFloatHistogram` at line 1089: same, for float variants
- `Encoder.EnableSTStorage` at line 746: the gate flag, already exists

### Established Patterns
- `Encoder.Samples()` at line 794: the V1/V2 dispatch pattern to follow
- V1 histogram encoders at lines 931-984: the existing API contract (filter custom buckets, return them)
- Private method naming: `samplesV1`, `samplesV2` (lowercase, version suffix)

### Integration Points
- Phase 3 decoders will need to match this exact wire format
- No caller changes needed: public method signatures unchanged, EnableSTStorage flag is the only control

</code_context>

<specifics>
## Specific Ideas

- The samplesV2 encoder uses `int64(s.Ref) - int64(prev.Ref)` for ref deltas. V2 histograms should use the same cast pattern.
- T deltas in samplesV2 are `s.T - first.T` (delta to first, not previous). V2 histograms must match this.
- The empty-slice early return should write just the type byte then return (matching both samplesV2 and V1 histogram patterns).

</specifics>

<deferred>
## Deferred Ideas

None. Discussion stayed within phase scope.

</deferred>

---

*Phase: 02-v2-encoders*
*Context gathered: 2026-03-02*
