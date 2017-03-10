// Copyright 2017 The Prometheus Authors
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

package remote

import (
	"sort"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/metric"
)

// This is a struct and not just a renamed type because otherwise the Metric
// field and Metric() methods would clash.
type sampleStreamIterator struct {
	ss *model.SampleStream
}

func (it sampleStreamIterator) Metric() metric.Metric {
	return metric.Metric{Metric: it.ss.Metric}
}

func (it sampleStreamIterator) ValueAtOrBeforeTime(ts model.Time) model.SamplePair {
	// TODO: This is a naive inefficient approach - in reality, queries go mostly
	// linearly through iterators, and we will want to make successive calls to
	// this method more efficient by taking into account the last result index
	// somehow (similarly to how it's done in Prometheus's
	// memorySeriesIterators).
	i := sort.Search(len(it.ss.Values), func(n int) bool {
		return it.ss.Values[n].Timestamp.After(ts)
	})
	if i == 0 {
		return model.SamplePair{Timestamp: model.Earliest}
	}
	return it.ss.Values[i-1]
}

func (it sampleStreamIterator) RangeValues(in metric.Interval) []model.SamplePair {
	n := len(it.ss.Values)
	start := sort.Search(n, func(i int) bool {
		return !it.ss.Values[i].Timestamp.Before(in.OldestInclusive)
	})
	end := sort.Search(n, func(i int) bool {
		return it.ss.Values[i].Timestamp.After(in.NewestInclusive)
	})

	if start == n {
		return nil
	}

	return it.ss.Values[start:end]
}

func (it sampleStreamIterator) Close() {}
