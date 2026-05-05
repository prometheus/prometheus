# Native Stateset — Step-by-Step Implementation Guide

This document gives a developer-ready ordered task list for implementing native
statesets as described in `docs/native-stateset-proposal.md`. Each step is
independently compilable when complete. Steps within a phase may be done in
sequence; phases must be done in the order shown.

Compilation checkpoints are marked **[BUILD]**. Test checkpoints are marked
**[TEST]**.

---

## Phase 1 — Data model (`model/stateset/`)

No dependencies on other Prometheus packages. Complete this first; everything
else imports it.

### Step 1 — Create the `StateSet` type

**New file**: `model/stateset/stateset.go`

```go
package stateset

import "sort"

// StateSet represents a single stateset sample. LabelName is the label name
// used to carry state values in the legacy OM1 wire format (e.g.
// "kube_pod_status_phase"). It is stored in chunk metadata, not in the TSDB
// series labelset. Names must be sorted lexicographically and contain at most
// 64 entries. Bit i of Values is set if Names[i] is an active state.
type StateSet struct {
    LabelName string
    Names     []string
    Values    uint64
}

// IsActive reports whether the named state is active.
func (s *StateSet) IsActive(name string) bool {
    for i, n := range s.Names {
        if n == name {
            return s.Values>>uint(i)&1 == 1
        }
    }
    return false
}

// ActiveNames returns the names of all active states.
func (s *StateSet) ActiveNames() []string {
    out := make([]string, 0, len(s.Names))
    for i, n := range s.Names {
        if s.Values>>uint(i)&1 == 1 {
            out = append(out, n)
        }
    }
    return out
}

// Copy returns a deep copy of s.
func (s *StateSet) Copy() *StateSet {
    c := &StateSet{
        LabelName: s.LabelName,
        Names:     make([]string, len(s.Names)),
        Values:    s.Values,
    }
    copy(c.Names, s.Names)
    return c
}

// Equals reports whether s and o represent the same stateset sample.
func (s *StateSet) Equals(o *StateSet) bool {
    if s.LabelName != o.LabelName || s.Values != o.Values ||
        len(s.Names) != len(o.Names) {
        return false
    }
    for i := range s.Names {
        if s.Names[i] != o.Names[i] {
            return false
        }
    }
    return true
}

// Sorted reports whether Names is in lexicographic order (required invariant).
func (s *StateSet) Sorted() bool {
    return sort.StringsAreSorted(s.Names)
}
```

### Step 2 — Add unit tests

**New file**: `model/stateset/stateset_test.go`

Cover: `IsActive`, `ActiveNames`, `Copy`, `Equals`, `Sorted`. Test with zero
states, one state, 64 states (boundary), and mismatched `LabelName`.

**[BUILD]** `go build ./model/stateset/...`  
**[TEST]** `go test ./model/stateset/...`

---

## Phase 2 — Chunk encoding (`tsdb/chunkenc/`)

### Step 3 — Add `EncStateset` and `ValStateset` constants

**File**: `tsdb/chunkenc/chunk.go`

1. After `EncXOR2 Encoding = 4`, add:
   ```go
   EncStateset Encoding = 5
   ```

2. In the `String()` method for `Encoding`, add:
   ```go
   case EncStateset:
       return "stateset"
   ```

3. Update `IsValidEncoding`:
   ```go
   return e == EncXOR || e == EncHistogram || e == EncFloatHistogram ||
       e == EncXOR2 || e == EncStateset
   ```

4. After `ValFloatHistogram ValueType = iota` (line ~176), add:
   ```go
   ValStateset // A stateset, retrieve with AtStateset.
   ```

5. In the `String()` method for `ValueType`, add:
   ```go
   case ValStateset:
       return "stateset"
   ```

6. In `ValueType.ChunkEncoding()`, add before the `default` return:
   ```go
   case ValStateset:
       return EncStateset
   ```

### Step 4 — Add `AtStateset` to the `Iterator` interface

**File**: `tsdb/chunkenc/chunk.go`

In the `Iterator` interface (around line 128), add after `AtFloatHistogram`:

```go
// AtStateset returns the current stateset. The caller must not retain
// references to the returned value after the next call to Next or Seek.
AtStateset(*stateset.StateSet) (int64, *stateset.StateSet)
```

Add the corresponding stub to `mockSeriesIterator` and `nopIterator`:

