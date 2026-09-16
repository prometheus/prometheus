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

func TestParseOnLabels(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"job", []string{"job"}},
		{"job,instance", []string{"job", "instance"}},
		{" job , instance ", []string{"job", "instance"}},
		{"job,,instance", []string{"job", "instance"}},
		{",", nil},
	}
	for _, c := range cases {
		got := parseOnLabels(c.in)
		if len(c.want) == 0 {
			require.Empty(t, got, "input %q", c.in)
			continue
		}
		require.Equal(t, c.want, got, "input %q", c.in)
	}
}

func TestMatchSig(t *testing.T) {
	a := labels.FromStrings("__name__", "a", "job", "x", "instance", "1", "code", "500")
	b := labels.FromStrings("__name__", "b", "job", "x", "instance", "1", "quantile", "0.99")

	// on unset: falls back to dropLabels (the pre-`on` default). a and b
	// differ outside __name__ (code vs quantile), so their signatures differ.
	sigA, _ := matchSig(a, nil, nil, "__name__")
	sigB, _ := matchSig(b, nil, nil, "__name__")
	require.NotEqual(t, sigA, sigB, "a and b should not match on full labelset")

	// on set to the shared labels: a and b now match, ignoring code/quantile.
	sigA, _ = matchSig(a, nil, []string{"job", "instance"}, "__name__")
	sigB, _ = matchSig(b, nil, []string{"job", "instance"}, "__name__")
	require.Equal(t, sigA, sigB, "a and b should match on shared on-labels")

	// on takes priority over dropLabels even when both are given.
	sigA2, _ := matchSig(a, nil, []string{"job", "instance"}, "__name__", "job")
	require.Equal(t, sigA, sigA2, "dropLabels must be ignored once on is non-empty")
}
