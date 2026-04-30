# Native Stateset — Implementation Summary

This document records every file created or modified on the
`native-stateset-representation` branch and explains what changed and why.
For the design rationale see `docs/native-stateset-proposal.md`.
For the ordered task list see `docs/native-stateset-steps.md`.
For memory benchmark results see `docs/native-stateset-memory-findings.md`.

---

## New files

### `model/stateset/stateset.go`

Core data type. Defines `StateSet`:

```go
type StateSet struct {
    LabelName string   // label that carries the state in OM1 wire format
    Names     []string // sorted lexicographically; max 64 entries
    Values    uint64   // bitset: bit i set ↔ Names[i] is active
}
```

Methods: `IsActive(name)`, `ActiveNames()`, `Copy()`, `Equals()`, `Sorted()`,
`Validate()`. No dependencies on other Prometheus packages.

### `model/stateset/stateset_test.go`

Unit tests for all `StateSet` methods. Covers zero states, one state, 64 states
(upper boundary), mismatched `LabelName`, unsorted names (rejected by
`Validate`).

### `tsdb/chunkenc/stateset.go`

Binary chunk encoding for stateset samples. Wire layout per chunk:

```
num_samples  u16
label_name   u8-len-prefixed string
num_states   u16
state_names  u8-len-prefixed strings × num_states   (sorted)
--- first sample ---
timestamp    i64 (absolute, big-endian)
values       u64 (absolute, big-endian)
--- subsequent samples ---
timestamp    varint delta from previous
values       uvarint XOR with previous values
```

`AppendStateset` returns a new `StateSetChunk` if the state names differ from
the chunk's names (recode trigger). `Iterator.AtStateset` decodes the current
sample in-place into a caller-supplied `*StateSet`, sharing the chunk's
`Names` slice (read-only).

### `tsdb/chunkenc/stateset_test.go`

Round-trip tests: stable state names, recode on name change, XOR correctness
for the common case of one bit changing per sample, `Seek`, boundary samples.

### `model/textparse/statesetparse.go`

`StateSetParser` wraps any `Parser` and aggregates OM1 per-state float series
into `EntryStateset` entries. Uses a three-state machine:

- **stateStart** — default; passes non-stateset entries through unchanged.
- **stateCollecting** — accumulates `(state_name, value)` pairs from
  consecutive `EntrySeries` entries belonging to the same labelset for the
  same stateset metric family.
- **stateEmitting** — holds a completed `StateSet` ready to be returned;
  re-drives the underlying parser on the next `Next()` call.

Key invariant: the series labelset stored for the emitted entry has the
state-carrying label (`LabelName`) stripped, matching what will be written to
TSDB.

### `model/textparse/statesetparse_test.go`

Tests: basic aggregation, multiple independent dimensions, multiple statesets
in one scrape, state names sorted lexicographically, disabled-by-option
pass-through, non-stateset series pass-through, all-active and all-inactive
statesets.

### `prompb/io/prometheus/write/v2/stateset.go`

Hand-written protobuf types and encoder/decoder for remote write v2 stateset
fields, written without `protoc` because only field numbers 7 and 8 are needed
and the repo does not run `protoc` at build time. Bytes are placed in
`TimeSeries.XXX_unrecognized` so they are binary-compatible with a
protoc-generated decoder.

Key types and functions:
- `StatesetMetadata{LabelName, StateNames}` — serialised at field 7.
- `StatesetSample{TimestampMs, Values}` — serialised at field 8 (repeated).
- `AppendStatesetToTimeSeries(ts, meta, samples)` — encodes both into
  `ts.XXX_unrecognized`.
- `ExtractStatesetFromTimeSeries(ts)` — decodes from `XXX_unrecognized`.

### `scripts/stateset_memory.sh`

Runner script that builds the test binary once, executes
`TestStatesetMemory/native_stateset` and `TestStatesetMemory/float_gauges` in
separate processes, and monitors `/proc/{pid}/status` to report peak VmRSS —
equivalent to what `systemd-run --wait` reports as `MemoryPeak`.

### `tsdb/stateset_memory_test.go`

`TestStatesetMemory` with two sub-tests (`native_stateset`, `float_gauges`).
Each sub-test populates a TSDB Head with 2,000 pods × 60 samples, runs a
full-scan query, forces two GCs, then logs HeapInuse (from
`runtime.ReadMemStats`) and VmRSS/VmHWM (from `/proc/self/status`). Designed
to run one sub-test per process so VmHWM starts clean. See
`docs/native-stateset-memory-findings.md` for results.

---

## Modified files

### `model/textparse/interface.go`

