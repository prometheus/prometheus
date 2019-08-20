package util

import (
	"testing"
	"time"
)

func TestAttemptTiming(t *testing.T) {
	testAttempt := AttemptStrategy{
		Total: 0.25e9,
		Delay: 0.1e9,
	}
	want := []time.Duration{0, 0.1e9, 0.2e9, 0.2e9}
	got := make([]time.Duration, 0, len(want)) // avoid allocation when testing timing
	t0 := time.Now()
	for a := testAttempt.Start(); a.Next(); {
		got = append(got, time.Now().Sub(t0))
	}
	got = append(got, time.Now().Sub(t0))
	if len(got) != len(want) {
		t.Fatalf("Failed!")
	}
	const margin = 0.01e9
	for i, got := range want {
		lo := want[i] - margin
		hi := want[i] + margin
		if got < lo || got > hi {
			t.Errorf("attempt %d want %g got %g", i, want[i].Seconds(), got.Seconds())
		}
	}
}

func TestAttemptNextHasNext(t *testing.T) {
	a := AttemptStrategy{}.Start()
	if !a.Next() {
		t.Fatalf("Failed!")
	}
	if a.Next() {
		t.Fatalf("Failed!")
	}

	a = AttemptStrategy{}.Start()
	if !a.Next() {
		t.Fatalf("Failed!")
	}
	if a.HasNext() {
		t.Fatalf("Failed!")
	}
	if a.Next() {
		t.Fatalf("Failed!")
	}
	a = AttemptStrategy{Total: 2e8}.Start()

	if !a.Next() {
		t.Fatalf("Failed!")
	}
	if !a.HasNext() {
		t.Fatalf("Failed!")
	}
	time.Sleep(2e8)

	if !a.HasNext() {
		t.Fatalf("Failed!")
	}
	if !a.Next() {
		t.Fatalf("Failed!")
	}
	if a.Next() {
		t.Fatalf("Failed!")
	}

	a = AttemptStrategy{Total: 1e8, Min: 2}.Start()
	time.Sleep(1e8)

	if !a.Next() {
		t.Fatalf("Failed!")
	}
	if !a.HasNext() {
		t.Fatalf("Failed!")
	}
	if !a.Next() {
		t.Fatalf("Failed!")
	}
	if a.HasNext() {
		t.Fatalf("Failed!")
	}
	if a.Next() {
		t.Fatalf("Failed!")
	}
}
