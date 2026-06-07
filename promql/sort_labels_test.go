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

func TestSortLabels(t *testing.T) {
	// Test that sortLabels produces deterministic output regardless of
	// the internal map iteration order of Labels.
	lbls := labels.FromStrings("z", "1", "a", "2", "m", "3")

	// Run multiple times to catch non-determinism.
	first := sortLabels(lbls)
	for range 100 {
		require.Equal(t, first, sortLabels(lbls), "sortLabels should be deterministic")
	}

	// Verify the output is sorted by label name.
	require.Equal(t, `{a="2", m="3", z="1"}`, first)
}

func TestSortLabelsEmpty(t *testing.T) {
	lbls := labels.EmptyLabels()
	require.Equal(t, `{}`, sortLabels(lbls))
}

func TestSortLabelsSingle(t *testing.T) {
	lbls := labels.FromStrings("foo", "bar")
	require.Equal(t, `{foo="bar"}`, sortLabels(lbls))
}
