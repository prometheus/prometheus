# Chunks Disk Format

The following describes the format of a single chunks file, which is created in the `chunks/` directory of a block. The maximum size per segment file is 512MiB.

Chunks in the files are referenced from the index by the in-file offset in the 4 LSB and the segment sequence number in the higher 4 MSBs.

```
┌────────────────────────────────────────┬──────────────────────┐
│ magic(0x85BD40DD) <4 byte>             │ version(1) <1 byte>  │
├────────────────────────────────────────┴──────────────────────┤
│ ┌───────────────┬───────────────────┬──────┬────────────────┐ │
│ │ len <uvarint> │ encoding <1 byte> │ data │ CRC32 <4 byte> │ │
│ └───────────────┴───────────────────┴──────┴────────────────┘ │
└───────────────────────────────────────────────────────────────┘
```
