# xxhash

[![GoDoc](https://godoc.org/github.com/cespare/mph?status.svg)](https://godoc.org/github.com/cespare/xxhash)

xxhash is a Go implementation of the 64-bit
[xxHash](http://cyan4973.github.io/xxHash/) algorithm, XXH64. This is a
high-quality hashing algorithm that is much faster than anything in the Go
standard library.

The API is very small, taking its cue from the other hashing packages in the
standard library:

    $ go doc github.com/cespare/xxhash                                                                                                                                                                                              !
    package xxhash // import "github.com/cespare/xxhash"

    Package xxhash implements the 64-bit variant of xxHash (XXH64) as described
    at http://cyan4973.github.io/xxHash/.

    func New() hash.Hash64
    func Sum64(b []byte) uint64

This implementation provides a fast pure-Go implementation and an even faster
assembly implementation for amd64.

Here are some quick benchmarks comparing the pure-Go and assembly
implementations of Sum64 against another popular Go XXH64 implementation,
[github.com/OneOfOne/xxhash](https://github.com/OneOfOne/xxhash):

| input size | OneOfOne | cespare (noasm) | cespare |
| --- | --- | --- | --- |
| 5 B   |  438.34 MB/s |  596.40 MB/s |  711.11 MB/s  |
| 100 B | 3676.54 MB/s | 4301.40 MB/s | 4598.95 MB/s  |
| 4 KB  | 8128.64 MB/s | 8840.83 MB/s | 10549.72 MB/s |
| 10 MB | 7335.19 MB/s | 7736.64 MB/s | 9024.04 MB/s  |
