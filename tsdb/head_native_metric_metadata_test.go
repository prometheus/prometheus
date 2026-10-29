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
	"context"
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unique"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/nativemetadata"
	"github.com/prometheus/prometheus/util/compression"
)

// nativeAllocationSink keeps allocation tests from being optimized away.
var nativeAllocationSink *nativeSeriesMetadata

func makeNativeMetricMetadataPoint(timestamp int64, m metadata.Metadata) nativeMetricMetadataPoint {
	if m.Type == "" {
		m.Type = model.MetricTypeUnknown
	}
	return nativeMetricMetadataPoint{
		EffectiveFrom: timestamp,
		Metadata:      cloneNativeMetricMetadata(m),
	}
}

// nativeMetadataTxn returns the appender's open metadata transaction, or nil if
// it never needed one. The first appender on a fresh Head is wrapped.
func nativeMetadataTxn(app storage.AppenderV2) *nativeMetricMetadataAppender {
	if init, ok := app.(*initAppenderV2); ok {
		if init.app == nil {
			return nil
		}
		app = init.app
	}
	return app.(*headAppenderV2).nativeMetricMetadata
}

// nativeMetricMetadataTestStore owns stand-in series separately from presence.
type nativeMetricMetadataTestStore struct {
	*nativeMetricMetadataStore
	seriesByRef sync.Map
}

func newNativeMetricMetadataTestStore() *nativeMetricMetadataTestStore {
	return &nativeMetricMetadataTestStore{nativeMetricMetadataStore: newNativeMetricMetadataStore()}
}

func (s *nativeMetricMetadataTestStore) indexedSeries(ref chunks.HeadSeriesRef) *memSeries {
	if !s.has(ref) {
		return nil
	}
	series, _ := s.seriesByRef.Load(ref)
	if series == nil {
		return nil
	}
	return series.(*memSeries)
}

func (s *nativeMetricMetadataTestStore) seriesForTest(ref chunks.HeadSeriesRef) *memSeries {
	series, _ := s.seriesByRef.LoadOrStore(ref, &memSeries{ref: ref})
	return series.(*memSeries)
}

func (s *nativeMetricMetadataTestStore) delete(refs map[storage.SeriesRef]struct{}) {
	var versions int64
	for ref := range refs {
		value, ok := s.seriesByRef.LoadAndDelete(chunks.HeadSeriesRef(ref))
		if !ok {
			continue
		}
		series := value.(*memSeries)
		series.Lock()
		series.setGCed()
		if native := series.nativeMetadataLocked(); native != nil {
			versions += int64(len(native.Older) + 1)
		}
		series.Unlock()
	}

	s.nativeMetricMetadataStore.delete(refs, versions)
}

func (s *nativeMetricMetadataTestStore) reset() {
	s.seriesByRef.Clear()
	s.nativeMetricMetadataStore.reset()
}

func legacyMetadataForTest(s *memSeries) *metadata.Metadata {
	s.Lock()
	defer s.Unlock()
	m := s.legacyMetadataLocked()
	if metadata.IsZeroHandle(m) {
		return nil
	}
	v := m.Value()
	return &v
}

func nativeMetadataForTest(s *memSeries) *nativeSeriesMetadata {
	s.Lock()
	defer s.Unlock()
	native := s.nativeMetadataLocked()
	if native == nil {
		return nil
	}
	return &nativeSeriesMetadata{
		History: nativemetadata.History{
			Metadata: native.Metadata, EffectiveFrom: native.EffectiveFrom,
			Older: slices.Clone(native.Older),
		},
		truncated: native.truncated,
	}
}

// stringWithinForTest reports whether s's bytes lie inside buf's backing.
func stringWithinForTest(buf, s string) bool {
	if s == "" {
		return false
	}
	start := uintptr(unsafe.Pointer(unsafe.StringData(buf)))
	p := uintptr(unsafe.Pointer(unsafe.StringData(s)))
	return p >= start && p < start+uintptr(len(buf))
}

func hasSidecarForTest(s *memSeries) bool {
	s.Lock()
	defer s.Unlock()
	return s.metadata != nil
}

func pendingCommitsForTest(s *memSeries) uint32 {
	s.Lock()
	defer s.Unlock()
	return s.pendingCommitCount()
}

// newNativeMetadataTestHead returns a Head with native metadata and a WAL.
func newNativeMetadataTestHead(t *testing.T, opts ...func(*HeadOptions)) *Head {
	t.Helper()
	o := newTestHeadDefaultOptions(1000, true)
	o.EnableNativeMetadata = true
	for _, opt := range opts {
		opt(o)
	}
	head, _ := newTestHeadWithOptions(t, compression.None, o)
	return head
}

func withoutOOO(o *HeadOptions) { o.OutOfOrderTimeWindow.Store(0) }

func setGCedForTest(s *memSeries) {
	s.Lock()
	s.setGCed()
	s.Unlock()
}

func (s *nativeMetricMetadataStore) has(ref chunks.HeadSeriesRef) bool {
	p := nativeMetricMetadataPostings{store: s}
	return p.has(ref)
}

// snapshot copies ref's series-owned history while holding its series lock.
// It reports whether the history exists, leaving snapshot unchanged on a miss.
func (s *nativeMetricMetadataTestStore) snapshot(ref chunks.HeadSeriesRef, snapshot *nativeMetricMetadataSnapshot) bool {
	series := s.indexedSeries(ref)
	if series == nil {
		return false
	}
	series.Lock()
	*snapshot = series.nativeMetadataLocked().snapshotLocked()
	series.Unlock()
	return true
}

func (s *nativeMetricMetadataTestStore) get(ref chunks.HeadSeriesRef) ([]NativeMetricMetadataVersion, bool, bool) {
	var snapshot nativeMetricMetadataSnapshot
	ok := s.snapshot(ref, &snapshot)
	if !ok {
		return nil, false, false
	}
	return snapshot.expand(), snapshot.truncated, true
}

// mustAppendV2 appends a float sample and requires success.
func mustAppendV2(t testing.TB, app storage.AppenderV2, ref storage.SeriesRef, lset labels.Labels, ts int64, v float64, opts storage.AOptions) storage.SeriesRef {
	ref, err := app.Append(ref, lset, 0, ts, v, nil, nil, opts)
	if err != nil {
		t.Helper()
		require.NoError(t, err)
	}
	return ref
}

// nativeMetricMetadataVersionsForTest snapshots a series independently of presence.
func nativeMetricMetadataVersionsForTest(series *memSeries) ([]NativeMetricMetadataVersion, bool, bool) {
	if series == nil {
		return nil, false, false
	}
	series.Lock()
	defer series.Unlock()
	native := series.nativeMetadataLocked()
	if native == nil || series.isGCed() {
		return nil, false, false
	}
	snapshot := native.snapshotLocked()
	return snapshot.expand(), snapshot.truncated, true
}

func (h *Head) nativeMetricMetadataForTest(ref chunks.HeadSeriesRef) ([]NativeMetricMetadataVersion, bool, bool) {
	return nativeMetricMetadataVersionsForTest(h.series.getByID(ref))
}

// requireNativeVersions requires that series has exactly want, without truncation.
func requireNativeVersions(t *testing.T, series *memSeries, want []NativeMetricMetadataVersion) {
	t.Helper()
	got, truncated, ok := nativeMetricMetadataVersionsForTest(series)
	require.True(t, ok)
	require.False(t, truncated)
	require.Equal(t, want, got)
}

// commitNativeMetricMetadata applies observations for ref as one transaction
// through the same path ingestion uses.
func commitNativeMetricMetadata(store *nativeMetricMetadataTestStore, ref chunks.HeadSeriesRef, observations ...nativeMetricMetadataPoint) {
	appender := store.getAppender()
	for _, observation := range observations {
		appender.observe(store.seriesForTest(ref), observation.EffectiveFrom, *observation.Metadata)
	}
	store.commitAppender(appender)
	store.putAppender(appender)
}

// applyNativeHistoryModel is an independent model of a bounded history: points
// override equal timestamps, adjacent equal values coalesce, and the oldest
// versions beyond the cap are evicted. It returns the new history and the
// number evicted.
func applyNativeHistoryModel(want []NativeMetricMetadataVersion, points []nativeMetricMetadataPoint) ([]NativeMetricMetadataVersion, uint64) {
	byTime := make(map[int64]metadata.Metadata)
	for _, version := range want {
		byTime[version.EffectiveFrom] = version.Metadata
	}
	for _, point := range points {
		byTime[point.EffectiveFrom] = *point.Metadata
	}
	timestamps := make([]int64, 0, len(byTime))
	for timestamp := range byTime {
		timestamps = append(timestamps, timestamp)
	}
	slices.Sort(timestamps)
	var got []NativeMetricMetadataVersion
	for _, timestamp := range timestamps {
		if m := byTime[timestamp]; len(got) == 0 || got[len(got)-1].Metadata != m {
			got = append(got, NativeMetricMetadataVersion{EffectiveFrom: timestamp, Metadata: m})
		}
	}
	var evicted uint64
	if extra := len(got) - maxNativeMetricMetadataVersions; extra > 0 {
		evicted = uint64(extra)
		got = got[extra:]
	}
	return got, evicted
}

