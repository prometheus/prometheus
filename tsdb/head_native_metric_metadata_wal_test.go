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
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/nativemetadata"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

// nativeMetadataEntryForTest is a native metadata WAL entry with its values
// inline: an entry of a compact record, or a Metadata record's latest-value
// entry as a legacy entry.
type nativeMetadataEntryForTest struct {
	Ref       chunks.HeadSeriesRef
	Kind      record.NativeMetadataKind
	Truncated bool
	Handoff   bool
	Points    []nativeMetadataPointForTest
}

type nativeMetadataPointForTest struct {
	EffectiveFrom int64
	Type          uint8
	Unit, Help    string
}

func nativeMetadataPointsForTest(dst []nativemetadata.Point, points []nativeMetadataPointForTest, intern func(metadata.Metadata) *metadata.Metadata) []nativemetadata.Point {
	for _, p := range points {
		dst = append(dst, nativemetadata.Point{EffectiveFrom: p.EffectiveFrom, Metadata: intern(metadata.Metadata{
			Type: record.ToMetricType(p.Type),
			Unit: p.Unit,
			Help: p.Help,
		})})
	}
	return dst
}

func appendNativeMetadataPointForTest(dst []nativeMetadataPointForTest, p nativemetadata.Point) []nativeMetadataPointForTest {
	return append(dst, nativeMetadataPointForTest{
		EffectiveFrom: p.EffectiveFrom,
		Type:          record.GetMetricType(p.Metadata.Type),
		Unit:          p.Metadata.Unit,
		Help:          p.Metadata.Help,
	})
}

// nativeMetadataWAL is the content of a WAL directory in WAL order: the last
// checkpoint, if any, then the segments after it.
type nativeMetadataWAL struct {
	checkpoint bool
	types      []record.Type
	entries    []nativeMetadataEntryForTest
	// values holds the dictionary size of each compact record.
	values []int
}

func readNativeMetadataWAL(t testing.TB, dir string) nativeMetadataWAL {
	t.Helper()
	var out nativeMetadataWAL
	var ranges []wlog.SegmentRange
	first := 0
	checkpoint, index, err := wlog.LastCheckpoint(dir)
	if err == nil {
		ranges = append(ranges, wlog.SegmentRange{Dir: checkpoint, Last: math.MaxInt32})
		first = index + 1
		out.checkpoint = true
	} else {
		require.ErrorIs(t, err, record.ErrNotFound)
	}
	ranges = append(ranges, wlog.SegmentRange{Dir: dir, First: first, Last: math.MaxInt32})
	sr, err := wlog.NewSegmentsRangeReader(ranges...)
	require.NoError(t, err)
	defer sr.Close()
	var dec record.Decoder
	r := wlog.NewReader(sr)
	for r.Next() {
		typ := dec.Type(r.Record())
		out.types = append(out.types, typ)
		if typ == record.Metadata || typ == record.NativeMetadataCompact {
			out.entries = append(out.entries, decodeNativeMetadataRecordForTest(t, r.Record())...)
		}
		if typ == record.NativeMetadataCompact {
			var compact record.CompactNativeMetadata
			require.NoError(t, dec.CompactNativeMetadata(r.Record(), &compact))
			out.values = append(out.values, len(compact.Values))
		}
	}
	require.NoError(t, r.Err())
	return out
}

// decodeNativeMetadataRecordForTest decodes the entries of a Metadata record or
// compact record.
func decodeNativeMetadataRecordForTest(t testing.TB, rec []byte) []nativeMetadataEntryForTest {
	t.Helper()
	var dec record.Decoder
	var entries []nativeMetadataEntryForTest
	if dec.Type(rec) == record.Metadata {
		meta, err := dec.Metadata(rec, nil)
		require.NoError(t, err)
		for _, m := range meta {
			entries = append(entries, nativeMetadataEntryForTest{Ref: m.Ref, Kind: record.NativeMetadataLegacy, Points: []nativeMetadataPointForTest{
				{EffectiveFrom: math.MinInt64, Type: m.Type, Unit: m.Unit, Help: m.Help},
			}})
		}
		return entries
	}
	var compact record.CompactNativeMetadata
	require.NoError(t, dec.CompactNativeMetadata(rec, &compact))
	return nativeMetadataEntriesForTest(compact.Values, compact.Entries)
}

// nativeMetadataEntriesForTest returns a compact record's entries with their
// values inline.
func nativeMetadataEntriesForTest(values []record.NativeMetadataValue, compact []record.RefCompactNativeMetadata) []nativeMetadataEntryForTest {
	var entries []nativeMetadataEntryForTest
	for _, e := range compact {
		entry := nativeMetadataEntryForTest{Ref: e.Ref, Kind: e.Kind, Truncated: e.Truncated, Handoff: e.Handoff}
		for _, p := range e.Points {
			v := values[p.Value]
			entry.Points = append(entry.Points, nativeMetadataPointForTest{EffectiveFrom: p.EffectiveFrom, Type: v.Type, Unit: v.Unit, Help: v.Help})
		}
		entries = append(entries, entry)
	}
	return entries
}

// reduce applies the entries in WAL order, as forwarding and replay do.
func (w nativeMetadataWAL) reduce() map[chunks.HeadSeriesRef]nativemetadata.State {
	states := map[chunks.HeadSeriesRef]nativemetadata.State{}
	reduceNativeMetadataEntries(states, w.entries, w.checkpoint)
	return states
}

// reduceNativeMetadataEntries applies entries in order to states, by ref. It
// models the trust rule independently of the readers: after a checkpoint,
// untrusted states that its provenance marker or a later boundary ends, compact
// entries are skipped, and latest-value entries always apply. It returns
// whether the reduction ended untrusted.
func reduceNativeMetadataEntries(states map[chunks.HeadSeriesRef]nativemetadata.State, entries []nativeMetadataEntryForTest, untrusted bool) bool {
	intern := func(m metadata.Metadata) *metadata.Metadata { return &m }
	for _, e := range entries {
		switch {
		case e.Kind == record.NativeMetadataProvenance || e.Kind == record.NativeMetadataBoundary:
			untrusted = false
			continue
		case e.Kind != record.NativeMetadataLegacy && untrusted:
			continue
		}
		state := states[e.Ref]
		state.Apply(e.Kind, e.Truncated, nativeMetadataPointsForTest(nil, e.Points, intern))
		states[e.Ref] = state
	}
	return untrusted
}

// compactNativeMetadataForTest returns entries, which must be groups,
// overrides or markers, as a compact record whose dictionary holds each
// distinct value once.
func compactNativeMetadataForTest(entries ...nativeMetadataEntryForTest) record.CompactNativeMetadata {
	var rec record.CompactNativeMetadata
	indices := map[record.NativeMetadataValue]uint32{}
	for _, e := range entries {
		compact := record.RefCompactNativeMetadata{Ref: e.Ref, Kind: e.Kind, Truncated: e.Truncated, Handoff: e.Handoff}
		for _, p := range e.Points {
			v := record.NativeMetadataValue{Type: p.Type, Unit: p.Unit, Help: p.Help}
			index, ok := indices[v]
			if !ok {
				index = uint32(len(rec.Values))
				indices[v] = index
				rec.Values = append(rec.Values, v)
			}
			compact.Points = append(compact.Points, record.CompactNativeMetadataPoint{EffectiveFrom: p.EffectiveFrom, Value: index})
		}
		rec.Entries = append(rec.Entries, compact)
	}
	return rec
}

