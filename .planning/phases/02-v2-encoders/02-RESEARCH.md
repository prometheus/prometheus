# Phase 2: V2 Encoders - Research

**Researched:** 2026-03-02
**Domain:** Go WAL record encoding, `tsdb/record/record.go`
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**V2 wire format**
- Use varint throughout for first sample (matching samplesV2 pattern), not BE64
- First sample: varint(ref) + varint(T) + varint(ST) + histogram payload
- Subsequent samples: varint(dRef) + varint(dT) + STmarker(1 byte) + [varint(dST)] + histogram payload
- This is a deliberate break from V1 histogram encoding which uses BE64 for base ref/time

**Ref delta style**
- Delta to previous ref (matching samplesV2), not delta to first ref
- Each sample's ref is encoded as delta from the immediately preceding sample
- T deltas remain against first T (matching samplesV2 convention)

**ST marker scheme**
- Reuse the existing noST/sameST/explicitST constants (already defined)
- First sample: ST encoded directly as varint (no marker needed)
- Subsequent samples: 1-byte marker, then optional varint delta to first ST
- Exact same logic as samplesV2's ST handling

**Custom bucket filtering**
- V2 public methods keep the same filter-and-return API contract as V1
- HistogramSamples() with EnableSTStorage still filters out custom-bucket histograms and returns them
- FloatHistogramSamples() same pattern
- Caller encodes returned custom-bucket histograms with the corresponding CustomBuckets V2 method

**Method signatures and dispatch**
- Public methods (HistogramSamples, FloatHistogramSamples, etc.) gain EnableSTStorage gate
- HistogramSamples() and FloatHistogramSamples() return ([]byte, []RefHistogramSample/RefFloatHistogramSample) in both V1 and V2
- CustomBuckets methods return []byte in both V1 and V2
- Private V2 methods use (*Encoder) receiver (matching samplesV2 pattern)

### Claude's Discretion
- Whether to extract a shared ST-encoding helper or inline the marker logic in each V2 method
- Exact comment wording on new methods
- Whether to change the Encoder struct comment to mention histogram V2

### Deferred Ideas (OUT OF SCOPE)

None. Discussion stayed within phase scope.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| ENC-01 | Encoder.HistogramSamples() gates on EnableSTStorage, dispatches to V2 when enabled | Public method receiver must change from `(*Encoder)` to `(e *Encoder)` to read the flag; call private `histogramSamplesV2` |
| ENC-02 | Encoder.FloatHistogramSamples() gates on EnableSTStorage, dispatches to V2 when enabled | Same receiver change; call private `floatHistogramSamplesV2` |
| ENC-03 | Encoder.CustomBucketsHistogramSamples() gates on EnableSTStorage, dispatches to V2 when enabled | Same receiver change; call private `customBucketsHistogramSamplesV2` |
| ENC-04 | Encoder.CustomBucketsFloatHistogramSamples() gates on EnableSTStorage, dispatches to V2 when enabled | Same receiver change; call private `customBucketsFloatHistogramSamplesV2` |
| ENC-05 | V2 histogram encoding uses noST/sameST/explicitST marker scheme | Constants already exist at line 825; wire format confirmed from samplesV2 loop |
</phase_requirements>

---

## Summary

Phase 1 is complete: `RefHistogramSample` and `RefFloatHistogramSample` both have `ST, T int64` fields, and the four V2 type constants (12-15) are defined with correct `String()` returns. The Decoder's `Type()` method already recognizes all four new types.

Phase 2 is a pure encoder addition. The entire pattern is already demonstrated in the codebase by `samplesV2` (line 836). The work is mechanical: write four private V2 methods that mirror `samplesV2`'s structure but call `EncodeHistogram`/`EncodeFloatHistogram` instead of writing a float value, then add dispatch gates to the four public methods.

The only structural change to existing code is that four public methods need their receiver changed from `(*Encoder)` to `(e *Encoder)` so they can read `e.EnableSTStorage`. Private V2 methods can stay as `(*Encoder)` because they do not read the flag (the flag check is in the public caller).

