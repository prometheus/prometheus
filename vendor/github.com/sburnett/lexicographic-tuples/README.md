lexicographic-tuples
====================

Go library for encoding and decoding tuples of values as bytes strings that sort
lexicographically the same as the original tuples.

See http://godoc.org/github.com/sburnett/lexicographic-tuples for more explanation.

Import as `github.com/sburnett/lexicographic-tuples` and use as `lex`.

```Go
import (
    "github.com/sburnett/lexicographic-tuples"
)

// ...

encodedValue := lex.EncodeOrDie("hello, world!", int32(42))

var greeting string
var meaningOfLife int32
lex.DecodeOrDie(encodedValue, &greeting, &meaningOfLife)

```

[![Build Status](https://travis-ci.org/sburnett/lexicographic-tuples.png)](https://travis-ci.org/sburnett/lexicographic-tuples)
