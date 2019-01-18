## master / unreleased

## 0.4.0
 - [CHANGE] New `WALSegmentSize` option to override the `DefaultOptions.WALSegmentSize`. Added to allow using smaller wal files. For example using tmpfs on a RPI to minimise the SD card wear out from the constant WAL writes. As part of this change the `DefaultOptions.WALSegmentSize` constant was also exposed.
 - [CHANGE] Empty blocks are not written during compaction [#374](https://github.com/prometheus/tsdb/pull/374)
 - [FEATURE]  Size base retention through `Options.MaxBytes`.  As part of this change:
    - added new metrics - `prometheus_tsdb_storage_blocks_bytes_total`, `prometheus_tsdb_size_retentions_total`, `prometheus_tsdb_time_retentions_total`
    - new public interface `SizeReader: Size() int64`
    - `OpenBlock` signature changed to take a logger.
 - [REMOVED] `PrefixMatcher` is considered unused so was removed.
 - [CLEANUP] `Options.WALFlushInterval` is removed as it wasn't used anywhere.
 - [FEATURE] Add new `LiveReader` to WAL pacakge. Added to allow live tailing of a WAL segment, used by Prometheus Remote Write after refactor. The main difference between the new reader and the existing `Reader` is that for `LiveReader` a call to `Next()` that returns false does not mean that there will never be more data to read.

## 0.3.1
 - [BUGFIX] Fixed most windows test and some actual bugs for unclosed file readers.

## 0.3.0
 - [CHANGE] `LastCheckpoint()` used to return just the segment name and now it returns the full relative path.
 - [CHANGE] `NewSegmentsRangeReader()` can now read over miltiple wal ranges by using the new `SegmentRange{}` struct.
 - [CHANGE] `CorruptionErr{}` now also exposes the Segment `Dir` which is added when displaying any errors.
 - [CHANGE] `Head.Init()` is changed to `Head.Init(minValidTime int64)`  
 - [CHANGE] `SymbolTable()` renamed to `SymbolTableSize()` to make the name consistent with the  `Block{ symbolTableSize uint64 }` field.
 - [CHANGE] `wal.Reader{}` now exposes `Segment()` for the current segment being read  and `Offset()` for the current offset.
  -[FEATURE] tsdbutil analyze subcomand to find churn, high cardinality, etc.
