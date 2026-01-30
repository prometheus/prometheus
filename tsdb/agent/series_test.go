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
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestNoDeadlock(t *testing.T) {
	const numWorkers = 1000

	var (
		wg           sync.WaitGroup
		started      = make(chan struct{})
		stripeSeries = newStripeSeries(3)
	)

	wg.Add(numWorkers)
	for range numWorkers {
		go func() {
			defer wg.Done()
			<-started
			_ = stripeSeries.GC(math.MaxInt64)
		}()
	}

	wg.Add(numWorkers)
	for i := range numWorkers {
		go func(i int) {
			defer wg.Done()
			<-started

			series := &memSeries{
				ref: chunks.HeadSeriesRef(i),
				lset: labels.FromMap(map[string]string{
					"id": strconv.Itoa(i),
				}),
			}
			stripeSeries.Set(series.lset.Hash(), series)
		}(i)
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	close(started)
	select {
	case <-finished:
		return
	case <-time.After(15 * time.Second):
		require.FailNow(t, "deadlock detected")
	}
}

func labelsWithHashCollision() (labels.Labels, labels.Labels) {
	// These two series have the same XXHash; thanks to https://github.com/pstibrany/labels_hash_collisions
	ls1 := labels.FromStrings("__name__", "metric", "lbl", "HFnEaGl")
	ls2 := labels.FromStrings("__name__", "metric", "lbl", "RqcXatm")

	if ls1.Hash() != ls2.Hash() {
		// These ones are the same when using -tags slicelabels
		ls1 = labels.FromStrings("__name__", "metric", "lbl1", "value", "lbl2", "l6CQ5y")
		ls2 = labels.FromStrings("__name__", "metric", "lbl1", "value", "lbl2", "v7uDlF")
	}

	if ls1.Hash() != ls2.Hash() {
		panic("This code needs to be updated: find new labels with colliding hash values.")
	}

	return ls1, ls2
}

// stripeSeriesWithCollidingSeries returns a stripeSeries with two memSeries having the same, colliding, hash.
func stripeSeriesWithCollidingSeries(*testing.T) (*stripeSeries, *memSeries, *memSeries) {
	lbls1, lbls2 := labelsWithHashCollision()
	ms1 := memSeries{
		lset: lbls1,
	}
	ms2 := memSeries{
		lset: lbls2,
	}
	hash := lbls1.Hash()
	s := newStripeSeries(1)

	s.Set(hash, &ms1)
	s.Set(hash, &ms2)

	return s, &ms1, &ms2
}

func TestStripeSeries_Get(t *testing.T) {
	s, ms1, ms2 := stripeSeriesWithCollidingSeries(t)
	hash := ms1.lset.Hash()

	// Verify that we can get both of the series despite the hash collision
	got := s.GetByHash(hash, ms1.lset)
	require.Same(t, ms1, got)
	got = s.GetByHash(hash, ms2.lset)
	require.Same(t, ms2, got)
}

func TestStripeSeries_gc(t *testing.T) {
	s, ms1, ms2 := stripeSeriesWithCollidingSeries(t)
	hash := ms1.lset.Hash()

	s.GC(1)

	// Verify that we can get neither ms1 nor ms2 after gc-ing corresponding series
	got := s.GetByHash(hash, ms1.lset)
	require.Nil(t, got)
	got = s.GetByHash(hash, ms2.lset)
	require.Nil(t, got)
}
