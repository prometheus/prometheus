package retrier

import (
	"testing"
	"time"
)

func TestConstantBackoff(t *testing.T) {
	b := ConstantBackoff(1, 10*time.Millisecond)
	if len(b) != 1 {
		t.Error("incorrect length")
	}
	for i := range b {
		if b[i] != 10*time.Millisecond {
			t.Error("incorrect value at", i)
		}
	}

	b = ConstantBackoff(10, 250*time.Hour)
	if len(b) != 10 {
		t.Error("incorrect length")
	}
	for i := range b {
		if b[i] != 250*time.Hour {
			t.Error("incorrect value at", i)
		}
	}
}

func TestExponentialBackoff(t *testing.T) {
	b := ExponentialBackoff(1, 10*time.Millisecond)
	if len(b) != 1 {
		t.Error("incorrect length")
	}
	if b[0] != 10*time.Millisecond {
		t.Error("incorrect value")
	}

	b = ExponentialBackoff(4, 1*time.Minute)
	if len(b) != 4 {
		t.Error("incorrect length")
	}
	if b[0] != 1*time.Minute {
		t.Error("incorrect value")
	}
	if b[1] != 2*time.Minute {
		t.Error("incorrect value")
	}
	if b[2] != 4*time.Minute {
		t.Error("incorrect value")
	}
	if b[3] != 8*time.Minute {
		t.Error("incorrect value")
	}
}
