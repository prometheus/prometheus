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

package storage_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestMockSeries(t *testing.T) {
	s := storage.MockSeries(nil, []int64{1, 2, 3}, []float64{1, 2, 3}, []string{"__name__", "foo"})
	it := s.Iterator(nil)
	ts := []int64{}
	vs := []float64{}
	for it.Next() == chunkenc.ValFloat {
		t, v := it.At()
		ts = append(ts, t)
		vs = append(vs, v)
	}
	require.Equal(t, []int64{1, 2, 3}, ts)
	require.Equal(t, []float64{1, 2, 3}, vs)
}

func TestMockSeriesWithST(t *testing.T) {
	s := storage.MockSeries([]int64{0, 1, 2}, []int64{1, 2, 3}, []float64{1, 2, 3}, []string{"__name__", "foo"})
	it := s.Iterator(nil)
	ts := []int64{}
	vs := []float64{}
	st := []int64{}
	for it.Next() == chunkenc.ValFloat {
		t, v := it.At()
		ts = append(ts, t)
		vs = append(vs, v)
		st = append(st, it.AtST())
	}
	require.Equal(t, []int64{1, 2, 3}, ts)
	require.Equal(t, []float64{1, 2, 3}, vs)
	require.Equal(t, []int64{0, 1, 2}, st)
}
