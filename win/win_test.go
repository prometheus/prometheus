package main

import (
	"fmt"
	"testing"
	"time"
)

func TestWindowsClock(t *testing.T) {
	n := time.Now()

	for time.Now().Before(n.Add(1 * time.Second)) {
		fmt.Printf("%v\n", time.Now())
	}
}
