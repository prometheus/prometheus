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
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

func newMetadataTestAppender(t testing.TB, db *DB, v2 bool) storage.AppenderTransaction {
	if v2 {
		return db.AppenderV2(t.Context())
	}
	return db.Appender(t.Context())
}

func appendMetadataTestSample(t testing.TB, app storage.AppenderTransaction, ref storage.SeriesRef, ls labels.Labels, ts int64, h *histogram.Histogram, m *metadata.Metadata) storage.SeriesRef {
	var err error
	if a, ok := app.(storage.Appender); ok {
		if h == nil {
			ref, err = a.Append(ref, ls, ts, 1)
		} else {
			ref, err = a.AppendHistogram(ref, ls, ts, h, nil)
		}
		require.NoError(t, err)
		if m != nil {
			ref, err = a.UpdateMetadata(ref, ls, *m)
		}
	} else {
		var opts storage.AOptions
		if m != nil {
			opts.Metadata = *m
		}
		ref, err = app.(storage.AppenderV2).Append(ref, ls, 0, ts, 1, h, nil, opts)
	}
	require.NoError(t, err)
	return ref
}

func TestMetadataInWALPendingUpdates(t *testing.T) {
	a := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "seconds", Help: "A"}
	b := metadata.Metadata{Type: model.MetricTypeCounter, Unit: "bytes", Help: "B"}
	typeOnly := metadata.Metadata{Type: model.MetricTypeCounter, Unit: a.Unit, Help: a.Help}
	unitOnly := metadata.Metadata{Type: a.Type, Unit: "bytes", Help: a.Help}
	helpOnly := metadata.Metadata{Type: a.Type, Unit: a.Unit, Help: "B"}
	large := metadata.Metadata{Type: model.MetricTypeGauge, Help: strings.Repeat("h", 1<<20)}
	unknown := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "A"}
	emptyType := metadata.Metadata{Help: "A"}
	type update struct {
		series    int
		meta      metadata.Metadata
		cached    bool
		histogram bool
	}
	largeUpdates := make([]update, 64)
	for i := range largeUpdates {
		largeUpdates[i] = update{meta: large, cached: i%2 == 0}
	}
	rec := func(series int, m metadata.Metadata) record.RefMetadata {
		return record.RefMetadata{Ref: chunks.HeadSeriesRef(series + 1), Type: record.GetMetricType(m.Type), Unit: m.Unit, Help: m.Help}
	}
	for _, v2 := range []bool{false, true} {
		t.Run(fmt.Sprintf("v2=%t", v2), func(t *testing.T) {
			for _, tc := range []struct {
				name              string
				initial           *metadata.Metadata
				updates           []update
				want              [][]record.RefMetadata
				wantOrder         []string
				semanticCommitted bool
			}{
				{name: "large shared help", updates: largeUpdates, want: [][]record.RefMetadata{{rec(0, large)}}},
				{name: "equal strings with different backing", updates: []update{{meta: a}, {meta: metadata.Metadata{Type: a.Type, Unit: strings.Clone(a.Unit), Help: strings.Clone(a.Help)}}}, want: [][]record.RefMetadata{{rec(0, a)}}},
				{name: "interleaved series", updates: []update{{meta: a}, {series: 1, meta: b}, {meta: a, cached: true}, {series: 1, meta: b, cached: true}}, want: [][]record.RefMetadata{{rec(0, a), rec(1, b)}}},
				{name: "changes and return", updates: []update{{meta: a}, {meta: b}, {meta: b}, {meta: a}, {meta: a}}, want: [][]record.RefMetadata{{rec(0, a), rec(0, b), rec(0, a)}}},
				{name: "type changes", updates: []update{{meta: a}, {meta: typeOnly}, {meta: typeOnly}}, want: [][]record.RefMetadata{{rec(0, a), rec(0, typeOnly)}}},
				{name: "unit changes", updates: []update{{meta: a}, {meta: unitOnly}, {meta: unitOnly}}, want: [][]record.RefMetadata{{rec(0, a), rec(0, unitOnly)}}},
				{name: "help changes", updates: []update{{meta: a}, {meta: helpOnly}, {meta: helpOnly}}, want: [][]record.RefMetadata{{rec(0, a), rec(0, helpOnly)}}},
				{name: "return to committed", initial: &a, updates: []update{{meta: b}, {meta: a}, {meta: a}}, want: [][]record.RefMetadata{{rec(0, b), rec(0, a)}}},
				{name: "unchanged committed", initial: &a, updates: []update{{meta: a}, {meta: a}}},
				{name: "unknown and empty pending type", updates: []update{{meta: unknown}, {meta: emptyType}}, want: [][]record.RefMetadata{{rec(0, unknown)}}},
				{name: "committed type comparison", initial: &unknown, updates: []update{{meta: emptyType}, {meta: emptyType}}, want: [][]record.RefMetadata{{rec(0, unknown)}}, semanticCommitted: true},
				{name: "batch boundaries", updates: []update{{meta: a, histogram: true}, {meta: a}, {meta: a}}, want: [][]record.RefMetadata{{rec(0, a)}, {rec(0, a)}}, wantOrder: []string{"metadata", "histograms", "metadata", "floats"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					opts := DefaultOptions()
					opts.EnableMetadataWALRecords = true
					db := newTestDB(t, withOpts(opts))
					db.DisableCompactions()
					sets := []labels.Labels{labels.FromStrings("__name__", "m", "series", "a"), labels.FromStrings("__name__", "m", "series", "b")}
					refs := make([]storage.SeriesRef, len(sets))
					app := newMetadataTestAppender(t, db, v2)
					refs[0] = appendMetadataTestSample(t, app, 0, sets[0], 0, nil, tc.initial)
					refs[1] = appendMetadataTestSample(t, app, 0, sets[1], 0, nil, nil)
					require.NoError(t, app.Commit())
					walDir := filepath.Join(db.Dir(), "wal")
					initialRecords := len(readTestWAL(t, walDir))

					app = newMetadataTestAppender(t, db, v2)
					final := []*metadata.Metadata{tc.initial, nil}
					counts := make([]int, len(sets))
					for i, u := range tc.updates {
						var ref storage.SeriesRef
						if u.cached {
							ref = refs[u.series]
						}
						var h *histogram.Histogram
						if u.histogram {
							h = tsdbutil.GenerateTestHistogram(int64(i + 1))
						}
						refs[u.series] = appendMetadataTestSample(t, app, ref, sets[u.series], int64(i+1), h, &u.meta)
						final[u.series] = &u.meta
						counts[u.series]++
					}
					require.NoError(t, app.Commit())
					require.ErrorIs(t, app.Commit(), ErrAppenderClosed)
					require.ErrorIs(t, app.Rollback(), ErrAppenderClosed)

					var got [][]record.RefMetadata
					var order []string
					for _, r := range readTestWAL(t, walDir)[initialRecords:] {
						switch r := r.(type) {
						case []record.RefMetadata:
							got = append(got, r)
							order = append(order, "metadata")
						case []record.RefSample:
							order = append(order, "floats")
						case []record.RefHistogramSample:
							order = append(order, "histograms")
						}
					}
					want := tc.want
					if v2 && tc.semanticCommitted {
						want = nil
					}
					require.Len(t, got, len(want))
					for i := range want {
						// Compare counts first to avoid printing large help strings on failure.
						wantEntries, gotEntries := len(want[i]), len(got[i])
						require.Equal(t, wantEntries, gotEntries)
						require.Equal(t, want[i], got[i])
					}
					if tc.wantOrder != nil {
						require.Equal(t, tc.wantOrder, order)
					}
					for replay := range 2 {
						for i, ls := range sets {
							var expected *metadata.Metadata
							if final[i] != nil {
								m := *final[i]
								m.Type = record.ToMetricType(record.GetMetricType(m.Type))
								expected = &m
							}
							require.Equal(t, expected, db.head.series.getByHash(ls.Hash(), ls).meta)
						}
						q, err := db.Querier(1, int64(len(tc.updates)))
						require.NoError(t, err)
						samples := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "__name__", "m"))
						for i, ls := range sets {
							require.Len(t, samples[ls.String()], counts[i])
						}
						if replay == 0 {
							dir := db.Dir()
							require.NoError(t, db.Close())
							db = newTestDB(t, withDir(dir), withOpts(opts))
							db.DisableCompactions()
						}
					}
				})
			}
		})
	}
}

