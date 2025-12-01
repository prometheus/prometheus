package teststorage

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/stretchr/testify/require"
)

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
