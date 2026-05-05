# Native Stateset in Prometheus — Implementation Plan

## Background and scope

[Issue #17914](https://github.com/prometheus/prometheus/issues/17914) proposes
storing OpenMetrics statesets natively in the TSDB rather than as a set of
independent float gauge series. The problems with the current approach are:

- **Cardinality explosion**: N states → N separate TSDB series per labelset.
- **Transactionality**: states can be observed in a split/inconsistent state
  across scrapes.
- **Staleness fragility**: determining "what state is active right now" requires
  working around staleness semantics.
- **Storage waste**: 0-valued placeholder series must be emitted to allow
  correct queries.

OpenMetrics 2.0 RC.0 deliberately kept the existing per-series representation
in the wire format, so native statesets must live entirely inside Prometheus.
The implementation follows the NHCB (native histogram from classic buckets)
pattern: the existing wire format is preserved and scrape-time aggregation
produces a new internal sample type.

---

## Design decisions

### Sample representation

A stateset sample is a struct (like `histogram.Histogram`) that is
self-describing:

```go
// model/stateset/stateset.go
type StateSet struct {
    LabelName string   // Label name used for states in the legacy OM1 representation
                       // (e.g. "kube_pod_status_phase"). Stored in the chunk header;
                       // NOT stored as a series label.
    Names     []string // State names; Names[i] corresponds to bit i of Values.
    Values    uint64   // Bitset; bit i is 1 if Names[i] is active.
}
```

`LabelName` is stored in the chunk header so that explosion to the legacy
representation and `stateset_active_states` output are lossless. It is
intentionally **not** stored as a series label: `__*__` labels are stripped at
remote write boundaries and affect series identity, making them unsuitable for
carrying per-chunk metadata. Two native stateset series that differ only in
`LabelName` are the same TSDB series — they would never be produced by a
correct scrape anyway, since `LabelName` is fixed at scrape configuration time.

`Names` is ordered and stable within a chunk. When new states appear the chunk
is closed and a new one opened (same recoding pattern as native histograms).
This keeps each chunk self-contained (no out-of-line metadata store required)
and matches the Parquet-friendliness goal mentioned in the issue.

Maximum 64 states per series (uint64 limit). Series with more states are
dropped with a per-scrape counter and a `__scrape_series_dropped__` annotation,
matching the histogram overflow approach.

### Encoding in chunks

Chunk header: length-prefixed array of state name strings, sorted
lexicographically (to give stable bit-position assignment regardless of the
order in which an emitter lists states).

Each sample: XOR of consecutive bitsets (delta encoding), giving very compact
storage for slowly-changing state. Timestamps use the same delta-of-delta
encoding as the XOR chunk.

### Scrape-time conversion

Controlled by a per-scrape-config flag `convert_stateset_to_native`, analogous
to `convert_classic_histograms_to_nhcb`. A new parser wrapper
(`StateSetParser`, modelled on `NHCBParser` in
`model/textparse/nhcbparse.go`) aggregates the individual float series for one
stateset metric into a single `EntryStateset`. The state-carrying label name
(e.g. `kube_pod_status_phase`) is inferred from the label whose name matches
the metric name, as per OM1 convention.

### PromQL exposure

Native statesets are **not** silently exploded into legacy series during
queries (unlike the implicit `_bucket` approach for native histograms).
Instead, explicit PromQL functions provide ergonomic access. Legacy queries on
the unconverted float series continue to work when conversion is not enabled.

---

## Phase 1 — Data model (`model/stateset/`)

**New file**: `model/stateset/stateset.go`

```go
package stateset

// StateSet represents a single stateset sample. Names must be sorted
// lexicographically and have at most 64 entries. Bit i of Values is 1 if
// Names[i] is an active state. LabelName is stored in chunk metadata only;
// callers must not include it in the TSDB series labelset.
type StateSet struct {
    LabelName string
    Names     []string
    Values    uint64
}

func (s *StateSet) IsActive(name string) bool { ... }
func (s *StateSet) ActiveNames() []string { ... }
func (s *StateSet) Copy() *StateSet { ... }
func (s *StateSet) Equals(o *StateSet) bool { ... }
```

Key constraints to document: `Names` must have ≤ 64 entries; `Names` must be
sorted lexicographically (to give stable bit-position assignment regardless of
the order in which an emitter lists states).

---

## Phase 2 — Chunk encoding (`tsdb/chunkenc/`)

**New file**: `tsdb/chunkenc/stateset.go`

Implement `StateSetChunk` satisfying the `Chunk` interface.

Chunk wire layout:

```
[label_name: len u8, bytes]  -- state-carrying label name (e.g. "kube_pod_status_phase")
[n uint16]                   -- number of state names
[name_i: len u8, bytes]      -- state names, sorted lexicographically
[samples: ...]               -- XOR-delta-encoded bitsets, one per sample
                                (timestamp delta-of-delta as per XOR chunk)
```

`label_name` is stored here rather than as a series label so that it (a) is
not stripped at remote write boundaries, (b) does not affect TSDB series
identity, and (c) travels with the chunk through compaction and remote read
without extra coordination.

**Changes to `tsdb/chunkenc/chunk.go`:**

1. Add `EncStateset Encoding = 5` (after the existing `EncXOR2 = 4`).
2. Add `ValStateset ValueType` to the `ValueType` enum.
3. Update `IsValidEncoding()` to include `EncStateset`.
4. Update `ValueType.ChunkEncoding()` switch to map `ValStateset` →
   `EncStateset`.
5. Add `AtStateset(*stateset.StateSet) (int64, *stateset.StateSet)` to the
   `Iterator` interface.

The `StateSetChunk.Appender` triggers a recode (new chunk) when a sample
introduces a state name not in the chunk header, analogous to histogram schema
recoding in `tsdb/chunkenc/histogram.go`.

---

## Phase 3 — WAL record types (`tsdb/record/`)

**Changes to `tsdb/record/record.go`:**

```go
// New record type constant.
StatesetSamples Type = 12

// New sample struct.
type RefStatesetSample struct {
    Ref chunks.HeadSeriesRef
    T   int64
    SS  *stateset.StateSet
}
```

Add `EncodeStatesets` / `DecodeStatesets` functions following the pattern of
`EncodeHistogramSamples` / `DecodeHistogramSamples`. The state names array in
each sample is length-prefixed. A future optimisation could deduplicate the
names array across samples within one WAL record (names are stable within a
scrape).

---

## Phase 4 — Storage interface (`storage/`)

**Changes to `storage/interface_append.go`:**

The `AppenderV2.Append` signature gains a `*stateset.StateSet` parameter:

```go
Append(ref SeriesRef, ls labels.Labels, st, t int64, v float64,
       h *histogram.Histogram, fh *histogram.FloatHistogram,
       ss *stateset.StateSet, opts AppendV2Options) (SeriesRef, error)
```

Type dispatch comment updated:
```go
// switch {
//  case ss != nil: stateset append.
//  case fh != nil: float histogram append.
//  case h != nil:  integer histogram append.
//  default:        float append.
// }
```

All existing `AppenderV2` implementations (head, OOO, fanout, remote write,
noop) gain the `ss` parameter. The legacy `Appender` interface is unchanged —
statesets are a new feature, not backported to the deprecated interface.

---

## Phase 5 — TSDB head (`tsdb/`)

### `tsdb/head_append.go`

1. Add `stStateset sampleType` to the `sampleType` enum (currently at line 348).
2. Extend `appendBatch` struct:
   ```go
   statesets       []record.RefStatesetSample
   statesetSeries  []*memSeries
   ```
3. Add a `stStateset` case to `getCurrentBatch()` (the batch boundary selector,
   currently at line ~610).
4. In `Append`, add a stateset branch:
   ```go
   case ss != nil:
       st = stStateset
       // lookup/create series, validate, batch
   ```
5. Implement `commitStatesets(b *appendBatch, acc *appenderCommitContext)`,
   parallel to `commitHistograms`.
6. Update the WAL commit to call `r.EncodeStatesets(b.statesets)`.

### `tsdb/head.go`

Extend `memSeries`:
```go
lastStatesetValue *stateset.StateSet
```

The `memSeries.chunk()` encoding switch gets a `stStateset` → `EncStateset`
case.

### WAL replay

The WAL replay loop in `tsdb/head.go:loadWAL` needs a case for
`record.StatesetSamples` to replay stateset samples on restart.

---

## Phase 6 — Scrape-time aggregation (`model/textparse/`, `scrape/`)

### New parser wrapper

**New file**: `model/textparse/statesetparse.go`

`StateSetParser` wraps any `Parser`. Its state machine (following `NHCBParser`
from `model/textparse/nhcbparse.go`):

```
stateStart
  └─ sees metric with type=stateset → stateCollecting
stateCollecting
  └─ accumulates (state_name, value) pairs while labels match
  └─ label-set changes or EOF → stateEmitting
stateEmitting
  └─ emits single EntryStateset, returns to stateStart
```

The wrapper identifies the state-carrying label as the one whose name equals
the metric base name (OM1 convention: `kube_pod_status_phase` series use label
`kube_pod_status_phase` to carry the state name). It calls a new `Stateset()`
method on the Parser interface.

### Parser interface

**Changes to `model/textparse/interface.go`:**

```go
EntryStateset Entry = 6

type Parser interface {
    // ... existing methods unchanged ...
    Stateset() ([]byte, *int64, *stateset.StateSet)
}
```

All existing parser implementations (`openmetricsparse.go`, `promparse.go`,
`protobufparse.go`) return `nil, nil, nil` from `Stateset()` and never emit
`EntryStateset` — they are not affected unless wrapped by `StateSetParser`.

### Scrape config and loop

**Changes to `scrape/scrape.go`:**

1. Add `convert_stateset_to_native: bool` to `ScrapeConfig`.
2. When enabled, wrap the base parser with `StateSetParser` at the point where
   NHCB wrapping currently happens (~line 1196).
3. In the scrape loop (~line 1773), add:
   ```go
   case textparse.EntryStateset:
       _, ts, ss := p.Stateset()
       // Resolve labels (state-carrying label is absent in native form).
       _, err = app.Append(ref, lset, startTimestamp, t, 0, nil, nil, ss, ...)
   ```

The state-carrying label (`kube_pod_status_phase="Running"` etc.) is **stripped**
from the series labels when building the native stateset, since its information
is encoded in `ss.Names`/`ss.Values`.

---

## Phase 7 — PromQL (`promql/`)

### New functions in `promql/functions.go`

| Function | Signature | Returns |
|---|---|---|
| `stateset_is_active` | `stateset_is_active(v instant-vector, state string)` | `1` if named state active, `0` otherwise, one sample per input series |
| `stateset_active_states` | `stateset_active_states(v instant-vector)` | One output series per (input-series × active-state); the label name for the state is taken from `StateSet.LabelName` (e.g. `kube_pod_status_phase`) |
| `stateset_known_states` | `stateset_known_states(v instant-vector)` | One output series per (input-series × known-state), value `1` if active else `0`; same label name convention |

`stateset_active_states` is the primary ergonomic function. Users can write:

```promql
stateset_active_states(kube_pod_status_phase)
```

and get one series per active state per pod, with the original label name
(`kube_pod_status_phase`) preserved, filterable with label selectors.

To find the current phase of a specific pod:

```promql
stateset_active_states(kube_pod_status_phase{pod="nginx-abc"})
```

To check if a specific state is active:

```promql
stateset_is_active(kube_pod_status_phase, "Running")
```

### Parser/AST

**Changes to `promql/parser/functions.go`:** register new function signatures,
specifying that the first argument accepts `ValueTypeStateset` (new value type
constant, mirroring `ValueTypeMatrix`, `ValueTypeVector`, etc.).

**Changes to `promql/engine.go`:** `eval()` switch needs a `ValStateset` case
for instant and range vectors, similarly to how `ValHistogram` was added.

### Sample struct

**Changes to `promql/value.go`:** the `Sample` struct gains:

```go
SS *stateset.StateSet  // Non-nil for stateset samples.
```

Parallel to the existing `H *histogram.FloatHistogram` field.

---

## Phase 8 — Remote write / read

### Remote write v2

**Changes to `prompb/io/prometheus/write/v2/types.proto`:** add a
`StatesetSample` message and extend `TimeSeries` with a repeated
`StatesetSample stateset_samples` field. The state-name list is sent once per
`TimeSeries` as metadata (analogous to histogram schema), and individual
samples carry only the bitset value.

### Remote read

The remote read path needs a `STATESET` sample type in the query response
proto and corresponding iterator support.

**Note:** Remote write v1 does not support native statesets; they are
downgraded to the legacy float representation on v1 endpoints (each active
state emitted as a separate `1`-valued series, inactive states omitted — or
emitted as `0` if the endpoint is known to require placeholders).

---

## Phase 9 — Testing

Following the project's AGENTS.md requirements:

1. **Unit tests** for `model/stateset/` — encoding, bit-position stability,
   copy/equals, >64 state rejection.
2. **Chunk encoding tests** in `tsdb/chunkenc/` — round-trip encode/decode,
   recode on new state name, XOR delta correctness.
3. **WAL replay tests** — append statesets, simulate crash, verify replay
   produces identical chunks.
4. **Parser tests** in `model/textparse/` — `StateSetParser` correctly
   aggregates OM1 stateset series into `EntryStateset`; handles missing
   0-valued placeholders; handles EOF mid-stateset.
5. **Scrape integration tests** — end-to-end from text exposition to TSDB
   series, with and without `convert_stateset_to_native`.
6. **PromQL tests** in `promql/promqltest/testdata/` — using the existing
   testdata format, test `stateset_is_active`, `stateset_active_states`,
   `stateset_known_states` against native statesets.
7. **Benchmark** — compare series count, chunk count, and query time for a
   5-state stateset (e.g. `kube_pod_status_phase` shape) between native and
   legacy representations. Run with `go test -count=6 -benchmem` and report
   with `benchstat`.

---

## Recommended implementation order

```
Phase 1: model/stateset          (no deps, unblocks everything)
Phase 2: chunkenc/stateset       (deps: Phase 1)
Phase 3: record types            (deps: Phase 1)
Phase 4: storage interface       (deps: Phase 1)
Phase 5: TSDB head               (deps: Phases 2, 3, 4)
Phase 6: scrape aggregation      (deps: Phase 1 + parser interface only)
Phase 7: PromQL                  (deps: Phases 4, 5)
Phase 8: remote write            (deps: Phases 1, 4)
Phase 9: tests (interleaved throughout)
```

Phases 1–3 are suitable as a preparatory PR ("Part 1: data model and encoding
skeleton") before the larger TSDB and PromQL work.

---

## Key source files

| File | Relevance |
|---|---|
| `model/textparse/nhcbparse.go` | Template for `StateSetParser` wrapper |
| `model/textparse/interface.go` | Parser interface — add `EntryStateset`, `Stateset()` |
| `tsdb/chunkenc/chunk.go` | Add `EncStateset`, `ValStateset`, update iterator interface |
| `tsdb/chunkenc/histogram.go` | Template for `StateSetChunk` recode logic |
| `tsdb/record/record.go` | Add `StatesetSamples` record type and structs |
| `storage/interface_append.go` | Extend `AppenderV2.Append` signature |
| `tsdb/head_append.go` | Add `stStateset`, extend batch/commit pipeline |
| `tsdb/head.go` | Extend `memSeries`, add WAL replay case |
| `promql/functions.go` | Register `stateset_*` functions |
| `promql/engine.go` | Add `ValStateset` case to eval dispatch |
| `promql/value.go` | Add `SS *stateset.StateSet` to `Sample` |
| `scrape/scrape.go` | Add config flag, wrap parser, handle `EntryStateset` |

---

## Open questions

1. **State-carrying label name when different emitters differ.** *(Resolved.)*

   If version 0.1 uses `{state="healthy"}` and version 0.2 uses
   `{health="healthy"}`, they produce different `StateSet.LabelName` values and
   therefore different TSDB series. This cannot be merged internally without
   operator intervention; the correct resolution is a `metric_relabel_configs`
   rule that normalises the label name before ingestion:

   ```yaml
   metric_relabel_configs:
     - source_labels: [health]
       target_label: state
       action: replace
     - regex: health
       action: labeldrop
   ```

   This is the standard Prometheus answer to label-name drift across emitter
   versions and should be documented as such.

   A `__state_label__` series label was considered but rejected because:
   - `__*__` labels are stripped at remote write boundaries and by many
     intermediaries, so the information would be silently lost in forwarding
     pipelines.
   - Making it a series label would unnecessarily affect series identity.
   - Storing it in the chunk header (as `StateSet.LabelName`) achieves the same
     goal — lossless explosion to the legacy representation — without those
     drawbacks.

2. **Cross-compatibility / implicit explosion.** The issue mentions making
   native statesets queryable as if they were legacy series (akin to the
   implicit `_bucket` approach for native histograms). This is deferred —
   explicit PromQL functions are the primary API for v1. Implicit explosion
   adds significant engine complexity.

3. **OTel compatibility.** Per @dashpole's comment in the issue, OTel would
   initially map statesets to gauges with `metric.metadata["prometheus.type"]
   = "stateset"`. No special handling is needed from Prometheus's side for v1.

4. **Generation counter for pruning.** @ringerc's proposal suggests reserving
   upper bits of the uint64 for a pruning generation counter. Given that chunks
   already handle state changes via recode/new-chunk, this complexity is not
   needed in the initial implementation and can be revisited if needed.

5. **OM2 wire format.** If a future OpenMetrics 2.0 revision adds a composite
   stateset wire type, the `StateSetParser` wrapper can be retired in favour of
   direct parser support for the new entry type, with no changes needed to the
   TSDB or PromQL layers.
