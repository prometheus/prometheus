# Phase 3: V2 Decoders - Research

**Researched:** 2026-03-02
**Domain:** Go WAL record decoding, tsdb/record/record.go
**Confidence:** HIGH

## Summary

Phase 3 adds V2 decoder support to `Decoder.HistogramSamples()` and `Decoder.FloatHistogramSamples()`. The wire format is already locked by Phase 2. The implementation is a direct mirror of `samplesV2` (lines 384-434), adapted for histograms instead of scalar samples.

The existing V1 decoder bodies read a BE64 baseRef/baseTime then loop over varint deltas. V2 replaces that with all-varint first sample, ref-delta-to-prev, T-delta-to-first, and a 1-byte ST marker for subsequent samples. The schema validation and `ReduceResolution` logic from V1 must be preserved unchanged in the V2 path.

**Primary recommendation:** Add `histogramSamplesV2` and `floatHistogramSamplesV2` private methods modeled exactly on `samplesV2`. Update the two public methods to switch on type byte (matching `Decoder.Samples()` dispatch at line 336). Leave V1 paths completely untouched.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- V2 wire format mirrors encoder exactly: first sample is varint(ref) + varint(T) + varint(ST) + histogram payload; subsequent samples are varint(dRef from prev) + varint(dT from first) + STmarker(1 byte) + [varint(dST from first)] + histogram payload
- Dispatch pattern: add V2 types to switch in HistogramSamples() and FloatHistogramSamples(); V2 types dispatch to new private methods
- Ref reconstruction: first sample absolute varint, subsequent samples delta to prev ref
- ST reconstruction: first sample absolute varint, subsequent samples use noST/sameST/explicitST marker; explicitST = firstST + delta
- Schema validation preserved: IsKnownSchema check + ReduceResolution for high-res histograms, same warn-and-skip behavior
- V1 path unchanged; V1 records decode with ST=0

### Claude's Discretion

- Whether to extract V1 decoder bodies into private methods (like encoders did) or leave inline
- Exact error message wording for V2 decode failures

### Deferred Ideas (OUT OF SCOPE)

None. Discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DEC-01 | Decoder.HistogramSamples() accepts both V1 and V2 record types | Switch on type byte (pattern from Decoder.Samples() line 336); add HistogramSamplesV2 and CustomBucketsHistogramSamplesV2 cases |
| DEC-02 | Decoder.FloatHistogramSamples() accepts both V1 and V2 record types | Same switch pattern; add FloatHistogramSamplesV2 and CustomBucketsFloatHistogramSamplesV2 cases |
| DEC-03 | V2 histogram decoding correctly reads ST marker bytes and reconstructs ST values | histogramSamplesV2 / floatHistogramSamplesV2 private methods mirror samplesV2 (lines 384-434); track firstT, firstST outside loop |
| DEC-04 | V1 records decoded with ST=0 (backward compat) | ST field zero-value in struct; V1 path never writes ST, so zero value is automatic |
</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `encoding.Decbuf` | in-repo | Binary decode buffer | Same type used by all existing decoders |
| `histogram.Histogram` / `histogram.FloatHistogram` | in-repo | Histogram structs | Target types for DecodeHistogram/DecodeFloatHistogram |
| `chunks.HeadSeriesRef` | in-repo | Ref type cast | Required for Ref field assignment |

No new dependencies. Everything is already imported.

## Architecture Patterns

### Recommended Structure

Two new private methods, two updated public methods. No new files.

```
tsdb/record/record.go
  Decoder.HistogramSamples()         — updated: switch on type byte
  Decoder.histogramSamplesV2()       — NEW private method
  Decoder.FloatHistogramSamples()    — updated: switch on type byte
  Decoder.floatHistogramSamplesV2()  — NEW private method
```

### Pattern 1: Public Method Dispatch (mirror Decoder.Samples())

**What:** Switch on the type byte immediately after reading it. Dispatch to private method. Return error for unknown types.

**When to use:** Always — this is the established pattern in this file.

