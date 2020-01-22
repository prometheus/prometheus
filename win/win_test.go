package main

import (
	"fmt"
	"testing"
	"time"
)

func TestWindowsClock(t *testing.T) {
	var ds []time.Duration
	for i := 0; i < 10; i++ {
		ds = append(ds, timediff())
	}
	fmt.Printf("%v\n", ds)
}

func timediff() time.Duration {
	t0 := time.Now()
	for {
		t := time.Now()
		if t != t0 {
			return t.Sub(t0)
		}
	}
}