// nativeMetadataMarkerForTest returns a compact record of one provenance
// marker or boundary.
func nativeMetadataMarkerForTest(kind record.NativeMetadataKind) []byte {
	var enc record.Encoder
	return enc.NativeMetadataMarker(kind, nil)
}

func nativeMetadataVersions(s nativemetadata.State) []NativeMetricMetadataVersion {
	var versions []NativeMetricMetadataVersion
	for _, p := range s.AppendPoints(nil) {
		versions = append(versions, NativeMetricMetadataVersion{EffectiveFrom: p.EffectiveFrom, Metadata: *p.Metadata})
	}
	return versions
}

func nativeMetadataSnapshot(t testing.TB, db *DB) map[string]NativeMetricMetadataSeries {
	t.Helper()
	series, _, err := db.NativeMetricMetadata(t.Context(), [][]*labels.Matcher{{labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+")}}, 0)
	require.NoError(t, err)
	out := make(map[string]NativeMetricMetadataSeries, len(series))
	for _, s := range series {
		out[s.Labels.String()] = s
	}
	return out
}

func TestNativeMetricMetadataWAL(t *testing.T) {
	m := func(help string) metadata.Metadata {
		return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}
	}
	point := func(from int64, help string) nativeMetadataPointForTest {
		return nativeMetadataPointForTest{EffectiveFrom: from, Type: uint8(record.Counter), Unit: "seconds", Help: help}
	}
	newDB := func(t *testing.T) *DB {
		opts := DefaultOptions()
		opts.EnableNativeMetadata = true
		opts.EnableNativeMetadataPersistence = true
		opts.OutOfOrderTimeWindow = math.MaxInt32
		return newTestDB(t, withOpts(opts))
	}
	a, b, c := labels.FromStrings(labels.MetricName, "a"), labels.FromStrings(labels.MetricName, "b"), labels.FromStrings(labels.MetricName, "c")

	t.Run("commits log unstable groups before samples", func(t *testing.T) {
		db := newDB(t)
		app := db.AppenderV2(t.Context())
		refA, err := app.Append(0, a, 0, 100, 1, nil, nil, storage.AOptions{Metadata: m("a")})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		app = db.AppenderV2(t.Context())
		_, err = app.Append(refA, a, 0, 200, 2, nil, nil, storage.AOptions{Metadata: m("a")})
		require.NoError(t, err)
		refB, err := app.Append(0, b, 0, 200, 1, nil, nil, storage.AOptions{Metadata: m("b")})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		// One batched transaction: a stable sample, then a change. Every later
		// observation of the series is a point; none is redundant in general.
		app = db.AppenderV2(t.Context())
		for i, ts := range []int64{300, 400, 500, 600} {
			help := "c"
			if i == 0 {
				help = "a"
			}
			_, err = app.Append(refA, a, 0, ts, 3, nil, nil, storage.AOptions{Metadata: m(help)})
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())

		wal := readNativeMetadataWAL(t, db.Head().wal.Dir())
		require.Equal(t, []record.Type{
			record.Series, record.NativeMetadataCompact, record.Samples,
			record.Series, record.NativeMetadataCompact, record.Samples,
			record.NativeMetadataCompact, record.Samples,
		}, wal.types)
		// Points of one value share its dictionary entry.
		require.Equal(t, []int{1, 1, 1}, wal.values)
		require.Equal(t, []nativeMetadataEntryForTest{
			{Ref: chunks.HeadSeriesRef(refA), Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(100, "a")}},
			{Ref: chunks.HeadSeriesRef(refB), Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(200, "b")}},
			{Ref: chunks.HeadSeriesRef(refA), Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(400, "c"), point(500, "c"), point(600, "c")}},
		}, wal.entries)
	})

	t.Run("rollbacks log no native metadata", func(t *testing.T) {
		db := newDB(t)
		app := db.AppenderV2(t.Context())
		_, err := app.Append(0, c, 0, 100, 1, nil, nil, storage.AOptions{Metadata: m("c")})
		require.NoError(t, err)
		require.NoError(t, app.Rollback())
		wal := readNativeMetadataWAL(t, db.Head().wal.Dir())
		require.Equal(t, []record.Type{record.Series}, wal.types)
	})

	t.Run("WAL order reproduces native histories", func(t *testing.T) {
		// A single writer per series never reorders merge groups, so reducing
		// the WAL must reproduce native histories exactly, including
		// out-of-order points, eviction and the truncation flag.
		rng := rand.New(rand.NewPCG(3, 4))
		for iteration := range 40 {
			db := newDB(t)
			lsets := []labels.Labels{a, b, c}
			refs := make([]storage.SeriesRef, len(lsets))
			used := make([]map[int64]bool, len(lsets))
			for i := range used {
				used[i] = map[int64]bool{}
			}
			for range 30 {
				app := db.AppenderV2(t.Context())
				for i, lset := range lsets {
					for range rng.IntN(4) {
						ts := int64(rng.IntN(2000))
						if used[i][ts] {
							continue
						}
						used[i][ts] = true
						ref, err := app.Append(refs[i], lset, 0, ts, float64(ts), nil, nil, storage.AOptions{Metadata: m(string(rune('a' + rng.IntN(4))))})
						require.NoError(t, err)
						refs[i] = ref
					}
				}
				require.NoError(t, app.Commit())
			}
			states := readNativeMetadataWAL(t, db.Head().wal.Dir()).reduce()
			snapshot := nativeMetadataSnapshot(t, db)
			require.Len(t, snapshot, len(states), "iteration %d", iteration)
			for i, lset := range lsets {
				state, ok := states[chunks.HeadSeriesRef(refs[i])]
				if !ok {
					continue
				}
				native := snapshot[lset.String()]
				require.Equal(t, native.Versions, nativeMetadataVersions(state), "iteration %d, series %s", iteration, lset)
				require.Equal(t, native.Truncated, state.Truncated, "iteration %d, series %s", iteration, lset)
			}
			// Every open DB maps its chunk segments; 40 of them exhaust a
			// 32-bit address space.
			require.NoError(t, db.Close())
		}
	})

	t.Run("compact records hold the reference merge groups", func(t *testing.T) {
		// The record's entries are the string-valued groups the appender's
		// observations reduce to, and its dictionary holds each value
		// reference the points use once, with the caller's strings. The
		// transaction table deduplicates 128 values by content; others are
		// shared only by consecutive observations.
		rng := rand.New(rand.NewPCG(5, 6))
		store := newNativeMetricMetadataTestStore()
		values := make([]metadata.Metadata, 300)
		for i := range values {
			typ := model.MetricTypeCounter
			if i%7 == 0 {
				typ = model.MetricTypeGauge
			}
			values[i] = metadata.Metadata{Type: typ, Unit: strconv.Itoa(i % 3), Help: strings.Repeat("h", i%5) + strconv.Itoa(i)}
		}
		overflow := 0
		for iteration := range 400 {
			a := store.getAppender()
			distinct := 1 + rng.IntN(3)*rng.IntN(150)
			var last metadata.Metadata
			callerStrings := map[unsafe.Pointer]bool{}
			for range rng.IntN(600) {
				ref := chunks.HeadSeriesRef(1 + rng.IntN(40))
				m := values[rng.IntN(distinct)]
				if rng.IntN(4) == 0 {
					m = last // Consecutive repetition.
				}
				if state := nativeMetadataForTest(store.seriesForTest(ref)); state != nil && rng.IntN(3) == 0 {
					m = *state.Metadata // Possibly stable.
				}
				// Fresh copies, so that dictionary strings can be traced to
				// their observations.
				m = metadata.Metadata{Type: m.Type, Unit: strings.Clone(m.Unit), Help: strings.Clone(m.Help)}
				callerStrings[unsafe.Pointer(unsafe.StringData(m.Help))] = true
				a.observe(store.seriesForTest(ref), int64(rng.IntN(60)), m)
				last = m
			}
			want := nativeMetadataWALEntriesForTest(a)
			recValues, entries := a.appendWALRecord()
			got := nativeMetadataEntriesForTest(recValues, entries)
			require.Equal(t, normalizeNativeMetadataForTest(want), normalizeNativeMetadataForTest(got), "iteration %d", iteration)
			used := make([]bool, len(recValues))
			for _, e := range entries {
				for _, p := range e.Points {
					used[p.Value] = true
				}
			}
			require.NotContains(t, used, false, "iteration %d: every value is used", iteration)
			// Values beyond the transaction table appear once per run of
			// consecutive observations, as owned copies; within it, once, with
			// the caller's strings.
			if len(a.directValues) == 0 {
				seen := map[record.NativeMetadataValue]bool{}
				for _, v := range recValues {
					require.False(t, seen[v], "iteration %d: duplicate value", iteration)
					seen[v] = true
					if v.Help != "" {
						require.True(t, callerStrings[unsafe.Pointer(unsafe.StringData(v.Help))], "iteration %d: values keep the caller's strings", iteration)
					}
				}
			} else {
				overflow++
			}
			// A second pass returns the same record.
			again, _ := a.appendWALRecord()
			require.Equal(t, recValues, again)
			store.commitAppender(a)
			store.putAppender(a)
		}
		require.Positive(t, overflow, "transactions exceed the transaction table")
	})

	t.Run("observations and references follow an independent model", func(t *testing.T) {
		// Each transaction goes through Append, so the appender's own
		// observation decisions run. Its ordered observations, the reference
		// each takes, its table of raw values and the record Commit logs are
		// compared with decisionModelForTest, which reads only the samples,
		// their series' refs and the committed versions.
		db := newDB(t)
		head := db.Head()
		value := func(i int) metadata.Metadata {
			return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: fmt.Sprintf("value %d", i)}
		}
		const seeded = 800
		var refs []chunks.HeadSeriesRef
		var lsets []labels.Labels
		app := db.AppenderV2(t.Context())
		for i := range seeded {
			lsets = append(lsets, labels.FromStrings(labels.MetricName, "decisions", "i", strconv.Itoa(i)))
			ref, err := app.Append(0, lsets[i], 0, 1000, 1, nil, nil, storage.AOptions{Metadata: value(i)})
			require.NoError(t, err)
			refs = append(refs, chunks.HeadSeriesRef(ref))
		}
		require.NoError(t, app.Commit())

		run := func(t *testing.T, samples []decisionSampleForTest) decisionStatsForTest {
			committed := map[chunks.HeadSeriesRef]decisionStateForTest{}
			for _, s := range samples {
				if s.series >= len(refs) {
					continue
				}
				if native := nativeMetadataForTest(head.series.getByID(refs[s.series])); native != nil {
					committed[refs[s.series]] = decisionStateForTest{value: *native.Metadata, from: native.EffectiveFrom}
				}
			}
			before := len(compactWALRecordsForTest(t, head.wal.Dir()))
			a := head.appenderV2()
			for _, s := range samples {
				for s.series >= len(refs) {
					lsets = append(lsets, labels.FromStrings(labels.MetricName, "decisions", "i", strconv.Itoa(len(lsets))))
					refs = append(refs, 0)
				}
				ref, err := a.Append(storage.SeriesRef(refs[s.series]), lsets[s.series], 0, s.t, float64(s.t), nil, nil, storage.AOptions{Metadata: s.m})
				require.NoError(t, err)
				refs[s.series] = chunks.HeadSeriesRef(ref)
			}
			want := decisionModelForTest(samples, refs, committed)
			var got decisionResultForTest
			if n := a.nativeMetricMetadata; n != nil {
				for _, o := range n.observations {
					got.observations = append(got.observations, decisionObservationForTest{ref: o.series.ref, from: o.effectiveFrom, reference: decisionReferenceForTestOf(o.metadataRef)})
				}
				for _, v := range n.values {
					got.table = append(got.table, v.metadata)
				}
				got.direct = len(n.directValues)
			}
			require.Equal(t, want.observations, got.observations, "ordered observations and their references")
			require.Equal(t, want.table, got.table, "the transaction table, in order")
			require.Equal(t, want.direct, got.direct, "direct references")
			require.NoError(t, a.Commit())
			records := compactWALRecordsForTest(t, head.wal.Dir())[before:]
			if want.record == nil {
				require.Empty(t, records)
			} else {
				require.Equal(t, [][]byte{want.record}, records, "the logged record, byte for byte")
			}
			return want.stats
		}

		t.Run("first commits past the table", func(t *testing.T) {
			var samples []decisionSampleForTest
			for i := seeded; i < seeded+300; i++ {
				samples = append(samples, decisionSampleForTest{series: i, t: 2000, m: value(i)})
			}
			stats := run(t, samples)
			require.Equal(t, 300-128, stats.direct)
		})
		t.Run("values recurring non-consecutively past the table", func(t *testing.T) {
			var samples []decisionSampleForTest
			// Values 128-199 fall past the table, and recur on the second pass.
			for i := range 400 {
				samples = append(samples, decisionSampleForTest{series: i, t: 2000, m: value(10000 + i%200)})
			}
			stats := run(t, samples)
			require.Positive(t, stats.duplicates)
		})
		t.Run("consecutive repeats past the table", func(t *testing.T) {
			var samples []decisionSampleForTest
			for i := 300; i < 450; i++ {
				samples = append(samples, decisionSampleForTest{series: i, t: 3000, m: value(20000 + i)})
			}
			for i := 450; i < 460; i++ {
				for ts := int64(3000); ts < 3003; ts++ {
					samples = append(samples, decisionSampleForTest{series: i, t: ts, m: value(20000 + i)})
				}
			}
			stats := run(t, samples)
			require.Positive(t, stats.reusedDirect)
		})
		t.Run("stripe-induced observations", func(t *testing.T) {
			// Refs are consecutive, so series i, i+256 and i+512 share a
			// stripe.
			for _, i := range []int{10, 20, 30} {
				require.Equal(t, refs[i]%256, refs[i+512]%256)
			}
			samples := []decisionSampleForTest{
				// First unchanged samples reserve each series without observing it.
				{series: 522, t: 3999, m: value(522)},
				{series: 542, t: 3999, m: value(542)},
				// Two changes, then an unchanged sample: observed and stable.
				{series: 10, t: 4000, m: value(30010)},
				{series: 266, t: 4000, m: value(30266)},
				{series: 522, t: 4000, m: value(522)},
				// One change: an unchanged sample stays unobserved.
				{series: 20, t: 4000, m: value(30020)},
				{series: 276, t: 4000, m: value(276)},
				// Two changes, then a change whose unchanged leading sample is
				// observed and logged.
				{series: 30, t: 4000, m: value(30030)},
				{series: 286, t: 4000, m: value(30286)},
				{series: 542, t: 4000, m: value(542)},
				{series: 542, t: 4001, m: value(30542)},
				// A value first observed in the stable group, then logged by
				// another series' change.
				{series: 700, t: 4000, m: value(522)},
			}
			stats := run(t, samples)
			require.Equal(t, 2, stats.induced)
			require.Equal(t, 1, stats.inducedLogged)
		})
		t.Run("unchanged metadata before its version's start", func(t *testing.T) {
			stats := run(t, []decisionSampleForTest{
				{series: 600, t: 500, m: value(600)},
				{series: 601, t: 5000, m: value(601)},
			})
			require.Equal(t, 1, stats.backdated)
		})
		t.Run("an equal-timestamp replacement winning with a direct reference", func(t *testing.T) {
			var samples []decisionSampleForTest
			for i := range 128 {
				samples = append(samples, decisionSampleForTest{series: i, t: 6000, m: value(40000 + i)})
			}
			samples = append(samples,
				decisionSampleForTest{series: 200, t: 6000, m: value(40000)},
				decisionSampleForTest{series: 200, t: 6000, m: value(40200)},
			)
			stats := run(t, samples)
			require.Equal(t, 1, stats.replacedByDirect)
		})
		t.Run("empty unit and help", func(t *testing.T) {
			empty := metadata.Metadata{Type: model.MetricTypeGauge}
			run(t, []decisionSampleForTest{{series: 650, t: 7000, m: empty}, {series: 651, t: 7000, m: empty}})
		})
	})
}