func TestNativeMetricMetadataStore(t *testing.T) {
	t.Run("atomic counter alignment", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		for _, tc := range []struct {
			name    string
			pointer unsafe.Pointer
		}{
			{name: "series", pointer: unsafe.Pointer(&store.series)},
			{name: "versions", pointer: unsafe.Pointer(&store.versions)},
			{name: "evictions", pointer: unsafe.Pointer(&store.evictions)},
			{name: "oversized", pointer: unsafe.Pointer(&store.oversized)},
			{name: "directory generation", pointer: unsafe.Pointer(&store.directoryGeneration)},
		} {
			require.Zero(t, uintptr(tc.pointer)%8, "%s counter must be 64-bit aligned", tc.name)
		}
	})

	t.Run("presence uses full references and resets without mutating removed history", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		refs := []chunks.HeadSeriesRef{1, 1 + (1 << 32), 1 + (2 << 32)}
		for _, ref := range refs {
			for version := range maxNativeMetricMetadataVersions + 1 {
				commitNativeMetricMetadata(store, ref, makeNativeMetricMetadataPoint(int64(version), metadata.Metadata{Help: strconv.Itoa(version)}))
			}
		}
		require.Equal(t, int64(len(refs)), store.series.Load())
		require.Equal(t, uint64(len(refs)), store.evictions.Load())
		store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(refs[0]): {}})
		store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(refs[0]): {}})
		require.False(t, store.has(refs[0]))
		require.True(t, store.has(refs[1]))
		require.Equal(t, int64(2*maxNativeMetricMetadataVersions), store.versions.Load())
		store.reset()
		for _, ref := range refs {
			require.False(t, store.has(ref))
		}
		require.Zero(t, store.series.Load())
		require.Zero(t, store.versions.Load())
		require.Equal(t, uint64(len(refs)), store.evictions.Load())
	})

	t.Run("concurrent first publication has one owner and one count", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		series := store.seriesForTest(1)
		start := make(chan struct{})
		var workers sync.WaitGroup
		for i := range 16 {
			workers.Go(func() {
				appender := store.getAppender()
				appender.observe(series, 100, metadata.Metadata{Help: strconv.Itoa(i)})
				<-start
				store.commitAppender(appender)
				store.putAppender(appender)
			})
		}
		close(start)
		workers.Wait()
		require.Same(t, series, store.indexedSeries(1))
		require.Equal(t, int64(1), store.series.Load())
		require.Equal(t, int64(1), store.versions.Load())
		require.Zero(t, store.evictions.Load())
		current := nativeMetadataForTest(series)
		require.NotNil(t, current.Metadata)
		require.Nil(t, current.Older)
	})

	t.Run("merge matches an independent bounded history model", func(t *testing.T) {
		random := rand.New(rand.NewPCG(19, 73))
		for trial := range 32 {
			store := newNativeMetricMetadataTestStore()
			var want []NativeMetricMetadataVersion
			var evictions uint64
			for round := range 64 {
				points := make([]nativeMetricMetadataPoint, 1+random.IntN(12))
				for i := range points {
					timestamp := int64(random.IntN(512))
					// Ordered rounds may start at the newest committed timestamp.
					if round%3 == 0 {
						timestamp = int64(1024 + round*3 + i)
					}
					points[i] = makeNativeMetricMetadataPoint(timestamp, metadata.Metadata{Help: strconv.Itoa(random.IntN(4))})
				}
				var evicted uint64
				want, evicted = applyNativeHistoryModel(want, points)
				evictions += evicted
				commitNativeMetricMetadata(store, 1, points...)
				got, truncated, found := store.get(1)
				require.True(t, found)
				require.Equal(t, want, got, "trial %d round %d", trial, round)
				require.Equal(t, evictions > 0, truncated)
				require.Equal(t, evictions, store.evictions.Load())
				require.Equal(t, int64(len(want)), store.versions.Load())
				require.Equal(t, int64(1), store.series.Load())
				current := nativeMetadataForTest(store.indexedSeries(1))
				require.Equal(t, want[len(want)-1].Metadata, *current.Metadata)
				require.Equal(t, want[len(want)-1].EffectiveFrom, current.EffectiveFrom)
				if len(want) == 1 {
					require.Nil(t, current.Older)
				}
			}
		}
	})

	t.Run("interleaved series match independent bounded history models across batches", func(t *testing.T) {
		// Refs 1+256k share one stripe and outnumber a batch. Refs 2..20 and
		// 2+256k add other stripes, mixed with the shared one.
		var refs []chunks.HeadSeriesRef
		for k := range maxNativeMetricMetadataBatch + 44 {
			refs = append(refs, chunks.HeadSeriesRef(1+k*nativeMetricMetadataStripes))
		}
		for ref := chunks.HeadSeriesRef(2); ref <= 20; ref++ {
			refs = append(refs, ref)
		}
		for k := 1; k <= 5; k++ {
			refs = append(refs, chunks.HeadSeriesRef(2+k*nativeMetricMetadataStripes))
		}

		type model struct {
			series    *memSeries
			want      []NativeMetricMetadataVersion
			evictions uint64
		}
		random := rand.New(rand.NewPCG(23, 41))
		for trial := range 4 {
			store := newNativeMetricMetadataTestStore()
			// Fetch each series once, so grouping by reference or pointer is equivalent.
			models := make(map[chunks.HeadSeriesRef]*model, len(refs))
			for _, ref := range refs {
				models[ref] = &model{series: store.seriesForTest(ref)}
			}
			for round := range 6 {
				pending := make(map[chunks.HeadSeriesRef][]nativeMetricMetadataPoint, len(refs))
				for i, ref := range refs {
					count := random.IntN(4)
					// Straddle the first batch's end: the 256th and 257th groups hold several observations.
					if i >= maxNativeMetricMetadataBatch-4 && i <= maxNativeMetricMetadataBatch+4 {
						count = 2 + random.IntN(3)
					}
					for j := range count {
						timestamp := int64(random.IntN(512))
						if round%3 == 0 {
							timestamp = int64(1024 + round*20 + j)
						}
						pending[ref] = append(pending[ref], makeNativeMetricMetadataPoint(timestamp, metadata.Metadata{Help: strconv.Itoa(random.IntN(4))}))
					}
				}

				appender := store.getAppender()
				next := make(map[chunks.HeadSeriesRef]int, len(pending))
				active := make([]chunks.HeadSeriesRef, 0, len(pending))
				for ref := range pending {
					active = append(active, ref)
				}
				slices.Sort(active)
				// Merge the per-series lists randomly, keeping each series' own order.
				for len(active) > 0 {
					i := random.IntN(len(active))
					ref := active[i]
					point := pending[ref][next[ref]]
					appender.observe(models[ref].series, point.EffectiveFrom, *point.Metadata)
					next[ref]++
					if next[ref] == len(pending[ref]) {
						active = slices.Delete(active, i, i+1)
					}
				}
				store.commitAppender(appender)
				store.putAppender(appender)

				var series, versions int64
				var evictions uint64
				for _, ref := range refs {
					m := models[ref]
					if points := pending[ref]; len(points) > 0 {
						var evicted uint64
						m.want, evicted = applyNativeHistoryModel(m.want, points)
						m.evictions += evicted
					}
					got, truncated, found := store.get(ref)
					require.Equal(t, len(m.want) > 0, found, "trial %d round %d ref %d", trial, round, ref)
					if !found {
						continue
					}
					require.Equal(t, m.want, got, "trial %d round %d ref %d", trial, round, ref)
					require.Equal(t, m.evictions > 0, truncated, "trial %d round %d ref %d", trial, round, ref)
					series++
					versions += int64(len(m.want))
					evictions += m.evictions
				}
				require.Equal(t, series, store.series.Load(), "trial %d round %d", trial, round)
				require.Equal(t, versions, store.versions.Load(), "trial %d round %d", trial, round)
				require.Equal(t, evictions, store.evictions.Load(), "trial %d round %d", trial, round)
			}
		}
	})

	t.Run("older storage grows lazily and is reused at the cap", func(t *testing.T) {
		series := &memSeries{}
		native := &nativeSeriesMetadata{}
		capacities := []int{0, 1, 2, 4, 4}
		for i, capacity := range capacities {
			point := makeNativeMetricMetadataPoint(int64(i), metadata.Metadata{Help: strconv.Itoa(i)})
			series.Lock()
			native.mergeLocked([]nativeMetricMetadataPoint{point})
			series.Unlock()
			require.Equal(t, capacity, cap(native.Older))
		}
		point := makeNativeMetricMetadataPoint(10, metadata.Metadata{Help: "next"})
		previous := native.Metadata
		allocs := testing.AllocsPerRun(1000, func() {
			series.Lock()
			native.mergeLocked([]nativeMetricMetadataPoint{point})
			series.Unlock()
			point.EffectiveFrom++
			point.Metadata, previous = previous, point.Metadata
		})
		if !lockOrderInstrumented {
			require.Zero(t, allocs)
		}
		require.True(t, native.truncated)
	})

	t.Run("out-of-order replacements collapsing the history release older storage", func(t *testing.T) {
		series := &memSeries{}
		native := &nativeSeriesMetadata{}
		for i := range maxNativeMetricMetadataVersions + 1 {
			series.Lock()
			native.mergeLocked([]nativeMetricMetadataPoint{makeNativeMetricMetadataPoint(int64(i), metadata.Metadata{Help: strconv.Itoa(i)})})
			series.Unlock()
		}
		var replacements []nativeMetricMetadataPoint
		for i := 1; i <= maxNativeMetricMetadataVersions; i++ {
			replacements = append(replacements, makeNativeMetricMetadataPoint(int64(i), metadata.Metadata{Help: "collapsed"}))
		}
		series.Lock()
		native.mergeLocked(replacements)
		series.Unlock()
		require.Nil(t, native.Older)
		require.Equal(t, int64(1), native.EffectiveFrom)
		require.Equal(t, "collapsed", native.Metadata.Help)
		require.True(t, native.truncated)
	})
}

