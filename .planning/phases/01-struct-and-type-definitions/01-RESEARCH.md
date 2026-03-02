# Phase 1: Struct and Type Definitions - Research

**Researched:** 2026-03-02
**Domain:** Go struct field additions and iota-style constant declarations in `tsdb/record/record.go`
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- Use `ST, T int64` on one line, matching RefSample pattern exactly. ST comes before T.
- Both RefHistogramSample and RefFloatHistogramSample get the ST field.
- V2 type naming: `HistogramSamplesV2`, `FloatHistogramSamplesV2`, `CustomBucketsHistogramSamplesV2`, `CustomBucketsFloatHistogramSamplesV2`.
- Sequential numbering after SamplesV2 (11): types 12, 13, 14, 15.
- Order: HistogramSamplesV2 (12), FloatHistogramSamplesV2 (13), CustomBucketsHistogramSamplesV2 (14), CustomBucketsFloatHistogramSamplesV2 (15).
- Mirror V1: four separate V2 types, one for each V1 type.
- String() returns snake_case with `_v2` suffix (e.g., "histogram_samples_v2").

### Claude's Discretion

- Exact comment wording on new constants.
- Whether to update the struct doc comment beyond removing the TODO.

### Deferred Ideas (OUT OF SCOPE)

None. Discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| STRUCT-01 | RefHistogramSample has ST field (int64) alongside T | Lines 188-194 in record.go show current struct; RefSample (lines 166-170) shows exact pattern to mirror |
| STRUCT-02 | RefFloatHistogramSample has ST field (int64) alongside T | Lines 196-202 in record.go show current struct; same pattern as STRUCT-01 |
| TYPE-01 | HistogramSamplesV2 record type constant added (value 12) | Constants block lines 38-63; SamplesV2 = 11 is the predecessor |
| TYPE-02 | FloatHistogramSamplesV2 record type constant added (value 13) | Same constants block |
| TYPE-03 | CustomBucketsHistogramSamplesV2 record type constant added (value 14) | Same constants block |
| TYPE-04 | CustomBucketsFloatHistogramSamplesV2 record type constant added (value 15) | Same constants block |
| TYPE-05 | Type.String() returns correct names for new types | Switch at lines 65-92; add four new cases following histogram_samples naming pattern |
</phase_requirements>

---

## Summary

This phase is pure Go declaration work in a single file: `tsdb/record/record.go`. No logic changes, no new functions, no encoding or decoding. The task is to add `ST int64` fields to two structs, declare four new type constants, extend the `String()` switch, update the `Type()` validity check, and remove two TODO comments.

The existing code provides exact templates for everything. `RefSample` (line 166) shows the `ST, T int64` field layout. The `SamplesV2 = 11` constant (line 62) shows the comment and numbering style. The `Type.String()` switch (lines 65-92) shows the case pattern. The `Decoder.Type()` switch case (line 229) shows where new types are registered as valid.

