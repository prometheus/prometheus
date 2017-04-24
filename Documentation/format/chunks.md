# Chunks Disk Format

The following describes the format of a single chunks file, which is created in the `chunks/` directory of a block.

```
 ┌────────────────────────────┬──────────────────┐
 │ magic(0x85BD40DD) <4 byte> | version <1 byte> │
 ├────────────────────────────┴──────────────────┤
 │                     Body ...                  │
 └───────────────────────────────────────────────┘
```

Available versions:
* v1 (`1`)

## Body (v1)

The body contains a sequence of chunks, each of which has the following format.

```
┌─────────────────────────────────────────────────────────┐
│ ┌──────────────┬───────────────────┬────────────┐       │
│ │ len <varint> | encoding <1 byte> │    data    │  ...  │
│ └──────────────┴───────────────────┴────────────┘       │
└─────────────────────────────────────────────────────────┘
```

The length marks the length of the encoding byte and data combined.
The CRC checksum is calculated over the encoding byte and data.