```go
func (*mockSeriesIterator) AtStateset(*stateset.StateSet) (int64, *stateset.StateSet) {
    return 0, nil
}
func (nopIterator) AtStateset(*stateset.StateSet) (int64, *stateset.StateSet) {
    return 0, nil
}
```

**[BUILD]** `go build ./tsdb/chunkenc/...` — will fail until all `Iterator`
implementations in the repo add `AtStateset`. Fix each by adding the same
`return 0, nil` stub. Run:

```
grep -rn "func.*Iterator\b" tsdb/ promql/ storage/ | grep -v "_test.go"
```

to find all implementations that need updating.

### Step 5 — Implement `StateSetChunk`

**New file**: `tsdb/chunkenc/stateset.go`

Wire layout (matches proposal):
```
[label_name_len u8][label_name bytes]
[n u16]                                  -- number of state names
[name_i_len u8][name_i bytes] * n        -- state names, lexicographically sorted
[samples]                                -- per-sample: delta-of-delta timestamp +
                                            XOR of consecutive bitsets
```

The `Appender.AppendStateset(t int64, ss *stateset.StateSet)` method:
- Returns a new `StateSetChunk` if `ss.Names` differs from the chunk's names
  list (recode trigger, same as histogram schema change).
- Otherwise appends the XOR delta of `ss.Values` against the previous value.

The `Iterator.AtStateset` method:
- Decodes the current timestamp and reconstructs the bitset from the running
  XOR state.
- Populates the passed `*stateset.StateSet` in-place (or allocates if nil),
  sharing the chunk's `Names` slice (read-only after construction).

The chunk's `Appender` interface must also satisfy the existing
`AppendHistogram` and `AppendFloatHistogram` methods by returning
`(nil, false, nil, ErrOutOfBounds)` or similar (same pattern as `XORChunk`
does for histograms).

**[BUILD]** `go build ./tsdb/chunkenc/...`

### Step 6 — Register `EncStateset` in the chunk pool

**File**: `tsdb/chunkenc/chunk.go`, `pool.Get` method (line ~331)

Add after the `EncXOR2` case:

```go
case EncStateset:
    c = &StateSetChunk{}
    c.Reset(b)
```

Add matching cases in `pool.Put` and any other `switch e { case Enc... }`
blocks in the same file (check lines ~354, ~386, ~401).

**[BUILD]** `go build ./tsdb/chunkenc/...`  
**[TEST]** `go test ./tsdb/chunkenc/...`

Add round-trip test in `tsdb/chunkenc/stateset_test.go`:
- Append N samples with stable state names → decode → verify.
- Append a sample with a new state name → verify recode returns new chunk.
- Verify XOR delta correctness across repeated identical bitsets (most common
  case: only one bit changes at a time).

---

## Phase 3 — WAL record types (`tsdb/record/`)

### Step 7 — Add the record type constant and sample struct

**File**: `tsdb/record/record.go`

1. After `SamplesV2 Type = 11`, add:
   ```go
   StatesetSamples Type = 12
   ```

2. After `RefFloatHistogramSample`, add:
   ```go
   // RefStatesetSample is a stateset sample with a series reference.
   type RefStatesetSample struct {
       Ref chunks.HeadSeriesRef
       T   int64
       SS  *stateset.StateSet
   }
   ```

### Step 8 — Implement `EncodeStatesets` and `DecodeStatesets`

**File**: `tsdb/record/record.go` (or a new `tsdb/record/stateset.go`)

Follow `EncodeHistogramSamples` / `DecodeHistogramSamples` as the template.

Wire format per sample:
```
[ref u64][t varint][label_name_len u16][label_name bytes]
[n u16]
[name_i_len u16][name_i bytes] * n
[values u64]
```

`DecodeStatesets` must reconstruct `*stateset.StateSet` fully, including
`LabelName` and sorted `Names`.

**[BUILD]** `go build ./tsdb/record/...`  
**[TEST]** `go test ./tsdb/record/...`

Add encode/decode round-trip test in `tsdb/record/record_test.go` covering:
zero states, one state, 64 states, multiple samples in one record.

---

## Phase 4 — Storage interface (`storage/`)

### Step 9 — Extend `AppenderV2.Append`

**File**: `storage/interface_append.go`

Add `ss *stateset.StateSet` parameter to `AppenderV2.Append`:

```go
Append(ref SeriesRef, ls labels.Labels, st, t int64, v float64,
       h *histogram.Histogram, fh *histogram.FloatHistogram,
       ss *stateset.StateSet, opts AppendV2Options) (SeriesRef, error)
```