func TestMetadataAppenderLifecycle(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		for _, scenario := range []string{"rollback", "overlapping appenders", "failed commit"} {
			t.Run(fmt.Sprintf("v2=%t/%s", v2, scenario), func(t *testing.T) {
				opts := DefaultOptions()
				opts.EnableMetadataWALRecords = true
				db := newTestDB(t, withOpts(opts))
				ls := labels.FromStrings("__name__", "m")
				a := metadata.Metadata{Type: model.MetricTypeGauge, Help: "A"}
				b := metadata.Metadata{Type: model.MetricTypeGauge, Help: "B"}
				c := metadata.Metadata{Type: model.MetricTypeGauge, Help: "C"}
				app := newMetadataTestAppender(t, db, v2)
				ref := appendMetadataTestSample(t, app, 0, ls, 0, nil, &a)
				require.NoError(t, app.Commit())
				app = newMetadataTestAppender(t, db, v2)
				appendMetadataTestSample(t, app, ref, ls, 1, nil, &b)
				appendMetadataTestSample(t, app, ref, ls, 2, nil, &b)
				expected := a
				switch scenario {
				case "rollback":
					require.NoError(t, app.Rollback())
				case "overlapping appenders":
					other := newMetadataTestAppender(t, db, v2)
					appendMetadataTestSample(t, other, ref, ls, 3, nil, &c)
					require.NoError(t, other.Commit())
					require.NoError(t, app.Rollback())
					expected = c
				case "failed commit":
					broken, err := wlog.New(nil, nil, t.TempDir(), compression.None)
					require.NoError(t, err)
					require.NoError(t, broken.Close())
					wal := db.head.wal
					db.head.wal = broken
					defer func() { db.head.wal = wal }()
					require.Error(t, app.Commit())
					db.head.wal = wal
				}
				require.Equal(t, expected, *db.head.series.getByID(chunks.HeadSeriesRef(ref)).meta)
				require.ErrorIs(t, app.Commit(), ErrAppenderClosed)
				require.ErrorIs(t, app.Rollback(), ErrAppenderClosed)
				require.Zero(t, prom_testutil.ToFloat64(db.head.metrics.activeAppenders))
				app = newMetadataTestAppender(t, db, v2)
				appendMetadataTestSample(t, app, ref, ls, 4, nil, &b)
				appendMetadataTestSample(t, app, ref, ls, 5, nil, &b)
				require.NoError(t, app.Commit())
				var got []record.RefMetadata
				for _, r := range readTestWAL(t, filepath.Join(db.Dir(), "wal")) {
					if m, ok := r.([]record.RefMetadata); ok {
						got = append(got, m...)
					}
				}
				want := []record.RefMetadata{{Ref: chunks.HeadSeriesRef(ref), Type: record.GetMetricType(a.Type), Help: a.Help}}
				if scenario == "overlapping appenders" {
					want = append(want, record.RefMetadata{Ref: chunks.HeadSeriesRef(ref), Type: record.GetMetricType(c.Type), Help: c.Help})
				}
				want = append(want, record.RefMetadata{Ref: chunks.HeadSeriesRef(ref), Type: record.GetMetricType(b.Type), Help: b.Help})
				require.Equal(t, want, got)
			})
		}
	}
}
