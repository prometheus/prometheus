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
	var ds2 []time.Duration
	for i := 0; i < 10; i++ {
		ds2 = append(ds2, timeprintdiff())
	}
	fmt.Printf("%v\n", ds2)
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
func timeprintdiff() time.Duration {
	t0 := time.Now()
	tp0 := fmt.Sprintf("%v\n", time.Now())
	for {
		t := time.Now()
		tp := fmt.Sprintf("%v\n", time.Now())
		if tp != tp0 {
			return t.Sub(t0)
		}
	}
}