The only judgment call the planner needs to make is comment wording (explicitly left to Claude's discretion in CONTEXT.md) and whether to refresh the struct doc comments beyond removing the TODOs. Both are trivial.

**Primary recommendation:** Make all changes in a single commit to `tsdb/record/record.go`. Verify with `go build ./tsdb/record/...` and `go vet ./tsdb/record/...`.

## Standard Stack

### Core
| Tool | Version | Purpose | Why Standard |
|------|---------|---------|--------------|
| Go stdlib | 1.23+ (module) | Language and type system | This is a Go project |
| `go build` | same | Compile-time verification | Catches type errors immediately |
| `go vet` | same | Static analysis | Project's stated verification method |

No external libraries are added in this phase. All changes are pure Go declarations.

**Verification commands:**
```bash
go build ./tsdb/record/...
go vet ./tsdb/record/...
```

## Architecture Patterns

### Pattern 1: Multi-field declaration on one line

The project uses grouped field declarations where fields share a type. The canonical example is `RefSample`:

```go
// Source: tsdb/record/record.go line 166-170
type RefSample struct {
    Ref   chunks.HeadSeriesRef
    ST, T int64
    V     float64
}
```

Apply this exactly to both histogram structs. `ST` precedes `T` on the same line.

### Pattern 2: Typed integer constants without iota

New record type constants follow the existing block style - explicit integer literals, not iota, with a comment for each:

```go
// Source: tsdb/record/record.go lines 38-63
// SamplesV2 is an enhanced sample record with an encoding scheme that allows storing float samples with timestamp and an optional ST per sample.
SamplesV2 Type = 11
```

New constants slot directly after `SamplesV2 = 11`. Each gets a brief comment describing the record type.

### Pattern 3: Type.String() switch case

New cases are added in the `func (rt Type) String() string` switch. Histogram V1 types use snake_case with underscores. V2 histogram types follow the same convention with `_v2` appended:

```go
// Source: tsdb/record/record.go lines 65-92
// Existing examples:
case HistogramSamples:
    return "histogram_samples"
case FloatHistogramSamples:
    return "float_histogram_samples"
case CustomBucketsHistogramSamples:
    return "custom_buckets_histogram_samples"
case CustomBucketsFloatHistogramSamples:
    return "custom_buckets_float_histogram_samples"

// New cases to add:
case HistogramSamplesV2:
    return "histogram_samples_v2"
case FloatHistogramSamplesV2:
    return "float_histogram_samples_v2"
case CustomBucketsHistogramSamplesV2:
    return "custom_buckets_histogram_samples_v2"
case CustomBucketsFloatHistogramSamplesV2:
    return "custom_buckets_float_histogram_samples_v2"
```

Note: `SamplesV2` uses "samples-v2" (hyphen) not "samples_v2". This is a known minor inconsistency. The decision is to use underscores for the new histogram V2 types to stay consistent with the histogram V1 naming style.

### Pattern 4: Decoder.Type() validity switch

The single case clause at line 229 lists all known valid types. New types are appended to this list:

```go
// Source: tsdb/record/record.go line 229 (current)
case Series, Samples, SamplesV2, Tombstones, Exemplars, MmapMarkers, Metadata, HistogramSamples, FloatHistogramSamples, CustomBucketsHistogramSamples, CustomBucketsFloatHistogramSamples:

// After change:
case Series, Samples, SamplesV2, Tombstones, Exemplars, MmapMarkers, Metadata, HistogramSamples, FloatHistogramSamples, CustomBucketsHistogramSamples, CustomBucketsFloatHistogramSamples, HistogramSamplesV2, FloatHistogramSamplesV2, CustomBucketsHistogramSamplesV2, CustomBucketsFloatHistogramSamplesV2:
```

### Anti-Patterns to Avoid

- **Renumbering existing constants:** Never. WAL format is on-disk. Renumbering would corrupt existing WALs.
- **Using iota:** The existing block uses explicit values for a reason. Keep it that way.
- **Adding ST field after T:** The decision locks ST before T, matching RefSample. Don't swap the order.
- **Naming inconsistency:** Don't mix hyphen and underscore in V2 histogram type strings. Use underscores throughout.
- **Forgetting Decoder.Type():** Adding constants and String() cases but not updating `Decoder.Type()` means the decoder will classify new record types as Unknown. That will cause silent data loss in Phase 3.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Compile check | Custom validator | `go build` | Compiler catches all type and field errors |
| Vet check | Custom linter | `go vet` | Already the project's stated acceptance criterion |

There is nothing to hand-roll in this phase. The work is declaration-only.

## Common Pitfalls

### Pitfall 1: Forgetting to update Decoder.Type()

**What goes wrong:** New constants compile fine, String() works, but `Decoder.Type()` still returns `Unknown` for types 12-15. Phase 3 decoder functions that switch on the type will never match, and records will be silently discarded or error-out with "invalid record type".

**Why it happens:** It's easy to focus on the visible constants block and String() switch and overlook the separate validity switch in `Decoder.Type()` at line 224.

**How to avoid:** The CONTEXT.md explicitly calls out line 229 as a touch point. Treat it as a checklist item in the task.

**Warning signs:** `dec.Type(encodedRecord)` returns `Unknown` for any of the four new type bytes (12-15).

### Pitfall 2: ST field position wrong

**What goes wrong:** If `T, ST int64` is written instead of `ST, T int64`, the struct memory layout differs from what Phase 2 encoders and Phase 3 decoders will assume, and any code that initializes the struct positionally (not by name) will silently assign values to the wrong fields.

**Why it happens:** Natural tendency to write `T` first since it's already there.

**How to avoid:** The decision explicitly states ST comes before T, matching RefSample at line 168. Cross-check the final diff.

**Warning signs:** Positional struct literal compilation errors if any callers use positional syntax (rare in this codebase but possible).

### Pitfall 3: TODO comment left in place

**What goes wrong:** The build passes, but the TODO comment remains. The task goal includes removing the TODOs.

**Why it happens:** Editing the field and forgetting to also edit the comment above it.

**How to avoid:** Each struct has a comment block directly above it. `RefHistogramSample` at line 188-189 and `RefFloatHistogramSample` at line 196-197. Both contain `// TODO(owilliams): Add support for ST.`. Remove both.

### Pitfall 4: String() naming inconsistency

**What goes wrong:** Using "histogram-samples-v2" (hyphens) instead of "histogram_samples_v2" (underscores) for new V2 histogram type strings.

**Why it happens:** `SamplesV2` uses "samples-v2" with a hyphen, which could be mistakenly treated as the V2 naming pattern.

**How to avoid:** The CONTEXT.md specifics section explicitly documents this: histogram V2 types should use underscores with v2 suffix. The hyphen in SamplesV2 is an acknowledged inconsistency.

## Code Examples

### Before and after: RefHistogramSample

```go
// BEFORE (lines 188-194)
// RefHistogramSample is a histogram.
// TODO(owilliams): Add support for ST.
type RefHistogramSample struct {
    Ref chunks.HeadSeriesRef
    T   int64
    H   *histogram.Histogram
}

// AFTER
// RefHistogramSample is a histogram.
type RefHistogramSample struct {
    Ref    chunks.HeadSeriesRef
    ST, T  int64
    H      *histogram.Histogram
}
```

### Before and after: RefFloatHistogramSample

```go
// BEFORE (lines 196-202)
// RefFloatHistogramSample is a float histogram.
// TODO(owilliams): Add support for ST.
type RefFloatHistogramSample struct {
    Ref chunks.HeadSeriesRef
    T   int64
    FH  *histogram.FloatHistogram
}

// AFTER
// RefFloatHistogramSample is a float histogram.
type RefFloatHistogramSample struct {
    Ref   chunks.HeadSeriesRef
    ST, T int64
    FH    *histogram.FloatHistogram
}
```

### New constants block (append after SamplesV2 = 11 at line 62)

```go
// HistogramSamplesV2 is an enhanced histogram record that supports start time per sample.
HistogramSamplesV2 Type = 12
// FloatHistogramSamplesV2 is an enhanced float histogram record that supports start time per sample.
FloatHistogramSamplesV2 Type = 13
// CustomBucketsHistogramSamplesV2 is an enhanced custom-buckets histogram record that supports start time per sample.
CustomBucketsHistogramSamplesV2 Type = 14
// CustomBucketsFloatHistogramSamplesV2 is an enhanced custom-buckets float histogram record that supports start time per sample.
CustomBucketsFloatHistogramSamplesV2 Type = 15
```

### New String() cases (insert in func (rt Type) String(), after existing histogram cases)

```go
case HistogramSamplesV2:
    return "histogram_samples_v2"
case FloatHistogramSamplesV2:
    return "float_histogram_samples_v2"
case CustomBucketsHistogramSamplesV2:
    return "custom_buckets_histogram_samples_v2"
case CustomBucketsFloatHistogramSamplesV2:
    return "custom_buckets_float_histogram_samples_v2"
```

### Updated Decoder.Type() case (line 229)

```go
// Source: tsdb/record/record.go line 228-230
case Series, Samples, SamplesV2, Tombstones, Exemplars, MmapMarkers, Metadata,
    HistogramSamples, FloatHistogramSamples, CustomBucketsHistogramSamples, CustomBucketsFloatHistogramSamples,
    HistogramSamplesV2, FloatHistogramSamplesV2, CustomBucketsHistogramSamplesV2, CustomBucketsFloatHistogramSamplesV2:
    return t
```

## State of the Art

| Old Approach | Current Approach | Impact |
|--------------|------------------|--------|
| RefHistogramSample with T only | RefHistogramSample with ST, T (this phase) | Enables start-time propagation in later phases |
| Types 1-11 only | Types 1-15 (this phase) | New record types needed for V2 encoding in Phase 2 |

No deprecated patterns introduced. This phase is purely additive.

## Open Questions

None. The CONTEXT.md resolves all ambiguities. The code is fully visible and the patterns are unambiguous.

## Sources

### Primary (HIGH confidence)
- `tsdb/record/record.go` (read in full) - all patterns, line numbers, and existing types verified directly from source
- `tsdb/record/record_test.go` (read partial) - test framework and patterns confirmed

### Secondary (MEDIUM confidence)
- `.planning/phases/01-struct-and-type-definitions/01-CONTEXT.md` - user decisions verified against source code

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - verified by reading the actual source file
- Architecture: HIGH - patterns extracted directly from live source, not docs
- Pitfalls: HIGH - derived from reading actual code structure and call sites

**Research date:** 2026-03-02
**Valid until:** 2026-04-01 (stable Go internal package; changes here would come from the same developer)