**Primary recommendation:** Mirror `samplesV2` exactly for all four V2 private methods. Change the four public histogram method receivers to named `(e *Encoder)`. Add a two-branch `if e.EnableSTStorage` guard in each public method.

---

## Standard Stack

### Core
| Element | Location | Purpose |
|---------|----------|---------|
| `encoding.Encbuf` | `tsdb/encoding` | Buffer with typed Put methods — all encoding uses this |
| `buf.PutByte()` | Encbuf | Writes the record type byte |
| `buf.PutVarint64()` | Encbuf | Writes signed varint — used for ref, T, ST, dRef, dT, dST |
| `buf.PutBE64int64()` | Encbuf | Big-endian int64 — used in V1 only, NOT in V2 |
| `buf.PutBE64()` | Encbuf | Big-endian uint64 — used in V1 only, NOT in V2 |
| `buf.Reset()` | Encbuf | Resets buffer to empty — used when all samples were custom-bucket filtered |
| `buf.Get()` | Encbuf | Returns the accumulated byte slice |
| `EncodeHistogram()` | record.go:989 | Encodes `*histogram.Histogram` payload into buf |
| `EncodeFloatHistogram()` | record.go:1089 | Encodes `*histogram.FloatHistogram` payload into buf |
| `noST`, `sameST`, `explicitST` | record.go:828-830 | Byte constants for ST marker scheme |

### No New Dependencies
No new imports are needed. All tools are already imported.

---

## Architecture Patterns

### Pattern 1: samplesV2 — the exact template to follow

```go
// Source: record.go:836
func (*Encoder) samplesV2(samples []RefSample, b []byte) []byte {
    buf := encoding.Encbuf{B: b}
    buf.PutByte(byte(SamplesV2))

    if len(samples) == 0 {
        return buf.Get()
    }

    // First sample: full varint values (no deltas, no marker)
    first := samples[0]
    buf.PutVarint64(int64(first.Ref))
    buf.PutVarint64(first.T)
    buf.PutVarint64(first.ST)
    buf.PutBE64(math.Float64bits(first.V))   // <-- replace with EncodeHistogram/EncodeFloatHistogram

    // Subsequent samples: deltas + ST marker
    for i := 1; i < len(samples); i++ {
        s := samples[i]
        prev := samples[i-1]

        buf.PutVarint64(int64(s.Ref) - int64(prev.Ref))  // delta to prev ref
        buf.PutVarint64(s.T - first.T)                    // delta to first T

        switch s.ST {
        case 0:
            buf.PutByte(noST)
        case prev.ST:
            buf.PutByte(sameST)
        default:
            buf.PutByte(explicitST)
            buf.PutVarint64(s.ST - first.ST)              // delta to first ST
        }
        buf.PutBE64(math.Float64bits(s.V))   // <-- replace with EncodeHistogram/EncodeFloatHistogram
    }
    return buf.Get()
}
```

### Pattern 2: V1 HistogramSamples — the filtering contract to preserve

```go
// Source: record.go:931
func (*Encoder) HistogramSamples(histograms []RefHistogramSample, b []byte) ([]byte, []RefHistogramSample) {
    buf := encoding.Encbuf{B: b}
    buf.PutByte(byte(HistogramSamples))

    if len(histograms) == 0 {
        return buf.Get(), nil
    }
    var customBucketHistograms []RefHistogramSample

    first := histograms[0]
    buf.PutBE64(uint64(first.Ref))      // V1 uses BE64 for base
    buf.PutBE64int64(first.T)

    for _, h := range histograms {
        if h.H.UsesCustomBuckets() {
            customBucketHistograms = append(customBucketHistograms, h)
            continue                    // skip, accumulate for caller
        }
        buf.PutVarint64(int64(h.Ref) - int64(first.Ref))
        buf.PutVarint64(h.T - first.T)
        EncodeHistogram(&buf, h.H)
    }

    // If ALL were custom buckets, reset the buffer (don't write type-byte-only record)
    if len(histograms) == len(customBucketHistograms) {
        buf.Reset()
    }

    return buf.Get(), customBucketHistograms
}
```