func TestNativeMetricMetadataAppender(t *testing.T) {
	t.Run("reference encoding", func(t *testing.T) {
		require.Equal(t, uintptr(4), unsafe.Sizeof(nativeMetricMetadataValueRef(0)))
		require.Equal(t, uintptr(4), unsafe.Sizeof(nativeMetricMetadataObservationRef(0)))
		appender := newNativeMetricMetadataAppender(newNativeMetricMetadataValueCache())
		store := newNativeMetricMetadataTestStore()
		for i := range maxNativeMetricMetadataValues {
			m := metadata.Metadata{Help: strconv.Itoa(i)}
			ref := appender.metadataReference(store.seriesForTest(1), m)
			require.Equal(t, nativeMetricMetadataValueRef(i+1), ref)
			require.Equal(t, m, appender.metadataValue(ref))
		}
		m := metadata.Metadata{Help: "direct"}
		appender.observe(store.seriesForTest(1), 100, m)
		require.Len(t, appender.observations, 1)
		require.Len(t, appender.directValues, 1)
		ref := appender.observations[0].metadataRef
		require.Equal(t, nativeMetricMetadataDirectRefMask, ref)
		require.Equal(t, m, appender.metadataValue(ref))
		require.Equal(t, nativeMetricMetadataObservationRef(1), appender.lastObservation)

		// An adjacent direct value is reused across series.
		appender.observe(store.seriesForTest(2), 200, m)
		require.Len(t, appender.directValues, 1)
		require.Equal(t, ref, appender.observations[1].metadataRef)
	})

	t.Run("reuses adjacent metadata across series", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		appender := store.getAppender()
		m := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "shared"}

		appender.observe(store.seriesForTest(1), 100, m)
		metadataRef := appender.observations[0].metadataRef
		appender.observe(store.seriesForTest(2), 200, m)
		appender.observe(store.seriesForTest(2), 300, m)

		require.Len(t, appender.values, 1)
		require.Len(t, appender.observations, 3)
		require.Equal(t, metadataRef, appender.observations[1].metadataRef)
		require.Equal(t, metadataRef, appender.observations[2].metadataRef)
		require.Equal(t, nativeMetricMetadataObservationRef(3), appender.lastObservation)
		store.putAppender(appender)
	})

	t.Run("defers cloning and bounds raw values", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		appender := store.getAppender()
		m := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "A"}

		first := appender.metadataReference(store.seriesForTest(1), m)
		require.Equal(t, first, appender.metadataReference(store.seriesForTest(1), m))
		require.Len(t, appender.values, 1)
		require.Nil(t, appender.values[0].owned)
		require.Empty(t, appender.directValues)
		require.Equal(t, m, *appender.metadataPointer(store.seriesForTest(1), first))
		require.NotNil(t, appender.values[0].owned)

		for i := 1; i <= maxNativeMetricMetadataValues; i++ {
			appender.metadataReference(store.seriesForTest(chunks.HeadSeriesRef(i+1)), metadata.Metadata{Type: model.MetricTypeUnknown, Help: strconv.Itoa(i)})
		}
		require.Len(t, appender.values, maxNativeMetricMetadataValues)
		require.Len(t, appender.directValues, 1)
		store.putAppender(appender)
	})

	t.Run("reuses a committed pointer after the raw value limit", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		appender := store.getAppender()
		ref := chunks.HeadSeriesRef(1)
		m := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "stable"}
		point := makeNativeMetricMetadataPoint(100, m)
		commitNativeMetricMetadata(store, ref, point)
		for i := range maxNativeMetricMetadataValues {
			appender.metadataReference(store.seriesForTest(chunks.HeadSeriesRef(i+2)), metadata.Metadata{Type: model.MetricTypeUnknown, Help: strconv.Itoa(i)})
		}

		metadataRef := appender.metadataReference(store.seriesForTest(ref), m)
		require.Equal(t, nativeMetricMetadataDirectRefMask, metadataRef)
		require.Len(t, appender.directValues, 1)
		require.Same(t, nativeMetadataForTest(store.indexedSeries(ref)).Metadata, appender.metadataPointer(store.seriesForTest(ref), metadataRef))

		changed := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "changed"}
		metadataRef = appender.metadataReference(store.seriesForTest(ref), changed)
		require.Equal(t, nativeMetricMetadataDirectRefMask|nativeMetricMetadataValueRef(1), metadataRef)
		require.Len(t, appender.directValues, 2)
		require.Equal(t, changed, appender.metadataValue(metadataRef))
		store.putAppender(appender)
	})

	t.Run("reuses history across raw and direct references", func(t *testing.T) {
		for _, versions := range []int{1, maxNativeMetricMetadataVersions} {
			for selected := range versions {
				for _, raw := range []int{0, 127, 128, 129} {
					t.Run(strconv.Itoa(versions)+" versions/"+strconv.Itoa(selected)+" selected/"+strconv.Itoa(raw)+" raw", func(t *testing.T) {
						store := newNativeMetricMetadataTestStore()
						series := store.seriesForTest(1)
						points := make([]nativeMetricMetadataPoint, versions)
						for i := range points {
							points[i] = makeNativeMetricMetadataPoint(int64(100+i), metadata.Metadata{Help: "history " + strconv.Itoa(i)})
							commitNativeMetricMetadata(store, series.ref, points[i])
						}
						series = store.indexedSeries(series.ref)
						// The fixture commits through the same resolver, so use the
						// actual retained pointer rather than the input point.
						native := nativeMetadataForTest(series)
						owned := native.Metadata
						if selected < versions-1 {
							owned = native.Older[selected].Metadata
						}
						appender := store.getAppender()
						for i := range raw {
							appender.metadataReference(series, metadata.Metadata{Help: "unrelated " + strconv.Itoa(i)})
						}
						m := *owned
						m.Help = strings.Clone(m.Help)
						ref := appender.metadataReference(series, m)
						require.Same(t, owned, appender.metadataPointer(series, ref))
						require.LessOrEqual(t, len(appender.values), maxNativeMetricMetadataValues)
						appender.observe(series, 1000, m)
						store.commitAppender(appender)
						store.putAppender(appender)
						native = nativeMetadataForTest(series)
						require.Same(t, owned, native.Metadata)
						if selected != versions-1 {
							require.Equal(t, int64(1000), native.EffectiveFrom, "A after B must remain a new observation")
						} else {
							require.Equal(t, int64(100+selected), native.EffectiveFrom)
						}
					})
				}
			}
		}
	})

	t.Run("history reuse requires exact equality", func(t *testing.T) {
		m := metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: strings.Repeat("a", 100)}
		owned := cloneNativeMetricMetadata(m)
		series := &memSeries{}
		series.Lock()
		series.ensureMetadataLocked().native = &nativeSeriesMetadata{History: nativemetadata.History{Metadata: owned}}
		series.Unlock()
		for name, change := range map[string]func(*metadata.Metadata){
			"equal":  func(*metadata.Metadata) {},
			"help":   func(m *metadata.Metadata) { m.Help = m.Help[:len(m.Help)-1] + "b" },
			"length": func(m *metadata.Metadata) { m.Help += "a" },
			"type":   func(m *metadata.Metadata) { m.Type = model.MetricTypeUnknown },
			"unit":   func(m *metadata.Metadata) { m.Unit = "minutes" },
		} {
			t.Run(name, func(t *testing.T) {
				candidate := m
				candidate.Help = strings.Clone(candidate.Help)
				change(&candidate)
				got := newNativeMetricMetadataAppender(newNativeMetricMetadataValueCache()).resolveMetadata(series, candidate)
				require.Equal(t, candidate, *got)
				if name == "equal" {
					require.Same(t, owned, got)
				} else {
					require.NotSame(t, owned, got)
				}
			})
		}
	})

	t.Run("evicted history values are owned again", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		series := store.seriesForTest(1)
		original := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "evicted"}
		commitNativeMetricMetadata(store, series.ref, makeNativeMetricMetadataPoint(100, original))
		series = store.indexedSeries(series.ref)
		owned := nativeMetadataForTest(series).Metadata
		for i := range maxNativeMetricMetadataVersions {
			commitNativeMetricMetadata(store, series.ref, makeNativeMetricMetadataPoint(int64(101+i), metadata.Metadata{Help: strconv.Itoa(i)}))
		}
		appender := newNativeMetricMetadataAppender(newNativeMetricMetadataValueCache())
		ref := appender.metadataReference(series, original)
		resolved := appender.metadataPointer(series, ref)
		require.Equal(t, original, *resolved)
		require.NotSame(t, owned, resolved)
		require.NotSame(t, unsafe.StringData(original.Help), unsafe.StringData(resolved.Help))
	})

	t.Run("pool reset releases transaction state", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		appender := store.getAppender()
		m := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "A"}
		appender.observe(store.seriesForTest(1), 100, m)
		appender.observe(store.seriesForTest(1+nativeMetricMetadataStripes), 100, m)
		require.NotZero(t, appender.multiSeriesStripes)
		appender.metadataPointer(store.seriesForTest(1), appender.observations[0].metadataRef)
		for i := 1; i <= maxNativeMetricMetadataValues; i++ {
			appender.metadataReference(store.seriesForTest(chunks.HeadSeriesRef(i+1)), metadata.Metadata{Type: model.MetricTypeUnknown, Help: strconv.Itoa(i)})
		}
		require.NotEmpty(t, appender.directValues)
		appender.points = append(appender.points, makeNativeMetricMetadataPoint(100, m), makeNativeMetricMetadataPoint(200, m))
		appender.points = appender.points[:1]
		store.putAppender(appender)

		// Inspect this exact object; sync.Pool need not return it again.
		require.Empty(t, appender.observations)
		require.Empty(t, appender.values)
		require.Empty(t, appender.valueRefs)
		require.Empty(t, appender.directValues)
		require.Empty(t, appender.sorted)
		require.Nil(t, appender.valueCache)
		require.Empty(t, appender.points)
		for _, point := range appender.points[:cap(appender.points)] {
			require.Equal(t, nativeMetricMetadataPoint{}, point)
		}
		require.Empty(t, appender.groups)
		require.Zero(t, appender.stripeFirst)
		require.Zero(t, appender.multiSeriesStripes)
		require.Zero(t, appender.lastObservation)
	})

	t.Run("putAppender drops a large string copier", func(t *testing.T) {
		for _, tc := range []struct {
			values  int
			dropped bool
		}{
			{values: nativeMetadataPooledStrings},
			{values: nativeMetadataPooledStrings + 1, dropped: true},
		} {
			t.Run(strconv.Itoa(tc.values)+" strings", func(t *testing.T) {
				store := newNativeMetricMetadataTestStore()
				appender := store.getAppender()
				ref := chunks.HeadSeriesRef(1)
				for i := range maxNativeMetricMetadataValues {
					appender.observe(store.seriesForTest(ref), 100, metadata.Metadata{Help: strconv.Itoa(i)})
					ref++
				}
				for i := range tc.values {
					help := strconv.Itoa(i)
					help = strings.Repeat("0", nativeMetadataInternMinBytes-len(help)) + help
					appender.observe(store.seriesForTest(ref), 100, metadata.Metadata{Help: help})
					ref++
				}
				require.Len(t, appender.directValues, tc.values)
				require.Len(t, appender.strings, tc.values)
				store.putAppender(appender)
				// Inspect this exact object; sync.Pool need not return it again.
				if tc.dropped {
					require.Nil(t, appender.strings)
				} else {
					require.NotNil(t, appender.strings)
					require.Empty(t, appender.strings)
				}
			})
		}
	})

	t.Run("tracks observations in shared stripes", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		appender := store.getAppender()
		m := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "A"}
		firstRef := chunks.HeadSeriesRef(1)
		secondRef := firstRef + nativeMetricMetadataStripes
		thirdRef := secondRef + nativeMetricMetadataStripes

		require.False(t, appender.mayHaveObservedSeries(firstRef))
		appender.observe(store.seriesForTest(firstRef), 100, m)
		require.True(t, appender.mayHaveObservedSeries(firstRef))
		require.False(t, appender.mayHaveObservedSeries(secondRef))

		appender.observe(store.seriesForTest(secondRef), 100, m)
		require.True(t, appender.mayHaveObservedSeries(secondRef))
		require.True(t, appender.mayHaveObservedSeries(thirdRef), "a stripe with multiple observed series must be conservative")
		store.putAppender(appender)
	})
}

