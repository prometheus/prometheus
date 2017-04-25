# Chunks Disk Format

The following describes the format of a single chunks file, which is created in the `chunks/` directory of a block.

```
 ┌─────────────────────────────┬─────────────────────┐
 │ magic(0x85BD40DD) <4 byte>  │ version(1) <1 byte> │
 ├─────────────────────────────┴─────────────────────┤
 │ ┌──────────────┬───────────────────┬────────┐     │
 │ │ len <varint> │ encoding <1 byte> │  data  │ ... │
 │ └──────────────┴───────────────────┴────────┘     │
 └───────────────────────────────────────────────────┘
```
