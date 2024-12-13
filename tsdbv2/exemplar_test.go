// Copyright 2024 The Prometheus Authors
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

package tsdbv2_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdbv2"
	"github.com/stretchr/testify/require"
)

func TestSelectExemplar(t *testing.T) {
	db, err := tsdbv2.New(t.TempDir())
	require.NoError(t, err)
	defer db.Close()

	lName, lValue := "service", "asdf"
	l := labels.FromStrings(lName, lValue)
	e := exemplar.Exemplar{
		Labels: labels.FromStrings("trace_id", "querty"),
		Value:  0.1,
		Ts:     12,
	}
	_, err = db.AppendExemplar(0, l, e)
	require.NoError(t, err)
	m, err := labels.NewMatcher(labels.MatchEqual, lName, lValue)
	require.NoError(t, err)

	es, err := db.ExemplarQuerier(context.Background())
	require.NoError(t, err)

	ret, err := es.Select(0, 100, []*labels.Matcher{m})
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")

	expectedResult := []exemplar.Exemplar{e}
	require.True(t, reflect.DeepEqual(expectedResult, ret[0].Exemplars), "select did not return expected exemplars\n\texpected: %+v\n\tactual: %+v\n", expectedResult, ret[0].Exemplars)
}

func TestSelectExemplar_MultiSeries(t *testing.T) {
	db, err := tsdbv2.New(t.TempDir())
	require.NoError(t, err)
	defer db.Close()

	l1Name := "test_metric"
	l1 := labels.FromStrings(labels.MetricName, l1Name, "service", "asdf")
	l2Name := "test_metric2"
	l2 := labels.FromStrings(labels.MetricName, l2Name, "service", "qwer")

	for i := range 5 {
		e1 := exemplar.Exemplar{
			Labels: labels.FromStrings("trace_id", "a"),
			Value:  float64(i+1) / 10,
			Ts:     int64(101 + i),
		}
		_, err = db.AppendExemplar(0, l1, e1)
		require.NoError(t, err)

		e2 := exemplar.Exemplar{
			Labels: labels.FromStrings("trace_id", "b"),
			Value:  float64(i+1) / 10,
			Ts:     int64(101 + i),
		}
		_, err = db.AppendExemplar(0, l2, e2)
		require.NoError(t, err)
	}

	es, err := db.ExemplarQuerier(context.Background())
	require.NoError(t, err)

	m, err := labels.NewMatcher(labels.MatchEqual, labels.MetricName, l2Name)
	require.NoError(t, err, "error creating label matcher for exemplar query")
	ret, err := es.Select(100, 200, []*labels.Matcher{m})
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")
	require.Len(t, ret[0].Exemplars, 5, "didn't get expected 5 exemplars, got %d", len(ret[0].Exemplars))

	m, err = labels.NewMatcher(labels.MatchEqual, labels.MetricName, l1Name)
	require.NoError(t, err, "error creating label matcher for exemplar query")
	ret, err = es.Select(100, 200, []*labels.Matcher{m})
	require.NoError(t, err)
	require.Len(t, ret, 1, "select should have returned samples for a single series only")
	require.Len(t, ret[0].Exemplars, 5, "didn't get expected 5 exemplars, got %d", len(ret[0].Exemplars))
}
