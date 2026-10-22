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

package nativemetadata

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/record"
)

func TestStateApply(t *testing.T) {
	for _, c := range []struct {
		name      string
		start     string
		kind      record.NativeMetadataKind
		truncated bool
		points    string
		want      string
		truncate  bool
	}{
		{name: "group", start: "A@100", kind: record.NativeMetadataGroup, points: "B@50 C@150", want: "B@50 A@100 C@150"},
		{name: "evicting group", start: "A@10 B@20 C@30 D@40 E@50", kind: record.NativeMetadataGroup, points: "F@60", want: "B@20 C@30 D@40 E@50 F@60", truncate: true},
		{name: "override", start: "A@10 B@20", kind: record.NativeMetadataOverride, truncated: true, points: "C@5 D@15", want: "C@5 D@15", truncate: true},
		{name: "empty override", start: "A@10", kind: record.NativeMetadataOverride, want: ""},
		{name: "legacy", start: "A@10 B@20", kind: record.NativeMetadataLegacy, points: fmt.Sprintf("C@%d", int64(math.MinInt64)), want: fmt.Sprintf("C@%d", int64(math.MinInt64))},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := State{History: history(c.start)}
			s.Apply(c.kind, c.truncated, points(c.points))
			require.Equal(t, c.want, format(&s.History))
			require.Equal(t, c.truncate, s.Truncated)
		})
	}
	t.Run("legacy entries keep the truncation flag", func(t *testing.T) {
		s := State{History: history("A@10"), Truncated: true}
		s.Apply(record.NativeMetadataLegacy, false, points("B@1"))
		require.True(t, s.Truncated)
	})
	t.Run("legacy points start at the unknown start", func(t *testing.T) {
		intern := func(m metadata.Metadata) *metadata.Metadata { return &m }
		p := LegacyPoint(record.RefMetadata{Ref: 3, Type: uint8(record.Counter), Unit: "seconds", Help: "h"}, intern)
		require.Equal(t, int64(math.MinInt64), p.EffectiveFrom)
		require.Equal(t, metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: "h"}, *p.Metadata)
	})
}

// TestStateApplyRecords checks that reducing encoded WAL entries reproduces
// native storage's merge: groups apply as one Merge call each, and an override
// written from a reduced state restores it, including the truncation flag.
func TestStateApplyRecords(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	helps := []string{"a", "b", "c", "d"}
	intern := func(m metadata.Metadata) *metadata.Metadata { return &m }
	randomGroup := func() []Point {
		var group []Point
		for from := int64(rng.IntN(5)); from < 60 && len(group) < 8; from += 1 + int64(rng.IntN(12)) {
			group = append(group, Point{EffectiveFrom: from, Metadata: value(helps[rng.IntN(len(helps))])})
		}
		return group
	}
	var enc record.Encoder
	var dec record.Decoder
	var decoded record.CompactNativeMetadata
	var dictionary Dictionary
	// compact round-trips an entry through a compact record, one value per point.
	compact := func(kind record.NativeMetadataKind, truncated bool, points []Point) (record.RefCompactNativeMetadata, []Point) {
		var values []record.NativeMetadataValue
		entry := record.RefCompactNativeMetadata{Ref: 1, Kind: kind, Truncated: truncated}
		for _, p := range points {
			entry.Points = append(entry.Points, record.CompactNativeMetadataPoint{EffectiveFrom: p.EffectiveFrom, Value: uint32(len(values))})
			values = append(values, record.NativeMetadataValue{Type: record.GetMetricType(p.Metadata.Type), Unit: p.Metadata.Unit, Help: p.Metadata.Help})
		}
		require.NoError(t, dec.CompactNativeMetadata(enc.CompactNativeMetadata(values, []record.RefCompactNativeMetadata{entry}, nil), &decoded))
		require.Len(t, decoded.Entries, 1)
		dictionary.Reset(decoded.Values, intern)
		return decoded.Entries[0], dictionary.AppendPoints(nil, decoded.Entries[0].Points)
	}
	for i := range 20000 {
		var native, reduced State
		for range 1 + rng.IntN(8) {
			group := randomGroup()
			if _, evictions := native.Merge(group); evictions > 0 {
				native.Truncated = true
			}
			entry, points := compact(record.NativeMetadataGroup, false, group)
			reduced.Apply(entry.Kind, entry.Truncated, points)
			require.Equal(t, format(&native.History), format(&reduced.History), "iteration %d", i)
			require.Equal(t, native.Truncated, reduced.Truncated, "iteration %d", i)
		}
		override, points := compact(record.NativeMetadataOverride, reduced.Truncated, reduced.AppendPoints(nil))
		restored := State{History: history("Z@-100 Y@-50"), Truncated: !reduced.Truncated}
		restored.Apply(override.Kind, override.Truncated, points)
		require.Equal(t, format(&native.History), format(&restored.History), "iteration %d", i)
		require.Equal(t, native.Truncated, restored.Truncated, "iteration %d", i)
		require.True(t, slices.IsSortedFunc(restored.AppendPoints(nil), func(a, b Point) int { return int(a.EffectiveFrom - b.EffectiveFrom) }))
	}
}

