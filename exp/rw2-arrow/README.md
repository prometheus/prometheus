# Remote Write 2.x Apache Arrow Experiment & Benchmark

This standalone experimental Go module benchmarks Prometheus Remote Write 2.0 (`writev2.Request` with `SymbolsTable` + `OptimizedMarshal`) against Apache Arrow IPC (`arrow-go/v18`) for encoding, decoding, and wire size across different batch shapes (issue [#14342](https://github.com/prometheus/prometheus/issues/14342)).

It is kept in its own `go.mod` under `exp/rw2-arrow` so that `github.com/apache/arrow-go/v18` and its transitive dependencies are not added to the main Prometheus `go.mod`.

## Variants Benchmarked

1. **Prometheus RW 2.0 (`writev2`)**: `SymbolsTable` + `Request.OptimizedMarshal` with Raw, Snappy, and Zstd compression.
2. **Arrow Columnar (`Dictionary(Int32, String)` per label key + `timestamp_ms: Int64` + `value: Float64`)**:
   - **Stateless**: fresh `RecordBuilder` per batch.
   - **Pooled**: reused `RecordBuilder` across batches (stateless on the wire).
   - **Stateful**: long-lived `ipc.Writer` that emits the schema once and writes only `RecordBatch` frames for subsequent batches.
   - **Zstd**: Arrow IPC native Zstd buffer compression (`ipc.WithZstd()`).
3. **Arrow List (`List<Struct<name: Dict, value: Dict>>` + `timestamp_ms: Int64` + `value: Float64`)**:
   - Fixed schema supporting arbitrary, heterogeneous label sets per series without schema evolution or sparse null columns.

## Running Tests & Benchmarks

```bash
cd exp/rw2-arrow
GOWORK=off go test -v ./...
GOWORK=off go test -run=^$ -bench=. -benchmem -benchtime=200ms ./...
```

## Summary of Findings (`darwin/arm64`, Apple M2)

### Wire Size (Bytes)

| Batch | RW2 Raw | RW2 Snappy | RW2 Zstd | ArrowCol Raw | ArrowCol Zstd | ArrowCol Stateful (batch 2+) | ArrowList Raw | ArrowList Zstd |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 200×1 (200 samples) | 11,018 | 3,923 | 2,068 | 14,456 | 5,544 | 8,480 | 19,040 | 4,776 |
| 500×10 (5,000 samples) | 108,608 | 33,045 | 13,311 | 212,040 | 10,968 | 200,480 | 351,024 | 31,056 |
| 5000×1 (5,000 samples) | 270,628 | 86,631 | 28,631 | 298,064 | 56,440 | 200,480 | 437,048 | 74,184 |

### Takeaways

- **Receiver Decode Win on Large Batches**: On `5000×1`, `ArrowCol_Decode_Raw` is ~2.1× faster than `RW2_Decode_Snappy` (0.56 ms vs 1.16 ms) with ~6× less allocated memory (0.96 MB vs 5.72 MB) and ~3.8× fewer allocations.
- **Multi-Sample Series (`500×10`)**: `ArrowCol + Zstd` achieves the smallest wire size (10,968 B vs 33,045 B Snappy / 13,311 B Zstd), while RW 2.0 encodes ~6× faster because it groups multiple samples under a single `TimeSeries` without repeating per-row dictionary lookups.
- **Small Batches (`200×1`)**: RW 2.0's `Symbols` table beats Arrow IPC on wire size (3,923 B Snappy / 2,068 B Zstd vs 5,544 B ArrowCol Zstd) because Arrow's schema and IPC framing overhead dominate at small row counts.
- **Sender Pooling**: Reusing `RecordBuilder` (`ArrowCol_Encode_Pooled_Raw`) brings `5000×1` encode latency close to `RW2_Encode_Raw` (1.55 ms vs 1.44 ms), though `BinaryDictionaryBuilder.AppendString` still incurs per-row dictionary hash lookups.