func TestNativeMetricMetadataReplay(t *testing.T) {
	m := func(help string) metadata.Metadata {
		return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}
	}
	l := labels.FromStrings(labels.MetricName, "metric")
	nativeOpts := func() *Options {
		opts := DefaultOptions()
		opts.EnableNativeMetadata = true
		opts.EnableNativeMetadataPersistence = true
		return opts
	}
	reopen := func(t *testing.T, db *DB, opts *Options) *DB {
		dir := db.Dir()
		require.NoError(t, db.Close())
		return newTestDB(t, withDir(dir), withOpts(opts))
	}
	commit := func(t *testing.T, db *DB, ts int64, md metadata.Metadata) {
		app := db.AppenderV2(t.Context())
		_, err := app.Append(0, l, 0, ts, 1, nil, nil, storage.AOptions{Metadata: md})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	for _, c := range []struct {
		name      string
		helps     []string
		truncated bool
	}{
		{name: "one version", helps: []string{"a"}},
		{name: "every version", helps: []string{"a", "b", "c"}},
		{name: "a truncated history", helps: []string{"a", "b", "c", "d", "e", "f"}, truncated: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			opts := nativeOpts()
			opts.OutOfOrderTimeWindow = math.MaxInt32
			db := newTestDB(t, withOpts(opts))
			for i, help := range c.helps {
				commit(t, db, int64(100*(i+1)), m(help))
			}
			want := nativeMetadataSnapshot(t, db)
			require.Equal(t, c.truncated, want[l.String()].Truncated)
			versions := len(want[l.String()].Versions)
			require.Equal(t, min(len(c.helps), maxNativeMetricMetadataVersions), versions)

			db = reopen(t, db, opts)
			require.Equal(t, want, nativeMetadataSnapshot(t, db), "replay restores every retained version and the truncation flag")
			require.Equal(t, int64(1), db.head.nativeMetricMetadata.series.Load())
			require.Equal(t, int64(versions), db.head.nativeMetricMetadata.versions.Load())
			// The sender reduces the same history from the WAL.
			ref := db.head.series.getByHash(l.Hash(), l).ref
			wal := readNativeMetadataWAL(t, db.Head().wal.Dir())
			require.Equal(t, want[l.String()].Versions, nativeMetadataVersions(wal.reduce()[ref]))
			require.Equal(t, c.truncated, wal.reduce()[ref].Truncated)

			// Seeded state is committed state: an unchanged sample is stable.
			newest := want[l.String()].Versions[versions-1].Metadata
			commit(t, db, 1000, newest)
			wal = readNativeMetadataWAL(t, db.Head().wal.Dir())
			require.Len(t, wal.entries, len(c.helps))

			// Later merges, in order and out of order, merge into the seeded
			// history as the sender merges them into its own.
			commit(t, db, 150, m("late"))
			commit(t, db, 2000, m("later"))
			wal = readNativeMetadataWAL(t, db.Head().wal.Dir())
			got := nativeMetadataSnapshot(t, db)[l.String()]
			require.Equal(t, got.Versions, nativeMetadataVersions(wal.reduce()[ref]))
			require.Equal(t, got.Truncated, wal.reduce()[ref].Truncated)
		})
	}

	t.Run("legacy entries start at an unknown time", func(t *testing.T) {
		opts := DefaultOptions()
		opts.EnableMetadataWALRecords = true
		db := newTestDB(t, withOpts(opts))
		commit(t, db, 100, m("a"))
		db = reopen(t, db, nativeOpts())
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: math.MinInt64, Metadata: m("a")}}, nativeMetadataSnapshot(t, db)[l.String()].Versions)
	})

	t.Run("reduction precedes ref remapping", func(t *testing.T) {
		point := func(from int64, help string) []nativeMetadataPointForTest {
			return []nativeMetadataPointForTest{{EffectiveFrom: from, Type: uint8(record.Counter), Unit: "seconds", Help: help}}
		}
		group := func(ref chunks.HeadSeriesRef, from int64, help string) []nativeMetadataEntryForTest {
			return []nativeMetadataEntryForTest{{Ref: ref, Kind: record.NativeMetadataGroup, Points: point(from, help)}}
		}
		other := labels.FromStrings(labels.MetricName, "other")
		for _, c := range []struct {
			name string
			recs []any
			want map[string]NativeMetricMetadataVersion
		}{
			{
				// Ref 1 was retired and its label set recreated as ref 2. A
				// delayed entry for ref 1 must not replace ref 2's history.
				name: "latest series record wins",
				recs: []any{
					[]record.RefSeries{{Ref: 1, Labels: l}},
					group(1, 100, "a"),
					[]record.RefSeries{{Ref: 2, Labels: l}},
					group(2, 200, "b"),
					[]record.RefSample{{Ref: 2, T: 200, V: 1}},
					group(1, 300, "c"),
				},
				want: map[string]NativeMetricMetadataVersion{l.String(): {EffectiveFrom: 200, Metadata: m("b")}},
			},
			{
				name: "the current incarnation without metadata",
				recs: []any{
					[]record.RefSeries{{Ref: 1, Labels: l}},
					group(1, 100, "a"),
					[]record.RefSample{{Ref: 1, T: 100, V: 1}},
					[]record.RefSeries{{Ref: 2, Labels: l}},
					[]record.RefSample{{Ref: 2, T: 200, V: 1}},
				},
				want: map[string]NativeMetricMetadataVersion{},
			},
			{
				// Another appender may commit metadata for a published series
				// before its creator logs the series record.
				name: "entries before their series record",
				recs: []any{
					group(7, 100, "a"), []record.RefSeries{{Ref: 7, Labels: other}}, []record.RefSample{{Ref: 7, T: 100, V: 1}},
				},
				want: map[string]NativeMetricMetadataVersion{other.String(): {EffectiveFrom: 100, Metadata: m("a")}},
			},
		} {
			t.Run(c.name, func(t *testing.T) {
				dir := t.TempDir()
				w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
				require.NoError(t, err)
				populateTestWL(t, w, c.recs, nil, false)
				require.NoError(t, w.Close())
				db := newTestDB(t, withDir(dir), withOpts(nativeOpts()))
				got := map[string]NativeMetricMetadataVersion{}
				for name, s := range nativeMetadataSnapshot(t, db) {
					require.Len(t, s.Versions, 1)
					got[name] = s.Versions[0]
				}
				require.Equal(t, c.want, got)
			})
		}
	})

	t.Run("compact records", func(t *testing.T) {
		point := func(from int64, help string) nativeMetadataPointForTest {
			return nativeMetadataPointForTest{EffectiveFrom: from, Type: uint8(record.Counter), Unit: "seconds", Help: help}
		}
		entry := func(ref chunks.HeadSeriesRef, kind record.NativeMetadataKind, truncated bool, points ...nativeMetadataPointForTest) nativeMetadataEntryForTest {
			return nativeMetadataEntryForTest{Ref: ref, Kind: kind, Truncated: truncated, Points: points}
		}
		ls := []labels.Labels{l, labels.FromStrings(labels.MetricName, "two"), labels.FromStrings(labels.MetricName, "three")}
		recs := []any{
			[]record.RefSeries{{Ref: 1, Labels: ls[0]}, {Ref: 2, Labels: ls[1]}, {Ref: 3, Labels: ls[2]}},
			[]record.RefSample{{Ref: 1, T: 300, V: 1}, {Ref: 2, T: 300, V: 1}, {Ref: 3, T: 300, V: 1}},
			// Latest-value and compact entries apply in WAL order.
			[]record.RefMetadata{{Ref: 1, Type: uint8(record.Counter), Unit: "seconds", Help: "a"}},
			[]nativeMetadataEntryForTest{
				entry(1, record.NativeMetadataGroup, false, point(100, "b")),
				entry(2, record.NativeMetadataGroup, false, point(100, "a"), point(200, "c")),
				entry(3, record.NativeMetadataOverride, true, point(50, "a"), point(60, "b")),
			},
			[]nativeMetadataEntryForTest{entry(1, record.NativeMetadataGroup, false, point(300, "c"))},
			// An empty override leaves no versions.
			[]nativeMetadataEntryForTest{entry(2, record.NativeMetadataOverride, false)},
		}
		dir := t.TempDir()
		w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(t, err)
		populateTestWL(t, w, recs, nil, false)
		require.NoError(t, w.Close())
		db := newTestDB(t, withDir(dir), withOpts(nativeOpts()))
		require.Equal(t, map[string]NativeMetricMetadataSeries{
			ls[0].String(): {Labels: ls[0], Versions: []NativeMetricMetadataVersion{{EffectiveFrom: math.MinInt64, Metadata: m("a")}, {EffectiveFrom: 100, Metadata: m("b")}, {EffectiveFrom: 300, Metadata: m("c")}}},
			ls[2].String(): {Labels: ls[2], Versions: []NativeMetricMetadataVersion{{EffectiveFrom: 50, Metadata: m("a")}, {EffectiveFrom: 60, Metadata: m("b")}}, Truncated: true},
		}, nativeMetadataSnapshot(t, db))
	})

	t.Run("compact handoffs declare their refs", func(t *testing.T) {
		point := func(from int64, help string) []nativeMetadataPointForTest {
			return []nativeMetadataPointForTest{{EffectiveFrom: from, Type: uint8(record.Counter), Unit: "seconds", Help: help}}
		}
		group := func(ref chunks.HeadSeriesRef, from int64, help string) []nativeMetadataEntryForTest {
			return []nativeMetadataEntryForTest{{Ref: ref, Kind: record.NativeMetadataGroup, Points: point(from, help)}}
		}
		override := func(ref chunks.HeadSeriesRef, handoff bool, points []nativeMetadataPointForTest) []nativeMetadataEntryForTest {
			return []nativeMetadataEntryForTest{{Ref: ref, Kind: record.NativeMetadataOverride, Handoff: handoff, Points: points}}
		}
		for _, c := range []struct {
			name       string
			last       []any
			want       []NativeMetricMetadataVersion
			undeclared float64
		}{
			{
				name: "an override isn't a declaration",
				last: []any{override(1, false, point(300, "c"))},
				want: []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("b")}},
			},
			{
				// Ref 1 survives the remap of ref 2 onto it; its handoff makes
				// it the current incarnation again.
				name: "a handoff declares its ref",
				last: []any{override(1, true, point(300, "c"))},
				want: []NativeMetricMetadataVersion{{EffectiveFrom: 300, Metadata: m("c")}},
			},
			{
				name: "an empty handoff",
				last: []any{override(1, true, nil)},
			},
			{
				name: "a handoff of a remapped ref",
				last: []any{override(1, true, point(300, "c")), override(2, true, point(400, "d"))},
				want: []NativeMetricMetadataVersion{{EffectiveFrom: 400, Metadata: m("d")}},
			},
			{
				// No Series record declares ref 9, so its handoff doesn't.
				name:       "a handoff without a series record",
				last:       []any{override(9, true, point(300, "z"))},
				want:       []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("b")}},
				undeclared: 1,
			},
		} {
			t.Run(c.name, func(t *testing.T) {
				recs := append([]any{
					[]record.RefSeries{{Ref: 1, Labels: l}},
					group(1, 100, "a"),
					[]record.RefSeries{{Ref: 2, Labels: l}},
					group(2, 200, "b"),
					[]record.RefSample{{Ref: 2, T: 200, V: 1}},
				}, c.last...)
				dir := t.TempDir()
				w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
				require.NoError(t, err)
				populateTestWL(t, w, recs, nil, false)
				require.NoError(t, w.Close())
				db := newTestDB(t, withDir(dir), withOpts(nativeOpts()))
				require.Equal(t, c.want, nativeMetadataSnapshot(t, db)[l.String()].Versions)
				require.Equal(t, c.undeclared, prom_testutil.ToFloat64(db.head.metrics.nativeMetadataUndeclaredHandoffs))
			})
		}
	})

	t.Run("refs of entries without a series record aren't reissued", func(t *testing.T) {
		// Ref 2's Series record never reached the WAL, as when both a failed
		// commit's and its rollback's writes drop it, but its metadata did.
		// A new series must not take ref 2 and with it that metadata.
		dir := t.TempDir()
		w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(t, err)
		other := labels.FromStrings(labels.MetricName, "other")
		populateTestWL(t, w, []any{
			[]record.RefSeries{{Ref: 1, Labels: other}},
			[]record.RefSample{{Ref: 1, T: 100, V: 1}},
			[]nativeMetadataEntryForTest{{Ref: 2, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{{EffectiveFrom: 100, Type: uint8(record.Counter), Unit: "seconds", Help: "lost"}}}},
		}, nil, false)
		require.NoError(t, w.Close())
		db := newTestDB(t, withDir(dir), withOpts(nativeOpts()))
		commit(t, db, 200, m("new"))
		ref := db.head.series.getByHash(l.Hash(), l).ref
		require.Greater(t, ref, chunks.HeadSeriesRef(2))
		wal := readNativeMetadataWAL(t, db.Head().wal.Dir())
		require.Equal(t, []NativeMetricMetadataVersion{{EffectiveFrom: 200, Metadata: m("new")}}, nativeMetadataVersions(wal.reduce()[ref]))
		require.Equal(t, nativeMetadataSnapshot(t, db)[l.String()].Versions, nativeMetadataVersions(wal.reduce()[ref]))
	})

	t.Run("compact records without persistence", func(t *testing.T) {
		// Without native metadata persistence, replay ignores compact records,
		// supported or not, as it ignores records of unknown types: legacy
		// metadata comes from Metadata records only, native metadata starts
		// empty, and the records after them replay.
		unsupported := nativeMetadataMarkerForTest(record.NativeMetadataBoundary)
		unsupported[1] = 9 // An unsupported format.
		for _, native := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
				dir := t.TempDir()
				w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
				require.NoError(t, err)
				two := labels.FromStrings(labels.MetricName, "two")
				populateTestWL(t, w, []any{
					[]record.RefSeries{{Ref: 1, Labels: l}, {Ref: 2, Labels: two}},
					[]record.RefSample{{Ref: 1, T: 1, V: 1}, {Ref: 2, T: 1, V: 1}},
					[]record.RefMetadata{{Ref: 2, Type: uint8(record.Counter), Unit: "seconds", Help: "a"}},
					[]nativeMetadataEntryForTest{
						{Ref: 1, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{{EffectiveFrom: 1, Type: uint8(record.Gauge), Unit: "seconds", Help: "b"}}},
						{Ref: 2, Kind: record.NativeMetadataOverride, Handoff: true},
					},
					unsupported,
					[]record.RefSample{{Ref: 1, T: 2, V: 2}, {Ref: 2, T: 2, V: 2}},
				}, nil, false)
				require.NoError(t, w.Close())
				opts := DefaultOptions()
				opts.EnableNativeMetadata = native
				db := newTestDB(t, withDir(dir), withOpts(opts))
				require.Nil(t, legacyMetadataForTest(db.head.series.getByID(1)))
				require.Equal(t, &metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: "a"}, legacyMetadataForTest(db.head.series.getByID(2)))
				if native {
					require.Empty(t, nativeMetadataSnapshot(t, db))
				}
				q, err := db.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				defer q.Close()
				require.Equal(t, map[string][]chunks.Sample{
					l.String():   {sample{t: 1, f: 1}, sample{t: 2, f: 2}},
					two.String(): {sample{t: 1, f: 1}, sample{t: 2, f: 2}},
				}, query(t, q, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+")))
			})
		}
	})

	t.Run("checkpoints", func(t *testing.T) {
		opts := nativeOpts()
		opts.WALSegmentSize = 32 << 10
		db := newTestDB(t, withOpts(opts))
		lsets := make([]labels.Labels, 200)
		for i := range lsets {
			lsets[i] = labels.FromStrings(labels.MetricName, "metric", "id", strconv.Itoa(i))
		}
		refs := make([]storage.SeriesRef, len(lsets))
		for step := range 60 {
			app := db.AppenderV2(t.Context())
			for i, lset := range lsets {
				ref, err := app.Append(refs[i], lset, 0, int64(step*1000), 1, nil, nil, storage.AOptions{Metadata: m(strconv.Itoa(min(step, 6) % 3))})
				require.NoError(t, err)
				refs[i] = ref
			}
			require.NoError(t, app.Commit())
		}
		want := nativeMetadataSnapshot(t, db)
		before := readNativeMetadataWAL(t, db.Head().wal.Dir()).reduce()
		require.NoError(t, db.Head().Truncate(30000))
		_, _, err := wlog.LastCheckpoint(db.Head().wal.Dir())
		require.NoError(t, err)
		after := readNativeMetadataWAL(t, db.Head().wal.Dir())
		require.Contains(t, after.types, record.NativeMetadataCompact)
		reduced := after.reduce()
		for i := range lsets {
			ref := chunks.HeadSeriesRef(refs[i])
			require.Equal(t, nativeMetadataVersions(before[ref]), nativeMetadataVersions(reduced[ref]))
			require.Equal(t, before[ref].Truncated, reduced[ref].Truncated)
			require.Equal(t, want[lsets[i].String()].Versions, nativeMetadataVersions(reduced[ref]))
		}
		db = reopen(t, db, opts)
		require.Equal(t, want, nativeMetadataSnapshot(t, db))
		for _, s := range want {
			require.Len(t, s.Versions, maxNativeMetricMetadataVersions)
			require.True(t, s.Truncated)
		}
	})
}

