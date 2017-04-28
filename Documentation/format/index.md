# Index Disk Format

The following describes the format of the `index` file found in each block directory.
It is terminated by a table of contents which serves as an entry point into the index.

```
┌────────────────────────────┬─────────────────────┐
│ magic(0xBAAAD700) <4b> │ version(1) <1 byte> │
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
│ │               Label Index Table              │ │
│ ├──────────────────────────────────────────────┤ │
│ │                   Postings 1                 │ │
│ ├──────────────────────────────────────────────┤ │
│ │                      ...                     │ │
│ ├──────────────────────────────────────────────┤ │
│ │                   Postings N                 │ │
│ ├──────────────────────────────────────────────┤ │
│ │                 Postings Table               │ │
│ ├──────────────────────────────────────────────┤ │
│ │                      TOC                     │ │
│ └──────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────┘
```


### Symbol Table

The symbol table holds a sorted list of deduplicated strings that occurred in label pairs of the stored series. They can be referenced from subsequent sections and significantly reduce the total index size.

The section contains a sequence of the string entries, each prefixed with the string's length in raw bytes.
Strings are referenced by pointing to the beginning of their length field. The strings are sorted in lexicographically ascending order.

The full list of strings is validated with a CRC32 checksum.

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
The file offset to the beginning of a series serves as the series' ID in all subsequent references. Thereby, a sorted list of series IDs implies a lexicographically sorted list of series label sets.

```
┌───────────────────────────────────────┐
│  #series <4b>                         │
├───────────────────────────────────────┤
│ ┌───────────────────────────────────┐ │
│ │   series_1                        │ │
│ ├───────────────────────────────────┤ │
│ │                 . . .             │ │
│ ├───────────────────────────────────┤ │
│ │   series_n                        │ │
│ └───────────────────────────────────┘ │
└───────────────────────────────────────┘
```

Every series entry first holds its number of labels, followed by tuples of symbol table references that resemble label name and value. The label pairs are lexicographically sorted.  
After the labels, the number of indexed chunks is encoded, followed by a sequence of metadata entries containing the chunks minimum and maximum timestamp and a reference to its position in the chunk file. Holding the time range data in the index allows dropping chunks irrelevant to queried time ranges without accessing them directly.  
The series entry is prefixed with its length and terminated by a CRC32 checksum over its contents.

```
┌─────────────────────────────────────────────────────────┐
│ len <varint>                                            │
├─────────────────────────────────────────────────────────┤
│ ┌──────────────────┬──────────────────────────────────┐ │
│ │                  │ ┌──────────────────────────┐     │ │
│ │                  │ │ ref(l_i.name) <uvarint>  │     │ │
│ │     #labels      │ ├──────────────────────────┤ ... │ │
│ │    <uvarint>     │ │ ref(l_i.value) <uvarint> │     │ │
│ │                  │ └──────────────────────────┘     │ │
│ ├──────────────────┼──────────────────────────────────┤ │
│ │                  │ ┌──────────────────────────┐     │ │
│ │                  │ │ c_i.mint <varint>        │     │ │
│ │                  │ ├──────────────────────────┤     │ │
│ │      #chunks     │ │ c_i.maxt <varint>        │     │ │
│ │     <uvarint>    │ ├──────────────────────────┤ ... │ │
│ │                  │ │ ref(c_i.data) <uvarint>  │     │ │
│ │                  │ └──────────────────────────┘     │ │
│ └──────────────────┴──────────────────────────────────┘ │
├─────────────────────────────────────────────────────────┤
│ CRC32 <4b>                                              │
└─────────────────────────────────────────────────────────┘
```



### Label Index

The label index indexes holds lists of possible values for label names. Each label index can be a composite index over more than a single label name, which is tracked by `#names`, followed by the total number of entries.  
The body holds `#entries` entries of possible values pointing back into the symbol table.

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

Postings sections store monotinically increasing lists of series references that contain a given label pair associated with the list.  

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