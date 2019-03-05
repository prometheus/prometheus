## master / unreleased

## 0.6.1
  - [BUGFIX] Update `last` after appending a non-overlapping chunk in `chunks.MergeOverlappingChunks`. [#539](https://github.com/prometheus/tsdb/pull/539)

## 0.6.0
  - [CHANGE] `AllowOverlappingBlock` is now `AllowOverlappingBlocks`.

## 0.5.0
 - [FEATURE] Time-ovelapping blocks are now allowed. [#370](https://github.com/prometheus/tsdb/pull/370)
   - Disabled by default and can be enabled via `AllowOverlappingBlock` option.
   - Added `MergeChunks` function in `chunkenc/xor.go` to merge 2 time-overlapping chunks.
   - Added `MergeOverlappingChunks` function in `chunks/chunks.go` to merge multiple time-overlapping Chunk Metas.
   - Added `MinTime` and `MaxTime` method for `BlockReader`.
 - [FEATURE] New `dump` command to tsdb tool to dump all samples.
 - [FEATURE] New `encoding` package for common binary encoding/decoding helpers. 
    - Added to remove some code duplication.
 - [ENHANCEMENT] When closing the db any running compaction will be cancelled so it doesn't block.
   - `NewLeveledCompactor` takes a context.
 - [CHANGE] `prometheus_tsdb_storage_blocks_bytes_total` is now `prometheus_tsdb_storage_blocks_bytes`.
 - [BUGFIX] Improved Postings Merge performance. Fixes a regression from the the previous release.
 - [BUGFIX] LiveReader can get into an infinite loop on corrupt WALs.

## 0.4.0
 - [CHANGE] New `WALSegmentSize` option to override the `DefaultOptions.WALSegmentSize`. Added to allow using smaller wal files. For example using tmpfs on a RPI to minimise the SD card wear out from the constant WAL writes. As part of this change the `DefaultOptions.WALSegmentSize` constant was also exposed.
 - [CHANGE] Empty blocks are not written during compaction [#374](https://github.com/prometheus/tsdb/pull/374)
 - [FEATURE]  Size base retention through `Options.MaxBytes`.  As part of this change:
   - Added new metrics - `prometheus_tsdb_storage_blocks_bytes_total`, `prometheus_tsdb_size_retentions_total`, `prometheus_tsdb_time_retentions_total`
   - New public interface `SizeReader: Size() int64`
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
 - [FEATURE] tsdbutil analyze subcomand to find churn, high cardinality, etc.