Update the dispatch comment:

```go
// switch {
//  case ss != nil: stateset append.
//  case fh != nil: float histogram append.
//  case h != nil:  integer histogram append.
//  default:        float append.
// }
```

### Step 10 — Update all `AppenderV2` implementations

Run:
```
grep -rn "AppenderV2\b" --include="*.go" . | grep -v "_test.go"
```

For each implementation, add `ss *stateset.StateSet` to the `Append` signature.
For implementations that are stubs or no-ops (fanout, noop, etc.) just add the
parameter and ignore it for now. The TSDB head implementation is handled in
Phase 5.

**[BUILD]** `go build ./...`

This is the first full-repo compilation checkpoint. Fix all signature
mismatches before proceeding.

---

## Phase 5 — TSDB head (`tsdb/`)

### Step 11 — Add `stStateset` to the `sampleType` enum

**File**: `tsdb/head_append.go` (line ~354)

```go
stStateset // Native statesets. Goes to `statesets`.
```

### Step 12 — Add stateset buffers to `appendBatch`

**File**: `tsdb/head_append.go` (line ~369)

```go
statesets       []record.RefStatesetSample
statesetSeries  []*memSeries
```

Also update:
- `appendBatch.close()` — add `h.putStatesetBuffer(b.statesets)` and
  `h.putSeriesBuffer(b.statesetSeries)`, set both to nil.
- The batch initialisation block (~line 570) — add
  `statesets: h.getStatesetBuffer()` and
  `statesetSeries: h.getSeriesBuffer()`.

**File**: `tsdb/head.go`

Add pool methods parallel to `getHistogramBuffer` / `putHistogramBuffer`:

```go
func (h *Head) getStatesetBuffer() []record.RefStatesetSample { ... }
func (h *Head) putStatesetBuffer(b []record.RefStatesetSample) { ... }
```

Use a `sync.Pool` with the same pattern as the histogram pool.

### Step 13 — Add `stStateset` to `getCurrentBatch`

**File**: `tsdb/head_append.go` (line ~619)

In the `getCurrentBatch` switch, add a case that treats `stStateset` as its
own batch type (does not share batches with floats or histograms). Follow the
`stHistogram` case as a template.

### Step 14 — Add `lastStatesetValue` to `memSeries`

**File**: `tsdb/head.go` (wherever `memSeries` is defined, search for
`lastHistogramValue`)

```go
lastStatesetValue *stateset.StateSet
```

Update `memSeries.chunk()` encoding switch to map `stStateset` → `EncStateset`.

### Step 15 — Add the stateset append path

**File**: `tsdb/head_append.go`, in the `Append` method

After the `fh != nil` block, add:

```go
case ss != nil:
    if err := validateStateset(ss); err != nil {
        return 0, err
    }
    st = stStateset
    b := a.getCurrentBatch(stStateset, s.ref)
    b.statesets = append(b.statesets, record.RefStatesetSample{
        Ref: s.ref,
        T:   t,
        SS:  ss,
    })
    b.statesetSeries = append(b.statesetSeries, s)
```

`validateStateset` checks: `len(ss.Names) <= 64`, `ss.Sorted()`, non-empty
`LabelName`. Returns `ErrTooManyStates` (new sentinel error) if limit exceeded.

### Step 16 — Implement `commitStatesets`

**File**: `tsdb/head_append.go`

Add `commitStatesets(b *appendBatch, acc *appenderCommitContext)` parallel to
`commitHistograms` (line 1470). This method:

1. Iterates `b.statesets` with corresponding `b.statesetSeries`.
2. For each sample, locks the series, calls `s.app.AppendStateset(t, ss)`.
3. If `AppendStateset` returns a new chunk (recode), calls
   `h.getOrCreateWithID` to register the new chunk.
4. Updates `s.lastStatesetValue = ss`.
5. Tracks `acc.statesetsAppended`.

### Step 17 — WAL encode in `Commit`

**File**: `tsdb/head_append.go`, in the `Commit` method

After `a.commitHistograms(b, acc)` and `a.commitFloatHistograms(b, acc)`, add:

```go
a.commitStatesets(b, acc)
```

In the WAL logging section, encode and log the stateset batch:

```go
if len(b.statesets) > 0 {
    encoded = r.EncodeStatesets(b.statesets, encoded[:0])
    if err := a.head.wal.Log(encoded); err != nil {
        return err
    }
}
```

