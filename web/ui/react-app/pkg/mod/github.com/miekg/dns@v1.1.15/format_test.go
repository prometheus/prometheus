package dns

import (
	"testing"
)

func TestFieldEmptyAOrAAAAData(t *testing.T) {
	res := Field(new(A), 1)
	if res != "" {
		t.Errorf("expected empty string but got %v", res)
	}
	res = Field(new(AAAA), 1)
	if res != "" {
		t.Errorf("expected empty string but got %v", res)
	}
}