- Added `EntryStateset Entry = 6` to the `Entry` constants.
- Added `Stateset() ([]byte, *int64, *stateset.StateSet)` to the `Parser`
  interface.
- Added `ConvertStateSetsToNative bool` to `ParserOptions`.
- Wired `StateSetParser` into `NewParser` when the option is set (after any
  NHCB wrapping).

### `model/textparse/openmetricsparse.go`, `promparse.go`, `protobufparse.go`, `nhcbparse.go`

Added stub `Stateset()` method (`return nil, nil, nil`) to every parser
implementation that is not `StateSetParser`, satisfying the interface.

### `tsdb/chunkenc/chunk.go`

- Added `EncStateset Encoding = 5`.
- Added `ValStateset ValueType` (after `ValFloatHistogram`).
- Updated `IsValidEncoding`, `Encoding.String()`, `ValueType.String()`, and
  `ValueType.ChunkEncoding()` for the new constants.
- Added `AtStateset(*stateset.StateSet) (int64, *stateset.StateSet)` to the
  `Iterator` interface.
- Added stub `AtStateset` to `nopIterator`, `mockSeriesIterator`, and every
  other `Iterator` implementation in the repo.
- Added `EncStateset` case to `pool.Get` and `pool.Put`.

### `tsdb/chunkenc/xor.go`, `xor2.go`, `histogram.go`, `float_histogram.go`

Added `AtStateset` stub to all existing chunk iterator types.

### `tsdb/record/record.go`

- Added `StatesetSamples Type = 12`.
- Added `RefStatesetSample{Ref chunks.HeadSeriesRef, T int64, SS *stateset.StateSet}`.
- Added `EncodeStatesets([]RefStatesetSample, []byte) []byte` — wire format:
  `[ref u64][t varint][label_name u16-len-prefixed][n u16][name_i u16-len-prefixed × n][values u64]` per sample.
- Added `DecodeStatesets([]byte, []RefStatesetSample) ([]RefStatesetSample, error)`.

### `tsdb/record/record_test.go`

Added encode/decode round-trip tests for `StatesetSamples`: zero, one, 64
states; multiple samples in one record.

### `tsdb/record/buffers.go`

- Added `statesets zeropool.Pool[[]RefStatesetSample]` to `BuffersPool`.
- Added `GetStatesets(capacity int)` and `PutStatesets(b)`.

### `storage/interface_append.go`

- Added `ss *stateset.StateSet` parameter to `AppenderV2.Append` (8th
  positional argument, before `opts`).
- Updated dispatch comment to document the `ss != nil` case.

### `storage/fanout.go`, `series.go`, `merge.go`, `buffer_test.go`, `memoized_iterator.go`

Updated all `AppenderV2.Append` call sites and implementations to pass the new
`ss` parameter.

### `storage/memoized_iterator.go`

- Added `prevStateset *stateset.StateSet` field.
- Extended `PeekPrev()` to a five-value return:
  `(t int64, v float64, fh *histogram.FloatHistogram, ss *stateset.StateSet, ok bool)`.
- `Next()` handles `ValStateset`: saves `(t, ss)` into `prevTime`/`prevStateset`.
- Added `AtStateset()` forwarding method.

### `storage/memoized_iterator_test.go`

Updated `PeekPrev()` call sites for the new five-value signature.

### `storage/remote/write_handler.go`

`remoteWriteAppenderV2.Append` updated for the new `ss` parameter; passes
`nil` (stateset samples are not handled in the v1 receive path).

### `storage/remote/write.go`

`WriteStorage.AppenderV2` forwarding updated.

### `storage/remote/queue_manager.go`

- Added `stateset *stateset.StateSet` to the `timeSeries` struct.
- Added `tStateset` to the `seriesType` constants.
- Added `AppendStatesets([]record.RefStatesetSample) bool` — identical pattern
  to `AppendFloatHistograms`: age-check, `sendNativeHistograms` gate, dropped-
  series accounting, `shards.enqueue` retry loop.
- `populateV2TimeSeries` handles `tStateset`: calls
  `writev2.AppendStatesetToTimeSeries`.
- `populateTimeSeries` (v1) handles `tStateset`: silently drops (stateset
  samples cannot be represented in v1 wire format without expansion).

### `storage/remote/codec.go`, `write_otlp_handler.go`, `otlptranslator/…`

Stub `ss` parameter propagation through OTLP and codec paths.

### `tsdb/head.go`

- Added `lastStateset *stateset.StateSet` field to `memSeries`.
- Added `statesetPool sync.Pool` to `Head` for `[]RefStatesetSample` buffers.
- Extended `memSeries.encoding()` switch: `EncStateset` → `stStateset`.

### `tsdb/head_append.go`

