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

	"github.com/gernest/roaring"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdbv2"
	"github.com/prometheus/prometheus/tsdbv2/compat"
	"github.com/prometheus/prometheus/tsdbv2/data"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
)

func TestAppendAndRef(t *testing.T) {
	db, err := tsdbv2.New(t.TempDir())
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	app1 := db.Appender(ctx)

	ref1, err := app1.Append(0, labels.FromStrings("a", "b"), 123, 0)
	require.NoError(t, err)

	// Reference should already work before commit.
	ref2, err := app1.Append(ref1, labels.EmptyLabels(), 124, 1)
	require.NoError(t, err)
	require.Equal(t, ref1, ref2)

	err = app1.Commit()
	require.NoError(t, err)

	app2 := db.Appender(ctx)

	// first ref should already work in next transaction.
	ref3, err := app2.Append(ref1, labels.EmptyLabels(), 125, 0)
	require.NoError(t, err)
	require.Equal(t, ref1, ref3)

	ref4, err := app2.Append(ref1, labels.FromStrings("a", "b"), 133, 1)
	require.NoError(t, err)
	require.Equal(t, ref1, ref4)

	// Reference must be valid to add another sample.
	ref5, err := app2.Append(ref2, labels.EmptyLabels(), 143, 2)
	require.NoError(t, err)
	require.Equal(t, ref1, ref5)

	// Missing labels & invalid refs should fail.
	_, err = app2.Append(9999999, labels.EmptyLabels(), 1, 1)
	require.ErrorIs(t, err, compat.ErrInvalidSample)

	require.NoError(t, app2.Commit())

	q, err := db.Querier(0, 200)
	require.NoError(t, err)

	res := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

	require.Equal(t, map[string][]chunks.Sample{
		labels.FromStrings("a", "b").String(): {
			sample{t: 123, f: 0},
			sample{t: 124, f: 1},
			sample{t: 125, f: 0},
			sample{t: 133, f: 1},
			sample{t: 143, f: 2},
		},
	}, res)
}

// query runs a matcher query against the querier and fully expands its data.
func query(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]chunks.Sample {
	ss := q.Select(context.Background(), false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()

	var it chunkenc.Iterator
	result := map[string][]chunks.Sample{}
	for ss.Next() {
		series := ss.At()

		it = series.Iterator(it)
		samples, err := storage.ExpandSamples(it, newSample)
		require.NoError(t, err)
		require.NoError(t, it.Err())

		if len(samples) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = samples
	}
	require.NoError(t, ss.Err())
	require.Empty(t, ss.Warnings())

	return result
}

type sample struct {
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
	t  int64
	f  float64
}

func newSample(t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram) chunks.Sample {
	return sample{h, fh, t, v}
}

func (s sample) T() int64                      { return s.t }
func (s sample) F() float64                    { return s.f }
func (s sample) H() *histogram.Histogram       { return s.h }
func (s sample) FH() *histogram.FloatHistogram { return s.fh }

func (s sample) Type() chunkenc.ValueType {
	switch {
	case s.h != nil:
		return chunkenc.ValHistogram
	case s.fh != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

func (s sample) Copy() chunks.Sample {
	c := sample{t: s.t, f: s.f}
	if s.h != nil {
		c.h = s.h.Copy()
	}
	if s.fh != nil {
		c.fh = s.fh.Copy()
	}
	return c
}

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

func TestStarttime(t *testing.T) {
	t.Run("no data recorded yet", func(t *testing.T) {
		db := data.Test(t)
		base := int64(6)
		ts, err := tsdbv2.StartTime(db, base)
		require.NoError(t, err)
		require.Equal(t, base, ts)
	})

	t.Run("with tenant data samples", func(t *testing.T) {
		db := data.Test(t)
		ra := roaring.NewBitmap()
		bitmaps.BitSliced(ra, 1, 7)
		bitmaps.BitSliced(ra, 2, 8)
		w := new(bitmaps.Batch)
		ba := db.NewBatch()
		require.NoError(t, w.WriteData(ba, 1, 0, fields.Timestamp.Hash(), ra))
		require.NoError(t, ba.Commit(nil))

		ts, err := tsdbv2.StartTime(db, 0)
		require.NoError(t, err)
		require.Equal(t, int64(7), ts)
	})
	t.Run("with tenant data samples out of order", func(t *testing.T) {
		db := data.Test(t)
		ra := roaring.NewBitmap()
		bitmaps.BitSliced(ra, 1, 8)
		bitmaps.BitSliced(ra, 2, 7)
		w := new(bitmaps.Batch)
		ba := db.NewBatch()
		require.NoError(t, w.WriteData(ba, 1, 0, fields.Timestamp.Hash(), ra))
		require.NoError(t, ba.Commit(nil))

		ts, err := tsdbv2.StartTime(db, 0)
		require.NoError(t, err)
		require.Equal(t, int64(7), ts)
	})
}