// TestNativeMetricMetadataReplay_Checkpointed checks that replay, the WAL's
// reduction and checkpoints of the WAL agree: entries apply in WAL order, and
// the latest declaration of a label set's refs is its current incarnation.
func TestNativeMetricMetadataReplay_Checkpointed(t *testing.T) {
	m := func(help string) metadata.Metadata {
		return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}
	}
	point := func(from int64, help string) nativeMetadataPointForTest {
		return nativeMetadataPointForTest{EffectiveFrom: from, Type: uint8(record.Counter), Unit: "seconds", Help: help}
	}
	l := labels.FromStrings(labels.MetricName, "metric")
	nativeOpts := func() *Options {
		opts := DefaultOptions()
		opts.EnableNativeMetadata = true
		opts.EnableNativeMetadataPersistence = true
		return opts
	}
	// write writes recs to a new WAL directory and returns the DB directory.
	write := func(t *testing.T, recs []any) string {
		dir := t.TempDir()
		w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(t, err)
		populateTestWL(t, w, recs, nil, false)
		require.NoError(t, w.Close())
		return dir
	}
	// checkpointed returns a copy of a DB directory whose WAL segments are
	// all checkpointed, keeping every series.
	checkpointed := func(t *testing.T, dir string) string {
		copied := t.TempDir()
		require.NoError(t, os.CopyFS(copied, os.DirFS(dir)))
		w, err := wlog.New(nil, nil, filepath.Join(copied, "wal"), compression.None)
		require.NoError(t, err)
		first, last, err := wlog.Segments(w.Dir())
		require.NoError(t, err)
		_, err = w.NextSegment()
		require.NoError(t, err)
		_, err = wlog.Checkpoint(promslog.NewNopLogger(), w, first, last, func(chunks.HeadSeriesRef) bool { return true }, math.MinInt64, false, true)
		require.NoError(t, err)
		require.NoError(t, w.Truncate(last+1))
		require.NoError(t, w.Close())
		return copied
	}
	// check requires replay of dir, and the reduction of its WAL, to give
	// ref the state want holds for it, and l the current incarnation's.
	check := func(t *testing.T, dir string, want map[chunks.HeadSeriesRef]nativemetadata.State, ref, current chunks.HeadSeriesRef) {
		t.Helper()
		reduced := readNativeMetadataWAL(t, filepath.Join(dir, "wal")).reduce()
		requireNativeMetadataEqual(t, want[ref], reduced[ref], "the WAL's reduction")
		db := newTestDB(t, withDir(dir), withOpts(nativeOpts()))
		got := nativeMetadataSnapshot(t, db)[l.String()]
		require.Equal(t, nativeMetadataVersions(want[current]), got.Versions, "replay")
		require.Equal(t, want[current].Truncated, got.Truncated, "replay")
	}

	t.Run("each pairing of entries", func(t *testing.T) {
		entries := map[string]nativeMetadataEntryForTest{
			"group":          {Ref: 1, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(200, "b"), point(300, "c")}},
			"override":       {Ref: 1, Kind: record.NativeMetadataOverride, Truncated: true, Points: []nativeMetadataPointForTest{point(50, "z"), point(250, "y")}},
			"empty override": {Ref: 1, Kind: record.NativeMetadataOverride},
			"handoff":        {Ref: 1, Kind: record.NativeMetadataOverride, Handoff: true, Points: []nativeMetadataPointForTest{point(150, "h")}},
		}
		for first, e1 := range entries {
			for second, e2 := range entries {
				recs := []any{
					[]record.RefSeries{{Ref: 1, Labels: l}},
					[]nativeMetadataEntryForTest{{Ref: 1, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(100, "a")}}},
					[]nativeMetadataEntryForTest{e1},
					[]nativeMetadataEntryForTest{e2},
					[]record.RefSample{{Ref: 1, T: 400, V: 1}},
				}
				t.Run(fmt.Sprintf("%s then %s", first, second), func(t *testing.T) {
					dir := write(t, recs)
					want := readNativeMetadataWAL(t, filepath.Join(dir, "wal")).reduce()
					cp := checkpointed(t, dir)
					check(t, dir, want, 1, 1)
					check(t, cp, want, 1, 1)
				})
			}
		}
	})

	t.Run("declarations", func(t *testing.T) {
		// Ref 1 survives; refs 2 and 3 are later incarnations of its label
		// set. Each of the last three records declares one ref: a Series
		// record ref 3, and a handoff ref 1 or ref 2. The latest
		// declaration's ref is current.
		prefix := []any{
			[]nativeMetadataEntryForTest{{Ref: 3, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(300, "c")}}},
			[]record.RefSeries{{Ref: 1, Labels: l}},
			[]nativeMetadataEntryForTest{{Ref: 1, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(100, "a")}}},
			[]record.RefSeries{{Ref: 2, Labels: l}},
			[]nativeMetadataEntryForTest{{Ref: 2, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{point(200, "b")}}},
		}
		declarations := map[string]struct {
			rec any
			ref chunks.HeadSeriesRef
		}{
			"series record":    {rec: []record.RefSeries{{Ref: 3, Labels: l}}, ref: 3},
			"handoff of ref 1": {rec: []nativeMetadataEntryForTest{{Ref: 1, Kind: record.NativeMetadataOverride, Handoff: true, Points: []nativeMetadataPointForTest{point(110, "a'")}}}, ref: 1},
			"handoff of ref 2": {rec: []nativeMetadataEntryForTest{{Ref: 2, Kind: record.NativeMetadataOverride, Handoff: true, Points: []nativeMetadataPointForTest{point(220, "b'")}}}, ref: 2},
		}
		orders := [][]string{
			{"series record", "handoff of ref 1", "handoff of ref 2"},
			{"series record", "handoff of ref 2", "handoff of ref 1"},
			{"handoff of ref 1", "series record", "handoff of ref 2"},
			{"handoff of ref 2", "series record", "handoff of ref 1"},
			{"handoff of ref 1", "handoff of ref 2", "series record"},
			{"handoff of ref 2", "handoff of ref 1", "series record"},
		}
		for _, order := range orders {
			recs := slices.Clone(prefix)
			for _, name := range order {
				recs = append(recs, declarations[name].rec)
			}
			// Samples after the last Series record keep the series.
			recs = append(recs, []record.RefSample{{Ref: 3, T: 400, V: 1}})
			current := declarations[order[2]].ref
			t.Run(strings.Join(order, ", "), func(t *testing.T) {
				dir := write(t, recs)
				want := readNativeMetadataWAL(t, filepath.Join(dir, "wal")).reduce()
				cp := checkpointed(t, dir)
				check(t, dir, want, current, current)
				check(t, cp, want, current, current)
			})
		}
	})

	t.Run("persisted mode, then legacy mode", func(t *testing.T) {
		// Legacy replay ignores compact records, and persisted mode writes no
		// Metadata records.
		db := newTestDB(t, withOpts(nativeOpts()))
		for i, help := range []string{"a", "b"} {
			app := db.AppenderV2(t.Context())
			_, err := app.Append(0, l, 0, int64(100*(i+1)), 1, nil, nil, storage.AOptions{Metadata: m(help)})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
		}
		dir := db.Dir()
		require.NoError(t, db.Close())
		db = newTestDB(t, withDir(dir), withOpts(DefaultOptions()))
		s := db.head.series.getByHash(l.Hash(), l)
		require.NotNil(t, s)
		require.Nil(t, legacyMetadataForTest(s))
	})
}