- Added `stStateset` to the `sampleType` enum.
- Added `statesets []record.RefStatesetSample` and
  `statesetSeries []*memSeries` to `appendBatch`; added pool
  get/put in `close()`.
- `AppenderV2.Append`: when `ss != nil`, validates the `StateSet`
  (`Validate()` — sorts names, checks 64-state limit), routes to `stStateset`
  batch.
- Added `commitStatesets` (parallel to `commitHistograms`): locks each series,
  calls `s.app.AppendStateset(t, ss)`, handles recode (new chunk returned).
- WAL `Commit`: encodes and logs `StatesetSamples` records after float and
  histogram records.

### `tsdb/head_append_v2.go`

`AppenderV2` implementation wired through.

### `tsdb/head_wal.go`

WAL replay (`loadWAL`): added `record.StatesetSamples` case alongside
`record.HistogramSamples`. Reconstructs the `StateSet` from the decoded
`RefStatesetSample` and calls `ms.app.AppendStateset`.

### `tsdb/head_append_v2_test.go`, `head_test.go`, `db_append_v2_test.go`

- Tests updated for new `Append` signature.
- New tests: stateset append → chunk query; recode on state-name change;
  WAL replay recovery; `ErrTooManyStates` for >64 states.

### `tsdb/head_bench_test.go`

- Added `model/stateset` import.
- Added `BenchmarkStateset`: two sub-benchmarks comparing native stateset
  append (one series, `AppenderV2`) vs equivalent 5 float-gauge series (one
  series per state, `Appender`), both for a `kube_pod_status_phase` shape.
  Results: ~16.8 µs/op vs ~15.2 µs/op; 11 vs 6 allocs/op.

### `tsdb/querier.go`

`blockQuerier.Select` and underlying series iterators: added `ValStateset`
pass-through so stateset chunks are returned in query results.

### `tsdb/querier_test.go`, `blockwriter_test.go`

Tests updated for stateset-aware iterator.

### `tsdb/agent/db_append_v2.go`, `db_append_v2_test.go`

Agent DB `AppenderV2.Append` updated for the `ss` parameter. When `ss != nil`,
the agent writes a `StatesetSamples` WAL record (passed to the WAL watcher for
remote write).

### `tsdb/wlog/watcher.go`

- Added `AppendStatesets([]record.RefStatesetSample) bool` to the `WriteTo`
  interface.
- `readSegment`: added `statesets` buffer (from `recordBuf.GetStatesets`);
  added `record.StatesetSamples` decode case (under `sendHistograms` gate,
  after `FloatHistogramSamples`).

### `tsdb/wlog/watcher_test.go`

- Added `statesetsAppended []record.RefStatesetSample` and `statesetAppends int`
  to `writeToMock`.
- Added `AppendStatesets` method to satisfy the updated `WriteTo` interface.

### `promql/parser/value.go`

Added `ValueTypeStateset ValueType = "stateset"`.

### `promql/value.go`

- Added `SSPoint{T int64, SS *stateset.StateSet}`.
- Added `Statesets []SSPoint` to `Series`.
- Added `SS *stateset.StateSet` to `Sample`.

### `promql/engine.go`

- `vectorSelectorSingle` extended to return `(t, f, fh, ss, ok)` — five values.
- `evalSeries`: handles `ssSample != nil` first (before histogram, before float).
- Instant-vector eval and `gatherVector` handle `len(series.Statesets) > 0`.
- Added `"github.com/prometheus/prometheus/model/stateset"` import.

### `promql/functions.go`

Three new PromQL functions:

| Function | Args | Returns |
|---|---|---|
| `stateset_is_active(v, name)` | vector, string | vector: `1` if named state active, else `0` |
| `stateset_active_states(v)` | vector | vector: one sample per active state, labelled with `LabelName=state` |
| `stateset_known_states(v)` | vector | vector: one sample per known state (`1` if active, `0` if not) |

### `promql/parser/functions.go`

Registered the three new functions with their argument types and return type.

### `promql/promqltest/test.go`

Updated test-format parser to recognise `ValStateset` results.

### `promql/histogram_stats_iterator_test.go`

Updated `PeekPrev()` call sites for five-value return.

### `prompb/io/prometheus/write/v2/types.proto`

Added `StatesetMetadata` and `StatesetSample` message definitions; added
`stateset_metadata = 7` and `repeated stateset_samples = 8` fields to
`TimeSeries`. Comment notes that until `protoc` codegen is wired up, Go types
live in `stateset.go` and bytes are placed in `XXX_unrecognized`.

### `scrape/scrape_append_v2.go`

