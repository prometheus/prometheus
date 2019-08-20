package common

import "testing"

func TestIsValidRegion(t *testing.T) {
	var tests = []struct {
		region string

		exp bool
	}{
		{"", false},

		{"cn-hangzhou", true},
		{"us-east-1", true},

		{"hangzhou", false},
		{"not-unknown", false},
	}
	for _, v := range tests {
		got := IsValidRegion(v.region)
		if got != v.exp {
			t.Fail()
		}
	}
}
