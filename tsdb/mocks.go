/*
 * Copyright 2019 The Prometheus Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tsdb

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/go-kit/kit/log"

	"github.com/prometheus/prometheus/tsdb/labels"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/util/testutil"
)

// This file holds types and functions that are used for testing
// purposes.

const (
	// MockDefaultLabelName is the default label name used in tests.
	MockDefaultLabelName = "labelName"
	// MockDefaultLabelValue is the default label value used in tests.
	MockDefaultLabelValue = "labelValue"
)

type mockSeries struct {
	labels   func() labels.Labels
	iterator func() SeriesIterator
}

// CreateBlock creates a block with given set of series and returns its dir.
// Intended for testing purposes.
func CreateBlock(tb testing.TB, dir string, series []Series) string {
	head := createHead(tb, series)
	compactor, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{1000000}, nil)
	testutil.Ok(tb, err)

	testutil.Ok(tb, os.MkdirAll(dir, 0777))

	// Add +1 millisecond to block maxt because block intervals are half-open: [b.MinTime, b.MaxTime).
	// Because of this block intervals are always +1 than the total samples it includes.
	ulid, err := compactor.Write(dir, head, head.MinTime(), head.MaxTime()+1, nil)
	testutil.Ok(tb, err)
	return filepath.Join(dir, ulid.String())
}

func createHead(tb testing.TB, series []Series) *Head {
	head, err := NewHead(nil, nil, nil, 2*60*60*1000)
	testutil.Ok(tb, err)
	defer head.Close()

	app := head.Appender()
	for _, s := range series {
		ref := uint64(0)
		it := s.Iterator()
		for it.Next() {
			t, v := it.At()
			if ref != 0 {
				err := app.AddFast(ref, t, v)
				if err == nil {
					continue
				}
			}
			ref, err = app.Add(s.Labels(), t, v)
			testutil.Ok(tb, err)
		}
		testutil.Ok(tb, it.Err())
	}
	err = app.Commit()
	testutil.Ok(tb, err)
	return head
}

// GenSeries generates series with a given number of labels and values.
// Intended for testing purposes.
func GenSeries(totalSeries, labelCount int, mint, maxt int64) []Series {
	if totalSeries == 0 || labelCount == 0 {
		return nil
	}

	series := make([]Series, totalSeries)

	for i := 0; i < totalSeries; i++ {
		lbls := make(map[string]string, labelCount)
		lbls[MockDefaultLabelName] = strconv.Itoa(i)
		for j := 1; len(lbls) < labelCount; j++ {
			lbls[MockDefaultLabelName+strconv.Itoa(j)] = MockDefaultLabelValue + strconv.Itoa(j)
		}
		samples := make([]tsdbutil.Sample, 0, maxt-mint+1)
		for t := mint; t < maxt; t++ {
			samples = append(samples, sample{t: t, v: rand.Float64()})
		}
		series[i] = newSeries(lbls, samples)
	}
	return series
}

func newSeries(l map[string]string, s []tsdbutil.Sample) Series {
	return &mockSeries{
		labels:   func() labels.Labels { return labels.FromMap(l) },
		iterator: func() SeriesIterator { return newListSeriesIterator(s) },
	}
}
func (m *mockSeries) Labels() labels.Labels    { return m.labels() }
func (m *mockSeries) Iterator() SeriesIterator { return m.iterator() }

type listSeriesIterator struct {
	list []tsdbutil.Sample
	idx  int
}

func newListSeriesIterator(list []tsdbutil.Sample) *listSeriesIterator {
	return &listSeriesIterator{list: list, idx: -1}
}

func (it *listSeriesIterator) At() (int64, float64) {
	s := it.list[it.idx]
	return s.T(), s.V()
}

func (it *listSeriesIterator) Next() bool {
	it.idx++
	return it.idx < len(it.list)
}

func (it *listSeriesIterator) Seek(t int64) bool {
	if it.idx == -1 {
		it.idx = 0
	}
	// Do binary search between current position and end.
	it.idx = sort.Search(len(it.list)-it.idx, func(i int) bool {
		s := it.list[i+it.idx]
		return s.T() >= t
	})

	return it.idx < len(it.list)
}

func (it *listSeriesIterator) Err() error {
	return nil
}
