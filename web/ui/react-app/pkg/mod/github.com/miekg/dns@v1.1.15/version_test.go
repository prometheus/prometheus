package dns

import "testing"

func TestVersion(t *testing.T) {
	v := V{1, 0, 0}
	if x := v.String(); x != "1.0.0" {
		t.Fatalf("Failed to convert version %v, got: %s", v, x)
	}
}