```go
// Source: record.go line 336 (Decoder.Samples)
func (d *Decoder) HistogramSamples(rec []byte, histograms []RefHistogramSample) ([]RefHistogramSample, error) {
    dec := encoding.Decbuf{B: rec}
    switch typ := Type(dec.Byte()); typ {
    case HistogramSamples, CustomBucketsHistogramSamples:
        return d.histogramSamplesV1(&dec, histograms)
    case HistogramSamplesV2, CustomBucketsHistogramSamplesV2:
        return d.histogramSamplesV2(&dec, histograms)
    default:
        return nil, fmt.Errorf("invalid record type %v", typ)
    }
}
```

Note: The existing V1 body uses `if t != A && t != B` guard. Refactoring to a switch (Claude's discretion) is cleaner and matches `Decoder.Samples()`. Either approach is valid; switch is preferred for readability.

### Pattern 2: V2 Private Decoder (mirror samplesV2)

**What:** Track `firstT` and `firstST` outside the loop. First iteration reads absolute ref/T/ST. Subsequent iterations read delta-ref, delta-T, then ST marker byte.

**When to use:** histogramSamplesV2 and floatHistogramSamplesV2 both follow this pattern.

```go
// Source: record.go lines 384-434 (samplesV2), adapted for histograms
func (d *Decoder) histogramSamplesV2(dec *encoding.Decbuf, histograms []RefHistogramSample) ([]RefHistogramSample, error) {
    if dec.Len() == 0 {
        return histograms, nil
    }
    var firstT, firstST int64
    for len(dec.B) > 0 && dec.Err() == nil {
        var ref, t, ST int64

        if len(histograms) == 0 {
            ref = dec.Varint64()
            firstT = dec.Varint64()
            t = firstT
            ST = dec.Varint64()
            firstST = ST
        } else {
            prev := histograms[len(histograms)-1]
            ref = int64(prev.Ref) + dec.Varint64()
            t = firstT + dec.Varint64()
            stMarker := dec.Byte()
            switch stMarker {
            case noST:
                // ST stays zero
            case sameST:
                ST = prev.ST
            default:
                ST = firstST + dec.Varint64()
            }
        }

        rh := RefHistogramSample{
            Ref: chunks.HeadSeriesRef(ref),
            ST:  ST,
            T:   t,
            H:   &histogram.Histogram{},
        }
        DecodeHistogram(dec, rh.H)

        // Schema validation — same as V1 path, must be preserved
        if !histogram.IsKnownSchema(rh.H.Schema) {
            d.logger.Warn("skipping histogram with unknown schema in WAL record", "schema", rh.H.Schema, "timestamp", rh.T)
            continue
        }
        if rh.H.Schema > histogram.ExponentialSchemaMax && rh.H.Schema <= histogram.ExponentialSchemaMaxReserved {
            if err := rh.H.ReduceResolution(histogram.ExponentialSchemaMax); err != nil {
                return nil, fmt.Errorf("error reducing resolution of histogram #%d: %w", len(histograms)+1, err)
            }
        }
        histograms = append(histograms, rh)
    }
    if dec.Err() != nil {
        return nil, fmt.Errorf("decode error after %d histograms: %w", len(histograms), dec.Err())
    }
    if len(dec.B) > 0 {
        return nil, fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
    }
    return histograms, nil
}
```

`floatHistogramSamplesV2` is identical except it uses `RefFloatHistogramSample`, `rh.FH`, and `DecodeFloatHistogram`.

### Anti-Patterns to Avoid

- **Reading BE64 baseRef/baseTime in V2 path:** V2 wire format is all-varint. Reading BE64 would corrupt the decode.
- **Delta-to-first for ref:** V2 uses delta-to-prev for ref (unlike T which is delta-to-first). Getting these mixed up produces wrong refs for all samples after the first.
- **Forgetting schema validation in V2 path:** The warn-and-skip logic for unknown schemas must exist in V2 just as in V1. Omitting it would silently accept records that V1 would reject.
- **Using `prev` before histograms is non-empty:** `histograms[len(histograms)-1]` panics on empty slice. Guard with `if len(histograms) == 0` (same guard as samplesV2).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Binary decode | Custom byte parsing | `encoding.Decbuf` | Handles error propagation, bounds checking |
| Histogram payload decode | Manual field reads | `DecodeHistogram` / `DecodeFloatHistogram` | Already correct, handles custom buckets |
| Schema check | Custom schema logic | `histogram.IsKnownSchema`, `histogram.ExponentialSchemaMax` | Shared constants, must stay in sync |

## Common Pitfalls

### Pitfall 1: ST marker default case
**What goes wrong:** The `default` case of the ST marker switch in samplesV2 means "explicitST". The byte value itself is not one of the named constants — it's any value that is neither `noST` nor `sameST`. Reading the varint delta only in the default case is correct.
**Why it happens:** Engineers expect a three-way switch with explicit constant matching and forget the default IS the third case.
**How to avoid:** Copy the switch structure verbatim from samplesV2 (line 408-415). Do not add a named `explicitST` case.

### Pitfall 2: prev.ST vs firstST
**What goes wrong:** `sameST` sets `ST = prev.ST` (the last decoded sample's ST). `explicitST` sets `ST = firstST + delta` (delta from the first sample's ST). Swapping these produces wrong ST values.
**How to avoid:** Keep `firstST` as a loop-outer variable set once on first iteration.

### Pitfall 3: skipped-custom-bucket effect on prev
**What goes wrong:** When schema validation causes a `continue` (skipping appending), `histograms[len(histograms)-1]` on the next iteration still refers to the last successfully decoded sample, not the skipped one.
**Why it happens:** This is correct behavior — the decoder's `prev` tracks the last appended sample, matching what the encoder encoded. No fix needed, just awareness.

### Pitfall 4: V1 path modification
**What goes wrong:** Accidentally modifying the V1 path while refactoring (e.g., extracting into a private method and introducing a bug).
**How to avoid:** If extracting V1 into `histogramSamplesV1`, do it as a pure mechanical extraction with zero logic changes. Run existing tests to confirm.

## Code Examples

### Existing dispatch pattern (Decoder.Samples, line 336)
```go
// Source: tsdb/record/record.go:336
func (d *Decoder) Samples(rec []byte, samples []RefSample) ([]RefSample, error) {
    dec := encoding.Decbuf{B: rec}
    switch typ := dec.Byte(); Type(typ) {
    case Samples:
        return d.samplesV1(&dec, samples)
    case SamplesV2:
        return d.samplesV2(&dec, samples)
    default:
        return nil, fmt.Errorf("invalid record type %v, expected Samples(2) or SamplesV2(11)", typ)
    }
}
```

### ST marker decode (samplesV2, lines 408-415)
```go
// Source: tsdb/record/record.go:408
stMarker := dec.Byte()
switch stMarker {
case noST:
case sameST:
    ST = prev.ST
default:
    ST = firstST + dec.Varint64()
}
```

### Current V1 HistogramSamples type guard (line 532)
```go
// Source: tsdb/record/record.go:532 — this gets replaced by the switch
if t != HistogramSamples && t != CustomBucketsHistogramSamples {
    return nil, errors.New("invalid record type")
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| BE64 baseRef + BE64 baseTime header | All-varint, no separate header | Phase 2 (V2 encoders) | Decoder must NOT read BE64 for V2 records |
| No ST field | ST field with noST/sameST/explicitST marker | Phase 1+2 | Decoder reads marker byte on every non-first sample |

## Open Questions

1. **Extract V1 into private method or leave inline?**
   - What we know: Encoders extracted V1/V2 into private methods. Decoders currently have V1 inline.
   - What's unclear: Whether the planner wants consistency with encoder style.
   - Recommendation: Claude's discretion per CONTEXT.md. Extracting is cleaner (switch reads naturally) but is optional. Mark as discretionary in plan.

## Sources

### Primary (HIGH confidence)
- `tsdb/record/record.go` lines 336-434 (Decoder.Samples, samplesV1, samplesV2) — direct pattern source
- `tsdb/record/record.go` lines 529-683 (HistogramSamples, FloatHistogramSamples) — exact code being modified
- `.planning/phases/03-v2-decoders/03-CONTEXT.md` — locked decisions
- `.planning/phases/02-v2-encoders/02-CONTEXT.md` — encoder wire format reference
- `.planning/REQUIREMENTS.md` — DEC-01 through DEC-04

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all in-repo, no external deps
- Architecture: HIGH — exact pattern exists in samplesV2, direct mirror
- Pitfalls: HIGH — derived from reading actual code and encoder decisions

**Research date:** 2026-03-02
**Valid until:** Until record.go is modified (stable internal code)