func TestNativeMetricMetadataAppenderCommit(t *testing.T) {
	a := metadata.Metadata{Type: model.MetricTypeGauge, Help: "A"}
	b := metadata.Metadata{Type: model.MetricTypeCounter, Help: "B"}
	c := metadata.Metadata{Type: model.MetricTypeUnknown, Help: "C"}

	t.Run("accounts for growth collapse and eviction within one batch", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		first := chunks.HeadSeriesRef(1)
		second := first + nativeMetricMetadataStripes
		third := second + nativeMetricMetadataStripes
		commitNativeMetricMetadata(store, first,
			makeNativeMetricMetadataPoint(100, a), makeNativeMetricMetadataPoint(200, b),
			makeNativeMetricMetadataPoint(300, a), makeNativeMetricMetadataPoint(400, c))
		commitNativeMetricMetadata(store, third,
			makeNativeMetricMetadataPoint(0, a), makeNativeMetricMetadataPoint(10, b),
			makeNativeMetricMetadataPoint(20, a), makeNativeMetricMetadataPoint(30, b), makeNativeMetricMetadataPoint(40, a))
		require.Equal(t, int64(9), store.versions.Load())
		appender := store.getAppender()
		appender.observe(store.seriesForTest(first), 200, a)
		appender.observe(store.seriesForTest(second), 100, b)
		appender.observe(store.seriesForTest(third), 50, b)
		store.commitAppender(appender)
		store.putAppender(appender)
		require.Equal(t, int64(3), store.series.Load())
		require.Equal(t, int64(8), store.versions.Load())
		require.Equal(t, uint64(1), store.evictions.Load())
		store.delete(map[storage.SeriesRef]struct{}{storage.SeriesRef(first): {}, storage.SeriesRef(third): {}})
		require.Equal(t, int64(1), store.series.Load())
		require.Equal(t, int64(1), store.versions.Load())
		require.Equal(t, uint64(1), store.evictions.Load())
	})

	type observation struct {
		series int
		t      int64
		meta   metadata.Metadata
	}
	for _, tc := range []struct {
		name string
		txns [][]observation
		// observed and owned check the last transaction: how many observations
		// it retains, and whether each of its values was cloned on commit.
		observed int
		owned    []bool
		want     [][]NativeMetricMetadataVersion
	}{
		{
			name: "sorts observations and applies the last value at an equal timestamp",
			txns: [][]observation{{{0, 100, a}}, {{0, 300, a}, {0, 150, b}, {0, 200, c}, {0, 200, b}}},
			want: [][]NativeMetricMetadataVersion{{{EffectiveFrom: 100, Metadata: a}, {EffectiveFrom: 150, Metadata: b}, {EffectiveFrom: 300, Metadata: a}}},
		},
		{
			name:     "retains the time range of a consecutive value",
			txns:     [][]observation{{{0, 50, a}, {0, 150, b}}, {{0, 300, a}, {0, 100, a}, {0, 200, a}}},
			observed: 3,
			want:     [][]NativeMetricMetadataVersion{{{EffectiveFrom: 50, Metadata: a}, {EffectiveFrom: 150, Metadata: b}, {EffectiveFrom: 200, Metadata: a}}},
		},
		{
			name:  "merges interleaved stable and changing series in one stripe",
			txns:  [][]observation{{{0, 100, a}}, {{1, 100, a}}, {{0, 200, a}, {1, 200, b}, {0, 250, a}}},
			owned: []bool{false, true},
			want: [][]NativeMetricMetadataVersion{
				{{EffectiveFrom: 100, Metadata: a}},
				{{EffectiveFrom: 100, Metadata: a}, {EffectiveFrom: 200, Metadata: b}},
			},
		},
		{
			name: "resolves ties and coalesces across transactions",
			txns: [][]observation{
				// Unordered input; the later observation wins the tie at 150.
				{{0, 200, b}, {0, 100, a}, {0, 150, c}, {0, 150, b}},
				// Equal values coalesce.
				{{0, 250, c}, {0, 300, c}},
				// A later transaction wins the tie at 150.
				{{0, 150, a}},
				// Equal incoming values preserve the intervening change at 250.
				{{0, 50, a}, {0, 300, a}},
			},
			want: [][]NativeMetricMetadataVersion{{{EffectiveFrom: 50, Metadata: a}, {EffectiveFrom: 250, Metadata: c}, {EffectiveFrom: 300, Metadata: a}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newNativeMetricMetadataTestStore()
			// Both series share a stripe.
			refs := []chunks.HeadSeriesRef{1, 1 + nativeMetricMetadataStripes}
			for i, txn := range tc.txns {
				last := i == len(tc.txns)-1
				appender := store.getAppender()
				for _, o := range txn {
					appender.observe(store.seriesForTest(refs[o.series]), o.t, o.meta)
				}
				if last && tc.observed > 0 {
					require.Len(t, appender.observations, tc.observed)
				}
				store.commitAppender(appender)
				if last {
					for j, owned := range tc.owned {
						require.Equal(t, owned, appender.values[j].owned != nil, "value %d", j)
					}
				}
				store.putAppender(appender)
			}
			for i, want := range tc.want {
				requireNativeVersions(t, store.indexedSeries(refs[i]), want)
			}
		})
	}

	t.Run("advances past a stable batch and commits later bounded batches", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		appender := store.getAppender()
		// One stable batch is followed by more changing series than one batch holds.
		const series = 2*maxNativeMetricMetadataBatch + 1
		for i := range series {
			ref := chunks.HeadSeriesRef(1 + i*nativeMetricMetadataStripes)
			commitNativeMetricMetadata(store, ref, makeNativeMetricMetadataPoint(100, a))
			m := a
			if i >= maxNativeMetricMetadataBatch {
				m = b
			}
			appender.observe(store.seriesForTest(ref), 200, m)
		}
		store.commitAppender(appender)
		require.Nil(t, appender.values[0].owned, "the stable batch needs no cloning")
		require.NotNil(t, appender.values[1].owned)
		require.Equal(t, int64(series), store.series.Load())
		require.Equal(t, int64(series+maxNativeMetricMetadataBatch+1), store.versions.Load())
		for _, i := range []int{maxNativeMetricMetadataBatch, series - 1} {
			ref := chunks.HeadSeriesRef(1 + i*nativeMetricMetadataStripes)
			requireNativeVersions(t, store.indexedSeries(ref), []NativeMetricMetadataVersion{
				{EffectiveFrom: 100, Metadata: a},
				{EffectiveFrom: 200, Metadata: b},
			})
		}
		store.putAppender(appender)
	})

	t.Run("keeps a large observation group intact", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		appender := store.getAppender()
		const observations = maxNativeMetricMetadataBatch + 1
		for i := range observations {
			// Exercise equal-timestamp precedence across the observation boundary.
			m := a
			if i%2 != 0 {
				m = b
			}
			appender.observe(store.seriesForTest(1), 100, m)
		}
		store.commitAppender(appender)
		requireNativeVersions(t, store.indexedSeries(1), []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: a}})
		require.Len(t, appender.groups, 1)
		require.Equal(t, observations, appender.groups[0].end-appender.groups[0].start)
		store.putAppender(appender)
	})

	t.Run("older transaction cannot overwrite the newest value", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		ref := chunks.HeadSeriesRef(1)
		commitNativeMetricMetadata(store, ref, makeNativeMetricMetadataPoint(100, a))
		series := store.seriesForTest(ref)
		stale := store.getAppender()
		stale.observe(series, 150, c)
		commitNativeMetricMetadata(store, ref, makeNativeMetricMetadataPoint(200, b), makeNativeMetricMetadataPoint(300, a))
		current := nativeMetadataForTest(series).Metadata
		store.commitAppender(stale)
		got := nativeMetadataForTest(series)
		require.Equal(t, int64(300), got.EffectiveFrom)
		require.Same(t, current, got.Metadata)
		require.Equal(t, a, *got.Metadata)
		store.putAppender(stale)
	})

	t.Run("coalesces repeated metadata into one version", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		ref := chunks.HeadSeriesRef(1)

		const observations = 4096
		points := make([]nativeMetricMetadataPoint, observations)
		for i := range points {
			points[i] = makeNativeMetricMetadataPoint(int64(i), a)
		}
		commitNativeMetricMetadata(store, ref, points...)

		series := store.indexedSeries(ref)
		backing := series.nativeMetadataLocked()
		require.Nil(t, backing.Older, "one version needs no older-point allocation")
		require.False(t, backing.truncated)
		require.Zero(t, store.evictions.Load())

		commitNativeMetricMetadata(store, ref, makeNativeMetricMetadataPoint(observations, a))
		commitNativeMetricMetadata(store, ref, makeNativeMetricMetadataPoint(0, a))
		require.Same(t, backing, series.nativeMetadataLocked())
		require.Equal(t, int64(1), store.versions.Load())
	})
}