// nativeMetadataWALEntriesForTest is the string-valued construction of the
// appender's merge groups that compact records replace, for comparisons.
func nativeMetadataWALEntriesForTest(a *nativeMetricMetadataAppender) []nativeMetadataEntryForTest {
	order := make([]nativeMetricMetadataObservationRef, len(a.observations))
	for i := range order {
		order[i] = nativeMetricMetadataObservationRef(i + 1)
	}
	slices.SortStableFunc(order, func(x, y nativeMetricMetadataObservationRef) int {
		left, right := a.observations[x-1], a.observations[y-1]
		if c := cmpForTest(left.series.ref, right.series.ref); c != 0 {
			return c
		}
		return cmpForTest(left.effectiveFrom, right.effectiveFrom)
	})
	var entries []nativeMetadataEntryForTest
	for position := 0; position < len(order); {
		end := position + 1
		series := a.observations[order[position]-1].series
		for end < len(order) && a.observations[order[end]-1].series.ref == series.ref {
			end++
		}
		series.Lock()
		stable := nativeMetricMetadataGroupStable(series.nativeMetadataLocked(), a, order[position:end])
		series.Unlock()
		if !stable {
			entry := nativeMetadataEntryForTest{Ref: series.ref, Kind: record.NativeMetadataGroup}
			for _, observationRef := range order[position:end] {
				observation := a.observations[observationRef-1]
				m := a.metadataValue(observation.metadataRef)
				point := nativeMetadataPointForTest{EffectiveFrom: observation.effectiveFrom, Type: record.GetMetricType(m.Type), Unit: m.Unit, Help: m.Help}
				if last := len(entry.Points) - 1; last >= 0 && entry.Points[last].EffectiveFrom == point.EffectiveFrom {
					entry.Points[last] = point
				} else {
					entry.Points = append(entry.Points, point)
				}
			}
			entries = append(entries, entry)
		}
		position = end
	}
	return entries
}

