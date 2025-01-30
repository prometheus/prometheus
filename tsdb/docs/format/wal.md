# WAL Disk Format

This document describes the official Prometheus WAL format.

The write ahead log operates in segments that are numbered and sequential,
and are limited to 128MB by default.

## Segment filename

The sequence number is captured in the segment filename,
e.g. `000000`, `000001`, `000002`, etc. The first unsigned integer represents
the sequence number of the segment, typically encoded with six digits.

## Segment encoding

This section describes the segment encoding.

A segment encodes an array of records. It does not contain any header. A segment
is written to pages of 32KB. Only the last page of the most recent segment
may be partial. A WAL record is an opaque byte slice that gets split up into sub-records
should it exceed the remaining space of the current page. Records are never split across
segment boundaries. If a single record exceeds the default segment size, a segment with
a larger size will be created.

The encoding of pages is largely borrowed from [LevelDB's/RocksDB's write ahead log.](https://github.com/facebook/rocksdb/wiki/Write-Ahead-Log-File-Format)

### Records encoding

Each record fragment is encoded as:

```
┌───────────┬──────────┬────────────┬──────────────┐
│ type <1b> │ len <2b> │ CRC32 <4b> │ data <bytes> │
└───────────┴──────────┴────────────┴──────────────┘
```

The initial type byte is made up of three components: a 3-bit reserved field,
a 1-bit zstd compression flag, a 1-bit snappy compression flag, and a 3-bit type flag.

```
┌─────────────────┬──────────────────┬────────────────────┬──────────────────┐
│ reserved <3bit> │ zstd_flag <1bit> │ snappy_flag <1bit> │ type_flag <3bit> │
└─────────────────┴──────────────────┴────────────────────┴──────────────────┘
```

The lowest 3 bits within the type flag represent the record type as follows:

* `0`: rest of page will be empty
* `1`: a full record encoded in a single fragment
* `2`: first fragment of a record
* `3`: middle fragment of a record
* `4`: final fragment of a record

After the type byte, 2-byte length and then 4-byte checksum of the following data are encoded.

All float values are represented using the [IEEE 754 format](https://en.wikipedia.org/wiki/IEEE_754).

### Record types

In the following sections, all the known record types are described. New types,
can be added in the future.

#### Series records

Series records encode the labels that identifies a series and its unique ID.

```
┌────────────────────────────────────────────┐
│ type = 1 <1b>                              │
├────────────────────────────────────────────┤
│ ┌─────────┬──────────────────────────────┐ │
│ │ id <8b> │ n = len(labels) <uvarint>    │ │
│ ├─────────┴────────────┬─────────────────┤ │
│ │ len(str_1) <uvarint> │ str_1 <bytes>   │ │
│ ├──────────────────────┴─────────────────┤ │
│ │  ...                                   │ │
│ ├───────────────────────┬────────────────┤ │
│ │ len(str_2n) <uvarint> │ str_2n <bytes> │ │
│ └───────────────────────┴────────────────┘ │
│                  . . .                     │
└────────────────────────────────────────────┘
```

#### Sample records

Sample records encode samples as a list of triples `(series_id, timestamp, value)`.
Series reference and timestamp are encoded as deltas w.r.t the first sample.
The first row stores the starting id and the starting timestamp.
The first sample record begins at the second row.

```
┌──────────────────────────────────────────────────────────────────┐
│ type = 2 <1b>                                                    │
├──────────────────────────────────────────────────────────────────┤
│ ┌────────────────────┬───────────────────────────┐               │
│ │ id <8b>            │ timestamp <8b>            │               │
│ └────────────────────┴───────────────────────────┘               │
│ ┌────────────────────┬───────────────────────────┬─────────────┐ │
│ │ id_delta <uvarint> │ timestamp_delta <uvarint> │ value <8b>  │ │
│ └────────────────────┴───────────────────────────┴─────────────┘ │
│                              . . .                               │
└──────────────────────────────────────────────────────────────────┘
```

#### Tombstone records

Tombstone records encode tombstones as a list of triples `(series_id, min_time, max_time)`
and specify an interval for which samples of a series got deleted.

```
┌─────────────────────────────────────────────────────┐
│ type = 3 <1b>                                       │
├─────────────────────────────────────────────────────┤
│ ┌─────────┬───────────────────┬───────────────────┐ │
│ │ id <8b> │ min_time <varint> │ max_time <varint> │ │
│ └─────────┴───────────────────┴───────────────────┘ │
│                        . . .                        │
└─────────────────────────────────────────────────────┘
```

#### Exemplar records

Exemplar records encode exemplars as a list of triples `(series_id, timestamp, value)`
plus the length of the labels list, and all the labels.
The first row stores the starting id and the starting timestamp.
Series reference and timestamp are encoded as deltas w.r.t the first exemplar.
The first exemplar record begins at the second row.

See: https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#exemplars

```
┌──────────────────────────────────────────────────────────────────┐
│ type = 4 <1b>                                                    │
├──────────────────────────────────────────────────────────────────┤
│ ┌────────────────────┬───────────────────────────┐               │
│ │ id <8b>            │ timestamp <8b>            │               │
│ └────────────────────┴───────────────────────────┘               │
│ ┌────────────────────┬───────────────────────────┬─────────────┐ │
│ │ id_delta <uvarint> │ timestamp_delta <uvarint> │ value <8b>  │ │
│ ├────────────────────┴───────────────────────────┴─────────────┤ │
│ │  n = len(labels) <uvarint>                                   │ │
│ ├──────────────────────┬───────────────────────────────────────┤ │
│ │ len(str_1) <uvarint> │ str_1 <bytes>                         │ │
│ ├──────────────────────┴───────────────────────────────────────┤ │
│ │  ...                                                         │ │
│ ├───────────────────────┬──────────────────────────────────────┤ │
│ │ len(str_2n) <uvarint> │ str_2n <bytes> │                     │ │
│ └───────────────────────┴────────────────┴─────────────────────┘ │
│                              . . .                               │
└──────────────────────────────────────────────────────────────────┘
```

#### Metadata records

Metadata records encode the metadata updates associated with a series.

```
┌────────────────────────────────────────────┐
│ type = 6 <1b>                              │
├────────────────────────────────────────────┤
│ ┌────────────────────────────────────────┐ │
│ │ series_id <uvarint>                    │ │
│ ├────────────────────────────────────────┤ │
│ │ metric_type <1b>                       │ │
│ ├────────────────────────────────────────┤ │
│ │ num_fields <uvarint>                   │ │
│ ├───────────────────────┬────────────────┤ │
│ │ len(name_1) <uvarint> │ name_1 <bytes> │ │
│ ├───────────────────────┼────────────────┤ │
│ │ len(val_1) <uvarint>  │ val_1 <bytes>  │ │
│ ├───────────────────────┴────────────────┤ │
│ │                . . .                   │ │
│ ├───────────────────────┬────────────────┤ │
│ │ len(name_n) <uvarint> │ name_n <bytes> │ │
│ ├───────────────────────┼────────────────┤ │
│ │ len(val_n) <uvarint>  │ val_n <bytes>  │ │
│ └───────────────────────┴────────────────┘ │
│                  . . .                     │
└────────────────────────────────────────────┘
```

#### Histogram records

Histogram records encode the integer and float native histogram samples.

A record with the integer native histograms with the exponential bucketing:

```
┌───────────────────────────────────────────────────────────────────────┐
│ type = 7 <1b>                                                         │
├───────────────────────────────────────────────────────────────────────┤
│ ┌────────────────────┬───────────────────────────┐                    │
│ │ id <8b>            │ timestamp <8b>            │                    │
│ └────────────────────┴───────────────────────────┘                    │
│ ┌────────────────────┬──────────────────────────────────────────────┐ │
│ │ id_delta <uvarint> │ timestamp_delta <uvarint>                    │ │
│ ├────────────────────┴────┬─────────────────────────────────────────┤ │
│ │ counter_reset_hint <1b> │ schema <varint>                         │ │
│ ├─────────────────────────┴────┬────────────────────────────────────┤ │
│ │ zero_threshold (float) <8b>  │   zero_count <uvarint>             │ │
│ ├─────────────────┬────────────┴────────────────────────────────────┤ │
│ │ count <uvarint> │ sum (float) <8b>                                │ │
│ ├─────────────────┴─────────────────────────────────────────────────┤ │
│ │ positive_spans_num <uvarint>                                      │ │
│ ├─────────────────────────────────┬─────────────────────────────────┤ │
│ │ positive_span_offset_1 <varint> │ positive_span_len_1 <uvarint32> │ │
│ ├─────────────────────────────────┴─────────────────────────────────┤ │
│ │ . . .                                                             │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ negative_spans_num <uvarint>                                      │ │
│ ├───────────────────────────────┬───────────────────────────────────┤ │
│ │ negative_span_offset <varint> │ negative_span_len <uvarint32>     │ │
│ ├───────────────────────────────┴───────────────────────────────────┤ │
│ │ . . .                                                             │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ positive_bkts_num <uvarint>                                       │ │
│ ├─────────────────────────┬───────┬─────────────────────────────────┤ │
│ │ positive_bkt_1 <varint> │ . . . │ positive_bkt_n <varint>         │ │
│ ├─────────────────────────┴───────┴─────────────────────────────────┤ │
│ │ negative_bkts_num <uvarint>                                       │ │
│ ├─────────────────────────┬───────┬─────────────────────────────────┤ │
│ │ negative_bkt_1 <varint> │ . . . │ negative_bkt_n <varint>         │ │
│ └─────────────────────────┴───────┴─────────────────────────────────┘ │
│                              . . .                                    │
└───────────────────────────────────────────────────────────────────────┘
```

A record with the float native histograms with the exponential bucketing:

```
┌───────────────────────────────────────────────────────────────────────┐
│ type = 8 <1b>                                                         │
├───────────────────────────────────────────────────────────────────────┤
│ ┌────────────────────┬───────────────────────────┐                    │
│ │ id <8b>            │ timestamp <8b>            │                    │
│ └────────────────────┴───────────────────────────┘                    │
│ ┌────────────────────┬──────────────────────────────────────────────┐ │
│ │ id_delta <uvarint> │ timestamp_delta <uvarint>                    │ │
│ ├────────────────────┴────┬─────────────────────────────────────────┤ │
│ │ counter_reset_hint <1b> │ schema <varint>                         │ │
│ ├─────────────────────────┴────┬────────────────────────────────────┤ │
│ │ zero_threshold (float) <8b>  │   zero_count (float) <8b>          │ │
│ ├────────────────────┬─────────┴────────────────────────────────────┤ │
│ │ count (float) <8b> │ sum (float) <8b>                             │ │
│ ├────────────────────┴──────────────────────────────────────────────┤ │
│ │ positive_spans_num <uvarint>                                      │ │
│ ├─────────────────────────────────┬─────────────────────────────────┤ │
│ │ positive_span_offset_1 <varint> │ positive_span_len_1 <uvarint32> │ │
│ ├─────────────────────────────────┴─────────────────────────────────┤ │
│ │ . . .                                                             │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ negative_spans_num <uvarint>                                      │ │
│ ├───────────────────────────────┬───────────────────────────────────┤ │
│ │ negative_span_offset <varint> │ negative_span_len <uvarint32>     │ │
│ ├───────────────────────────────┴───────────────────────────────────┤ │
│ │ . . .                                                             │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ positive_bkts_num <uvarint>                                       │ │
│ ├─────────────────────────────┬───────┬─────────────────────────────┤ │
│ │ positive_bkt_1 (float) <8b> │ . . . │ positive_bkt_n (float) <8b> │ │
│ ├─────────────────────────────┴───────┴─────────────────────────────┤ │
│ │ negative_bkts_num <uvarint>                                       │ │
│ ├─────────────────────────────┬───────┬─────────────────────────────┤ │
│ │ negative_bkt_1 (float) <8b> │ . . . │ negative_bkt_n (float) <8b> │ │
│ └─────────────────────────────┴───────┴─────────────────────────────┘ │
│                              . . .                                    │
└───────────────────────────────────────────────────────────────────────┘
```

A record with the integer native histograms with the custom bucketing, also known as NHCB.
This record format is backwards compatible with type 7.

```
┌───────────────────────────────────────────────────────────────────────┐
│ type = 9 <1b>                                                         │
├───────────────────────────────────────────────────────────────────────┤
│ ┌────────────────────┬───────────────────────────┐                    │
│ │ id <8b>            │ timestamp <8b>            │                    │
│ └────────────────────┴───────────────────────────┘                    │
│ ┌────────────────────┬──────────────────────────────────────────────┐ │
│ │ id_delta <uvarint> │ timestamp_delta <uvarint>                    │ │
│ ├────────────────────┴────┬─────────────────────────────────────────┤ │
│ │ counter_reset_hint <1b> │ schema <varint>                         │ │
│ ├─────────────────────────┴────┬────────────────────────────────────┤ │
│ │ zero_threshold (float) <8b>  │   zero_count <uvarint>             │ │
│ ├─────────────────┬────────────┴────────────────────────────────────┤ │
│ │ count <uvarint> │ sum (float) <8b>                                │ │
│ ├─────────────────┴─────────────────────────────────────────────────┤ │
│ │ positive_spans_num <uvarint>                                      │ │
│ ├─────────────────────────────────┬─────────────────────────────────┤ │
│ │ positive_span_offset_1 <varint> │ positive_span_len_1 <uvarint32> │ │
│ ├─────────────────────────────────┴─────────────────────────────────┤ │
│ │ . . .                                                             │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ negative_spans_num <uvarint> = 0                                  │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ positive_bkts_num <uvarint>                                       │ │
│ ├─────────────────────────┬───────┬─────────────────────────────────┤ │
│ │ positive_bkt_1 <varint> │ . . . │ positive_bkt_n <varint>         │ │
│ ├─────────────────────────┴───────┴─────────────────────────────────┤ │
│ │ negative_bkts_num <uvarint> = 0                                   │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ custom_values_num <uvarint>                                       │ │
│ ├─────────────────────────────┬───────┬─────────────────────────────┤ │
│ │ custom_value_1 (float) <8b> │ . . . │ custom_value_n (float) <8b> │ │
│ └─────────────────────────────┴───────┴─────────────────────────────┘ │
│                              . . .                                    │
└───────────────────────────────────────────────────────────────────────┘
```

A record with the float native histograms with the custom bucketing, also known as NHCB.
This record format is backwards compatible with type 8.

```
┌───────────────────────────────────────────────────────────────────────┐
│ type = 10 <1b>                                                        │
├───────────────────────────────────────────────────────────────────────┤
│ ┌────────────────────┬───────────────────────────┐                    │
│ │ id <8b>            │ timestamp <8b>            │                    │
│ └────────────────────┴───────────────────────────┘                    │
│ ┌────────────────────┬──────────────────────────────────────────────┐ │
│ │ id_delta <uvarint> │ timestamp_delta <uvarint>                    │ │
│ ├────────────────────┴────┬─────────────────────────────────────────┤ │
│ │ counter_reset_hint <1b> │ schema <varint>                         │ │
│ ├─────────────────────────┴────┬────────────────────────────────────┤ │
│ │ zero_threshold (float) <8b>  │   zero_count (float) <8b>          │ │
│ ├────────────────────┬─────────┴────────────────────────────────────┤ │
│ │ count (float) <8b> │ sum (float) <8b>                             │ │
│ ├────────────────────┴──────────────────────────────────────────────┤ │
│ │ positive_spans_num <uvarint>                                      │ │
│ ├─────────────────────────────────┬─────────────────────────────────┤ │
│ │ positive_span_offset_1 <varint> │ positive_span_len_1 <uvarint32> │ │
│ ├─────────────────────────────────┴─────────────────────────────────┤ │
│ │ . . .                                                             │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ negative_spans_num <uvarint> = 0                                  │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ positive_bkts_num <uvarint>                                       │ │
│ ├─────────────────────────────┬───────┬─────────────────────────────┤ │
│ │ positive_bkt_1 (float) <8b> │ . . . │ positive_bkt_n (float) <8b> │ │
│ ├─────────────────────────────┴───────┴─────────────────────────────┤ │
│ │ negative_bkts_num <uvarint> = 0                                   │ │
│ ├───────────────────────────────────────────────────────────────────┤ │
│ │ custom_values_num <uvarint>                                       │ │
│ ├─────────────────────────────┬───────┬─────────────────────────────┤ │
│ │ custom_value_1 (float) <8b> │ . . . │ custom_value_n (float) <8b> │ │
│ └─────────────────────────────┴───────┴─────────────────────────────┘ │
│                              . . .                                    │
└───────────────────────────────────────────────────────────────────────┘
```
