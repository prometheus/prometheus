# Head Chunks on Disk Format

The following describes the format of a chunks file,
which is created in the `wal/chunks/` inside the data directory.

Chunks in the files are referenced from the index by uint64 composed of
in-file offset (lower 4 bytes) and segment sequence number (upper 4 bytes).

The mint and maxt in the file header is the maximum and minimum times
among all the chunks that it holds.

```
┌──────────────────────────────┐
│  magic(0x85BD40DD) <4 byte>  │
├──────────────────────────────┤
│    version(1) <1 byte>       │
├──────────────────────────────┤
│    padding(0) <3 byte>       │
├──────────────────────────────┤
│    mint <8 byte, uint64>     │
├──────────────────────────────┤
│    maxt <8 byte, uint64>     │
├──────────────────────────────┤
│ ┌──────────────────────────┐ │
│ │         Chunk 1          │ │
│ ├──────────────────────────┤ │
│ │          ...             │ │
│ ├──────────────────────────┤ │
│ │         Chunk N          │ │
│ └──────────────────────────┘ │
└──────────────────────────────┘
```


# Chunk

```
┌─────────────────────┬───────────────────────┬───────────────────────┬───────────────────┬───────────────┬──────────────┐
| series ref <8 byte> | mint <8 byte, uint64> | maxt <8 byte, uint64> | encoding <1 byte> | len <uvarint> | data <bytes> |
└─────────────────────┴───────────────────────┴───────────────────────┴───────────────────┴───────────────┴──────────────┘
```