// normalizeNativeMetadataForTest clears capacity differences for comparisons.
func normalizeNativeMetadataForTest(entries []nativeMetadataEntryForTest) []nativeMetadataEntryForTest {
	out := make([]nativeMetadataEntryForTest, 0, len(entries))
	for _, e := range entries {
		e.Points = slices.Clip(e.Points)
		out = append(out, e)
	}
	return out
}

type decisionSampleForTest struct {
	series int
	t      int64
	m      metadata.Metadata
}

type decisionStateForTest struct {
	value metadata.Metadata
	from  int64
}

// decisionReferenceForTest is a transaction-local value reference: a
// one-based table index, or a negative one-based direct index.
type decisionReferenceForTest int

func decisionReferenceForTestOf(ref nativeMetricMetadataValueRef) decisionReferenceForTest {
	if ref&nativeMetricMetadataDirectRefMask != 0 {
		return -decisionReferenceForTest(ref&^nativeMetricMetadataDirectRefMask) - 1
	}
	return decisionReferenceForTest(ref)
}

type decisionObservationForTest struct {
	ref       chunks.HeadSeriesRef
	from      int64
	reference decisionReferenceForTest
}

type decisionStatsForTest struct {
	direct, reusedDirect, duplicates, induced, inducedLogged, backdated, replacedByDirect int
}

