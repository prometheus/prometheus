## master / unreleased

 - `LastCheckpoint` used to return just the segment name and now it returns the full relative path.
 - `NewSegmentsRangeReader` can now read over miltiple wal ranges by using the new `SegmentRange` struct.
 - `CorruptionErr` now also exposes the Segment `Dir` which is added when displaying any errors.
