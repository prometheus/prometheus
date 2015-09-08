deadline
========

[![Build Status](https://travis-ci.org/eapache/go-resiliency.svg?branch=master)](https://travis-ci.org/eapache/go-resiliency)
[![GoDoc](https://godoc.org/github.com/eapache/go-resiliency/deadline?status.svg)](https://godoc.org/github.com/eapache/go-resiliency/deadline)
[![Code of Conduct](https://img.shields.io/badge/code%20of%20conduct-active-blue.svg)](https://eapache.github.io/conduct.html)

The deadline/timeout resiliency pattern for golang.

Creating a deadline takes one parameter: how long to wait.

```go
dl := deadline.New(1 * time.Second)

err := dl.Run(func(stopper <-chan struct{}) error {
	// do something possibly slow
	// check stopper function and give up if timed out
	return nil
})

switch err {
case deadline.ErrTimedOut:
	// execution took too long, oops
default:
	// some other error
}
```
