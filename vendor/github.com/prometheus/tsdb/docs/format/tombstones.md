# Tombstones Disk Format

The following describes the format of a tombstones file, which is placed
at the top level directory of a block.

The last 8 bytes specifies the offset to the start of Stones section.
The stones section is 0 padded to a multiple of 4 for fast scans.

```
┌────────────────────────────┬─────────────────────┐
│ magic(0x130BA30) <4b>      │ version(1) <1 byte> │
├────────────────────────────┴─────────────────────┤
│ ┌──────────────────────────────────────────────┐ │
│ │                Tombstone 1                   │ │
│ ├──────────────────────────────────────────────┤ │
│ │                      ...                     │ │
│ ├──────────────────────────────────────────────┤ │
│ │                Tombstone N                   │ │
│ ├──────────────────────────────────────────────┤ │
│ │                  CRC<4b>                     │ │
│ └──────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

# Tombstone 

```
┌─────────────┬───────────────┬──────────────┐
│ref <varint> │ mint <varint> │ maxt <varint>│
└─────────────┴───────────────┴──────────────┘
```
