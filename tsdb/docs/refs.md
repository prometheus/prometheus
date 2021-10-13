# An overview of different Series and Chunk reference types

## Used by callers of TSDB

| Location           | Series access               | Chunk access                                                             |
|--------------------|-----------------------------|--------------------------------------------------------------------------|
| Global interfaces  | SeriesRef(in postings list) | chunks.ChunkRef (ChunkReader interface, Meta.Ref)                        |
| Head               | HeadSeriesRef (autoinc)     | HeadChunkRef (5/3B) -> could be head chunk or mmapped                    |
| blocks             | BlockSeriesRef (sorted 16B) | BlockChunkRef (4/4B split) -< this one is for both external and internal |

## Used internally in TSDB

* ChunkDiskMapperRef: to load mmapped chunks from disk.

#### SeriesRefs

Note: we cover the implementations as used in Prometheus.  Other projects may use different implementations.

## HeadSeriesRef

HeadSeriesRef is simply a 64bit counter that increments when a new series comes in.
Due to series churn, the set of actively used HeadSeriesRefs may be well above zero (e.g. 0-10M may not be used, and 10M-11M is active)

Usage:
* [stripeSeries](https://github.com/prometheus/prometheus/blob/fdbc40a9efcc8197a94f23f0e479b0b56e52d424/tsdb/head.go#L1292-L1298) (note: when you don't know a HeadSeriesRef for a series, you can also access it by a hash of the series' labels)
* WAL
* Head chunks

Notes:
1) head chunks, while they use HeadSeriesRefs, don't contain an index and depend on the series listing in memory.
Once mmapped, chunks have HeadSeriesRefs inside them, allowing you to recreate the index from reading chunks
(along with WAL which has the labels. it also has all those points, but by using chunks we can save cpu/time and not replay all of WAL on startup)

2) Due to an unrelated hack (TODO clarify), you may not use values >= uint32 inside the uint64's. so we presume prometheus gets restarted before you reach 2^32 seriesRefs in head, but don't check this.
the last series ref is always replayed from the WAL and is continued from there.
3) During querying, HeadSeriesRef are limited to 2^40 (see HeadChunkRef)

## BlockSeriesRef

Persistent blocks are all independent entities and the format/structure is completely different from head block.

In blocks, series are lexicographically ordered by labels and the byte offset in the index file (divided by 16 because they're all aligned on 16 bytes) becomes the BlockSeriesRef.
They are not sequential because index entries may be multiples of 16 bytes. And they don't start from 0 because the byte offset is absolute and includes the magic number, symbols table, etc.
BlockSeriesRef. are only 32 bits, because 64 bits would slow down the postings lists disk access. (note: this limits the index size to 2^32 * 16 = 64 GB)

See also:
* https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/#3-index
* https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/#c-series

## ChunkRef

Note: we cover the implementations as used in Prometheus.  Other projects may use different implementations.

### HeadChunkRef

A HeadChunkRef is an 8 byte integer, that is only used during querying.  It packs together:

* 5 Bytes for HeadSeriesRef.
* 3 Bytes for ChunkID (uint64). This is simply an index into a slice of mmappedChunks for a given series

There are two implications here:

* While HeadSeriesRefs can during ingestion go higher, during querying they are limited to 2^40.  Querying too high numbers will lead to query failures (but not impact ingestion).
* ChunkID keeps growing as we enter new chunks. If prometheus runs too long, we might hit 2^24. If id=len(mmappedchunks) then it's the head chunk. note.

### BlockChunkRef

A BlockChunkRef is an 8 byte integer
TODO is it only used during querying like HeadChunkRef?

It packs together:

* 4 Bytes for chunk file index in the block. This number just increments. Filenames [start at 1](https://ganeshvernekar.com/blog/prometheus-tsdb-persistent-block-and-its-index/#contents-of-a-block) but the references probably start at 0 (TODO confirm)
* 4 Bytes offset within the file

### Why does HeadChunkRef contain a series reference and BlockChunkRef does not?

In head the index entry has both series and data hence a chunkref includes a seriesref
In a block, series and chunk data are separate files.  A typical query gets resolved like so:

* `indexReader.Postings()` returns `seriesRefs`
* look up the refs with `index.Series()`, which populates a slice `Meta`, with a `ChunkRef` set for reading the data using `chunkReader`.