func TestHeadAppenderV2MetadataSidecar(t *testing.T) {
	t.Run("sidecar pointer and layout sizes", func(t *testing.T) {
		require.Equal(t, 2*unsafe.Sizeof((*metadata.Metadata)(nil)), unsafe.Sizeof(memSeriesMetadata{}))
	})
	t.Run("an epoch without versions", func(t *testing.T) {
		// Native state that only holds an epoch is invisible to readers, and
		// keeps its epoch once it gains versions.
		series := &memSeries{}
		series.Lock()
		defer series.Unlock()
		require.Zero(t, series.nativeMetadataEpochLocked())
		series.bumpNativeMetadataEpochLocked()
		require.Equal(t, uint32(1), series.nativeMetadataEpochLocked())
		require.Nil(t, series.nativeMetadataLocked())
		native := series.initNativeMetadataLocked()
		require.Same(t, native, series.initNativeMetadataLocked())
		native.mergeLocked([]nativeMetricMetadataPoint{makeNativeMetricMetadataPoint(1, metadata.Metadata{Help: "a"})})
		require.Same(t, native, series.nativeMetadataLocked())
		require.Equal(t, uint32(1), series.nativeMetadataEpochLocked())
	})
	t.Run("native metadata adds nothing to a series", func(t *testing.T) {
		// Native metadata, its epoch included, lives in the lazily
		// allocated sidecar, so a series is as large as without it: 168
		// bytes beside its labels on 64-bit platforms, 120 on 32-bit ones.
		base := uintptr(168)
		if strconv.IntSize == 32 {
			base = 120
		}
		require.Equal(t, base+unsafe.Sizeof(labels.Labels{}), unsafe.Sizeof(memSeries{}))
	})
	t.Run("native state layout and allocation", func(t *testing.T) {
		// The epoch fills padding on 64-bit platforms. On 32-bit ones, it
		// grows the state from 28 to 32 bytes, the size class it took anyway.
		var native nativeSeriesMetadata
		wantSize := uintptr(48)
		if strconv.IntSize == 32 {
			wantSize = 32
		}
		require.Equal(t, wantSize, unsafe.Sizeof(native))
		require.Equal(t, unsafe.Offsetof(native.truncated)+4, unsafe.Offsetof(native.epoch))

		const runs = 1000
		for _, tc := range []struct {
			name        string
			legacy      bool
			wantObjects float64
		}{
			{name: "no sidecar yet", wantObjects: 2},
			{name: "legacy sidecar present", legacy: true, wantObjects: 1},
		} {
			// Fresh series for each measurement; AllocsPerRun makes one extra warm-up call.
			prepare := func() []memSeries {
				series := make([]memSeries, runs+1)
				if tc.legacy {
					for i := range series {
						series[i].ensureMetadataLocked()
					}
				}
				return series
			}
			series, next := prepare(), 0
			allocate := func() {
				nativeAllocationSink = series[next].initNativeMetadataLocked()
				next++
			}
			require.Equal(t, tc.wantObjects, testing.AllocsPerRun(runs, allocate), tc.name)
		}
	})

	meta := metadata.Metadata{Type: model.MetricTypeCounter, Unit: "requests", Help: "requests"}
	for _, order := range []string{"native first", "legacy first", "concurrent"} {
		t.Run("initialization order "+order, func(t *testing.T) {
			head := newNativeMetadataTestHead(t, withoutOOO)
			seed := head.AppenderV2(t.Context())
			ref, err := seed.Append(0, labels.FromStrings(labels.MetricName, "initialization_order"), 0, 100, 1, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, seed.Commit())
			series := head.series.getByID(chunks.HeadSeriesRef(ref))
			require.Nil(t, series.metadata)

			legacyValue := metadata.Metadata{Type: model.MetricTypeGauge, Help: "legacy"}
			// Seed legacy state directly to exercise sidecar reuse, including the
			// state a legacy WAL can leave behind during an upgrade.
			legacy := func() {
				series.Lock()
				series.setLegacyMetadataLocked(unique.Make(legacyValue))
				series.Unlock()
			}
			native := head.AppenderV2(t.Context())
			mustAppendV2(t, native, ref, labels.EmptyLabels(), 200, 2, storage.AOptions{Metadata: meta})
			require.Nil(t, series.metadata, "preparing either transaction must remain lazy")

			var first *memSeriesMetadata
			switch order {
			case "native first":
				require.NoError(t, native.Commit())
				first = series.metadata
				require.Same(t, first.native, series.nativeMetadataLocked())
				legacy()
			case "legacy first":
				legacy()
				first = series.metadata
				require.Nil(t, series.nativeMetadataLocked(), "a legacy sidecar is not committed native history")
				require.Zero(t, head.nativeMetricMetadata.series.Load())
				require.Zero(t, head.nativeMetricMetadata.versions.Load())
				require.False(t, head.nativeMetricMetadata.has(chunks.HeadSeriesRef(ref)))
				require.NoError(t, native.Commit())
			case "concurrent":
				start := make(chan struct{})
				done := make(chan error, 2)
				go func() { <-start; legacy(); done <- nil }()
				go func() { <-start; done <- native.Commit() }()
				close(start)
				// Join both writers before assertions can end the subtest.
				firstErr, secondErr := <-done, <-done
				require.NoError(t, firstErr)
				require.NoError(t, secondErr)
			}
			if first != nil {
				require.Same(t, first, series.metadata, "adding the other mode must not replace the sidecar")
			}
			require.Equal(t, int64(1), head.nativeMetricMetadata.series.Load())
			require.Equal(t, int64(1), head.nativeMetricMetadata.versions.Load())
			require.True(t, head.nativeMetricMetadata.has(chunks.HeadSeriesRef(ref)))
			require.Equal(t, legacyValue, *legacyMetadataForTest(series))
			retained := nativeMetadataForTest(series).Metadata
			require.Equal(t, meta, *retained)
			first = series.metadata
			series.Lock()
			backing := series.nativeMetadataLocked()
			series.Unlock()

			legacyValue.Help = "updated legacy"
			legacy()
			require.Same(t, first, series.metadata)
			series.Lock()
			unchanged := backing == series.nativeMetadataLocked()
			series.Unlock()
			require.True(t, unchanged, "legacy updates must retain the native backing")
			require.Equal(t, legacyValue, *legacyMetadataForTest(series))
			require.Same(t, retained, nativeMetadataForTest(series).Metadata)
		})
	}
	tooLarge := metadata.Metadata{Type: model.MetricTypeGauge, Help: strings.Repeat("h", maxNativeMetricMetadataBytes)}
	for _, tc := range []struct {
		name           string
		legacy, native bool
		meta           metadata.Metadata
		// end is how the transaction ends: a commit by default, or a rollback,
		// an out-of-order rejection or a WAL failure.
		end                    string
		wantLegacy, wantNative bool
		skipped                bool
	}{
		{name: "off", meta: meta},
		{name: "legacy", legacy: true, meta: meta, wantLegacy: true},
		{name: "native", native: true, meta: meta, wantNative: true},
		// Legacy metadata does not depend on the native size limit.
		{name: "oversized native", native: true, meta: tooLarge, skipped: true},
		{name: "oversized legacy", legacy: true, meta: tooLarge, wantLegacy: true},
		{name: "empty legacy metadata", legacy: true},
		{name: "legacy rollback", legacy: true, meta: meta, end: "rollback"},
		{name: "legacy rejected append", legacy: true, meta: meta, end: "reject"},
		{name: "legacy WAL failure", legacy: true, meta: meta, end: "WAL failure"},
		{name: "empty metadata", native: true},
		{name: "rollback", native: true, meta: meta, end: "rollback"},
		{name: "rejected append", native: true, meta: meta, end: "reject"},
		{name: "WAL failure", native: true, meta: meta, end: "WAL failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := newTestHeadDefaultOptions(1000, true)
			opts.EnableMetadataWALRecords = tc.legacy
			opts.EnableNativeMetadata = tc.native
			head, wal := newTestHeadWithOptions(t, compression.None, opts)
			lset := labels.FromStrings(labels.MetricName, "requests_total", "job", "api")

			var ref storage.SeriesRef
			ts, aopts := int64(100), storage.AOptions{Metadata: tc.meta}
			if tc.end == "reject" {
				seed := head.AppenderV2(t.Context())
				ref = mustAppendV2(t, seed, 0, lset, ts, 1, storage.AOptions{})
				require.NoError(t, seed.Commit())
				ts, aopts.RejectOutOfOrder = 50, true
			}
			app := head.AppenderV2(t.Context())
			got, err := app.Append(ref, lset, 0, ts, 1, nil, nil, aopts)
			if tc.end == "reject" {
				require.ErrorIs(t, err, storage.ErrOutOfOrderSample)
			} else {
				require.NoError(t, err)
				ref = got
			}
			series := head.series.getByID(chunks.HeadSeriesRef(ref))
			require.False(t, hasSidecarForTest(series), "uncommitted metadata must not allocate the sidecar")
			switch tc.end {
			case "":
				require.NoError(t, app.Commit())
			case "WAL failure":
				require.NoError(t, wal.Close())
				require.Error(t, app.Commit())
			default:
				require.NoError(t, app.Rollback())
			}

			require.Equal(t, tc.wantLegacy || tc.wantNative, hasSidecarForTest(series))
			if tc.wantLegacy {
				require.Equal(t, &tc.meta, legacyMetadataForTest(series))
			} else {
				require.Nil(t, legacyMetadataForTest(series))
			}
			store := head.nativeMetricMetadata
			if !tc.native {
				require.Nil(t, store)
				return
			}
			var wantSeries int64
			if tc.wantNative {
				wantSeries = 1
				requireNativeVersions(t, head.series.getByID(chunks.HeadSeriesRef(ref)), []NativeMetricMetadataVersion{{EffectiveFrom: ts, Metadata: tc.meta}})
			} else {
				require.Nil(t, nativeMetadataForTest(series))
			}
			require.Equal(t, wantSeries, store.series.Load())
			var wantSkipped uint64
			if tc.skipped {
				wantSkipped = 1
			}
			require.Equal(t, wantSkipped, store.oversized.Load())
		})
	}
}

