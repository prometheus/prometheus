# Index Disk Format

The following describes the format of the `index` file found in each block directory.
It is terminated by a table of contents which serves as an entry point into the index.

```
┌────────────────────────────┬─────────────────────┐
│ magic(0xBAAAD700) <4b>     │ version(1) <1 byte> │
├────────────────────────────┴─────────────────────┤
│ ┌──────────────────────────────────────────────┐ │
│ │                 Symbol Table                 │ │
│ ├──────────────────────────────────────────────┤ │
│ │                    Series                    │ │
│ ├──────────────────────────────────────────────┤ │
│ │                 Label Index 1                │ │
│ ├──────────────────────────────────────────────┤ │
│ │                      ...                     │ │
│ ├──────────────────────────────────────────────┤ │
│ │                 Label Index N                │ │
│ ├──────────────────────────────────────────────┤ │
│ │                   Postings 1                 │ │
│ ├──────────────────────────────────────────────┤ │
│ │                      ...                     │ │
│ ├──────────────────────────────────────────────┤ │
│ │                   Postings N                 │ │
│ ├──────────────────────────────────────────────┤ │
│ │               Label Index Table              │ │
│ ├──────────────────────────────────────────────┤ │
│ │                 Postings Table               │ │
│ ├──────────────────────────────────────────────┤ │
│ │                      TOC                     │ │
│ └──────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```

When the index is written, an arbitrary number of padding bytes may be added between the lined out main sections above. When sequentially scanning through the file, any zero bytes after a section's specified length must be skipped.

Most of the sections described below start with a `len` field. It always specifies the number of bytes just before the trailing CRC32 checksum. The checksum is always calculated over those `len` bytes.


### Symbol Table

The symbol table holds a sorted list of deduplicated strings that occurred in label pairs of the stored series. They can be referenced from subsequent sections and significantly reduce the total index size.

The section contains a sequence of the string entries, each prefixed with the string's length in raw bytes. All strings are utf-8 encoded.
Strings are referenced by sequential indexing. The strings are sorted in lexicographically ascending order.

```
┌────────────────────┬─────────────────────┐
│ len <4b>           │ #symbols <4b>       │
├────────────────────┴─────────────────────┤
│ ┌──────────────────────┬───────────────┐ │
│ │ len(str_1) <uvarint> │ str_1 <bytes> │ │
│ ├──────────────────────┴───────────────┤ │
│ │                . . .                 │ │
│ ├──────────────────────┬───────────────┤ │
│ │ len(str_n) <uvarint> │ str_1 <bytes> │ │
│ └──────────────────────┴───────────────┘ │
├──────────────────────────────────────────┤
│ CRC32 <4b>                               │
└──────────────────────────────────────────┘
```


### Series

The section contains a sequence of series that hold the label set of the series as well as its chunks within the block. The series are sorted lexicographically by their label sets.  
Each series section is aligned to 16 bytes. The ID for a series is the `offset/16`. This serves as the series' ID in all subsequent references. Thereby, a sorted list of series IDs implies a lexicographically sorted list of series label sets. 

```
┌───────────────────────────────────────┐
│ ┌───────────────────────────────────┐ │
│ │   series_1                        │ │
│ ├───────────────────────────────────┤ │
│ │                 . . .             │ │
│ ├───────────────────────────────────┤ │
│ │   series_n                        │ │
│ └───────────────────────────────────┘ │
└───────────────────────────────────────┘
```

Every series entry first holds its number of labels, followed by tuples of symbol table references that contain the label name and value. The label pairs are lexicographically sorted.  
After the labels, the number of indexed chunks is encoded, followed by a sequence of metadata entries containing the chunks minimum (`mint`) and maximum (`maxt`) timestamp and a reference to its position in the chunk file. The `mint` is the time of the first sample and `maxt` is the time of the last sample in the chunk. Holding the time range data in the index allows dropping chunks irrelevant to queried time ranges without accessing them directly.

`mint` of the first chunk is stored, it's `maxt` is stored as a delta and the `mint` and `maxt` are encoded as deltas to the previous time for subsequent chunks. Similarly, the reference of the first chunk is stored and the next ref is stored as a delta to the previous one.