### Pattern 3: Public dispatch gate — how to add EnableSTStorage

```go
// Source: record.go:794 — Samples() dispatch pattern
func (e *Encoder) Samples(samples []RefSample, b []byte) []byte {
    if e.EnableSTStorage {
        return e.samplesV2(samples, b)
    }
    return e.samplesV1(samples, b)
}
```

Applied to HistogramSamples (currently `(*Encoder)`, must become `(e *Encoder)`):

```go
func (e *Encoder) HistogramSamples(histograms []RefHistogramSample, b []byte) ([]byte, []RefHistogramSample) {
    if e.EnableSTStorage {
        return e.histogramSamplesV2(histograms, b)
    }
    return e.histogramSamplesV1(histograms, b)
}
```

### Recommended Project Structure

No structural changes. All new code goes in `tsdb/record/record.go`:

- Rename existing public bodies to `histogramSamplesV1`, `floatHistogramSamplesV1`, etc.
- Add four private V2 methods immediately after the corresponding V1 methods.
- Update four public methods to named receiver + dispatch gate.

### Anti-Patterns to Avoid

- **Using BE64 for first sample in V2.** V1 uses `PutBE64` / `PutBE64int64` for the base ref and T. V2 uses `PutVarint64` for everything. Mixing them breaks the decoder.
- **Ref delta to first in V2.** V1 histogram uses `h.Ref - first.Ref` throughout. V2 uses `s.Ref - prev.Ref` (delta to previous). Match `samplesV2`, not V1 histogram.
- **Writing ST marker for the first sample.** The first sample writes `PutVarint64(first.ST)` directly — no marker byte. Markers only appear for index 1+.
- **Forgetting `buf.Reset()` in V2 filtering methods.** When all histograms are custom-bucket, the buffer has only the type byte. Reset it so the caller gets an empty (not malformed) slice.
- **Not renaming existing V1 bodies.** The public methods will call V1 bodies; extract them as `histogramSamplesV1` first, then add the dispatch, to avoid duplicating logic.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| ST marker encoding | Custom bit-packing | noST/sameST/explicitST + PutByte + PutVarint64 | Already designed and tested in samplesV2 |
| Histogram payload encoding | Custom field serialization | EncodeHistogram / EncodeFloatHistogram | These functions handle schema, spans, buckets, custom values correctly |
| Buffer accumulation | Manual []byte appending | encoding.Encbuf | Handles growth, alignment, and all Put variants |

---

## Common Pitfalls

### Pitfall 1: Receiver change breaks compilation silently if overlooked

**What goes wrong:** The four public methods are currently `(*Encoder)`. If you add the V2 dispatch body while forgetting to rename the receiver to `(e *Encoder)`, the compiler accepts it but `e.EnableSTStorage` is inaccessible. You'd be forced to write `e.EnableSTStorage` which the compiler will catch — so this won't silently break, but it is easy to forget.

**How to avoid:** Change all four public method receivers at once as the first edit, before writing dispatch logic.

### Pitfall 2: First-sample encoding diverges from V1

**What goes wrong:** V1 histogram encodes `first.Ref` with `PutBE64` and `first.T` with `PutBE64int64`. If you copy the V1 loop header into a V2 method without converting to `PutVarint64`, the decoder (Phase 3) will fail silently or read garbage.

**How to avoid:** V2 first-sample header is always three varint64 calls: `PutVarint64(int64(first.Ref))`, `PutVarint64(first.T)`, `PutVarint64(first.ST)`. Then `EncodeHistogram`.

### Pitfall 3: T delta convention differs between V1 and samplesV2

**What goes wrong:** V1 histogram uses `h.T - first.T` throughout the loop. samplesV2 also uses `s.T - first.T`. These agree. But if you accidentally write `s.T - prev.T` for T (following the ref-delta-to-prev pattern), the decoder will reconstruct wrong timestamps.

