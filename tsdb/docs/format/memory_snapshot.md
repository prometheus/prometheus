# Memory Snapshot Format

Memory snapshot uses the WAL package and writes each series as a WAL record.
Below are the formats of the individual records.

The order of records in the snapshot is always:
1. Starts with series records, one per series, in an unsorted fashion.
2. After all series are done, we write a tombstone record containing all the tombstones.
3. Exemplar records follow, batching up the exemplars in each record. Exemplars are in the order they were written to the circular buffer.
4. At the end, we write one or more WAL series expiry records. An empty record is written even when there are no expiries.

When the WAL is enabled, snapshots without WAL series expiry records are replayed from the WAL instead. This
includes snapshots written by older versions, which did not preserve the state
needed to keep series metadata during subsequent WAL checkpointing. Readers from
older versions also fall back to the WAL when they encounter the new record type.
Legacy snapshots remain usable when the WAL is disabled, since no WAL checkpoint
can remove their series metadata in that configuration.

### Series records

This record is a snapshot of a single series. Only one series exists per record.
It includes the metadata of the series and the in-memory chunk data if it exists.
The sampleBuf is the last 4 samples in the in-memory chunk.

```
┌──────────────────────────┬────────────────────────────┐
│     Record Type <byte>   │   Series Ref <uint64>      │
├──────────────────────────┴────────────────────────────┤
│               Number of Labels <uvarint>              │
├──────────────────────────────┬────────────────────────┤
│     len(name_1) <uvarint>    │    name_1 <bytes>      │
├──────────────────────────────┼────────────────────────┤
│     len(val_1) <uvarint>     │    val_1 <bytes>       │
├──────────────────────────────┴────────────────────────┤
│                         . . .                         │
├──────────────────────────────┬────────────────────────┤
│     len(name_N) <uvarint>    │    name_N <bytes>      │
├──────────────────────────────┼────────────────────────┤
│     len(val_N) <uvarint>     │    val_N <bytes>       │
├──────────────────────────────┴────────────────────────┤
│                  Chunk Range <int64>                  │
├───────────────────────────────────────────────────────┤
│                 Chunk Exists <uvarint>                │
│ # 1 if head chunk exists, 0 otherwise to detect a nil |
| # chunk. Below fields exists only when it's 1 here.   |
├───────────────────────────┬───────────────────────────┤
│     Chunk Mint <int64>    │    Chunk Maxt <int64>     │
├───────────────────────────┴───────────────────────────┤
│                 Chunk Encoding <byte>                 │
├──────────────────────────────┬────────────────────────┤
│      len(Chunk) <uvarint>    │    Chunk <bytes>       │
├──────────────────────────┬───┴────────────────────────┤
|  sampleBuf[0].t <int64>  |  sampleBuf[0].v <float64>  | 
├──────────────────────────┼────────────────────────────┤
|  sampleBuf[1].t <int64>  |  sampleBuf[1].v <float64>  | 
├──────────────────────────┼────────────────────────────┤
|  sampleBuf[2].t <int64>  |  sampleBuf[2].v <float64>  | 
├──────────────────────────┼────────────────────────────┤
|  sampleBuf[3].t <int64>  |  sampleBuf[3].v <float64>  | 
└──────────────────────────┴────────────────────────────┘
```

### Tombstone record

This includes all the tombstones in the Head block. A single record is written into
the snapshot for all the tombstones. The encoded tombstones uses the same encoding
as tombstone file in blocks.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Record Type <byte>                       │
├───────────────────────────────────┬─────────────────────────────┤
│ len(Encoded Tombstones) <uvarint> │ Encoded Tombstones <bytes>  │
└───────────────────────────────────┴─────────────────────────────┘
```

### Exemplar record

A single exemplar record contains one or more exemplars, encoded in the same way as we do in WAL but with changed record type.

```
┌───────────────────────────────────────────────────────────────────┐
│                      Record Type <byte>                           │
├───────────────────────────────────────────────────────────────────┤
│ ┌────────────────────┬───────────────────────────┐                │
│ │ series ref <8b>    │ timestamp <8b>            │                │
│ └────────────────────┴───────────────────────────┘                │
│ ┌─────────────────────┬───────────────────────────┬─────────────┐ │
│ │ ref_delta <uvarint> │ timestamp_delta <uvarint> │ value <8b>  │ │
│ ├─────────────────────┴───────────────────────────┴─────────────┤ │
│ │  n = len(labels) <uvarint>                                    │ │
│ ├───────────────────────────────┬───────────────────────────────┤ │
│ │     len(str_1) <uvarint>      │       str_1 <bytes>           │ │
│ ├───────────────────────────────┴───────────────────────────────┤ │
│ │                              ...                              │ │
│ ├───────────────────────────────┬───────────────────────────────┤ │
│ │     len(str_2n) <uvarint>     │       str_2n <bytes>          │ │
│ ├───────────────────────────────┴───────────────────────────────┤ │
│                               . . .                               │
└───────────────────────────────────────────────────────────────────┘
```

### WAL series expiry record

Record type `4` preserves references that are no longer in the Head but whose
series metadata must remain in the WAL. It contains up to 10,000 pairs, in no
particular order, each consisting of a big-endian `uint64` series reference and
a big-endian `int64` keep-until timestamp. There is no pair count; the record
length determines the number of pairs. A one-byte record contains no expiries.

The timestamp is inclusive: the series record is retained in a checkpoint whose
minimum timestamp is less than or equal to the keep-until timestamp. Restoring
these references also advances the series ID counter so that new series cannot
reuse references still needed by WAL readers.

| Field | Size |
| --- | --- |
| Record type (`4`) | 1 byte |
| Series reference | 8 bytes |
| Keep-until timestamp | 8 bytes |
| Further reference/timestamp pairs | 16 bytes each |
