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

package teststorage

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/util/testutil"
)

// TestSample_RequireEqual ensures standard testutil.RequireEqual is enough for comparisons.
// This is thanks to the fact metadata has now Equals method.
func TestSample_RequireEqual(t *testing.T) {
	a := []Sample{
		{},
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123},
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings("__name__", "yolo")}}},
	}
	testutil.RequireEqual(t, a, a)

	b1 := []Sample{
		{},
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings("__name__", "test_metric2_diff", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}, V: 123.123}, // test_metric2_diff is different.
		{ES: []exemplar.Exemplar{{Labels: labels.FromStrings("__name__", "yolo")}}},
	}
	requireNotEqual(t, a, b1)
}

func requireNotEqual(t testing.TB, a, b any) {
	t.Helper()
	if !cmp.Equal(a, b, cmp.Comparer(labels.Equal)) {
		return
	}
	require.Fail(t, fmt.Sprintf("Equal, but expected not: \n"+
		"a: %s\n"+
		"b: %s", a, b))
}