`scrapeAppendV2` loop: added `textparse.EntryStateset` case — reads
`p.Stateset()`, sets the series reference, calls
`app.Append(..., ss, appOpts)`. Also passes
`ConvertStatesets: sl.enableNativeStatesetScraping` in `ParserOptions` so
the `StateSetParser` wrapper is activated when the config flag is set.

### `scrape/scrape.go`

Added `enableNativeStatesetScraping bool` field to `scrapeLoop` (initialised
from `ScrapeNativeStatesetsEnabled()`). Added
`case textparse.EntryStateset: continue` in the V1 append path so statesets
are silently skipped rather than mis-processed (they require `AppenderV2`).

### `config/config.go`

Added `ScrapeNativeStatesets *bool` field to both `GlobalConfig` and
`ScrapeConfig`, with YAML tag `scrape_native_statesets`. Added default
propagation in both `UnmarshalYAML` methods and a
`ScrapeNativeStatesetsEnabled() bool` helper method on `ScrapeConfig`.

### `docs/configuration/configuration.md`

Documented `scrape_native_statesets` in both the global and per-job scrape
config sections.

### `cmd/prometheus/testdata/features.json`

Golden file updated to include `stateset_is_active`, `stateset_active_states`,
and `stateset_known_states` in the `/api/v1/metadata/features` response.

### `util/jsonutil/marshal.go`

Added `MarshalStateset(*stateset.StateSet, *jsoniter.Stream)` — encodes a
stateset as a JSON object with three fields:

```json
{"labelName": "phase", "names": ["Failed","Pending","Running"], "values": "4"}
```

`names` is the sorted slice of state names; `values` is the active-state bitset
as a quoted decimal uint64 (bit i set ↔ `names[i]` is active).

### `web/api/v1/json_codec.go`

Added stateset encoding to the HTTP query API:

- Registered `unsafeMarshalSSPointJSON` for `promql.SSPoint` via
  `jsoniter.RegisterTypeEncoderFunc`.
- Added `marshalSSPointJSON` — writes `[<timestamp>, {<stateset>}]`, mirroring
  `marshalHPointJSON`.
- Extended `marshalSeriesJSON` to emit a `"statesets"` array alongside
  `"values"` and `"histograms"` for range (matrix) results.
- Extended `marshalSampleJSON` to emit a `"stateset"` field (instead of
  `"value"` or `"histogram"`) when `s.SS != nil` for instant (vector) results.

### `promql/value.go`

- Added `SSPoint.MarshalJSON()` — fallback JSON marshaler for non-jsoniter
  callers; encodes `[timestamp/1000, {labelName, names, values}]`.
- Updated `Sample.MarshalJSON()` to handle `SS != nil` with a `"stateset"`
  field, consistent with the jsoniter path.

### `promql/promqltest/testdata/statesets.test`

Added 41 promqltest cases covering all three PromQL functions:

- `stateset_is_active(v, name)` — returns 1/0 per series; drops metric name;
  unknown state name returns 0; tracks changes over time.
- `stateset_active_states(v)` — one sample per active state (value 1);
  preserves metric name; empty result when no states active.
- `stateset_known_states(v)` — one sample per known state (1 if active, 0 if
  not); preserves metric name.
- All three functions return an empty vector for plain float (non-stateset)
  input.

### `util/teststorage/appender.go`, `appender_test.go`

Test-storage `AppenderV2` stub updated for the new `ss` parameter.

### `storage/fanout_test.go`, `merge_test.go`

Test call sites updated for the new `Append` signature.

### `web/api/testhelpers/mocks.go`

Mock `AppenderV2` updated for the new `ss` parameter.

---

## Dependency graph

```
model/stateset
    └── tsdb/chunkenc          (EncStateset, StateSetChunk, ValStateset)
            └── tsdb/record    (RefStatesetSample, StatesetSamples)
                    └── storage/interface_append  (AppenderV2.Append ss param)
                            ├── tsdb/head          (append, commit, WAL replay)
                            │       └── tsdb/wlog  (WriteTo.AppendStatesets)
                            │               └── storage/remote/queue_manager
                            │                       └── prompb/…/stateset.go
                            ├── storage/memoized_iterator  (PeekPrev 5-return)
                            │       └── promql/engine      (vectorSelectorSingle)
                            │               └── promql/functions (stateset_*)
                            └── model/textparse/statesetparse  (StateSetParser)
                                    └── scrape/scrape_append_v2
```

---

## What is not yet done

The following item from `docs/native-stateset-steps.md` remains deferred:

- **v1 remote write expansion**: the v1 downgrade path (exploding a stateset
  into per-state float-gauge series) is silently dropped rather than expanded.
  Clients receiving remote write v1 will not see stateset data.
