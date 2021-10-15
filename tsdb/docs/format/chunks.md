# Chunks Disk Format

The following describes the format of a chunks file,
which is created in the `chunks/` directory of a block.
The maximum size per segment file is 512MiB.

Chunks in the files are referenced from the index by uint64 composed of
in-file offset (lower 4 bytes) and segment sequence number (upper 4 bytes).

```
┌──────────────────────────────┐
│  magic(0x85BD40DD) <4 byte>  │
├──────────────────────────────┤
│    version(1) <1 byte>       │
├──────────────────────────────┤
│    padding(0) <3 byte>       │
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
┌───────────────┬───────────────────┬──────────────┬────────────────┐
│ len <uvarint> │ encoding <1 byte> │ data <bytes> │ CRC32 <4 byte> │
└───────────────┴───────────────────┴──────────────┴────────────────┘
```

Notes:
* `<uvarint>` has 1 to 10 bytes.
* `encoding`: Currently either `XOR` or `histogram`.
* `data`: See below for each encoding.

## XOR chunk data

```
┌──────────────────────┬───────────────┬───────────────┬──────────────────────┬──────────────────────┬──────────────────────┬──────────────────────┬─────┐
│ num_samples <uint16> │ ts_0 <varint> │ v_0 <float64> │ ts_1_delta <uvarint> │ v_1_xor <varbit_xor> │ ts_n_dod <varbit_ts> │ v_n_xor <varbit_xor> │ ... │
└──────────────────────┴───────────────┴───────────────┴──────────────────────┴──────────────────────┴──────────────────────┴──────────────────────┴─────┘
```

### Notes:

* `ts` is the timestamp, `v` is the value.
* `...` means to repeat the previous two fields as needed, with `n` starting at 2 and going up to `num_samples` – 1.
* `<uint16>` has 2 bytes in big-endian order.
* `<varint>` and `<uvarint>` have 1 to 10 bytes each.
* `ts_1_delta` is `ts_1` – `ts_0`.
* `ts_n_dod` is the “delta of deltas” of timestamps, i.e. (`ts_n` – `ts_n-1`) – (`ts_n-1` – `ts_n-2`).
* `v_n_xor>` is the result of `v_n` XOR `v_n-1`.
* `<varbit_xor>` is a specific variable bitwidth encoding of the result of XORing the current and the previous value. It has between 1 bit and 77 bits.
  See [code for details](https://github.com/prometheus/prometheus/blob/7309c20e7e5774e7838f183ec97c65baa4362edc/tsdb/chunkenc/xor.go#L220-L253).
* `<varbit_ts>` is a specific variable bitwidth encoding for the “delta of deltas” of timestamps (signed integers that are ideally small).
  It has between 1 and 68 bits.
  see [code for details](https://github.com/prometheus/prometheus/blob/7309c20e7e5774e7838f183ec97c65baa4362edc/tsdb/chunkenc/xor.go#L179-L205).

## Histogram chunk data

```
┌──────────────────────┬───────────────────────────────┬─────────────────────┬──────────────────┬──────────────────┬────────────────┐
│ num_samples <uint16> │ zero_threshold <1 or 9 bytes> │ schema <varbit_int> │ pos_spans <data> │ neg_spans <data> │ samples <data> │
└──────────────────────┴───────────────────────────────┴─────────────────────┴──────────────────┴──────────────────┴────────────────┘
```

### Positive and negative spans data:

```
┌───────────────────┬────────────────────────┬───────────────────────┬─────┬──────────────────────────┬─────────────────────────┐
│ num <varbit_uint> │ length_1 <varbit_uint> │ offset_1 <varbit_int> │ ... │ length_num <varbit_uint> │ offset_num <varbit_int> │
└───────────────────┴────────────────────────┴───────────────────────┴─────┴──────────────────────────┴─────────────────────────┘
```

### Samples data:

```
TODO
```

### Notes:

* `zero_threshold` has a specific encoding:
  * If 0, it is a single zero byte.
  * If a power of two between 2^-243 and 2^10, it is a single byte between 1 and 254.
  * Otherwise, it is a byte with all bits set (255), followed by a float64, resulting in 9 bytes length.
* `schema` is a specific value defined by the exposition format. Currently valid values are -4 <= n <= 8.
* `<varbit_int>` is a variable bitwidth encoding for signed integers, optimized for “delta of deltas” of bucket deltas. It has between 1 bit and 9 bytes.
* `<varbit_uint>` is a variable bitwidth encoding for unsigned integers with the same bit-bucketing as `<varbit_int>`.
