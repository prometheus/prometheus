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

package promql

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestParseOnMatching(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want onMatching
	}{
		{"", true, onMatching{}},
		{"   ", true, onMatching{}},
		{"job", true, onMatching{explicit: true, labels: []string{"job"}}},
		// Names are sorted and de-duplicated, as HashForLabels requires.
		{"job,instance", true, onMatching{explicit: true, labels: []string{"instance", "job"}}},
		{" (job , instance, job) ", true, onMatching{explicit: true, labels: []string{"instance", "job"}}},
		{"()", true, onMatching{explicit: true}},
		{"( )", true, onMatching{explicit: true}},
		{"(job", false, onMatching{}},
		{"job)", false, onMatching{}},
		{"(a)(b)", false, onMatching{}},
		{",", false, onMatching{}},
	}
	for _, c := range cases {
		got, ok := parseOnMatching(c.in)
		require.Equal(t, c.ok, ok, "ok for %q", c.in)
		if c.ok {
			require.Equal(t, c.want.explicit, got.explicit, "explicit for %q", c.in)
			require.Len(t, got.labels, len(c.want.labels), "labels for %q", c.in)
			if len(c.want.labels) > 0 {
				require.Equal(t, c.want.labels, got.labels, "labels for %q", c.in)
			}
		}
	}
}

func TestOnMatchingSig(t *testing.T) {
	a := labels.FromStrings("__name__", "a", "job", "x", "instance", "1", "code", "500")
	b := labels.FromStrings("__name__", "b", "job", "x", "instance", "1", "quantile", "0.99")
	c := labels.FromStrings("__name__", "a", "job", "x", "instance", "2", "code", "500")
	sig := func(on string, lb labels.Labels) uint64 {
		m, ok := parseOnMatching(on)
		require.True(t, ok, on)
		h, _ := m.sig(lb, nil, "__name__")
		return h
	}

	// Default key: a and b differ outside __name__ (code vs quantile).
	require.NotEqual(t, sig("", a), sig("", b))
	// Explicit key: a and b share job and instance.
	require.Equal(t, sig("job,instance", a), sig("job,instance", b))
	// a and c differ only in instance, which sorts before job. Hashing the
	// user-order list unsorted would ignore instance and wrongly match them.
	require.NotEqual(t, sig("job,instance", a), sig("job,instance", c))
	// "()" gives every series the same signature.
	require.Equal(t, sig("()", a), sig("()", b))
	require.Equal(t, sig("()", a), sig("()", c))
}
