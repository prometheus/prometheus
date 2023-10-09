# An overview of different Series and Chunk reference types

## Used by callers of TSDB

| Location           | Series access                  | Chunk access                                                       |
|--------------------|--------------------------------|--------------------------------------------------------------------|
| Global interfaces  | `SeriesRef` (in postings list) | `chunks.ChunkRef`            (`ChunkReader` interface, `Meta.Ref`) |
| Head               | `HeadSeriesRef`      (autoinc) | `HeadChunkRef` (could be head chunk or mmapped chunk.  5/3B split) |
| blocks             | `BlockSeriesRef` (16B aligned) | `BlockChunkRef`                                       (4/4B split) |

### `SeriesRef`

Note: we cover the implementations as used in Prometheus.  Other projects may use different implementations.

#### `HeadSeriesRef`

`HeadSeriesRef` is simply a 64bit counter that increments when a new series comes in.
Due to series churn, the set of actively used `HeadSeriesRef`s may be well above zero (e.g. 0-10M may not be used, and 10M-11M is active)

Usage:
* [`stripeSeries`](https://github.com/prometheus/prometheus/blob/fdbc40a9efcc8197a94f23f0e479b0b56e52d424/tsdb/head.go#L1292-L1298) (note: when you don't know a `HeadSeriesRef` for a series, you can also access it by a hash of the series' labels)
* WAL
* `HeadChunkRef`s include them for addressing head chunks, as those are owned by the `memSeries`.

Notes:
1) M-mapped Head chunks, while they use `HeadSeriesRef`s, don't contain an index and depend on the series listing in memory.
Once mmapped, chunks have `HeadSeriesRef`s inside them, allowing you to recreate the index from reading chunks
(Along with WAL which has the labels for those `HeadSeriesRef`s. It also has all those samples, but by using m-mapped chunks we can save cpu/time and not replay all of WAL on startup)

2) During querying, `HeadSeriesRef` are limited to 2^40 (see `HeadChunkRef`)

3) The last `HeadSeriesRef` is always replayed from the WAL and is continued from there.

#### `BlockSeriesRef`

Persistent blocks are independent entities and the format/structure is completely different from head block.

In blocks, series are lexicographically ordered by labels and the byte offset in the index file (divided by 16 because they're all aligned on 16 bytes) becomes the `BlockSeriesRef`.

They are not sequential because index entries may be multiples of 16 bytes. And they don't start from 0 because the byte offset is absolute and includes the magic number, symbols table, etc.

`BlockSeriesRef` are only 32 bits for now, because 64 bits would slow down the postings lists disk access. (note: this limits the index size to 2^32 * 16 = 64 GB)


See also:
* https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/#3-index
* https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/#c-series

### `ChunkRef`

Chunk references are used to load chunk data during query execution.
Note: we cover the implementations as used in Prometheus.  Other projects may use different implementations.

#### `HeadChunkRef`

A `HeadChunkRef` is an 8 byte integer that packs together:

* 5 Bytes for `HeadSeriesRef`.
* 3 Bytes for `HeadChunkID` (uint64) (see below).

There are two implications here:

* While `HeadSeriesRef`s can during ingestion go higher, during querying they are limited to 2^40.  Querying too high numbers will lead to query failures (but not impact ingestion).
* `ChunkID` keeps growing as we enter new chunks until Prometheus restarts. If Prometheus runs too long, we might hit 2^24.
  ([957 years](https://www.wolframalpha.com/input/?i=2%5E24+*+120+*+15+seconds+in+years) at 1 sample per 15 seconds).  If `ChunkID=len(mmappedchunks)` then it's the head chunk.

#### `BlockChunkRef`

A `BlockChunkRef` is an 8 byte integer.  Unlike `HeadChunkRef`, it is static and independent of factors such as Prometheus restarting.

It packs together:

* 4 Bytes for chunk file index in the block. This number just increments. Filenames [start at 1](https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/#contents-of-a-block)
but the `BlockChunkRef` start at 0.
* 4 Bytes for the byte offset within the file.

#### Why does `HeadChunkRef` contain a series reference and `BlockChunkRef` does not?

The `ChunkRef` types allow retrieving the chunk data as efficiently as possible.
* In the Head block the chunks are in the series struct. So we need to reach the series before we can access the chunk from it.
  Hence we need to pack the `HeadSeriesRef` to get to the series.
* In persistent blocks, the chunk files are separated from the index and static. Hence you only need the co-ordinates within the `chunks` directory
  to get to the chunk. Hence no need of `BlockSeriesRef`.

## Used internally in TSDB

* [`HeadChunkID`](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb/chunks#HeadChunkID) references a chunk of a `memSeries` (either an `mmappedChunk` or `headChunk`).
  If a caller has, for whatever reason, an "old" `HeadChunkID` that refers to a chunk that has been compacted into a block, querying the memSeries for it will not return any data.
* [`ChunkDiskMapperRef`](https://pkg.go.dev/github.com/prometheus/prometheus/tsdb/chunks#ChunkDiskMapperRef) is an 8 Byte integer.
  4 Bytes are used to refer to a chunks file number and 4 bytes serve as byte offset (similar to `BlockChunkRef`).  `mmappedChunk` provide this value such that callers can load the mmapped chunk from disk.

