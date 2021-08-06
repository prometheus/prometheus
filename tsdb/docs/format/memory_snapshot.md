# Memory Snapshot Format

Memory snapshot uses the WAL package and writes each series as a WAL record.
Below are the formats of the individual records.

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