func TestHeadAppenderV2NativeMetricMetadataTransactions(t *testing.T) {
	lset := labels.FromStrings(labels.MetricName, "requests_total", "job", "api")
	a := metadata.Metadata{Type: model.MetricTypeCounter, Help: "A"}
	b := metadata.Metadata{Type: model.MetricTypeCounter, Help: "B"}
	c := metadata.Metadata{Type: model.MetricTypeCounter, Help: "C"}

	t.Run("interleaved series in shared stripes match full-observation history models", func(t *testing.T) {
		// A large chunk range and no out-of-order ingestion keep every append in
		// bounds. Timestamps strictly increase per series, so a model that records
		// every observation agrees with the appender's discarding of unchanged ones.
		opts := newTestHeadDefaultOptions(1e9, false)
		opts.EnableNativeMetadata = true
		head, _ := newTestHeadWithOptions(t, compression.None, opts)
		ctx := context.Background()

		const numSeries = 320 // More than the stripe count, so stripes are shared.
		values := make([]metadata.Metadata, 6)
		for i := range values {
			values[i] = metadata.Metadata{Type: model.MetricTypeCounter, Help: "help " + strconv.Itoa(i)}
		}
		lsets := make([]labels.Labels, numSeries)
		refs := make([]chunks.HeadSeriesRef, numSeries)
		current := make([]metadata.Metadata, numSeries)
		want := make([][]NativeMetricMetadataVersion, numSeries)
		random := rand.New(rand.NewPCG(5, 11))
		ts := int64(100)

		observe := func(app storage.AppenderV2, i int, m metadata.Metadata) {
			t.Helper()
			ref := mustAppendV2(t, app, storage.SeriesRef(refs[i]), lsets[i], ts, float64(ts), storage.AOptions{Metadata: m})
			refs[i] = chunks.HeadSeriesRef(ref)
			if len(want[i]) == 0 || want[i][len(want[i])-1].Metadata != m {
				want[i] = append(want[i], NativeMetricMetadataVersion{EffectiveFrom: ts, Metadata: m})
				if len(want[i]) > maxNativeMetricMetadataVersions {
					want[i] = want[i][1:]
				}
			}
			current[i] = m
			ts++
		}
		other := func(m metadata.Metadata) metadata.Metadata {
			for {
				if v := values[random.IntN(len(values))]; v != m {
					return v
				}
			}
		}

		for i := range numSeries {
			lsets[i] = labels.FromStrings(labels.MetricName, "model_series", "id", strconv.Itoa(i))
		}
		seed := head.AppenderV2(ctx)
		for i := range numSeries {
			observe(seed, i, values[i%2])
		}
		require.NoError(t, seed.Commit())

		// X and S share a stripe. X's observation comes first, so S is never the
		// stripe's first observed series.
		x, sIdx := 0, -1
		for i := 1; i < numSeries; i++ {
			if refs[i]%nativeMetricMetadataStripes == refs[x]%nativeMetricMetadataStripes {
				sIdx = i
				break
			}
		}
		require.NotEqual(t, -1, sIdx, "no two series share a stripe")

		for round := range 6 {
			app := head.AppenderV2(ctx)
			if round == 2 {
				committed := current[sIdx]
				observe(app, x, other(current[x]))
				observe(app, sIdx, other(committed))
				observe(app, sIdx, committed)
			}
			for i := range numSeries {
				if round == 2 && (i == x || i == sIdx) {
					continue
				}
				for range random.IntN(3) {
					observe(app, i, other(current[i]))
				}
			}
			require.NoError(t, app.Commit())

			for i := range numSeries {
				got, _, ok := head.nativeMetricMetadataForTest(refs[i])
				require.True(t, ok, "round %d series %d", round, i)
				require.Equal(t, want[i], got, "round %d series %d", round, i)
			}
			require.Equal(t, int64(numSeries), head.nativeMetricMetadata.series.Load())
			if round == 2 {
				got, _, _ := head.nativeMetricMetadataForTest(refs[sIdx])
				require.Equal(t, got[len(got)-1].Metadata, got[len(got)-3].Metadata, "S returns to its committed value")
			}
		}
	})

	type observation struct {
		t    int64
		meta metadata.Metadata
		// recording is whether the appender holds a metadata transaction after
		// the append.
		recording bool
	}
	for _, tc := range []struct {
		name string
		txns [][]observation
		want []NativeMetricMetadataVersion
	}{
		{
			// Every sample reserves the series once, however often its metadata changes.
			name: "multiple observations add no reservations",
			txns: [][]observation{{{100, a, true}, {200, b, true}, {200, c, true}}},
			want: []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: a}, {EffectiveFrom: 200, Metadata: c}},
		},
		{
			name: "later out-of-order change can follow a discarded stable observation",
			txns: [][]observation{{{100, a, true}}, {{300, a, false}, {200, b, true}}},
			want: []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: a}, {EffectiveFrom: 200, Metadata: b}},
		},
		{
			name: "restores metadata changed earlier in the same transaction",
			txns: [][]observation{{{100, a, true}}, {{200, b, true}, {300, a, true}}},
			want: []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: a}, {EffectiveFrom: 200, Metadata: b}, {EffectiveFrom: 300, Metadata: a}},
		},
		{
			// Unchanged metadata takes no metadata transaction from the pool.
			name: "unchanged metadata opens no metadata transaction",
			txns: [][]observation{{{100, a, true}}, {{200, a, false}}, {{300, b, true}}},
			want: []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: a}, {EffectiveFrom: 300, Metadata: b}},
		},
		{
			// The newest version is still b@200 after a@150, so a@250 is a change.
			name: "out-of-order append behind the newest version still records",
			txns: [][]observation{{{100, a, true}}, {{200, b, true}}, {{150, a, true}}, {{250, a, true}}},
			want: []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: a}, {EffectiveFrom: 200, Metadata: b}, {EffectiveFrom: 250, Metadata: a}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head := newNativeMetadataTestHead(t)
			var ref storage.SeriesRef
			for _, txn := range tc.txns {
				reservations := uint32(len(txn))
				if ref == 0 {
					reservations++ // The series created by this transaction.
				}
				app := head.AppenderV2(context.Background())
				for _, o := range txn {
					ref = mustAppendV2(t, app, ref, lset, o.t, float64(o.t), storage.AOptions{Metadata: o.meta})
					require.Equal(t, o.recording, nativeMetadataTxn(app) != nil, "append at %d", o.t)
				}
				series := head.series.getByID(chunks.HeadSeriesRef(ref))
				require.Equal(t, reservations, pendingCommitsForTest(series))
				require.NoError(t, app.Commit())
				require.Zero(t, pendingCommitsForTest(series))
			}
			requireNativeVersions(t, head.series.getByID(chunks.HeadSeriesRef(ref)), tc.want)
		})
	}

	t.Run("outstanding unchanged metadata does not restore an intervening change", func(t *testing.T) {
		head := newNativeMetadataTestHead(t)

		seed := head.AppenderV2(context.Background())
		ref := mustAppendV2(t, seed, 0, lset, 100, 1, storage.AOptions{Metadata: a})
		require.NoError(t, seed.Commit())

		later := head.AppenderV2(context.Background())
		mustAppendV2(t, later, ref, lset, 200, 2, storage.AOptions{Metadata: a})

		intervening := head.AppenderV2(context.Background())
		mustAppendV2(t, intervening, ref, lset, 150, 1.5, storage.AOptions{Metadata: b})
		require.NoError(t, intervening.Commit())
		require.NoError(t, later.Commit())

		// later observed metadata that already matched the series, so it
		// recorded nothing and has nothing left to re-assert once intervening
		// moves the series to b. The history therefore ends at b rather than
		// restoring a at 200. This is the cost of deciding at append time.
		requireNativeVersions(t, head.series.getByID(chunks.HeadSeriesRef(ref)), []NativeMetricMetadataVersion{
			{EffectiveFrom: 100, Metadata: a},
			{EffectiveFrom: 150, Metadata: b},
		})
	})

	for _, tc := range []struct {
		name     string
		rollback bool
	}{
		{name: "sample reservation protects metadata through commit"},
		{name: "rollback releases the sample reservation", rollback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			head := newNativeMetadataTestHead(t)

			seed := head.AppenderV2(context.Background())
			ref := mustAppendV2(t, seed, 0, lset, 50, 0.5, storage.AOptions{})
			require.NoError(t, seed.Commit())

			app := head.AppenderV2(context.Background())
			meta := metadata.Metadata{Type: model.MetricTypeCounter, Help: "requests"}
			mustAppendV2(t, app, ref, lset, 100, 1, storage.AOptions{Metadata: meta})

			series := head.series.getByID(chunks.HeadSeriesRef(ref))
			pending := pendingCommitsForTest(series)
			require.Equal(t, uint32(1), pending)
			require.Empty(t, head.gcSeries([]storage.SeriesRef{ref}, math.MaxInt64, func(*memSeries) bool { return true }))
			_, _, ok := head.nativeMetricMetadataForTest(chunks.HeadSeriesRef(ref))
			require.False(t, ok)

			if tc.rollback {
				require.NoError(t, app.Rollback())
				pending = pendingCommitsForTest(series)
				require.Zero(t, pending)
				require.Contains(t, head.gcSeries([]storage.SeriesRef{ref}, math.MaxInt64, func(*memSeries) bool { return true }), ref)
				_, _, ok = head.nativeMetricMetadataForTest(chunks.HeadSeriesRef(ref))
				require.False(t, ok)
				return
			}

			require.NoError(t, app.Commit())
			pending = pendingCommitsForTest(series)
			require.Zero(t, pending)
			requireNativeVersions(t, head.series.getByID(chunks.HeadSeriesRef(ref)), []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: meta}})
		})
	}

	t.Run("changing metadata reuses the series cache entry", func(t *testing.T) {
		head := newNativeMetadataTestHead(t)
		ctx := context.Background()

		app := head.AppenderV2(ctx)
		ref := mustAppendV2(t, app, 0, lset, 100, 1, storage.AOptions{Metadata: a})
		require.NoError(t, app.Commit())

		series := head.series.getByID(chunks.HeadSeriesRef(ref))
		series.Lock()
		first := series.metadata
		series.Unlock()
		require.NotNil(t, first)

		// Metadata that keeps changing must not allocate a fresh entry per
		// commit; the series keeps one for its lifetime.
		for i := range 5 {
			app = head.AppenderV2(ctx)
			mustAppendV2(t, app, ref, lset, int64(200+i*100), float64(i), storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeCounter, Help: strconv.Itoa(i)}})
			require.NoError(t, app.Commit())

			series.Lock()
			same := series.metadata
			series.Unlock()
			require.Same(t, first, same)
		}
	})

	t.Run("legacy updates are rejected", func(t *testing.T) {
		head := newNativeMetadataTestHead(t)
		app := head.Appender(t.Context())
		ref, err := app.Append(0, lset, 50, 1)
		require.NoError(t, err)
		_, err = app.UpdateMetadata(ref, lset, a)
		require.ErrorIs(t, err, ErrNativeMetadataLegacyUpdate)
		require.NoError(t, app.Rollback())
	})

	t.Run("oversized metadata is skipped and counted", func(t *testing.T) {
		const limit = maxNativeMetricMetadataBytes
		small := metadata.Metadata{Type: model.MetricTypeCounter, Help: "small"}
		for _, tc := range []struct {
			name   string
			meta   metadata.Metadata
			stored bool
		}{
			{name: "help at the limit", meta: metadata.Metadata{Type: model.MetricTypeGauge, Help: strings.Repeat("h", limit-len(model.MetricTypeGauge))}, stored: true},
			{name: "help one over", meta: metadata.Metadata{Type: model.MetricTypeGauge, Help: strings.Repeat("h", limit-len(model.MetricTypeGauge)+1)}},
			{name: "unit one over", meta: metadata.Metadata{Type: model.MetricTypeGauge, Unit: strings.Repeat("u", limit-len(model.MetricTypeGauge)+1)}},
			{name: "type at the limit", meta: metadata.Metadata{Type: model.MetricType(strings.Repeat("t", limit))}, stored: true},
			{name: "type one over", meta: metadata.Metadata{Type: model.MetricType(strings.Repeat("t", limit+1))}},
			{name: "empty type counts as unknown", meta: metadata.Metadata{Unit: strings.Repeat("u", limit/2), Help: strings.Repeat("h", limit/2)}},
			{name: "empty type, unit and help at the limit", meta: metadata.Metadata{Unit: strings.Repeat("u", limit/2), Help: strings.Repeat("h", limit/2-len(model.MetricTypeUnknown))}, stored: true},
			{name: "empty type, unit and help one over", meta: metadata.Metadata{Unit: strings.Repeat("u", limit/2), Help: strings.Repeat("h", limit/2-len(model.MetricTypeUnknown)+1)}},
		} {
			// The limit applies before sample kinds diverge; see TestHeadNativeMetricMetadataSampleKinds.
			t.Run(tc.name, func(t *testing.T) {
				head := newNativeMetadataTestHead(t)
				lset := labels.FromStrings(labels.MetricName, "oversized")
				app := head.AppenderV2(t.Context())
				ref := mustAppendV2(t, app, 0, lset, 100, 1, storage.AOptions{Metadata: small})
				mustAppendV2(t, app, ref, lset, 200, 2, storage.AOptions{Metadata: tc.meta})
				require.NoError(t, app.Commit())

				want := []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: small}}
				var skipped float64 = 1
				if tc.stored {
					stored := tc.meta
					if stored.Type == "" {
						stored.Type = model.MetricTypeUnknown
					}
					want = append(want, NativeMetricMetadataVersion{EffectiveFrom: 200, Metadata: stored})
					skipped = 0
				}
				requireNativeVersions(t, head.series.getByID(chunks.HeadSeriesRef(ref)), want)
				require.Equal(t, skipped, float64(head.nativeMetricMetadata.oversized.Load()))
			})
		}
	})

	t.Run("values are owned once and shared across series", func(t *testing.T) {
		for _, tc := range []struct {
			name              string
			series, helpBytes int
			// fillers first use up the raw values a transaction retains, so later
			// values are owned as they are observed.
			fillers bool
			// unique follows each series with one of its own value, so only the
			// value cache can share a value between series.
			unique bool
			// variants is how many distinct values the series cycle through; zero
			// gives each series its own.
			variants int
			// direct is whether the last observation was owned as it was observed.
			direct bool
			// backings counts distinct help backings, and copies distinct cached
			// values unless zero.
			backings, copies int
		}{
			{name: "raw", series: 100, helpBytes: 1024, backings: 1},
			{name: "direct", series: 100, helpBytes: 1024, fillers: true, direct: true, backings: 1},
			{name: "raw short", series: 100, helpBytes: 63, backings: 100},
			{name: "direct short", series: 100, helpBytes: 63, fillers: true, direct: true, backings: 100},
			{name: "series sharing metadata share one cached copy", series: 8, helpBytes: 63, variants: 2, backings: 2, copies: 2},
			{name: "more distinct values than one transaction retains raw", series: 3 * maxNativeMetricMetadataValues, helpBytes: 63, direct: true, backings: 3 * maxNativeMetricMetadataValues},
			{name: "identical values beyond the raw value limit", series: 100, helpBytes: maxNativeMetricMetadataBytes - len(model.MetricTypeGauge) - len("u0"), fillers: true, unique: true, variants: 1, direct: true, backings: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				head := newNativeMetadataTestHead(t)
				buf := strings.Repeat("h", 4*tc.helpBytes)
				help := buf[tc.helpBytes : 2*tc.helpBytes]
				app := head.AppenderV2(t.Context())
				appendSeries := func(name string, i int, m metadata.Metadata) storage.SeriesRef {
					lset := labels.FromStrings(labels.MetricName, name, "id", strconv.Itoa(i))
					return mustAppendV2(t, app, 0, lset, 100, 1, storage.AOptions{Metadata: m})
				}
				if tc.fillers {
					for i := range maxNativeMetricMetadataValues {
						appendSeries("filler", i, metadata.Metadata{Type: model.MetricTypeCounter, Help: "filler " + strconv.Itoa(i)})
					}
				}
				refs := make([]storage.SeriesRef, 0, tc.series)
				want := make([]metadata.Metadata, 0, tc.series)
				for i := range tc.series {
					variant := i
					if tc.variants > 0 {
						variant %= tc.variants
					}
					m := metadata.Metadata{Type: model.MetricTypeGauge, Unit: "u" + strconv.Itoa(variant), Help: help}
					refs = append(refs, appendSeries("shared", i, m))
					want = append(want, m)
					if tc.unique {
						appendSeries("unique", i, metadata.Metadata{Type: model.MetricTypeCounter, Help: "unique " + strconv.Itoa(i)})
					}
				}
				txn := nativeMetadataTxn(app)
				require.Equal(t, tc.direct, txn.observations[len(txn.observations)-1].metadataRef&nativeMetricMetadataDirectRefMask != 0)
				require.NoError(t, app.Commit())

				backings := map[*byte]struct{}{}
				copies := map[*metadata.Metadata]struct{}{}
				for i, ref := range refs {
					native := nativeMetadataForTest(head.series.getByID(chunks.HeadSeriesRef(ref)))
					require.NotNil(t, native)
					require.Equal(t, want[i], *native.Metadata)
					require.False(t, stringWithinForTest(buf, native.Metadata.Help), "series %d retains the caller's buffer", i)
					backings[unsafe.StringData(native.Metadata.Help)] = struct{}{}
					copies[native.Metadata] = struct{}{}
				}
				require.Len(t, backings, tc.backings)
				if tc.copies > 0 {
					require.Len(t, copies, tc.copies)
				}
			})
		}
	})

	t.Run("concurrent garbage collection leaves no orphans", func(t *testing.T) {
		head := newNativeMetadataTestHead(t)
		ctx := context.Background()

		const (
			workers      = 4
			rounds       = 20
			perTx        = 512
			evictAllTime = math.MaxInt64
		)
		observe := func(app storage.AppenderV2, worker, round, i int) error {
			lset := labels.FromStrings(labels.MetricName, "requests_total",
				"worker", strconv.Itoa(worker), "id", strconv.Itoa(i))
			_, err := app.Append(0, lset, 0, int64(round+1), float64(round), nil, nil, storage.AOptions{
				Metadata: metadata.Metadata{Type: model.MetricTypeCounter, Help: strconv.Itoa(round)},
			})
			return err
		}

		// Metadata is published while the appender's pending commits still keep
		// its series in the Head. Committing it any later would let a
		// concurrent collection delete the series first, leaving metadata that
		// no later collection can reach.
		var appendWG sync.WaitGroup
		for worker := range workers {
			appendWG.Go(func() {
				for round := range rounds {
					app := head.AppenderV2(ctx)
					for i := range perTx {
						if err := observe(app, worker, round, i); err != nil {
							t.Errorf("append failed: %v", err)
							_ = app.Rollback()
							return
						}
					}
					if err := app.Commit(); err != nil {
						t.Errorf("commit failed: %v", err)
						return
					}
				}
			})
		}

		done := make(chan struct{})
		var gcWG sync.WaitGroup
		gcWG.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				refs := make([]storage.SeriesRef, 0, workers*perTx)
				for all := head.postings.All(); all.Next(); {
					refs = append(refs, all.At())
				}
				head.gcSeries(refs, evictAllTime, func(*memSeries) bool { return true })
			}
		})

		appendWG.Wait()
		close(done)
		gcWG.Wait()

		// Leave something behind so an empty store cannot pass vacuously.
		app := head.AppenderV2(ctx)
		for i := range perTx {
			require.NoError(t, observe(app, 0, rounds, i))
		}
		require.NoError(t, app.Commit())

		var (
			orphans []chunks.HeadSeriesRef
			stored  int64
		)
		head.nativeMetricMetadata.directory.Range(func(key, value any) bool {
			first := chunks.HeadSeriesRef(key.(uint64) << nativeMetadataPageBits)
			for offset := range nativeMetadataPageSize {
				ref := first + chunks.HeadSeriesRef(offset)
				if value.(*nativeMetricMetadataPage).contains(ref) {
					stored++
					if head.series.getByID(ref) == nil {
						orphans = append(orphans, ref)
					}
				}
			}
			return true
		})
		require.Empty(t, orphans, "metadata retained for series the Head no longer has")
		require.Positive(t, stored)
		require.Equal(t, stored, head.nativeMetricMetadata.series.Load())
	})
}