**How to avoid:** T is always delta to `first.T`. Only `Ref` is delta to `prev.Ref`.

### Pitfall 4: buf.Reset() edge case in V2 filtering methods

**What goes wrong:** If every histogram in the input slice uses custom buckets, the V2 method writes one type byte and then filters everything. Returning that single-byte buffer would produce a record with just a type byte and no samples — technically parseable but wasteful and potentially confusing for callers. V1 resets the buffer in this case.

**How to avoid:** After the loop, check `if len(histograms) == len(customBucketHistograms) { buf.Reset() }` — exact copy of V1.

### Pitfall 5: CustomBuckets V2 methods must NOT filter

**What goes wrong:** `CustomBucketsHistogramSamples` and its float variant do NOT filter — they assume all input already IS custom-bucket histograms. Their V2 counterparts must follow the same assumption and must NOT add filtering logic.

**How to avoid:** The CustomBuckets V2 methods have no `if h.H.UsesCustomBuckets()` check. They also have no `buf.Reset()` call. They just write type byte + first sample + loop. Return type is `[]byte`, not `([]byte, []RefHistogramSample)`.

---

## Code Examples

### histogramSamplesV2 — complete implementation

```go
// Source: derived from samplesV2 (record.go:836) + HistogramSamples (record.go:931)
func (*Encoder) histogramSamplesV2(histograms []RefHistogramSample, b []byte) ([]byte, []RefHistogramSample) {
    buf := encoding.Encbuf{B: b}
    buf.PutByte(byte(HistogramSamplesV2))

    if len(histograms) == 0 {
        return buf.Get(), nil
    }

    var customBucketHistograms []RefHistogramSample

    first := histograms[0]
    buf.PutVarint64(int64(first.Ref))
    buf.PutVarint64(first.T)
    buf.PutVarint64(first.ST)
    EncodeHistogram(&buf, first.H)

    for i := 1; i < len(histograms); i++ {
        h := histograms[i]
        if h.H.UsesCustomBuckets() {
            customBucketHistograms = append(customBucketHistograms, h)
            continue
        }
        prev := histograms[i-1]

        buf.PutVarint64(int64(h.Ref) - int64(prev.Ref))
        buf.PutVarint64(h.T - first.T)

        switch h.ST {
        case 0:
            buf.PutByte(noST)
        case prev.ST:
            buf.PutByte(sameST)
        default:
            buf.PutByte(explicitST)
            buf.PutVarint64(h.ST - first.ST)
        }
        EncodeHistogram(&buf, h.H)
    }

    if len(histograms) == len(customBucketHistograms) {
        buf.Reset()
    }

    return buf.Get(), customBucketHistograms
}
```

**Note on first-sample filtering:** The V1 method processes `first` before the loop and does not check `first.H.UsesCustomBuckets()` — if the first histogram is a custom-bucket type it gets encoded into the V1 record (this is a pre-existing V1 quirk). The V2 method should handle this consistently: write first's header fields before the filtering loop begins, then filter inside the loop. If only the non-first samples are custom buckets, first is already written. This matches V1 behavior exactly.

### customBucketsHistogramSamplesV2 — complete implementation

```go
// Source: derived from CustomBucketsHistogramSamples (record.go:965) + samplesV2 pattern
func (*Encoder) customBucketsHistogramSamplesV2(histograms []RefHistogramSample, b []byte) []byte {
    buf := encoding.Encbuf{B: b}
    buf.PutByte(byte(CustomBucketsHistogramSamplesV2))

    if len(histograms) == 0 {
        return buf.Get()
    }

    first := histograms[0]
    buf.PutVarint64(int64(first.Ref))
    buf.PutVarint64(first.T)
    buf.PutVarint64(first.ST)
    EncodeHistogram(&buf, first.H)

    for i := 1; i < len(histograms); i++ {
        h := histograms[i]
        prev := histograms[i-1]

        buf.PutVarint64(int64(h.Ref) - int64(prev.Ref))
        buf.PutVarint64(h.T - first.T)

        switch h.ST {
        case 0:
            buf.PutByte(noST)
        case prev.ST:
            buf.PutByte(sameST)
        default:
            buf.PutByte(explicitST)
            buf.PutVarint64(h.ST - first.ST)
        }
        EncodeHistogram(&buf, h.H)
    }

    return buf.Get()
}
```