### Step 18 — WAL replay

**File**: `tsdb/head.go`, in `loadWAL` (or wherever histogram samples are
replayed, search for `record.HistogramSamples`)

Add a case for `record.StatesetSamples`:

```go
case record.StatesetSamples:
    samples, err := dec.Statesets(rec, nil)
    // ... same error handling as histogram replay ...
    for _, s := range samples {
        ms := h.series.getByID(s.Ref)
        if ms == nil {
            continue
        }
        if _, err := ms.app.AppendStateset(s.T, s.SS); err != nil {
            // handle
        }
    }
```

**[BUILD]** `go build ./tsdb/...`  
**[TEST]** `go test ./tsdb/...`

Add tests in `tsdb/head_test.go`:
- Append stateset samples, verify they appear in a chunk query.
- Append statesets that trigger a recode (new state name), verify two chunks.
- Simulate WAL replay: write statesets, truncate in-memory state, replay WAL,
  verify samples recovered.
- Verify >64 states returns `ErrTooManyStates`.

---

## Phase 6 — Scrape-time aggregation (`model/textparse/`, `scrape/`)

### Step 19 — Add `EntryStateset` and `Stateset()` to the parser interface

**File**: `model/textparse/interface.go`

1. After `EntryHistogram Entry = 5`, add:
   ```go
   EntryStateset Entry = 6
   ```

2. In the `Parser` interface, add after `Histogram()`:
   ```go
   // Stateset returns the bytes of the metric name, an optional timestamp,
   // and the aggregated stateset for the current EntryStateset entry.
   Stateset() ([]byte, *int64, *stateset.StateSet)
   ```

3. Add stub to all existing parser types
   (`openMetricsParser`, `promParser`, `protobufParser`):

   ```go
   func (p *openMetricsParser) Stateset() ([]byte, *int64, *stateset.StateSet) {
       return nil, nil, nil
   }
   ```

**[BUILD]** `go build ./model/textparse/...`

### Step 20 — Implement `StateSetParser`

**New file**: `model/textparse/statesetparse.go`

State machine (four states, same pattern as `NHCBParser`):

```go
const (
    sssStart      statesetParserState = iota
    sssCollecting                     // accumulating state series for current labelset
    sssEmitting                       // ready to return EntryStateset
)
```

Key logic:

- On `EntryType` with `model.MetricTypeStateset`: enable collection for that
  metric name.
- On `EntrySeries` for an enabled metric name: extract the state-carrying label
  (the label whose name equals the base metric name, per OM1 convention), strip
  it from the accumulated label set, accumulate `(state_name, value)` pair.
- When the accumulated label set changes or a non-stateset entry is encountered:
  build a `StateSet` (sort names, assign bits to states with value > 0.5),
  transition to `sssEmitting`.
- `Stateset()` returns the built `StateSet` and transitions back to `sssStart`.

Edge cases to handle:
- Missing 0-valued placeholders: collect only what arrives; unknown states are
  implicitly inactive.
- EOF mid-collection: emit the partial stateset.
- Mixed-version emitters in one scrape (different state-carrying label names for
  the same base metric name): treat as separate metrics (different labelsets
  after stripping).

### Step 21 — Add `ParserOptions` flag and wire up in `NewParser`

**File**: `model/textparse/interface.go`

```go
// ConvertStateSetsToNative enables aggregation of OM1 stateset series into
// native stateset samples (analogous to ConvertClassicHistogramsToNHCB).
ConvertStateSetsToNative bool
```

**File**: `model/textparse/interface.go` or wherever `NewParser` lives:

```go
if opts.ConvertStateSetsToNative {
    p = NewStateSetParser(p)
}
```

Apply after any NHCB wrapping so both can be active simultaneously.

**[BUILD]** `go build ./model/textparse/...`  
**[TEST]** `go test ./model/textparse/...`

Add tests in `model/textparse/statesetparse_test.go`:
- Full stateset with all 0-valued placeholders present.
- Stateset with only the active state emitted (no 0-valued placeholders).
- Multiple independent statesets in one scrape.
- Stateset immediately followed by a different metric type.
- EOF mid-stateset.

### Step 22 — Wire into the scrape loop

**File**: `scrape/scrape.go`

1. Add to `scrapeLoop` struct (line ~865 area):
   ```go
   convertStateSetToNative bool
   ```