func TestHeadNativeMetricMetadataDeletion(t *testing.T) {
	for _, mode := range []string{"gc", "evict", "replay"} {
		t.Run(mode, func(t *testing.T) {
			head := newNativeMetadataTestHead(t, withoutOOO)
			refs := map[string]storage.SeriesRef{}
			for _, name := range []string{"capped", "changed", "absent", "fresh", "exemplar", "pending"} {
				versions := 1
				if name == "capped" {
					versions = maxNativeMetricMetadataVersions + 1
				}
				if name == "changed" {
					versions = 2
				}
				app := head.AppenderV2(t.Context())
				for version := range versions {
					timestamp := int64(100 + version)
					if name == "fresh" {
						timestamp = 400
					}
					var opts storage.AOptions
					if name != "absent" {
						opts.Metadata = metadata.Metadata{Type: model.MetricTypeGauge, Help: strconv.Itoa(version)}
					}
					refs[name] = mustAppendV2(t, app, refs[name], labels.FromStrings(labels.MetricName, "metadata_deletion", "id", name), timestamp, float64(version), opts)
				}
				require.NoError(t, app.Commit())
			}
			protected := head.series.getByID(chunks.HeadSeriesRef(refs["exemplar"]))
			protected.updateExemplarTimestamp(400)
			pending := head.AppenderV2(t.Context())
			mustAppendV2(t, pending, refs["pending"], labels.EmptyLabels(), 200, 2, storage.AOptions{})
			retired := head.series.getByID(chunks.HeadSeriesRef(refs["capped"]))
			before := nativeMetadataForTest(retired)
			require.Equal(t, int64(5), head.nativeMetricMetadata.series.Load())
			require.Equal(t, int64(maxNativeMetricMetadataVersions+5), head.nativeMetricMetadata.versions.Load())
			require.Equal(t, uint64(1), head.nativeMetricMetadata.evictions.Load())
			head.minValidTime.Store(300)
			for range 2 {
				switch mode {
				case "gc":
					head.minTime.Store(300)
					head.gc()
				case "evict":
					var candidates []storage.SeriesRef
					for _, ref := range refs {
						candidates = append(candidates, ref)
					}
					candidates = append(candidates, refs["capped"], math.MaxUint64)
					head.gcSeries(candidates, 300, func(*memSeries) bool { return true })
				case "replay":
					// Replay never reconstructs native histories. Exercise its deletion
					// bookkeeping directly with populated histories and duplicate references.
					head.deleteSeriesByID([]chunks.HeadSeriesRef{
						chunks.HeadSeriesRef(refs["capped"]), chunks.HeadSeriesRef(refs["changed"]),
						chunks.HeadSeriesRef(refs["absent"]), chunks.HeadSeriesRef(refs["capped"]), math.MaxUint64,
					})
				}
				for _, name := range []string{"capped", "changed", "absent"} {
					require.Nil(t, head.series.getByID(chunks.HeadSeriesRef(refs[name])))
					require.False(t, head.nativeMetricMetadata.has(chunks.HeadSeriesRef(refs[name])))
				}
				require.Equal(t, uint64(3), head.NumSeries())
				require.Equal(t, int64(3), head.nativeMetricMetadata.series.Load())
				require.Equal(t, int64(3), head.nativeMetricMetadata.versions.Load())
				require.Equal(t, before, nativeMetadataForTest(retired), "retirement must not mutate captured history")
			}
			require.NoError(t, pending.Rollback())
			require.Empty(t, head.gcSeries([]storage.SeriesRef{refs["pending"]}, math.MaxInt64, func(*memSeries) bool { return false }))
			require.Equal(t, int64(3), head.nativeMetricMetadata.versions.Load())
			require.NoError(t, head.resetInMemoryState())
			require.Zero(t, head.nativeMetricMetadata.series.Load())
			require.Zero(t, head.nativeMetricMetadata.versions.Load())
			require.Equal(t, uint64(1), head.nativeMetricMetadata.evictions.Load())
		})
	}
}

