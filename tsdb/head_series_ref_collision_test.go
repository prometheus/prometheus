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

package tsdb

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

// TestWALReplayDuplicateSeriesRef is a regression test for
// https://github.com/prometheus/prometheus/issues/18547: if the WAL contains
// two different series sharing the same ref, replay must not let the second
// series overwrite the first in the ref-to-series map, which used to make
// queries return a series belonging to another metric.
func TestWALReplayDuplicateSeriesRef(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(path.Join(dir, "wal"), 0o777))
	w, err := wlog.New(nil, nil, path.Join(dir, "wal"), compression.None)
	require.NoError(t, err)

	enc := record.Encoder{}
	err = w.Log(
		enc.Series([]record.RefSeries{
			{Ref: 800000, Labels: labels.FromStrings("__name__", "metric_a")},
			{Ref: 800000, Labels: labels.FromStrings("__name__", "metric_b")},
		}, nil),
		enc.Samples([]record.RefSample{
			{Ref: 800000, T: 1000, V: 1},
		}, nil),
	)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	db := newTestDB(t, withDir(dir))

	// The first series keeps the ref; the colliding record must not overwrite it.
	s := db.head.series.getByID(800000)
	require.NotNil(t, s)
	require.Equal(t, "metric_a", s.labels().Get("__name__"))

	q, err := db.Querier(0, 2000)
	require.NoError(t, err)

	res := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric_a"))
	require.Len(t, res, 1)
	for ls := range res {
		require.Contains(t, ls, `__name__="metric_a"`)
	}

	// The colliding series must not be queryable under the other metric name.
	// query() closes the querier, so open a new one.
	q, err = db.Querier(0, 2000)
	require.NoError(t, err)
	res = query(t, q, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric_b"))
	require.Empty(t, res)

	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.head.metrics.walReplayDupSeriesRefTotal))
}