2. Set from config (line ~1220 area):
   ```go
   convertStateSetToNative: opts.sp.config.ConvertStateSetsToNativeEnabled(),
   ```

3. Pass through `ParserOptions` (line ~1605 area):
   ```go
   ConvertStateSetsToNative: sl.convertStateSetToNative,
   ```

4. In the scrape loop entry dispatch (~line 1773), add:
   ```go
   case textparse.EntryStateset:
       _, ts, ss := p.Stateset()
       t := defTime
       if ts != nil {
           t = *ts
       }
       lset = lset[:0] // already populated without the state label
       if sl.honorTimestamps {
           // ... timestamp handling ...
       }
       ref, err = app.Append(ref, lset, startTimestamp, t, 0, nil, nil, ss, appOpts)
   ```

**File**: `config/config.go`

Add `ConvertStateSetsToNative bool` to `ScrapeConfig` with YAML tag
`convert_stateset_to_native`.

**[BUILD]** `go build ./scrape/... ./config/...`  
**[TEST]** `go test ./scrape/...`

---

## Phase 7 — PromQL (`promql/`)

### Step 23 — Add `ValueTypeStateset` to the parser value types

**File**: `promql/parser/value.go` (line ~27)

```go
ValueTypeStateset ValueType = "stateset"
```

### Step 24 — Add `SS` field to `Sample`

**File**: `promql/value.go`

In the `Sample` struct, after `H *histogram.FloatHistogram`:

```go
SS *stateset.StateSet // Non-nil for stateset samples.
```

Update any JSON marshalling, string formatting, or equality helpers on
`Sample` to handle the `SS` field.

### Step 25 — Thread `ValStateset` through the eval engine

**File**: `promql/engine.go`

In `eval()` and any other switch on `chunkenc.ValueType` or `ValueType`, add
`ValStateset` / `ValueTypeStateset` cases. For now, stateset series may be
treated as empty in contexts that don't understand them (return no samples),
matching how histograms behave in float-only contexts.

Update `populateSeries` or equivalent to propagate `SS` from chunk iterators
into `Sample.SS`.

**[BUILD]** `go build ./promql/...`

### Step 26 — Implement `stateset_is_active`

**File**: `promql/functions.go`

```go
func funcStatesetIsActive(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) Vector {
    stateName := stringFromArg(args[1])
    for _, el := range vals[0].(Vector) {
        if el.SS == nil {
            continue
        }
        v := 0.0
        if el.SS.IsActive(stateName) {
            v = 1.0
        }
        enh.Out = append(enh.Out, Sample{Metric: el.Metric, F: v, DropName: true})
    }
    return enh.Out
}
```

Register in `FunctionCalls` and add signature in `promql/parser/functions.go`:

```go
"stateset_is_active": {
    Name:       "stateset_is_active",
    ArgTypes:   []parser.ValueType{parser.ValueTypeVector, parser.ValueTypeString},
    ReturnType: parser.ValueTypeVector,
},
```

### Step 27 — Implement `stateset_active_states`

**File**: `promql/functions.go`

For each input `Sample` with non-nil `SS`:
- For each active state in `ss.ActiveNames()`:
  - Clone the sample's `Metric` labels.
  - Add label `ss.LabelName = state_name`.
  - Emit a `Sample` with value `1.0`.

```go
func funcStatesetActiveStates(vals []parser.Value, args parser.Expressions, enh *EvalNodeHelper) Vector {
    for _, el := range vals[0].(Vector) {
        if el.SS == nil {
            continue
        }
        for _, name := range el.SS.ActiveNames() {
            m := labels.NewBuilder(el.Metric).
                Set(el.SS.LabelName, name).
                Labels()
            enh.Out = append(enh.Out, Sample{Metric: m, F: 1.0})
        }
    }
    return enh.Out
}
```

Register with signature `(vector) → vector`.

### Step 28 — Implement `stateset_known_states`

**File**: `promql/functions.go`

Same as `stateset_active_states` but iterates `ss.Names` and emits `1.0` for
active, `0.0` for inactive.

Register with signature `(vector) → vector`.

**[BUILD]** `go build ./promql/...`  
**[TEST]** `go test ./promql/...`

Add PromQL test cases in `promql/promqltest/testdata/`:

```
# native stateset basics
load 1m
    kube_pod_status_phase{{pod="a"}} {{stateset LabelName="kube_pod_status_phase" Names=["Failed","Pending","Running","Succeeded","Unknown"] Values=0b00100}}

eval instant at 0m stateset_is_active(kube_pod_status_phase, "Running")
    {pod="a"} 1

eval instant at 0m stateset_is_active(kube_pod_status_phase, "Pending")
    {pod="a"} 0

eval instant at 0m stateset_active_states(kube_pod_status_phase)
    {pod="a", kube_pod_status_phase="Running"} 1
```

(The exact `load` syntax for statesets will need to be defined; follow the
pattern used for native histograms in the test format.)

---

## Phase 8 — Remote write v2

### Step 29 — Extend the proto schema

**File**: `prompb/io/prometheus/write/v2/types.proto`

Add a message for a stateset metadata chunk (sent once per `TimeSeries`):

```proto
message StatesetMetadata {
    string label_name = 1;
    repeated string state_names = 2; // sorted lexicographically
}

message StatesetSample {
    int64 timestamp_ms = 1;
    uint64 values = 2; // bitset; bit i corresponds to state_names[i]
}
```

Extend `TimeSeries`:

```proto
optional StatesetMetadata stateset_metadata = 8;
repeated StatesetSample stateset_samples = 9;
```

Regenerate Go code: `make proto` (or equivalent).

### Step 30 — Encode statesets in the remote write client

**File**: `storage/remote/write.go` (or wherever samples are serialised into
`TimeSeries` proto messages)

When a chunk iterator returns `ValStateset` samples:
1. Populate `StatesetMetadata` from the first sample's `LabelName` and `Names`.
2. For each sample, emit a `StatesetSample{TimestampMs: t, Values: ss.Values}`.

### Step 31 — Decode statesets on the receive side

**File**: `storage/remote/receive.go` (or equivalent)

When a `TimeSeries` has `stateset_metadata` set:
1. Reconstruct `*stateset.StateSet` for each `StatesetSample` using the shared
   metadata.
2. Call `app.Append(..., ss)`.

### Step 32 — v1 downgrade path

**File**: `storage/remote/write.go`

When writing to a v1 endpoint that does not support statesets, explode each
native stateset sample into legacy float series:
- For each `name` in `ss.Names`: emit `{...labels, ss.LabelName=name}` with
  value `1.0` if active, `0.0` otherwise.

This matches the behaviour of v1 downgrade for native histograms.

**[BUILD]** `go build ./storage/remote/...`  
**[TEST]** `go test ./storage/remote/...`

---

## Phase 9 — Benchmarks

### Step 33 — Write storage benchmark

**File**: `tsdb/head_bench_test.go` (or new `tsdb/stateset_bench_test.go`)

Benchmark appending and querying a 5-state stateset (matching `kube_pod_status_phase` shape)
vs. the equivalent 5 legacy float gauge series.

Measure: chunk count, WAL bytes written, query time for
`stateset_active_states`.

Run before and after with:

```
go test -count=6 -benchmem -bench BenchmarkStateset ./tsdb/...
benchstat before.txt after.txt
```

Include results in the PR body.

---

## Full compilation order

```
Step  1–2   model/stateset            go build + go test
Step  3–6   tsdb/chunkenc             go build + go test
Step  7–8   tsdb/record               go build + go test
Step  9–10  storage                   go build (full repo)
Step 11–18  tsdb (head)               go build + go test
Step 19–22  model/textparse + scrape  go build + go test
Step 23–28  promql                    go build + go test
Step 29–32  storage/remote            go build + go test
Step 33     benchmarks                go test -bench
```

---

## Suggested PR split

| PR | Steps | Title |
|---|---|---|
| 1 | 1–8 | `feat(stateset): data model, chunk encoding, WAL record types` |
| 2 | 9–10 | `feat(stateset): extend AppenderV2 interface` |
| 3 | 11–18 | `feat(stateset): TSDB head append, commit, and WAL replay` |
| 4 | 19–22 | `feat(stateset): scrape-time OM1→native aggregation` |
| 5 | 23–28 | `feat(stateset): PromQL stateset_* functions` |
| 6 | 29–32 | `feat(stateset): remote write v2 encode/decode` |
| 7 | 33 | `perf(stateset): storage and query benchmarks` |

PRs 1 and 2 are the preparatory PRs that unblock parallel work on PRs 3, 4,
and 5. PRs 3–5 can be reviewed in parallel once PR 2 merges. PR 6 depends on
PR 3. PR 7 can be opened against any of PRs 3–5.
