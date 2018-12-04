## master / unreleased

 - `LastCheckpoint` used to return just the segment name and now it returns the full relative path.
 - `NewSegmentsRangeReader` can now read over miltiple wal ranges by using the new `SegmentRange` struct.
 - `CorruptionErr` now also exposes the Segment `Dir` which is added when displaying any errors.
 - `Head.Init()` is changed to `Head.Init(minValidTime int64)` where `minValidTime` is taken from the maxt of the last persisted block and any samples below `minValidTime` will not be loaded from the wal in the head. The same value is used when using the `Heah.Appender()` to disallow adding samples below the `minValidTime` timestamp. This change was nececary to fix a bug where a `Snapshot()` with the head included would create a block with custom time range(not bound to the default time ranges) and the next block population from the head would create an overlapping block.  
    - https://github.com/prometheus/tsdb/issues/446