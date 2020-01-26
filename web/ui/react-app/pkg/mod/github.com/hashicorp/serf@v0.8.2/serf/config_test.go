package serf

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.ProtocolVersion != 4 {
		t.Fatalf("bad: %#v", c)
	}
}
