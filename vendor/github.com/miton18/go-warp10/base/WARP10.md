# Warp10

Low-level Warp10 platform bindings

## Client

## GTS Helper

- Parse sensision format into GTS structs
- Get Sensision (Warp10 input format) value of GTS struct

Example:

```go
gts, err := ParseGTSArrayFromString("1234// my.metric{} 10\n1234// my.metric{} 10")

```
