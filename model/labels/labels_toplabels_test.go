// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build toplabels

package labels

import "testing"

var expectedSizeOfLabels = []uint64{ // Values must line up with testCaseLabels.
	12,
	0,
	37,
	266,
	270,
	293,
}

var expectedByteSize = []uint64{ // Values must line up with testCaseLabels.
	12,
	0,
	37,
	266,
	270,
	309,
}

func TestMappedLabels_Get(t *testing.T) {
	// Verifies that Get works correctly when labels contain both mapped strings
	// (e.g. __name__, instance, job) and unmapped strings (e.g. method).
	ls := FromStrings(
		"__name__", "http_requests_total",
		"instance", "localhost:9090",
		"job", "prometheus",
		"method", "GET",
	)

	tests := []struct {
		name     string
		expected string
	}{
		// Fetch a mapped key that exists.
		{name: "__name__", expected: "http_requests_total"},
		// Fetch another mapped key.
		{name: "instance", expected: "localhost:9090"},
		// Fetch a mapped key that appears after unmapped keys in sort order.
		{name: "job", expected: "prometheus"},
		// Fetch an unmapped key.
		{name: "method", expected: "GET"},
		// Fetch a key that doesn't exist but sorts before all labels.
		{name: "ZZZZZ", expected: ""},
		// Fetch a key that doesn't exist but sorts between mapped and unmapped keys.
		{name: "joc", expected: ""},
		// Fetch a key that doesn't exist and sorts after all labels.
		{name: "zzz", expected: ""},
		// Fetch empty key.
		{name: "", expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ls.Get(tc.name)
			if got != tc.expected {
				t.Errorf("Get(%q) = %q, want %q", tc.name, got, tc.expected)
			}
		})
	}
}

func TestMappedLabels_Has(t *testing.T) {
	// Verifies that Has works correctly when labels are encoded with mapped strings.
	ls := FromStrings(
		"__name__", "up",
		"instance", "localhost:9090",
		"job", "node",
		"cluster", "prod",
	)

	tests := []struct {
		name     string
		expected bool
	}{
		// Mapped key that exists.
		{name: "__name__", expected: true},
		{name: "instance", expected: true},
		{name: "job", expected: true},
		// Unmapped key that exists.
		{name: "cluster", expected: true},
		// Key that doesn't exist, sorts before mapped keys.
		{name: "AAAA", expected: false},
		// Key that doesn't exist, sorts between existing keys.
		{name: "joa", expected: false},
		// Key that doesn't exist, sorts after all keys.
		{name: "zzz", expected: false},
		// Empty key.
		{name: "", expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ls.Has(tc.name)
			if got != tc.expected {
				t.Errorf("Has(%q) = %v, want %v", tc.name, got, tc.expected)
			}
		})
	}
}

func TestMappedLabels_Compare(t *testing.T) {
	// Verifies that Compare works correctly when labels contain mapped strings.
	tests := []struct {
		desc     string
		a, b     Labels
		expected int
	}{
		// Two identical label sets with mapped keys.
		{
			desc:     "equal sets with mapped keys",
			a:        FromStrings("__name__", "up", "instance", "localhost", "job", "prom"),
			b:        FromStrings("__name__", "up", "instance", "localhost", "job", "prom"),
			expected: 0,
		},
		// Differ in a mapped value.
		{
			desc:     "differ in mapped key value",
			a:        FromStrings("__name__", "up", "job", "aaa"),
			b:        FromStrings("__name__", "up", "job", "zzz"),
			expected: -1,
		},
		// Differ in an unmapped key, mapped keys equal.
		{
			desc:     "differ in unmapped key",
			a:        FromStrings("__name__", "up", "cluster", "a"),
			b:        FromStrings("__name__", "up", "cluster", "b"),
			expected: -1,
		},
		// Different number of labels.
		{
			desc:     "different label count",
			a:        FromStrings("__name__", "up"),
			b:        FromStrings("__name__", "up", "job", "prom"),
			expected: -1,
		},
	}

	sign := func(a int) int {
		if a < 0 {
			return -1
		}
		if a > 0 {
			return 1
		}
		return 0
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := Compare(tc.a, tc.b)
			if sign(got) != sign(tc.expected) {
				t.Errorf("Compare(%v, %v) = %d, want sign %d", tc.a, tc.b, got, tc.expected)
			}
			// Reverse must give opposite sign.
			got = Compare(tc.b, tc.a)
			if sign(got) != -sign(tc.expected) {
				t.Errorf("Compare(%v, %v) reverse = %d, want sign %d", tc.b, tc.a, got, -tc.expected)
			}
		})
	}
}

func TestMappedLabels_DropMetricName(t *testing.T) {
	// Verifies DropMetricName works when __name__ is stored as a mapped string.
	ls := FromStrings("__name__", "http_requests_total", "job", "api", "method", "GET")
	dropped := ls.DropMetricName()

	expected := FromStrings("job", "api", "method", "GET")
	if !Equal(dropped, expected) {
		t.Errorf("DropMetricName() = %v, want %v", dropped, expected)
	}
}

