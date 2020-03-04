## Leaktest [![Build Status](https://travis-ci.org/fortytw2/leaktest.svg?branch=master)](https://travis-ci.org/fortytw2/leaktest) [![codecov](https://codecov.io/gh/fortytw2/leaktest/branch/master/graph/badge.svg)](https://codecov.io/gh/fortytw2/leaktest) [![Sourcegraph](https://sourcegraph.com/github.com/fortytw2/leaktest/-/badge.svg)](https://sourcegraph.com/github.com/fortytw2/leaktest?badge) [![Documentation](https://godoc.org/github.com/fortytw2/gpt?status.svg)](http://godoc.org/github.com/fortytw2/leaktest)

Refactored, tested variant of the goroutine leak detector found in both
`net/http` tests and the `cockroachdb` source tree.

Takes a snapshot of running goroutines at the start of a test, and at the end -
compares the two and _voila_. Ignores runtime/sys goroutines. Doesn't play nice
with `t.Parallel()` right now, but there are plans to do so.

### Installation

Go 1.7+

```
go get -u github.com/fortytw2/leaktest
```

Go 1.5/1.6 need to use the tag `v1.0.0`, as newer versions depend on
`context.Context`.

### Example

These tests fail, because they leak a goroutine

```go
// Default "Check" will poll for 5 seconds to check that all
// goroutines are cleaned up
func TestPool(t *testing.T) {
    defer leaktest.Check(t)()

    go func() {
        for {
            time.Sleep(time.Second)
        }
    }()
}

// Helper function to timeout after X duration
func TestPoolTimeout(t *testing.T) {
    defer leaktest.CheckTimeout(t, time.Second)()

    go func() {
        for {
            time.Sleep(time.Second)
        }
    }()
}

// Use Go 1.7+ context.Context for cancellation
func TestPoolContext(t *testing.T) {
    ctx, _ := context.WithTimeout(context.Background(), time.Second)
    defer leaktest.CheckContext(ctx, t)()

    go func() {
        for {
            time.Sleep(time.Second)
        }
    }()
}
```

## LICENSE

Same BSD-style as Go, see LICENSE