type decisionResultForTest struct {
	observations []decisionObservationForTest
	table        []metadata.Metadata
	direct       int
	record       []byte
	stats        decisionStatsForTest
}

// decisionModelForTest models one transaction from its samples in append
// order, their series' refs and the committed versions, without the
// appender. A sample is observed if its series has no committed version, if
// its metadata differs from it, or if it predates its start; or, within the
// transaction, with multiple pending samples for its series, if its series was
// its stripe's first observation, or once its
// stripe holds observations of two or more series. An observation equal to the
// previous one takes its reference; otherwise a value takes its reference in
// a content-keyed table of up to 128 values, and beyond it each observation
// takes a new direct reference. Groups follow stripes in order of their first
// observation, then refs; points follow starts, then observation order, and
// an equal start keeps the last point. A series with a committed version
// whose observations all equal it, at or after its start, logs nothing. The
// record defines each reference's value at its first use.
func decisionModelForTest(samples []decisionSampleForTest, refs []chunks.HeadSeriesRef, committed map[chunks.HeadSeriesRef]decisionStateForTest) decisionResultForTest {
	const stripes, tableCap = 256, 128
	var out decisionResultForTest
	type stripeState struct {
		first chunks.HeadSeriesRef
		multi bool
	}
	type observation struct {
		decisionObservationForTest
		value   metadata.Metadata
		induced bool
	}
	var observations []observation
	stripeOf := map[uint64]*stripeState{}
	table := map[metadata.Metadata]decisionReferenceForTest{}
	var last observation
	haveLast := false
	pending := map[chunks.HeadSeriesRef]int{}
	for _, s := range samples {
		ref := refs[s.series]
		pending[ref]++
		stripe := uint64(ref) % stripes
		state := stripeOf[stripe]
		mayHave := pending[ref] > 1 && state != nil && (state.first == ref || state.multi)
		c, ok := committed[ref]
		changed := !ok || c.value != s.m || s.t < c.from
		if !mayHave && !changed {
			continue
		}
		o := observation{decisionObservationForTest: decisionObservationForTest{ref: ref, from: s.t}, value: s.m, induced: !changed}
		if ok && s.t < c.from {
			out.stats.backdated++
		}
		switch r, inTable := table[s.m]; {
		case haveLast && last.value == s.m:
			o.reference = last.reference
			if o.reference < 0 {
				out.stats.reusedDirect++
			}
		case inTable:
			o.reference = r
		case len(table) < tableCap:
			o.reference = decisionReferenceForTest(len(table) + 1)
			table[s.m] = o.reference
			out.table = append(out.table, s.m)
		default:
			out.direct++
			o.reference = -decisionReferenceForTest(out.direct)
		}
		if o.induced {
			out.stats.induced++
		}
		if state == nil {
			stripeOf[stripe] = &stripeState{first: ref}
		} else if state.first != ref {
			state.multi = true
		}
		observations = append(observations, o)
		out.observations = append(out.observations, o.decisionObservationForTest)
		last, haveLast = o, true
	}
	out.stats.direct = out.direct

	type point struct {
		from      int64
		value     metadata.Metadata
		reference decisionReferenceForTest
	}
	type entry struct {
		ref    chunks.HeadSeriesRef
		points []point
	}
	var entries []entry
	group := make([]int, len(observations))
	for i := range group {
		group[i] = i
	}
	slices.SortStableFunc(group, func(x, y int) int {
		if observations[x].ref != observations[y].ref {
			return cmpForTest(observations[x].ref, observations[y].ref)
		}
		return cmpForTest(observations[x].from, observations[y].from)
	})
	for position := 0; position < len(group); {
		end := position + 1
		for end < len(group) && observations[group[end]].ref == observations[group[position]].ref {
			end++
		}
		run := group[position:end]
		position = end
		c, ok := committed[observations[run[0]].ref]
		stable := ok
		for _, i := range run {
			if o := observations[i]; !ok || o.value != c.value || o.from < c.from {
				stable = false
			}
		}
		if stable {
			continue
		}
		e := entry{ref: observations[run[0]].ref}
		for _, i := range run {
			o := observations[i]
			p := point{from: o.from, value: o.value, reference: o.reference}
			if n := len(e.points); n > 0 && e.points[n-1].from == o.from {
				if p.reference < 0 && e.points[n-1].reference > 0 {
					out.stats.replacedByDirect++
				}
				e.points[n-1] = p
			} else {
				e.points = append(e.points, p)
			}
			if o.induced {
				out.stats.inducedLogged++
			}
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return out
	}
	var base int64
	for i, e := range entries {
		for j, p := range e.points {
			if (i == 0 && j == 0) || p.from < base {
				base = p.from
			}
		}
	}
	b := []byte{byte(record.NativeMetadataCompact), 1}
	b = binary.AppendVarint(b, base)
	b = binary.AppendUvarint(b, uint64(len(entries)))
	indices := map[decisionReferenceForTest]uint64{}
	defined := map[metadata.Metadata]bool{}
	var previous chunks.HeadSeriesRef
	for _, e := range entries {
		b = append(b, 0) // A group.
		b = binary.AppendVarint(b, int64(e.ref-previous))
		previous = e.ref
		b = binary.AppendUvarint(b, uint64(len(e.points)))
		for _, p := range e.points {
			b = binary.AppendUvarint(b, uint64(p.from)-uint64(base))
			if index, ok := indices[p.reference]; ok {
				b = binary.AppendUvarint(b, index+1)
				continue
			}
			indices[p.reference] = uint64(len(indices))
			if defined[p.value] {
				out.stats.duplicates++
			}
			defined[p.value] = true
			b = append(b, 0, record.GetMetricType(p.value.Type))
			b = binary.AppendUvarint(b, uint64(len(p.value.Unit)))
			b = append(b, p.value.Unit...)
			b = binary.AppendUvarint(b, uint64(len(p.value.Help)))
			b = append(b, p.value.Help...)
		}
	}
	out.record = b
	return out
}

func cmpForTest[T chunks.HeadSeriesRef | int64](x, y T) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// compactWALRecordsForTest returns the raw compact native metadata records in a
// WAL directory's segments, in WAL order.
func compactWALRecordsForTest(t testing.TB, dir string) [][]byte {
	t.Helper()
	sr, err := wlog.NewSegmentsRangeReader(wlog.SegmentRange{Dir: dir, Last: math.MaxInt32})
	require.NoError(t, err)
	defer sr.Close()
	var out [][]byte
	var dec record.Decoder
	r := wlog.NewReader(sr)
	for r.Next() {
		if dec.Type(r.Record()) == record.NativeMetadataCompact {
			out = append(out, slices.Clone(r.Record()))
		}
	}
	require.NoError(t, r.Err())
	return out
}