func TestHeadNativeMetricMetadataFeatureGate(t *testing.T) {
	ctx := context.Background()
	matcher := labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+")

	t.Run("disabled", func(t *testing.T) {
		head, _ := newTestHead(t, 1000, compression.None, false)
		_, _, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{matcher}}, 0)
		require.ErrorIs(t, err, ErrNativeMetadataDisabled)
	})

	t.Run("reset clears in-memory metadata", func(t *testing.T) {
		head := newNativeMetadataTestHead(t, withoutOOO)
		app := head.AppenderV2(ctx)
		_, err := app.Append(0, labels.FromStrings(labels.MetricName, "metric"), 0, 100, 1, nil, nil, storage.AOptions{
			Metadata: metadata.Metadata{Type: model.MetricTypeGauge},
		})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		require.NoError(t, head.resetInMemoryState())
		result, _, err := head.nativeMetricMetadataForMatchers(ctx, [][]*labels.Matcher{{matcher}}, 0)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	nativeMetricsHead := func(t *testing.T, enabled bool) (*Head, *prometheus.Registry) {
		opts := newTestHeadDefaultOptions(1000, false)
		opts.ChunkDirRoot = t.TempDir()
		opts.EnableNativeMetadata = enabled
		reg := prometheus.NewRegistry()
		head, err := NewHead(reg, nil, nil, nil, opts, nil)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, head.Close()) })
		return head, reg
	}
	nativeMetricValues := func(t *testing.T, reg *prometheus.Registry) map[string]float64 {
		families, err := reg.Gather()
		require.NoError(t, err)
		values := map[string]float64{}
		for _, family := range families {
			name := family.GetName()
			if !strings.HasPrefix(name, "prometheus_tsdb_head_native_metric_metadata_") {
				continue
			}
			m := family.GetMetric()[0]
			if g := m.GetGauge(); g != nil {
				values[name] = g.GetValue()
			} else {
				values[name] = m.GetCounter().GetValue()
			}
		}
		return values
	}

	t.Run("metrics absent when disabled", func(t *testing.T) {
		_, reg := nativeMetricsHead(t, false)
		require.Empty(t, nativeMetricValues(t, reg))
	})

	t.Run("metrics present when enabled", func(t *testing.T) {
		head, reg := nativeMetricsHead(t, true)
		lset := labels.FromStrings(labels.MetricName, "requests_total")
		var ref storage.SeriesRef
		for i := range maxNativeMetricMetadataVersions + 1 {
			app := head.AppenderV2(ctx)
			ref = mustAppendV2(t, app, ref, lset, int64(100+i), float64(i), storage.AOptions{
				Metadata: metadata.Metadata{Type: model.MetricTypeCounter, Help: strconv.Itoa(i)},
			})
			require.NoError(t, app.Commit())
		}
		app := head.AppenderV2(ctx)
		mustAppendV2(t, app, ref, lset, 200, 1, storage.AOptions{
			Metadata: metadata.Metadata{Type: model.MetricTypeCounter, Help: strings.Repeat("h", maxNativeMetricMetadataBytes)},
		})
		require.NoError(t, app.Commit())
		require.Equal(t, map[string]float64{
			"prometheus_tsdb_head_native_metric_metadata_series":                  1,
			"prometheus_tsdb_head_native_metric_metadata_versions":                maxNativeMetricMetadataVersions,
			"prometheus_tsdb_head_native_metric_metadata_version_evictions_total": 1,
			"prometheus_tsdb_head_native_metric_metadata_oversized_total":         1,
		}, nativeMetricValues(t, reg))
	})
}

func TestHeadNativeMetricMetadataSampleKinds(t *testing.T) {
	for _, kind := range []string{"float", "histogram", "float-histogram"} {
		t.Run(kind, func(t *testing.T) {
			head := newNativeMetadataTestHead(t, withoutOOO, func(o *HeadOptions) { o.EnableSTAsZeroSample = true })
			lset := labels.FromStrings(labels.MetricName, "a")
			var hist *histogram.Histogram
			var floatHist *histogram.FloatHistogram
			if kind == "histogram" {
				hist = &histogram.Histogram{}
			}
			if kind == "float-histogram" {
				floatHist = &histogram.FloatHistogram{}
			}
			m := metadata.Metadata{Help: "description", Unit: "seconds"}
			app := head.AppenderV2(t.Context())
			ref, err := app.Append(0, lset, 50, 100, 1, hist, floatHist, storage.AOptions{Metadata: m})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			app = head.AppenderV2(t.Context())
			_, err = app.Append(ref, lset, 50, 200, 2, hist, floatHist, storage.AOptions{Metadata: metadata.Metadata{Help: strings.Clone(m.Help), Unit: strings.Clone(m.Unit)}})
			require.NoError(t, err)
			require.Nil(t, nativeMetadataTxn(app), "unchanged content must not stage metadata")
			require.NoError(t, app.Commit())
			versions, _, ok := head.nativeMetricMetadataForTest(chunks.HeadSeriesRef(ref))
			require.True(t, ok)
			wantMetadata := metadata.Metadata{Type: model.MetricTypeUnknown, Help: m.Help, Unit: m.Unit}
			require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 100, Metadata: wantMetadata}}, versions, "synthetic ST samples must not carry metadata")
		})
	}
}

func TestNativeMetricMetadataCacheOwnership(t *testing.T) {
	for _, helpBytes := range []int{18, 100} {
		t.Run(strconv.Itoa(helpBytes)+" byte help", func(t *testing.T) {
			for _, mode := range []string{"raw", "direct", "committed"} {
				t.Run(mode, func(t *testing.T) {
					testNativeMetricMetadataCacheOwnership(t, mode, helpBytes)
				})
			}
		})
	}

	t.Run("deletion leaves deleted history immutable", func(t *testing.T) {
		store := newNativeMetricMetadataTestStore()
		commitNativeMetricMetadata(store, 1,
			makeNativeMetricMetadataPoint(100, metadata.Metadata{Help: "old"}),
			makeNativeMetricMetadataPoint(200, metadata.Metadata{Help: "current"}))
		series := store.indexedSeries(1)
		cached := nativeMetadataForTest(series)
		older := cached.Older[0]
		stale := store.getAppender()
		stale.observe(series, 300, metadata.Metadata{Help: "stale publisher"})
		// Head GC marks the series before removing presence.
		setGCedForTest(series)
		store.delete(map[storage.SeriesRef]struct{}{1: {}})
		store.commitAppender(stale)
		store.putAppender(stale)
		require.False(t, store.has(1))
		require.Same(t, cached.Metadata, nativeMetadataForTest(series).Metadata)
		require.Equal(t, older, cached.Older[0])
		require.Zero(t, store.series.Load())
		require.Zero(t, store.versions.Load())
	})
}

func testNativeMetricMetadataCacheOwnership(t *testing.T, mode string, helpBytes int) {
	store := newNativeMetricMetadataTestStore()
	buf := strings.Repeat("p", 3*helpBytes)
	m := metadata.Metadata{Type: model.MetricTypeUnknown, Help: buf[helpBytes : 2*helpBytes], Unit: "seconds"}
	first, second := store.seriesForTest(1), store.seriesForTest(1+nativeMetricMetadataStripes)
	appender := store.getAppender()
	if mode == "direct" {
		for i := range maxNativeMetricMetadataValues {
			appender.metadataReference(first, metadata.Metadata{Help: strconv.Itoa(i)})
		}
	}
	appender.observe(first, 100, m)
	appender.observe(second, 100, m)
	require.Equal(t, mode == "direct", appender.observations[0].metadataRef&nativeMetricMetadataDirectRefMask != 0)
	store.commitAppender(appender)
	cached := nativeMetadataForTest(first)
	require.Same(t, cached.Metadata, nativeMetadataForTest(second).Metadata)
	require.Equal(t, m, *cached.Metadata)
	require.NotSame(t, unsafe.StringData(m.Help), unsafe.StringData(cached.Metadata.Help), "published strings must not retain caller buffers")
	store.putAppender(appender)
	if mode == "committed" {
		appender = store.getAppender()
		older := metadata.Metadata{Help: "older"}
		appender.observe(first, 50, older)
		appender.observe(second, 50, older)
		store.commitAppender(appender)
		require.Same(t, cached.Metadata, nativeMetadataForTest(first).Metadata)
		store.putAppender(appender)
	}
	// Overwrite this exact object's scratch; pool identity is unspecified.
	poison := metadata.Metadata{Help: "reused transaction"}
	appender.values = append(appender.values, nativeMetricMetadataValue{metadata: poison})
	appender.points = append(appender.points, makeNativeMetricMetadataPoint(500, poison))
	appender.directValues = append(appender.directValues, &poison)
	runtime.GC()
	require.Equal(t, m, *cached.Metadata)
	require.Equal(t, m, *nativeMetadataForTest(second).Metadata)
	for _, published := range []*metadata.Metadata{cached.Metadata, nativeMetadataForTest(second).Metadata} {
		require.False(t, stringWithinForTest(buf, published.Help), "published strings must not retain caller buffers")
	}
}