```
┌─────────────────────────────────────────────────────────────────────────┐
│ len <uvarint>                                                           │
├─────────────────────────────────────────────────────────────────────────┤
│ ┌──────────────────┬──────────────────────────────────────────────────┐ │
│ │                  │ ┌──────────────────────────────────────────┐     │ │
│ │                  │ │ ref(l_i.name) <uvarint>                  │     │ │
│ │     #labels      │ ├──────────────────────────────────────────┤ ... │ │
│ │    <uvarint>     │ │ ref(l_i.value) <uvarint>                 │     │ │
│ │                  │ └──────────────────────────────────────────┘     │ │
│ ├──────────────────┼──────────────────────────────────────────────────┤ │
│ │                  │ ┌──────────────────────────────────────────┐     │ │
│ │                  │ │ c_0.mint <varint>                        │     │ │
│ │                  │ ├──────────────────────────────────────────┤     │ │
│ │                  │ │ c_0.maxt - c_0.mint <uvarint>            │     │ │
│ │                  │ ├──────────────────────────────────────────┤     │ │
│ │                  │ │ ref(c_0.data) <uvarint>                  │     │ │
│ │      #chunks     │ └──────────────────────────────────────────┘     │ │
│ │     <uvarint>    │ ┌──────────────────────────────────────────┐     │ │
│ │                  │ │ c_i.mint - c_i-1.maxt <uvarint>          │     │ │
│ │                  │ ├──────────────────────────────────────────┤     │ │
│ │                  │ │ c_i.maxt - c_i.mint <uvarint>            │     │ │
│ │                  │ ├──────────────────────────────────────────┤ ... │ │
│ │                  │ │ ref(c_i.data) - ref(c_i-1.data) <varint> │     │ │
│ │                  │ └──────────────────────────────────────────┘     │ │
│ └──────────────────┴──────────────────────────────────────────────────┘ │
├─────────────────────────────────────────────────────────────────────────┤
│ CRC32 <4b>                                                              │
└─────────────────────────────────────────────────────────────────────────┘
```



### Label Index

A label index section indexes the existing (combined) values for one or more label names.  
The `#names` field determines the number indexed label names, followed by the total number of entries in the `#entries` field. The body holds `#entries` symbol table reference tuples of length of length `#names`. The value tuples are sorted in lexicographically increasing order.

```
┌───────────────┬────────────────┬────────────────┐
│ len <4b>      │ #names <4b>    │ #entries <4b>  │
├───────────────┴────────────────┴────────────────┤
│ ┌─────────────────────────────────────────────┐ │
│ │ ref(value_0) <4b>                           │ │
│ ├─────────────────────────────────────────────┤ │
│ │ ...                                         │ │
│ ├─────────────────────────────────────────────┤ │
│ │ ref(value_n) <4b>                           │ │
│ └─────────────────────────────────────────────┘ │
│                      . . .                      │
├─────────────────────────────────────────────────┤
│ CRC32 <4b>                                      │
└─────────────────────────────────────────────────┘
```

The sequence of label index sections is finalized by an offset table pointing to the beginning of each label index section for a given set of label names.

### Postings

Postings sections store monotonically increasing lists of series references that contain a given label pair associated with the list.  

```
┌────────────────────┬────────────────────┐
│ len <4b>           │ #entries <4b>      │
├────────────────────┴────────────────────┤
│ ┌─────────────────────────────────────┐ │
│ │ ref(series_1) <4b>                  │ │
│ ├─────────────────────────────────────┤ │
│ │ ...                                 │ │
│ ├─────────────────────────────────────┤ │
│ │ ref(series_n) <4b>                  │ │
│ └─────────────────────────────────────┘ │
├─────────────────────────────────────────┤
│ CRC32 <4b>                              │
└─────────────────────────────────────────┘
```

The sequence of postings sections is finalized by an offset table pointing to the beginning of each postings section for a given set of label names.

### Offset Table

An offset table stores a sequence of entries that maps a list of strings to an offset. They are used to track label index and postings sections. They are read into memory when an index file is loaded.

```
┌─────────────────────┬────────────────────┐
│ len <4b>            │ #entries <4b>      │
├─────────────────────┴────────────────────┤
│ ┌──────────────────────────────────────┐ │
│ │  n = #strs <uvarint>                 │ │
│ ├──────────────────────┬───────────────┤ │
│ │ len(str_1) <uvarint> │ str_1 <bytes> │ │
│ ├──────────────────────┴───────────────┤ │
│ │  ...                                 │ │
│ ├──────────────────────┬───────────────┤ │
│ │ len(str_n) <uvarint> │ str_n <bytes> │ │
│ ├──────────────────────┴───────────────┤ │
│ │  offset <uvarint>                    │ │
│ └──────────────────────────────────────┘ │
│                  . . .                   │
├──────────────────────────────────────────┤
│  CRC32 <4b>                              │
└──────────────────────────────────────────┘
```


### TOC

The table of contents serves as an entry point to the entire index and points to various sections in the file.
If a reference is zero, it indicates the respective section does not exist and empty results should be returned upon lookup.

```
┌─────────────────────────────────────────┐
│ ref(symbols) <8b>                       │
├─────────────────────────────────────────┤
│ ref(series) <8b>                        │
├─────────────────────────────────────────┤
│ ref(label indices start) <8b>           │
├─────────────────────────────────────────┤
│ ref(label indices table) <8b>           │
├─────────────────────────────────────────┤
│ ref(postings start) <8b>                │
├─────────────────────────────────────────┤
│ ref(postings table) <8b>                │
├─────────────────────────────────────────┤
│ CRC32 <4b>                              │
└─────────────────────────────────────────┘
```
