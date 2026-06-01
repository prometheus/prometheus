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
			_ = stripeSeries.GC(math.MaxInt64, false)
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
			stripeSeries.SetUnlessAlreadySet(series.lset.Hash(), series)
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
		ref:  chunks.HeadSeriesRef(1),
		lset: lbls1,
	}
	ms2 := memSeries{
		ref:  chunks.HeadSeriesRef(2),
		lset: lbls2,
	}
	hash := lbls1.Hash()
	s := newStripeSeries(1)

	s.SetUnlessAlreadySet(hash, &ms1)
	s.SetUnlessAlreadySet(hash, &ms2)

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

func TestStripeSeries_SetUnlessAlreadySet(t *testing.T) {
	lbl := labels.FromStrings("__name__", "metric", "lbl", "HFnEaGl")

	ss := newStripeSeries(1)

	ms, created := ss.SetUnlessAlreadySet(lbl.Hash(), &memSeries{ref: chunks.HeadSeriesRef(1), lset: lbl})
	require.True(t, created)
	require.Equal(t, lbl, ms.lset)

	ms2, created := ss.SetUnlessAlreadySet(lbl.Hash(), &memSeries{ref: chunks.HeadSeriesRef(2), lset: lbl})
	require.False(t, created)
	require.Equal(t, ms, ms2)
}

// TestSetUnlessAlreadySetConcurrentSameLabels verifies that concurrent SetUnlessAlreadySet calls for
// the same label set produce exactly one canonical series: all callers get the
// same pointer, the winning ref is reachable via GetByID, and losing refs are
// cleaned up before the call returns and are therefore unreachable.
func TestSetUnlessAlreadySetConcurrentSameLabels(t *testing.T) {
	// size=1 forces all goroutines into the same hash bucket.
	ss := newStripeSeries(1)
	lset := labels.FromStrings("__name__", "test_metric")
	hash := lset.Hash()

	const n = 100
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]*memSeries, n)

	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], _ = ss.SetUnlessAlreadySet(hash, &memSeries{ref: chunks.HeadSeriesRef(i + 1), lset: lset})
		}(i)
	}
	close(start)
	wg.Wait()

	canonical := results[0]
	for _, r := range results[1:] {
		require.Same(t, canonical, r)
	}
	require.Same(t, canonical, ss.GetByID(canonical.ref))
	for i := range n {
		if ref := chunks.HeadSeriesRef(i + 1); ref != canonical.ref {
			require.Nil(t, ss.GetByID(ref))
		}
	}
}

// TestSetUnlessAlreadySetConcurrentGC verifies that concurrent SetUnlessAlreadySet and GC do not
// deadlock, that surviving series remain reachable throughout, and that GC-eligible series are
// actually removed.
func TestSetUnlessAlreadySetConcurrentGC(t *testing.T) {
	ss := newStripeSeries(512)

	var (
		mu        sync.Mutex
		survivors []*memSeries
		eligible  []*memSeries
		wg        sync.WaitGroup
		start     = make(chan struct{})
	)

	wg.Add(50)
	for w := range 50 {
		go func(w int) {
			defer wg.Done()
			<-start
			for r := range 20 {
				lset := labels.FromStrings("w", strconv.Itoa(w), "r", strconv.Itoa(r))
				// Odd r: survivor (lastTs=math.MaxInt64).
				// Even r: GC-eligible (lastTs=0, removed by GC(1)).
				lastTs := int64(0)
				if r%2 == 1 {
					lastTs = math.MaxInt64
				}
				s := &memSeries{ref: chunks.HeadSeriesRef(w*20 + r + 1), lset: lset, lastTs: lastTs}
				if got, ok := ss.SetUnlessAlreadySet(lset.Hash(), s); ok {
					mu.Lock()
					if lastTs == math.MaxInt64 {
						survivors = append(survivors, got)
					} else {
						eligible = append(eligible, got)
					}
					mu.Unlock()
				}
			}
		}(w)
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				ss.GC(1, false) // removes series with lastTs < 1, i.e. lastTs==0
			}
		}
	}()

	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	close(start)
	select {
	case <-finished:
		close(done)
	case <-time.After(15 * time.Second):
		close(done)
		t.Fatal("deadlock")
	}

	// Survivors must still be reachable by both ID and hash despite concurrent GC.
	for _, s := range survivors {
		require.Same(t, s, ss.GetByID(s.ref))
		require.Same(t, s, ss.GetByHash(s.lset.Hash(), s.lset))
	}

	// A final synchronous GC pass ensures all eligible series are fully removed,
	// then verify they are unreachable via both lookup paths.
	ss.GC(1, false)
	for _, s := range eligible {
		require.Nil(t, ss.GetByID(s.ref))
		require.Nil(t, ss.GetByHash(s.lset.Hash(), s.lset))
	}
}

func TestStripeSeries_gc(t *testing.T) {
	s, ms1, ms2 := stripeSeriesWithCollidingSeries(t)
	hash := ms1.lset.Hash()

	s.GC(1, false)

	// Verify that we can get neither ms1 nor ms2 after gc-ing corresponding series
	got := s.GetByHash(hash, ms1.lset)
	require.Nil(t, got)
	got = s.GetByHash(hash, ms2.lset)
	require.Nil(t, got)
}
