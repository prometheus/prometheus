circuit-breaker
===============

[![Build Status](https://travis-ci.org/eapache/go-resiliency.svg?branch=master)](https://travis-ci.org/eapache/go-resiliency)
[![GoDoc](https://godoc.org/github.com/eapache/go-resiliency/breaker?status.svg)](https://godoc.org/github.com/eapache/go-resiliency/breaker)
[![Code of Conduct](https://img.shields.io/badge/code%20of%20conduct-active-blue.svg)](https://eapache.github.io/conduct.html)

The circuit-breaker resiliency pattern for golang.

Creating a breaker takes three parameters:
- error threshold (for opening the breaker)
- success threshold (for closing the breaker)
- timeout (how long to keep the breaker open)

```go
b := breaker.New(3, 1, 5*time.Second)

for {
	result := b.Run(func() error {
		// communicate with some external service and
		// return an error if the communication failed
		return nil
	})

	switch result {
	case nil:
		// success!
	case breaker.ErrBreakerOpen:
		// our function wasn't run because the breaker was open
	default:
		// some other error
	}
}
```
