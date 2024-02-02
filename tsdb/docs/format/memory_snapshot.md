# Memory Snapshot Format

Memory snapshot uses the WAL package and writes each series as a WAL record.
Below are the formats of the individual records.

The order of records in the snapshot is always:
1. Starts with series records, one per series, in an unsorted fashion.
2. After all series are done, we write a tombstone record containing all the tombstones.
3. At the end, we write one or more exemplar records while batching up the exemplars in each record. Exemplars are in the order they were written to the circular buffer.

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