### Public method dispatch — HistogramSamples (same pattern for all four)

```go
// Change receiver from (*Encoder) to (e *Encoder) and extract V1 body
func (e *Encoder) HistogramSamples(histograms []RefHistogramSample, b []byte) ([]byte, []RefHistogramSample) {
    if e.EnableSTStorage {
        return e.histogramSamplesV2(histograms, b)
    }
    return e.histogramSamplesV1(histograms, b)
}

// Rename original body to histogramSamplesV1
func (*Encoder) histogramSamplesV1(histograms []RefHistogramSample, b []byte) ([]byte, []RefHistogramSample) {
    // ... original body unchanged ...
}
```

---

## State of the Art

| Old Approach | Current Approach | Impact |
|--------------|------------------|--------|
| V1: BE64 base ref+T, varint deltas | V2: all-varint (base and deltas) | Smaller records for typical ref/T magnitudes |
| V1: No ST field on histograms | V2: ST varint for first, marker byte for subsequent | ST propagated through WAL without overhead on trivial cases |
| Public methods as `(*Encoder)` | Public dispatch methods as `(e *Encoder)` | Enables reading EnableSTStorage flag |

---

## Open Questions

1. **First-sample custom-bucket edge case**
   - What we know: V1 writes first.Ref and first.T before the filtering loop, so if histograms[0] is a custom-bucket type, it still gets a partial record written (header fields only, no payload filtered).
   - What's unclear: Whether the V2 method should pre-check `first.H.UsesCustomBuckets()` and skip the header write, or follow V1 exactly.
   - Recommendation: Follow V1's exact behavior (write header for first, filter in loop). Changing this would diverge from V1 semantics and the scope of this phase is V2 encoding only, not fixing V1 quirks.

2. **Inlining vs. shared ST helper**
   - What we know: The ST marker switch appears identically in all four V2 methods.
   - What's unclear: Whether a small helper reduces maintenance burden enough to justify an unexported function.
   - Recommendation (Claude's discretion): Inline in all four methods. The switch is 5 lines, the helper would cost an extra function signature and a `buf *encoding.Encbuf` parameter pass. Not worth it for 4 call sites.

---

## Sources

### Primary (HIGH confidence)
- `tsdb/record/record.go` lines 794-873 — `Samples()` dispatch and `samplesV2` implementation, the exact template for all V2 histogram methods
- `tsdb/record/record.go` lines 825-831 — `noST`, `sameST`, `explicitST` constants
- `tsdb/record/record.go` lines 931-987 — V1 `HistogramSamples` and `CustomBucketsHistogramSamples`, the filtering contract
- `tsdb/record/record.go` lines 1030-1087 — V1 `FloatHistogramSamples` and `CustomBucketsFloatHistogramSamples`
- `tsdb/record/record.go` lines 989-1128 — `EncodeHistogram` and `EncodeFloatHistogram` payload encoders
- `tsdb/record/record.go` lines 63-70 — Phase 1 complete: V2 type constants 12-15 confirmed present
- `tsdb/record/record.go` lines 205-216 — Phase 1 complete: `ST, T int64` fields confirmed on both sample structs

### Secondary (MEDIUM confidence)
- `.planning/phases/02-v2-encoders/02-CONTEXT.md` — user decisions on wire format, delta conventions, ST marker scheme

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all tooling is existing code in the same file
- Architecture: HIGH — the exact template (`samplesV2`) exists and is directly verifiable
- Pitfalls: HIGH — derived from direct code reading, not speculation

**Research date:** 2026-03-02
**Valid until:** N/A — this is internal code, no external dependency versioning
