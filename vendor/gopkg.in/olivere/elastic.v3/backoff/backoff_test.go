// Copyright 2012-2016 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package backoff

import (
	"math/rand"
	"testing"
	"time"
)

func TestSimpleBackoff(t *testing.T) {
	b := NewSimpleBackoff(1, 2, 7)

	if got, want := b.Next(), time.Duration(1)*time.Millisecond; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
	if got, want := b.Next(), time.Duration(2)*time.Millisecond; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
	if got, want := b.Next(), time.Duration(7)*time.Millisecond; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
	if got, want := b.Next(), time.Duration(7)*time.Millisecond; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}

	b.Reset()

	if got, want := b.Next(), time.Duration(1)*time.Millisecond; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
	if got, want := b.Next(), time.Duration(2)*time.Millisecond; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
	if got, want := b.Next(), time.Duration(7)*time.Millisecond; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
	if got, want := b.Next(), time.Duration(7)*time.Millisecond; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
}

func TestSimpleBackoffWithStop(t *testing.T) {
	b := NewSimpleBackoff(1, 2, 7).SendStop(true)

	// It should eventually return Stop (-1) after some loops.
	var last time.Duration
	for i := 0; i < 10; i++ {
		last = b.Next()
		if last == Stop {
			break
		}
	}
	if got, want := last, Stop; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}

	b.Reset()

	// It should eventually return Stop (-1) after some loops.
	for i := 0; i < 10; i++ {
		last = b.Next()
		if last == Stop {
			break
		}
	}
	if got, want := last, Stop; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
}

func TestExponentialBackoff(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	min := time.Duration(8) * time.Millisecond
	max := time.Duration(256) * time.Millisecond
	b := NewExponentialBackoff(min, max)

	between := func(value time.Duration, a, b int) bool {
		x := int(value / time.Millisecond)
		return a <= x && x <= b
	}

	if got := b.Next(); !between(got, 8, 256) {
		t.Errorf("expected [%v..%v]; got: %v", 8, 256, got)
	}
	if got := b.Next(); !between(got, 8, 256) {
		t.Errorf("expected [%v..%v]; got: %v", 8, 256, got)
	}
	if got := b.Next(); !between(got, 8, 256) {
		t.Errorf("expected [%v..%v]; got: %v", 8, 256, got)
	}
	if got := b.Next(); !between(got, 8, 256) {
		t.Errorf("expected [%v..%v]; got: %v", 8, 256, got)
	}

	b.Reset()

	if got := b.Next(); !between(got, 8, 256) {
		t.Errorf("expected [%v..%v]; got: %v", 8, 256, got)
	}
	if got := b.Next(); !between(got, 8, 256) {
		t.Errorf("expected [%v..%v]; got: %v", 8, 256, got)
	}
	if got := b.Next(); !between(got, 8, 256) {
		t.Errorf("expected [%v..%v]; got: %v", 8, 256, got)
	}
	if got := b.Next(); !between(got, 8, 256) {
		t.Errorf("expected [%v..%v]; got: %v", 8, 256, got)
	}
}

func TestExponentialBackoffWithStop(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	min := time.Duration(8) * time.Millisecond
	max := time.Duration(256) * time.Millisecond
	b := NewExponentialBackoff(min, max).SendStop(true)

	// It should eventually return Stop (-1) after some loops.
	var last time.Duration
	for i := 0; i < 10; i++ {
		last = b.Next()
		if last == Stop {
			break
		}
	}
	if got, want := last, Stop; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}

	b.Reset()

	// It should eventually return Stop (-1) after some loops.
	for i := 0; i < 10; i++ {
		last = b.Next()
		if last == Stop {
			break
		}
	}
	if got, want := last, Stop; got != want {
		t.Errorf("expected %v; got: %v", want, got)
	}
}
