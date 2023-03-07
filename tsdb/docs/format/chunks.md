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
┌──────────────────────┬───────────────┬───────────────┬──────────────────────┬──────────────────────┬──────────────────────┬──────────────────────┬─────┬──────────────────────┬──────────────────────┬──────────────────┐
│ num_samples <uint16> │ ts_0 <varint> │ v_0 <float64> │ ts_1_delta <uvarint> │ v_1_xor <varbit_xor> │ ts_2_dod <varbit_ts> │ v_2_xor <varbit_xor> │ ... │ ts_n_dod <varbit_ts> │ v_n_xor <varbit_xor> │ padding <x bits> │
└──────────────────────┴───────────────┴───────────────┴──────────────────────┴──────────────────────┴──────────────────────┴──────────────────────┴─────┴──────────────────────┴──────────────────────┴──────────────────┘
```

### Notes:

* `ts` is the timestamp, `v` is the value.
* `...` means to repeat the previous two fields as needed, with `n` starting at 2 and going up to `num_samples` – 1.
* `<uint16>` has 2 bytes in big-endian order.
* `<varint>` and `<uvarint>` have 1 to 10 bytes each.
* `ts_1_delta` is `ts_1` – `ts_0`.
* `ts_n_dod` is the “delta of deltas” of timestamps, i.e. (`ts_n` – `ts_n-1`) – (`ts_n-1` – `ts_n-2`).
* `v_n_xor` is the result of `v_n` XOR `v_n-1`.
* `<varbit_xor>` is a specific variable bitwidth encoding of the result of XORing the current and the previous value. It has between 1 bit and 77 bits.
  See [code for details](https://github.com/prometheus/prometheus/blob/7309c20e7e5774e7838f183ec97c65baa4362edc/tsdb/chunkenc/xor.go#L220-L253).
* `<varbit_ts>` is a specific variable bitwidth encoding for the “delta of deltas” of timestamps (signed integers that are ideally small).
  It has between 1 and 68 bits.
  see [code for details](https://github.com/prometheus/prometheus/blob/7309c20e7e5774e7838f183ec97c65baa4362edc/tsdb/chunkenc/xor.go#L179-L205).
* `padding` of 0 to 7 bits so that the whole chunk data is byte-aligned.
* The chunk can have as few as one sample, i.e. `ts_1`, `v_1`, etc. are optional.

## Histogram chunk data

```
┌──────────────────────┬──────────────────────────┬───────────────────────────────┬─────────────────────┬──────────────────┬──────────────────┬────────────────┬──────────────────┐
│ num_samples <uint16> │ histogram_flags <1 byte> │ zero_threshold <1 or 9 bytes> │ schema <varbit_int> │ pos_spans <data> │ neg_spans <data> │ samples <data> │ padding <x bits> │
└──────────────────────┴──────────────────────────┴───────────────────────────────┴─────────────────────┴──────────────────┴──────────────────┴────────────────┴──────────────────┘
```

### Positive and negative spans data:

```
┌─────────────────────────┬────────────────────────┬───────────────────────┬────────────────────────┬───────────────────────┬─────┬────────────────────────┬───────────────────────┐
│ num_spans <varbit_uint> │ length_0 <varbit_uint> │ offset_0 <varbit_int> │ length_1 <varbit_uint> │ offset_1 <varbit_int> │ ... │ length_n <varbit_uint> │ offset_n <varbit_int> │
└─────────────────────────┴────────────────────────┴───────────────────────┴────────────────────────┴───────────────────────┴─────┴────────────────────────┴───────────────────────┘
```

### Samples data:

```
┌──────────────────────────┐
│    sample_0 <data>       │
├──────────────────────────┤
│    sample_1 <data>       │
├──────────────────────────┤
│    sample_2 <data>       │
├──────────────────────────┤
│          ...             │
├──────────────────────────┤
│    Sample_n <data>       │
└──────────────────────────┘
```

#### Sample 0 data:

```
┌─────────────────┬─────────────────────┬──────────────────────────┬───────────────┬───────────────────────────┬─────┬───────────────────────────┬───────────────────────────┬─────┬───────────────────────────┐
│ ts <varbit_int> │ count <varbit_uint> │ zero_count <varbit_uint> │ sum <float64> │ pos_bucket_0 <varbit_int> │ ... │ pos_bucket_n <varbit_int> │ neg_bucket_0 <varbit_int> │ ... │ neg_bucket_n <varbit_int> │
└─────────────────┴─────────────────────┴──────────────────────────┴───────────────┴───────────────────────────┴─────┴───────────────────────────┴───────────────────────────┴─────┴───────────────────────────┘
```

#### Sample 1 data:

```
┌────────────────────────┬───────────────────────────┬────────────────────────────────┬──────────────────────┬─────────────────────────────────┬─────┬─────────────────────────────────┬─────────────────────────────────┬─────┬─────────────────────────────────┐
│ ts_delta <varbit_uint> │ count_delta <varbit_uint> │ zero_count_delta <varbit_uint> │ sum_xor <varbit_xor> │ pos_bucket_0_delta <varbit_int> │ ... │ pos_bucket_n_delta <varbit_int> │ neg_bucket_0_delta <varbit_int> │ ... │ neg_bucket_n_delta <varbit_int> │
└────────────────────────┴───────────────────────────┴────────────────────────────────┴──────────────────────┴─────────────────────────────────┴─────┴─────────────────────────────────┴─────────────────────────────────┴─────┴─────────────────────────────────┘
```

#### Sample 2 data and following:

```
┌─────────────────────┬────────────────────────┬─────────────────────────────┬──────────────────────┬───────────────────────────────┬─────┬───────────────────────────────┬───────────────────────────────┬─────┬───────────────────────────────┐
│ ts_dod <varbit_int> │ count_dod <varbit_int> │ zero_count_dod <varbit_int> │ sum_xor <varbit_xor> │ pos_bucket_0_dod <varbit_int> │ ... │ pos_bucket_n_dod <varbit_int> │ neg_bucket_0_dod <varbit_int> │ ... │ neg_bucket_n_dod <varbit_int> │
└─────────────────────┴────────────────────────┴─────────────────────────────┴──────────────────────┴───────────────────────────────┴─────┴───────────────────────────────┴───────────────────────────────┴─────┴───────────────────────────────┘
```

### Notes:

* `histogram_flags` is a byte of which currently only the first two bits are used:
  * `10`: Counter reset between the previous chunk and this one.
  * `01`: No counter reset between the previous chunk and this one.
  * `00`: Counter reset status unknown.
  * `11`: Chunk is part of a gauge histogram, no counter resets are happening.
* `zero_threshold` has a specific encoding:
  * If 0, it is a single zero byte.
  * If a power of two between 2^-243 and 2^10, it is a single byte between 1 and 254.
  * Otherwise, it is a byte with all bits set (255), followed by a float64, resulting in 9 bytes length.
* `schema` is a specific value defined by the exposition format. Currently valid values are -4 <= n <= 8.
* `<varbit_int>` is a variable bitwidth encoding for signed integers, optimized for “delta of deltas” of bucket deltas. It has between 1 bit and 9 bytes.
  See [code for details](https://github.com/prometheus/prometheus/blob/8c1507ebaa4ca552958ffb60c2d1b21afb7150e4/tsdb/chunkenc/varbit.go#L31-L60).
* `<varbit_uint>` is a variable bitwidth encoding for unsigned integers with the same bit-bucketing as `<varbit_int>`.
  See [code for details](https://github.com/prometheus/prometheus/blob/8c1507ebaa4ca552958ffb60c2d1b21afb7150e4/tsdb/chunkenc/varbit.go#L136-L165).
* `<varbit_xor>` is a specific variable bitwidth encoding of the result of XORing the current and the previous value. It has between 1 bit and 77 bits.
  See [code for details](https://github.com/prometheus/prometheus/blob/8c1507ebaa4ca552958ffb60c2d1b21afb7150e4/tsdb/chunkenc/histogram.go#L538-L574).
* `padding` of 0 to 7 bits so that the whole chunk data is byte-aligned.
* Note that buckets are inherently deltas between the current bucket and the previous bucket. Only `bucket_0` is an absolute count.
* The chunk can have as few as one sample, i.e. sample 1 and following are optional.
* Similarly, there could be down to zero spans and down to zero buckets.