func TestMappedLabels_DropReserved(t *testing.T) {
	// Verifies DropReserved works when reserved labels are stored as mapped strings.
	ls := FromStrings("__name__", "up", "__type__", "counter", "job", "prom")
	dropped := ls.DropReserved(func(n string) bool {
		return n == "__name__" || n == "__type__"
	})

	expected := FromStrings("job", "prom")
	if !Equal(dropped, expected) {
		t.Errorf("DropReserved() = %v, want %v", dropped, expected)
	}
}

func TestMappedLabels_Range(t *testing.T) {
	// Verifies Range decodes both mapped and unmapped labels correctly.
	ls := FromStrings("__name__", "up", "instance", "host:9090", "cluster", "prod")

	got := map[string]string{}
	ls.Range(func(l Label) {
		got[l.Name] = l.Value
	})

	expected := map[string]string{
		"__name__": "up",
		"instance": "host:9090",
		"cluster":  "prod",
	}
	if len(got) != len(expected) {
		t.Fatalf("Range() returned %d labels, want %d", len(got), len(expected))
	}
	for k, v := range expected {
		if got[k] != v {
			t.Errorf("Range() label %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestMappedLabels_EmptyValue(t *testing.T) {
	// Verifies that empty string values are encoded correctly via the mapping
	// (empty string is mapped to index 0).
	ls := FromStrings("__name__", "up", "job", "")
	// WithoutEmpty should remove the job="" label.
	clean := ls.WithoutEmpty()
	expected := FromStrings("__name__", "up")
	if !Equal(clean, expected) {
		t.Errorf("WithoutEmpty() = %v, want %v", clean, expected)
	}
}

func TestMappedLabels_Builder(t *testing.T) {
	// Verifies Builder works correctly with mapped labels.
	base := FromStrings("__name__", "up", "instance", "host:9090", "job", "prom")

	// Delete a mapped key and add a new unmapped key.
	b := NewBuilder(base)
	b.Del("instance")
	b.Set("cluster", "prod")
	result := b.Labels()

	expected := FromStrings("__name__", "up", "cluster", "prod", "job", "prom")
	if !Equal(result, expected) {
		t.Errorf("Builder result = %v, want %v", result, expected)
	}

	// Replace a mapped key value.
	b2 := NewBuilder(base)
	b2.Set("job", "new-job")
	result2 := b2.Labels()

	expected2 := FromStrings("__name__", "up", "instance", "host:9090", "job", "new-job")
	if !Equal(result2, expected2) {
		t.Errorf("Builder replace result = %v, want %v", result2, expected2)
	}
}

func TestMappedLabels_ByteSize(t *testing.T) {
	// Verifies that labels created after init() are smaller due to mapping.
	ls := FromStrings(
		"__name__", "http_requests_total",
		"instance", "localhost:9090",
		"job", "prometheus",
	)
	// __name__ (mapped): 2 bytes, instance (mapped): 2 bytes, job (mapped): 2 bytes.
	// vs stringlabels: 9 + 9 + 4 = 22 bytes for just the names.
	// Mapped saves 16 bytes on names alone.
	unmappedLS := FromStrings(
		"aaa", "http_requests_total",
		"bbbbb", "localhost:9090",
		"ccc", "prometheus",
	)
	if ls.ByteSize() >= unmappedLS.ByteSize() {
		t.Errorf("mapped ByteSize() = %d should be less than unmapped ByteSize() = %d", ls.ByteSize(), unmappedLS.ByteSize())
	}
}

func TestMappedLabels_Hash(t *testing.T) {
	// Verifies that Hash is consistent: same logical labels produce the same hash
	// regardless of creation order.
	ls1 := FromStrings("__name__", "up", "job", "prom")
	ls2 := FromStrings("job", "prom", "__name__", "up")
	if ls1.Hash() != ls2.Hash() {
		t.Errorf("Hash mismatch: %d != %d", ls1.Hash(), ls2.Hash())
	}
}

func TestMappedLabels_Len(t *testing.T) {
	// Verifies Len works with mapped labels.
	ls := FromStrings("__name__", "up", "instance", "host", "job", "prom")
	if ls.Len() != 3 {
		t.Errorf("Len() = %d, want 3", ls.Len())
	}
}

func TestMapLabels(t *testing.T) {
	// Verifies that MapLabels correctly populates the mapping and caps at 256.
	original := make([]string, len(mappedLabels))
	copy(original, mappedLabels)
	defer MapLabels(original) // Restore original mapping after test.

	// Verify empty string is injected when not provided.
	MapLabels([]string{"foo", "bar"})
	if mappedLabels[0] != "" {
		t.Errorf("mappedLabels[0] = %q, want empty string", mappedLabels[0])
	}
	if len(mappedLabels) != 3 {
		t.Errorf("len(mappedLabels) = %d, want 3", len(mappedLabels))
	}

	// Verify cap at 256.
	big := make([]string, 300)
	for i := range big {
		big[i] = string(rune('A' + i))
	}
	big[0] = "" // Ensure empty string is present.
	MapLabels(big)
	if len(mappedLabels) != 256 {
		t.Errorf("len(mappedLabels) = %d, want 256", len(mappedLabels))
	}
}
