# Phase 1: Struct and Type Definitions - Context

**Gathered:** 2026-03-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Add ST fields to RefHistogramSample and RefFloatHistogramSample structs, and declare four new V2 record type constants. No encoding/decoding logic. Code must compile and pass vet.

</domain>

<decisions>
## Implementation Decisions

### ST field style
- Use `ST, T int64` on one line, matching RefSample pattern exactly
- ST comes before T (same ordering as RefSample)
- Both RefHistogramSample and RefFloatHistogramSample get the field

### V2 type naming
- Use `V2` suffix: HistogramSamplesV2, FloatHistogramSamplesV2, CustomBucketsHistogramSamplesV2, CustomBucketsFloatHistogramSamplesV2
- Matches existing SamplesV2 naming convention
- String() returns snake_case with `_v2` suffix (e.g., "histogram_samples_v2")

### V2 type numbering
- Sequential after SamplesV2 (11): types 12, 13, 14, 15
- Order: HistogramSamplesV2 (12), FloatHistogramSamplesV2 (13), CustomBucketsHistogramSamplesV2 (14), CustomBucketsFloatHistogramSamplesV2 (15)

### CustomBuckets V2 strategy
- Mirror V1: four separate V2 types, one for each V1 type
- Keeps the type space consistent and predictable
- CustomBuckets encoder functions already exist separately. V2 versions will too.

### Claude's Discretion
- Exact comment wording on new constants
- Whether to update the struct doc comment beyond removing the TODO

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `RefSample` struct at line 166: direct pattern to mirror for field placement
- `SamplesV2` constant at line 62: naming and numbering pattern to follow
- `Type.String()` switch at line 65: add new cases here
- `Decoder.Type()` switch at line 229: add new types to the valid set

### Established Patterns
- Record type constants are sequential integers starting from 1
- `Type.String()` uses snake_case (e.g., "histogram_samples", "samples-v2")
- Decoder.Type() lists all valid types in a single case clause
- Struct comments are brief, one-line descriptions

### Integration Points
- New types will be referenced by Phase 2 (encoders) and Phase 3 (decoders)
- EnableSTStorage flag on Encoder struct gates V1 vs V2 dispatch

</code_context>

<specifics>
## Specific Ideas

- The existing SamplesV2 String() uses "samples-v2" (hyphen, not underscore). Histogram V1 types use underscores ("histogram_samples"). V2 histogram types should use underscores with v2: "histogram_samples_v2". This keeps consistency within histogram types while acknowledging the SamplesV2 hyphen is a minor inconsistency.

</specifics>

<deferred>
## Deferred Ideas

None. Discussion stayed within phase scope.

</deferred>

---

*Phase: 01-struct-and-type-definitions*
*Context gathered: 2026-03-02*
