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

package agent

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

func TestRepeatedFloatSample(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		name := "v1"
		if v2 {
			name = "v2"
		}
		t.Run(name, func(t *testing.T) {
			for _, window := range []int64{0, 1000} {
				for _, v := range []float64{42, math.NaN(), math.Float64frombits(value.StaleNaN), math.Copysign(0, -1)} {
					db := createTestAgentDB(t, nil, DefaultOptions())
					db.opts.OutOfOrderTimeWindow = window
					dir := filepath.Dir(db.wal.Dir())
					ls := labels.FromStrings("__name__", "machine_memory_bytes")
					appendSample := func(ts int64, val float64, rollback bool) error {
						if v2 {
							app := db.AppenderV2(context.Background())
							_, err := app.Append(0, ls, 0, ts, val, nil, nil, storage.AOptions{})
							if rollback || err != nil {
								require.NoError(t, app.Rollback())
							} else {
								require.NoError(t, app.Commit())
							}
							return err
						}
						app := db.Appender(context.Background())
						_, err := app.Append(0, ls, ts, val)
						if rollback || err != nil {
							require.NoError(t, app.Rollback())
						} else {
							require.NoError(t, app.Commit())
						}
						return err
					}
					require.NoError(t, appendSample(10000, v, false))
					for _, restart := range []bool{false, true} {
						if restart {
							require.NoError(t, db.Close())
							var err error
							opts := DefaultOptions()
							opts.OutOfOrderTimeWindow = window
							db, err = Open(promslog.NewNopLogger(), nil, nil, dir, opts)
							require.NoError(t, err)
						}
						require.NoError(t, appendSample(10000, v, false))
						if window > 0 {
							require.NoError(t, appendSample(9999, 17, false))
							require.NoError(t, appendSample(10000, v, false))
						}
						require.ErrorIs(t, appendSample(10000, 123, false), storage.ErrDuplicateSampleForTimestamp)
						if v == 0 && math.Signbit(v) {
							require.ErrorIs(t, appendSample(10000, 0, false), storage.ErrDuplicateSampleForTimestamp)
						}
						require.NoError(t, appendSample(11000, 123, true))
						require.NoError(t, appendSample(10000, v, false))
						if window > 0 {
							require.NoError(t, appendSample(9999, 17, false))
							require.NoError(t, appendSample(10000, v, false))
						}
						require.ErrorIs(t, appendSample(10000-window-1, v, false), storage.ErrOutOfOrderSample)
					}
					require.NoError(t, db.Close())
					reader, err := wlog.NewSegmentsReader(db.wal.Dir())
					require.NoError(t, err)
					r := wlog.NewReader(reader)
					dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
					var latestSamples int
					for r.Next() {
						if dec.Type(r.Record()) != record.Samples {
							continue
						}
						samples, err := dec.Samples(r.Record(), nil)
						require.NoError(t, err)
						for _, sample := range samples {
							if sample.T == 10000 {
								latestSamples++
							}
						}
					}
					require.NoError(t, r.Err())
					require.NoError(t, reader.Close())
					require.Equal(t, 1, latestSamples)
				}
			}
			for _, floatHistogram := range []bool{false, true} {
				db := createTestAgentDB(t, nil, DefaultOptions())
				dir := filepath.Dir(db.wal.Dir())
				ls := labels.FromStrings("__name__", "changing_type")
				app := db.Appender(context.Background())
				_, err := app.Append(0, ls, 100, 0)
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				var h *histogram.Histogram
				var fh *histogram.FloatHistogram
				if floatHistogram {
					fh = &histogram.FloatHistogram{}
				} else {
					h = &histogram.Histogram{}
				}
				if v2 {
					a := db.AppenderV2(context.Background())
					_, err = a.Append(0, ls, 0, 101, 0, h, fh, storage.AOptions{})
					require.NoError(t, err)
					require.NoError(t, a.Commit())
				} else {
					a := db.Appender(context.Background())
					_, err = a.AppendHistogram(0, ls, 101, h, fh)
					require.NoError(t, err)
					require.NoError(t, a.Commit())
				}
				for _, restart := range []bool{false, true} {
					if restart {
						require.NoError(t, db.Close())
						db, err = Open(promslog.NewNopLogger(), nil, nil, dir, DefaultOptions())
						require.NoError(t, err)
					}
					if v2 {
						a := db.AppenderV2(context.Background())
						_, err = a.Append(0, ls, 0, 101, 0, nil, nil, storage.AOptions{})
						require.ErrorIs(t, err, storage.ErrOutOfOrderSample)
						require.NoError(t, a.Rollback())
					} else {
						a := db.Appender(context.Background())
						_, err = a.Append(0, ls, 101, 0)
						require.ErrorIs(t, err, storage.ErrOutOfOrderSample)
						require.NoError(t, a.Rollback())
					}
				}
				require.NoError(t, db.Close())
			}
		})
	}
}