func TestTrust(t *testing.T) {
	var zero Trust
	require.True(t, zero.Trusted(), "without a checkpoint, a reader starts trusted")
	for _, c := range []struct {
		name  string
		steps func(*Trust)
		want  bool
	}{
		{name: "an unmarked checkpoint", steps: func(tr *Trust) { tr.StartCheckpoint() }, want: false},
		{name: "a marked checkpoint", steps: func(tr *Trust) { tr.StartCheckpoint(); tr.Mark() }, want: true},
		{name: "a boundary after an unmarked checkpoint", steps: func(tr *Trust) { tr.StartCheckpoint(); tr.Mark() }, want: true},
		{name: "a marker while trusted", steps: func(tr *Trust) { tr.Mark() }, want: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var tr Trust
			c.steps(&tr)
			require.Equal(t, c.want, tr.Trusted())
		})
	}
}

func TestDictionary(t *testing.T) {
	values := []record.NativeMetadataValue{{Type: uint8(record.Counter), Unit: "u", Help: "a"}, {Help: "b"}, {Help: "unused"}}
	var calls []metadata.Metadata
	intern := func(m metadata.Metadata) *metadata.Metadata {
		calls = append(calls, m)
		return &m
	}
	point := func(from int64, value uint32) record.CompactNativeMetadataPoint {
		return record.CompactNativeMetadataPoint{EffectiveFrom: from, Value: value}
	}
	var d Dictionary
	d.Reset(values, intern)
	points := d.AppendPoints(nil, []record.CompactNativeMetadataPoint{point(1, 1), point(2, 0), point(3, 1)})
	more := d.AppendPoints(points[:0:0], []record.CompactNativeMetadataPoint{point(4, 0)})
	// Each used value is resolved once, on first use; unused values never are.
	require.Equal(t, []metadata.Metadata{{Type: model.MetricTypeUnknown, Help: "b"}, {Type: model.MetricTypeCounter, Unit: "u", Help: "a"}}, calls)
	require.Equal(t, []int64{1, 2, 3}, []int64{points[0].EffectiveFrom, points[1].EffectiveFrom, points[2].EffectiveFrom})
	require.Same(t, points[0].Metadata, points[2].Metadata)
	require.Same(t, points[1].Metadata, more[0].Metadata)
	require.Equal(t, int64(4), more[0].EffectiveFrom)

	t.Run("reset resolves the next record's values anew", func(t *testing.T) {
		calls = nil
		d.Reset(values[1:], intern)
		again := d.AppendPoints(nil, []record.CompactNativeMetadataPoint{point(5, 0)})
		require.Equal(t, []metadata.Metadata{{Type: model.MetricTypeUnknown, Help: "b"}}, calls)
		require.NotSame(t, points[0].Metadata, again[0].Metadata)
	})

	t.Run("reset releases resolved values", func(t *testing.T) {
		d.Reset(nil, nil)
		require.Nil(t, d.values)
		require.Empty(t, slices.DeleteFunc(slices.Clone(d.resolved[:cap(d.resolved)]), func(v *metadata.Metadata) bool { return v == nil }))
	})
}
