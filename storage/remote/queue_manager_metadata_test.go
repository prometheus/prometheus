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

package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
	"unique"
	"unsafe"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
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

func nativeMetadataPoint(from int64, m metadata.Metadata) nativeMetadataPointForTest {
	return nativeMetadataPointForTest{EffectiveFrom: from, Type: record.GetMetricType(m.Type), Unit: m.Unit, Help: m.Help}
}

func nativeMetadataEntry(ref chunks.HeadSeriesRef, kind record.NativeMetadataKind, points ...nativeMetadataPointForTest) []nativeMetadataEntryForTest {
	return []nativeMetadataEntryForTest{{Ref: ref, Kind: kind, Points: points}}
}

// compactNativeMetadataRecordForTest encodes entries, which must be groups or
// overrides, as a compact record whose dictionary holds each distinct value
// once, in order of first use.
func compactNativeMetadataRecordForTest(entries []nativeMetadataEntryForTest) []byte {
	var values []record.NativeMetadataValue
	var compact []record.RefCompactNativeMetadata
	indices := map[record.NativeMetadataValue]uint32{}
	for _, e := range entries {
		c := record.RefCompactNativeMetadata{Ref: e.Ref, Kind: e.Kind, Truncated: e.Truncated, Handoff: e.Handoff}
		for _, p := range e.Points {
			v := record.NativeMetadataValue{Type: p.Type, Unit: p.Unit, Help: p.Help}
			index, ok := indices[v]
			if !ok {
				index = uint32(len(values))
				indices[v] = index
				values = append(values, v)
			}
			c.Points = append(c.Points, record.CompactNativeMetadataPoint{EffectiveFrom: p.EffectiveFrom, Value: index})
		}
		compact = append(compact, c)
	}
	var enc record.Encoder
	return enc.CompactNativeMetadata(values, compact, nil)
}

// storeNativeMetadataForTest passes w the compact record of entries.
func storeNativeMetadataForTest(t testing.TB, w nativeMetadataWriter, entries []nativeMetadataEntryForTest, segment int) {
	t.Helper()
	storeNativeMetadataRecordForTest(t, w, compactNativeMetadataRecordForTest(entries), segment)
}

// storeNativeMetadataRecordForTest passes w a compact record decoded as the
// watcher decodes it, borrowing from its buffer, then overwrites the buffer,
// as the watcher's next read does.
func storeNativeMetadataRecordForTest(t testing.TB, w nativeMetadataWriter, rec []byte, segment int) {
	t.Helper()
	buf := slices.Clone(rec)
	var dec record.Decoder
	var compact record.CompactNativeMetadata
	require.NoError(t, dec.CompactNativeMetadataBorrowed(buf, &compact))
	w.StoreNativeMetadataRecord(&compact, segment)
	for i := range buf {
		buf[i] = 'x'
	}
}

// decodeNativeMetadataRecordForTest decodes the entries of a Metadata record,
// as legacy entries, or of a compact record.
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
	for _, e := range compact.Entries {
		entry := nativeMetadataEntryForTest{Ref: e.Ref, Kind: e.Kind, Truncated: e.Truncated, Handoff: e.Handoff}
		for _, p := range e.Points {
			v := compact.Values[p.Value]
			entry.Points = append(entry.Points, nativeMetadataPointForTest{EffectiveFrom: p.EffectiveFrom, Type: v.Type, Unit: v.Unit, Help: v.Help})
		}
		entries = append(entries, entry)
	}
	return entries
}

// nativeMetadataPointsForTest returns points with their values resolved.
func nativeMetadataPointsForTest(points []nativeMetadataPointForTest) []nativemetadata.Point {
	var out []nativemetadata.Point
	for _, p := range points {
		m := metadata.Metadata{Type: record.ToMetricType(p.Type), Unit: p.Unit, Help: p.Help}
		out = append(out, nativemetadata.Point{EffectiveFrom: p.EffectiveFrom, Metadata: &m})
	}
	return out
}

// isNativeMetadataRecordForTest reports whether rec is a compact record or a
// Metadata record.
func isNativeMetadataRecordForTest(rec []byte) bool {
	typ := record.Type(rec[0])
	return typ == record.Metadata || typ == record.NativeMetadataCompact
}

// queuedMetadata returns the values of the queued items' metadata, nil for none.
func queuedMetadata(batch []timeSeries) []*metadata.Metadata {
	out := make([]*metadata.Metadata, len(batch))
	for i, s := range batch {
		if s.metadata != (unique.Handle[metadata.Metadata]{}) {
			m := s.metadata.Value()
			out[i] = &m
		}
	}
	return out
}

// newNativeTestQueueManager returns a remote write 2.0 queue that forwards
// native metadata.
func newNativeTestQueueManager(t testing.TB, cfg config.QueueConfig, c WriteClient) *QueueManager {
	metrics := newQueueManagerMetrics(nil, "", "")
	return NewQueueManager(metrics, nil, nil, nil, t.TempDir(), newEWMARate(ewmaWeight, shardUpdateDuration), cfg, config.DefaultMetadataConfig, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false, false, remoteapi.WriteV2MessageType, record.NewBuffersPool(), false, false, true)
}

func TestQueueManagerNativeMetadata(t *testing.T) {
	m := func(help string) metadata.Metadata {
		return metadata.Metadata{Type: model.MetricTypeCounter, Unit: "seconds", Help: help}
	}
	a, b, c := m("a"), m("b"), m("c")
	series := []record.RefSeries{{Ref: 1, Labels: labels.FromStrings(labels.MetricName, "metric")}}
	newQM := func(t *testing.T) (*QueueManager, nativeMetadataWriter, *queue) {
		qm := newNativeTestQueueManager(t, testDefaultQueueConfig(), NewNopWriteClient())
		qm.sendExemplars, qm.sendNativeHistograms = true, true
		qm.StoreSeries(series, 0)
		// Inspect the queued values before a sender can consume them.
		q := newQueue(64, 64)
		qm.shards.queues = []*queue{q}
		return qm, nativeMetadataWriter{qm}, q
	}

	t.Run("items are labelled at their own timestamps", func(t *testing.T) {
		timestamps := []int64{50, 100, 150, 200, 250, 300, 1000, 120}
		want := []*metadata.Metadata{nil, &a, &a, &b, &b, &c, &c, &a}
		for _, kind := range []string{"samples", "exemplars", "histograms", "float histograms"} {
			t.Run(kind, func(t *testing.T) {
				qm, w, q := newQM(t)
				// One transaction carries A@100 and B@200, a later one C@300.
				storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(100, a), nativeMetadataPoint(200, b)), 0)
				storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(300, c)), 0)
				for _, ts := range timestamps {
					var ok bool
					switch kind {
					case "samples":
						ok = qm.Append([]record.RefSample{{Ref: 1, T: ts}})
					case "exemplars":
						ok = qm.AppendExemplars([]record.RefExemplar{{Ref: 1, T: ts}})
					case "histograms":
						ok = qm.AppendHistograms([]record.RefHistogramSample{{Ref: 1, T: ts, H: &histogram.Histogram{}}})
					case "float histograms":
						ok = qm.AppendFloatHistograms([]record.RefFloatHistogramSample{{Ref: 1, T: ts, FH: &histogram.FloatHistogram{}}})
					}
					require.True(t, ok)
				}
				require.Equal(t, want, queuedMetadata(q.batch))
			})
		}
	})

	t.Run("entry kinds", func(t *testing.T) {
		qm, w, q := newQM(t)
		w.StoreLegacyNativeMetadata([]record.RefMetadata{{Ref: 1, Type: record.GetMetricType(a.Type), Unit: a.Unit, Help: a.Help}}, 0)
		require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: math.MinInt64 + 1}}))
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataOverride, nativeMetadataPoint(100, b)), 0)
		require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 99}, {Ref: 1, T: 100}}))
		// A handoff is applied as an override.
		storeNativeMetadataForTest(t, w, []nativeMetadataEntryForTest{{Ref: 1, Kind: record.NativeMetadataOverride, Handoff: true, Points: []nativeMetadataPointForTest{nativeMetadataPoint(200, c)}}}, 0)
		require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 150}, {Ref: 1, T: 200}}))
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataOverride), 0)
		require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 300}}))
		// Legacy metadata is ignored when forwarding native metadata.
		qm.StoreMetadata([]record.RefMetadata{{Ref: 1, Type: record.GetMetricType(c.Type), Help: "legacy"}})
		require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 400}}))
		require.Equal(t, []*metadata.Metadata{&a, nil, &b, nil, &c, nil, nil}, queuedMetadata(q.batch))
	})

	t.Run("trust", func(t *testing.T) {
		// After a checkpoint, compact entries are skipped and counted until a
		// provenance marker or a boundary; legacy entries always apply.
		var enc record.Encoder
		for _, mark := range []record.NativeMetadataKind{record.NativeMetadataProvenance, record.NativeMetadataBoundary} {
			qm, w, q := newQM(t)
			w.ResetNativeMetadata(true)
			storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(100, a)), 0)
			require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 100}}))
			w.StoreLegacyNativeMetadata([]record.RefMetadata{{Ref: 1, Type: record.GetMetricType(b.Type), Unit: b.Unit, Help: b.Help}}, 0)
			require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 100}}))
			require.Equal(t, 1.0, client_testutil.ToFloat64(qm.metrics.untrustedMetadataTotal))
			storeNativeMetadataRecordForTest(t, w, enc.NativeMetadataMarker(mark, nil), 0)
			storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(200, c)), 0)
			require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 200}}))
			require.Equal(t, 1.0, client_testutil.ToFloat64(qm.metrics.untrustedMetadataTotal))
			require.Equal(t, []*metadata.Metadata{nil, &b, &c}, queuedMetadata(q.batch), "mark %d", mark)
			// A reset without a checkpoint is trusted.
			w.ResetNativeMetadata(false)
			storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(100, a)), 0)
			require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 100}}))
			require.Equal(t, &a, queuedMetadata(q.batch)[3])
		}
	})

	t.Run("resets and series garbage collection", func(t *testing.T) {
		qm, w, q := newQM(t)
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(100, a)), 0)
		// Entries may precede their series record.
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(2, record.NativeMetadataGroup, nativeMetadataPoint(100, b)), 0)
		require.Equal(t, 2.0, client_testutil.ToFloat64(qm.metrics.nativeMetadataSeries))
		qm.StoreSeries([]record.RefSeries{{Ref: 2, Labels: labels.FromStrings(labels.MetricName, "other")}}, 1)
		require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 100}, {Ref: 2, T: 100}}))
		qm.SeriesReset(1)
		require.NotContains(t, qm.seriesNativeMetadata, chunks.HeadSeriesRef(1))
		require.Contains(t, qm.seriesNativeMetadata, chunks.HeadSeriesRef(2))
		require.Equal(t, 1.0, client_testutil.ToFloat64(qm.metrics.nativeMetadataSeries))
		w.ResetNativeMetadata(false)
		require.Empty(t, qm.seriesNativeMetadata)
		require.Zero(t, client_testutil.ToFloat64(qm.metrics.nativeMetadataSeries))
		require.True(t, qm.Append([]record.RefSample{{Ref: 2, T: 100}}))
		require.Equal(t, []*metadata.Metadata{&a, &b, nil}, queuedMetadata(q.batch))
	})

	t.Run("state without a series record", func(t *testing.T) {
		// The watcher's calls in order. A native state is dropped by the
		// reset of checkpoint N only if no series record is at or after N,
		// and neither its latest entry nor a checkpoint that kept it is.
		retired, raced, later := chunks.HeadSeriesRef(3), chunks.HeadSeriesRef(7), chunks.HeadSeriesRef(9)
		qm, w, _ := newQM(t)
		qm.StoreSeries([]record.RefSeries{{Ref: retired, Labels: labels.FromStrings(labels.MetricName, "retired")}}, 2)
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(retired, record.NativeMetadataGroup, nativeMetadataPoint(100, a)), 2)
		// Ref 7's creator published it, and another appender committed
		// metadata, before its series record was logged.
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(raced, record.NativeMetadataGroup, nativeMetadataPoint(100, b)), 4)
		// Ref 9's latest entry is beyond the checkpoint.
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(later, record.NativeMetadataGroup, nativeMetadataPoint(100, c)), 6)

		// Checkpoint 5 keeps ref 7's metadata, since ref 7 is live in the
		// Head, and drops ref 3, retired.
		w.UpdateNativeMetadataSegment([]chunks.HeadSeriesRef{raced}, 5)
		qm.SeriesReset(5)
		require.NotContains(t, qm.seriesNativeMetadata, retired)
		require.Contains(t, qm.seriesNativeMetadata, raced)
		require.Contains(t, qm.seriesNativeMetadata, later)

		// Ref 3's repair, written before its retirement in segment 7, is
		// read only now, after the reset. It recreates ref 3's state, which
		// no series record will ever cover: an orphan.
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(retired, record.NativeMetadataOverride, nativeMetadataPoint(100, a)), 7)
		qm.StoreSeries([]record.RefSeries{{Ref: raced, Labels: labels.FromStrings(labels.MetricName, "raced")}}, 8)
		// A checkpoint before the orphan's segment keeps it, but drops ref 9.
		qm.SeriesReset(7)
		require.Contains(t, qm.seriesNativeMetadata, retired)
		require.Contains(t, qm.seriesNativeMetadata, raced)
		require.NotContains(t, qm.seriesNativeMetadata, later)
		// A checkpoint at or beyond it, not keeping ref 3, removes it; ref 7
		// has its series record.
		qm.SeriesReset(8)
		require.NotContains(t, qm.seriesNativeMetadata, retired)
		require.Contains(t, qm.seriesNativeMetadata, raced)
	})

	t.Run("queues share values", func(t *testing.T) {
		_, first, _ := newQM(t)
		_, second, _ := newQM(t)
		help := "shared " + t.Name()
		for _, w := range []nativeMetadataWriter{first, second} {
			storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(1, m(strings.Clone(help)))), 0)
		}
		require.Equal(t, first.seriesNativeMetadata[1].handles, second.seriesNativeMetadata[1].handles)
		require.Same(t, unsafe.StringData(first.seriesNativeMetadata[1].Metadata.Help), unsafe.StringData(second.seriesNativeMetadata[1].Metadata.Help))
	})

	t.Run("deliveries from a WAL", func(t *testing.T) {
		// Compact records alternate with Metadata records, as after a switch
		// from metadata WAL records. Every queued sample must carry the
		// WAL-prefix oracle's metadata.
		lsets := map[chunks.HeadSeriesRef]labels.Labels{
			1: series[0].Labels,
			2: labels.FromStrings(labels.MetricName, "two"),
			3: labels.FromStrings(labels.MetricName, "three"),
		}
		entry := func(ref chunks.HeadSeriesRef, kind record.NativeMetadataKind, points ...nativeMetadataPointForTest) nativeMetadataEntryForTest {
			return nativeMetadataEntryForTest{Ref: ref, Kind: kind, Points: points}
		}
		handoff := entry(3, record.NativeMetadataOverride, nativeMetadataPoint(5, c))
		handoff.Handoff, handoff.Truncated = true, true
		legacy := func(ref chunks.HeadSeriesRef, m metadata.Metadata) []record.RefMetadata {
			return []record.RefMetadata{{Ref: ref, Type: record.GetMetricType(m.Type), Unit: m.Unit, Help: m.Help}}
		}
		var enc record.Encoder
		recs := [][]byte{
			enc.Series([]record.RefSeries{{Ref: 1, Labels: lsets[1]}, {Ref: 2, Labels: lsets[2]}, {Ref: 3, Labels: lsets[3]}}, nil),
			enc.Metadata(legacy(1, c), nil),
			compactNativeMetadataRecordForTest([]nativeMetadataEntryForTest{entry(1, record.NativeMetadataGroup, nativeMetadataPoint(10, a)), entry(2, record.NativeMetadataGroup, nativeMetadataPoint(10, b), nativeMetadataPoint(30, a))}),
			compactNativeMetadataRecordForTest([]nativeMetadataEntryForTest{entry(1, record.NativeMetadataGroup, nativeMetadataPoint(20, b)), entry(3, record.NativeMetadataGroup, nativeMetadataPoint(15, a))}),
			enc.Metadata(legacy(2, a), nil),
			compactNativeMetadataRecordForTest([]nativeMetadataEntryForTest{entry(2, record.NativeMetadataOverride, nativeMetadataPoint(25, c)), handoff}),
			compactNativeMetadataRecordForTest([]nativeMetadataEntryForTest{entry(1, record.NativeMetadataOverride)}),
			compactNativeMetadataRecordForTest([]nativeMetadataEntryForTest{entry(1, record.NativeMetadataGroup, nativeMetadataPoint(40, c)), entry(2, record.NativeMetadataGroup, nativeMetadataPoint(5, a))}),
		}
		var withSamples [][]byte
		for i, rec := range recs {
			withSamples = append(withSamples, rec)
			var samples []record.RefSample
			for ref := chunks.HeadSeriesRef(1); ref <= 3; ref++ {
				for _, ts := range []int64{0, 12, 22, 35, 45} {
					samples = append(samples, record.RefSample{Ref: ref, T: ts + int64(i)})
				}
			}
			withSamples = append(withSamples, enc.Samples(samples, nil))
		}
		dir := t.TempDir()
		wl, err := wlog.NewSize(nil, nil, dir, wlog.DefaultSegmentSize, compression.None)
		require.NoError(t, err)
		require.NoError(t, wl.Log(withSamples...))
		require.NoError(t, wl.Close())
		expected := nativeMetadataWALPrefixOracle(t, dir)

		qm, w, _ := newQM(t)
		qm.StoreSeries([]record.RefSeries{{Ref: 2, Labels: lsets[2]}, {Ref: 3, Labels: lsets[3]}}, 0)
		var dec record.Decoder
		delivered := 0
		for _, rec := range withSamples {
			switch dec.Type(rec) {
			case record.Metadata:
				meta, err := dec.Metadata(rec, nil)
				require.NoError(t, err)
				w.StoreLegacyNativeMetadata(meta, 0)
			case record.NativeMetadataCompact:
				storeNativeMetadataRecordForTest(t, w, rec, 0)
			case record.Samples:
				samples, err := dec.Samples(rec, nil)
				require.NoError(t, err)
				// Room for every sample in one batch, kept in the queue.
				q := newQueue(len(samples)+1, len(samples)+1)
				qm.shards.queues = []*queue{q}
				require.True(t, qm.Append(samples))
				for _, item := range q.batch {
					m := metadata.Metadata{Type: model.MetricTypeUnknown}
					if item.metadata != (unique.Handle[metadata.Metadata]{}) {
						m = item.metadata.Value()
					}
					d := nativeMetadataDelivery(item.seriesLabels, item.timestamp, m)
					require.True(t, expected[d], "unexpected delivery %s", d)
					delivered++
				}
			}
		}
		require.Equal(t, len(recs)*3*5, delivered)
	})

	t.Run("unknown series and old samples are filtered", func(t *testing.T) {
		qm, w, q := newQM(t)
		storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(0, a)), 0)
		require.True(t, qm.Append([]record.RefSample{{Ref: 2, T: 1}}))
		qm.cfg.SampleAgeLimit = model.Duration(time.Hour)
		require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 1}}))
		require.Empty(t, q.batch)
	})

	t.Run("protocols and destinations", func(t *testing.T) {
		for _, native := range []bool{false, true} {
			s := NewWriteStorage(nil, nil, t.TempDir(), defaultFlushDeadline, nil, false, native)
			rw1, rw2 := baseRemoteWriteConfig("http://rw1.test"), baseRemoteWriteConfig("http://rw2.test")
			rw1.ProtobufMessage, rw2.ProtobufMessage = remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType
			rw1.MetadataConfig.Send, rw2.MetadataConfig.Send = false, false
			cfg := &config.Config{RemoteWriteConfigs: []*config.RemoteWriteConfig{rw1, rw2}}
			require.NoError(t, s.ApplyConfig(cfg))
			check := func() {
				for _, qm := range s.queues {
					want := native && qm.protoMsg == remoteapi.WriteV2MessageType
					require.Equal(t, want, qm.nativeMetadata)
				}
			}
			check()
			rw1.ProtobufMessage, rw2.ProtobufMessage = rw2.ProtobufMessage, rw1.ProtobufMessage
			require.NoError(t, s.ApplyConfig(cfg))
			require.Len(t, s.queues, 2)
			check()
			require.NoError(t, s.Close())
		}
	})
}

func TestQueueManagerNativeMetadataRetryAndReshard(t *testing.T) {
	old := metadata.Metadata{Type: model.MetricTypeCounter, Help: "old"}
	newest := metadata.Metadata{Type: model.MetricTypeCounter, Help: "new"}
	var w nativeMetadataWriter
	requests := make(chan []byte, 3)
	attempts := 0
	client := &MockWriteClient{
		NameFunc: func() string { return "test" }, EndpointFunc: func() string { return "http://test" },
		StoreFunc: func(_ context.Context, req []byte, _ int) (WriteResponseStats, error) {
			requests <- bytes.Clone(req)
			attempts++
			if attempts == 1 {
				// Later WAL entries must not change queued items.
				storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataOverride, nativeMetadataPoint(0, newest)), 0)
				return WriteResponseStats{}, RecoverableError{errors.New("retry"), 0}
			}
			return WriteResponseStats{Samples: 1, Confirmed: true}, nil
		},
	}
	cfg := testDefaultQueueConfig()
	cfg.MaxSamplesPerSend = 1
	qm := newNativeTestQueueManager(t, cfg, client)
	w = nativeMetadataWriter{qm}
	qm.StoreSeries([]record.RefSeries{{Ref: 1, Labels: labels.FromStrings(labels.MetricName, "metric")}}, 0)
	storeNativeMetadataForTest(t, w, nativeMetadataEntry(1, record.NativeMetadataGroup, nativeMetadataPoint(0, old)), 0)
	qm.shards.start(1)
	defer qm.shards.stop()
	require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 100}}))
	var first []byte
	for i := range 2 {
		select {
		case request := <-requests:
			if i == 0 {
				first = request
			} else {
				require.Equal(t, first, request, "retry must not resolve newer metadata")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("missing retry")
		}
	}
	qm.shards.stop()
	qm.shards.start(2)
	require.True(t, qm.Append([]record.RefSample{{Ref: 1, T: 200}}))
	select {
	case request := <-requests:
		for i, encoded := range [][]byte{first, request} {
			decoded, err := compression.Decode(compression.Snappy, encoded, nil)
			require.NoError(t, err)
			var req writev2.Request
			require.NoError(t, req.Unmarshal(decoded))
			require.Len(t, req.Timeseries, 1)
			m, err := req.Timeseries[0].ToMetadata(req.Symbols)
			require.NoError(t, err)
			require.Equal(t, []metadata.Metadata{old, newest}[i], m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing delivery after resharding")
	}
}

// TestNativeMetadataSenderRetainedState checks the sender's retained native
// state, and its gauge, across the watcher's garbage collection: a checkpoint
// whose scan fails releases nothing, and a later one releases exactly the
// state that neither a kept series nor a later entry holds.
func TestNativeMetadataSenderRetainedState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		walDir := filepath.Join(dir, "wal")
		a, b := labels.FromStrings(labels.MetricName, "a"), labels.FromStrings(labels.MetricName, "b")
		m := metadata.Metadata{Type: model.MetricTypeCounter, Help: "help"}
		var enc record.Encoder
		w, err := wlog.New(nil, nil, walDir, compression.None)
		require.NoError(t, err)
		require.NoError(t, w.Log(
			enc.Series([]record.RefSeries{{Ref: 1, Labels: a}, {Ref: 2, Labels: b}}, nil),
			compactNativeMetadataRecordForTest([]nativeMetadataEntryForTest{
				{Ref: 1, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{nativeMetadataPoint(1, m)}},
				{Ref: 2, Kind: record.NativeMetadataGroup, Points: []nativeMetadataPointForTest{nativeMetadataPoint(1, m)}},
			}),
		))
		for range 3 {
			_, err := w.NextSegmentSync()
			require.NoError(t, err)
		}
		qm := newNativeTestQueueManager(t, testDefaultQueueConfig(), NewNopWriteClient())
		qm.watcher = wlog.NewWatcher(wlog.NewWatcherMetrics(nil), wlog.NewLiveReaderMetrics(nil), promslog.NewNopLogger(), "test", nativeMetadataWriter{qm}, dir, false, false, true, record.NewBuffersPool())
		qm.Start()
		defer func() {
			qm.Stop()
			require.NoError(t, w.Close())
		}()
		synctest.Wait()
		retained := func() (float64, []chunks.HeadSeriesRef) {
			qm.seriesMtx.Lock()
			defer qm.seriesMtx.Unlock()
			refs := slices.Sorted(maps.Keys(qm.seriesNativeMetadata))
			return client_testutil.ToFloat64(qm.metrics.nativeMetadataSeries), refs
		}
		// checkpoint writes a checkpoint of ref 1 and recs, and waits for
		// the watcher's next checkpoint tick.
		checkpoint := func(index int, recs ...[]byte) {
			cp, err := wlog.New(nil, nil, wlog.CheckpointDir(walDir, index), compression.None)
			require.NoError(t, err)
			require.NoError(t, cp.Log(append([][]byte{enc.Series([]record.RefSeries{{Ref: 1, Labels: a}}, nil)}, recs...)...))
			require.NoError(t, cp.Close())
			time.Sleep(6 * time.Second)
			synctest.Wait()
		}
		gauge, refs := retained()
		require.Equal(t, 2.0, gauge)
		require.Equal(t, []chunks.HeadSeriesRef{1, 2}, refs)

		// A checkpoint whose scan fails on a record it can't decode.
		unsupported := enc.NativeMetadataMarker(record.NativeMetadataBoundary, nil)
		unsupported[1] = 9
		checkpoint(1, unsupported)
		gauge, refs = retained()
		require.Equal(t, 2.0, gauge, "nothing is released")
		require.Equal(t, []chunks.HeadSeriesRef{1, 2}, refs)

		// A later checkpoint that keeps only ref 1.
		checkpoint(2)
		gauge, refs = retained()
		require.Equal(t, 1.0, gauge)
		require.Equal(t, []chunks.HeadSeriesRef{1}, refs)
	})
}

func TestNativeMetadataWALDelivery(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run("restart="+strconv.FormatBool(restart), func(t *testing.T) {
			dir := t.TempDir()
			opts := tsdb.DefaultOptions()
			opts.EnableNativeMetadata = true
			opts.EnableNativeMetadataPersistence = true
			opts.EnableMetadataWALRecords = false
			opts.EnableExemplarStorage = true
			opts.MaxExemplars = 100
			opts.EnableSTAsZeroSample = true
			db, err := tsdb.Open(dir, nil, nil, opts, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			base := time.Now().Add(time.Hour).UnixMilli()
			refs := make([]storage.SeriesRef, 3)
			unknown := metadata.Metadata{Type: model.MetricTypeUnknown}
			expected := map[string]metadata.Metadata{}
			key := func(kind string, index int, timestamp int64) string {
				return fmt.Sprintf("%s/%d/%d", kind, index, timestamp)
			}
			var previous metadata.Metadata
			newest := make([]metadata.Metadata, len(refs))
			for version := range 3 {
				app := db.AppenderV2(t.Context())
				timestamp := base + int64(version*100)
				for i := range refs {
					m := metadata.Metadata{Type: model.MetricTypeCounter, Help: []string{"a", "b", "a"}[version], Unit: "seconds"}
					if i > 0 {
						m.Type = model.MetricTypeHistogram
					} else if version == 1 {
						m.Type = model.MetricTypeUnknown
					}
					var h *histogram.Histogram
					var fh *histogram.FloatHistogram
					switch i {
					case 1:
						h = &histogram.Histogram{Count: 1, ZeroCount: 1, ZeroThreshold: 0.001}
					case 2:
						fh = &histogram.FloatHistogram{Count: 1, ZeroCount: 1, ZeroThreshold: 0.001}
					}
					refs[i], err = app.Append(refs[i], labels.FromStrings(labels.MetricName, "metric", "id", strconv.Itoa(i)), base-10, timestamp, 1, h, fh, storage.AOptions{Metadata: m})
					require.NoError(t, err)
					expected[key("sample", i, timestamp)] = m
					newest[i] = m
					if version == 0 {
						expected[key("sample", i, base-10)] = unknown
					}
					if i == 0 {
						_, err := app.(storage.ExemplarAppenderV2).AppendExemplars(refs[i], labels.EmptyLabels(), []exemplar.Exemplar{{Labels: labels.FromStrings("trace_id", strconv.Itoa(version)), Value: 1, Ts: timestamp - 1, HasTs: true}})
						require.NoError(t, err)
						if version == 0 {
							previous = unknown
						}
						expected[key("exemplar", i, timestamp-1)] = previous
						previous = m
					}
				}
				require.NoError(t, app.Commit())
			}
			if restart {
				require.NoError(t, db.Close())
				db, err = tsdb.Open(dir, nil, nil, opts, nil)
				require.NoError(t, err)
				// The watcher skips old segments on startup. Append to the new
				// segment without supplying metadata: replay seeded each series
				// with its history, whose newest version labels the samples.
				clear(expected)
				app := db.AppenderV2(t.Context())
				for i, ref := range refs {
					_, err := app.Append(ref, labels.EmptyLabels(), 0, base+1000, 1, nil, nil, storage.AOptions{})
					require.NoError(t, err)
					expected[key("sample", i, base+1000)] = newest[i]
				}
				require.NoError(t, app.Commit())
			}

			received := make(chan *writev2.Request, 32)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				decoded, err := compression.Decode(compression.Snappy, body, nil)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				req := new(writev2.Request)
				if err := req.Unmarshal(decoded); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				var samples, histograms, exemplars int
				for _, series := range req.Timeseries {
					samples += len(series.Samples)
					histograms += len(series.Histograms)
					exemplars += len(series.Exemplars)
				}
				w.Header().Set("X-Prometheus-Remote-Write-Samples-Written", strconv.Itoa(samples))
				w.Header().Set("X-Prometheus-Remote-Write-Histograms-Written", strconv.Itoa(histograms))
				w.Header().Set("X-Prometheus-Remote-Write-Exemplars-Written", strconv.Itoa(exemplars))
				received <- req
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			s := NewStorage(nil, nil, db.StartTime, dir, defaultFlushDeadline, nil, false, true)
			defer s.Close()
			rw := baseRemoteWriteConfig(server.URL)
			rw.ProtobufMessage = remoteapi.WriteV2MessageType
			rw.SendExemplars, rw.SendNativeHistograms = true, true
			rw.QueueConfig = testDefaultQueueConfig()
			rw.QueueConfig.MaxShards = 1
			rw.MetadataConfig.Send = false
			rw.WriteRelabelConfigs = []*relabel.Config{{Action: relabel.Replace, Regex: relabel.MustNewRegexp(".*"), TargetLabel: labels.MetricName, Replacement: "renamed", NameValidationScheme: model.UTF8Validation}}
			require.NoError(t, s.ApplyConfig(&config.Config{GlobalConfig: config.GlobalConfig{ExternalLabels: labels.FromStrings("cluster", "test")}, RemoteWriteConfigs: []*config.RemoteWriteConfig{rw}}))
			deadline := time.After(5 * time.Second)
			notify := time.NewTicker(10 * time.Millisecond)
			defer notify.Stop()
			builder := labels.NewScratchBuilder(0)
			for len(expected) > 0 {
				select {
				case <-notify.C:
					s.Notify()
				case req := <-received:
					for _, series := range req.Timeseries {
						lset, err := series.ToLabels(&builder, req.Symbols)
						require.NoError(t, err)
						require.Equal(t, "renamed", lset.Get(labels.MetricName))
						require.Equal(t, "test", lset.Get("cluster"))
						id, err := strconv.Atoi(lset.Get("id"))
						require.NoError(t, err)
						m, err := series.ToMetadata(req.Symbols)
						require.NoError(t, err)
						var k string
						switch {
						case len(series.Samples) > 0:
							k = key("sample", id, series.Samples[0].Timestamp)
						case len(series.Histograms) > 0:
							k = key("sample", id, series.Histograms[0].Timestamp)
						case len(series.Exemplars) > 0:
							k = key("exemplar", id, series.Exemplars[0].Timestamp)
						}
						want, ok := expected[k]
						require.True(t, ok, "unexpected or duplicate item %s", k)
						require.Equal(t, want, m, k)
						delete(expected, k)
					}
				case <-deadline:
					t.Fatalf("missing metadata deliveries: %v", expected)
				}
			}
		})
	}

	// Each item gets the version in effect at its own timestamp, as of its WAL
	// position. A sender that starts after all commits reads them in WAL order,
	// as a delayed watcher would.
	t.Run("WAL-prefix semantics", func(t *testing.T) {
		type commit struct {
			offset int64
			help   string
		}
		for _, c := range []struct {
			name    string
			commits []commit
		}{
			{
				// Backfilled B@125, committed after the sample at 150, does not
				// relabel it. Lookups after the backfill would return B.
				name:    "delayed watcher",
				commits: []commit{{100, "A"}, {150, "A"}, {125, "B"}},
			},
			{
				// Native storage evicts A and B. Lookups would miss them.
				name:    "backlog beyond the version cap",
				commits: []commit{{100, "A"}, {200, "B"}, {300, "C"}, {400, "D"}, {500, "E"}, {600, "F"}, {700, "G"}},
			},
		} {
			t.Run(c.name, func(t *testing.T) {
				dir := t.TempDir()
				opts := tsdb.DefaultOptions()
				opts.EnableNativeMetadata = true
				opts.EnableNativeMetadataPersistence = true
				opts.OutOfOrderTimeWindow = time.Hour.Milliseconds()
				db, err := tsdb.Open(dir, nil, nil, opts, nil)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, db.Close()) })
				base := time.Now().Add(time.Hour).UnixMilli()
				lset := labels.FromStrings(labels.MetricName, "metric")
				expected := map[int64]string{}
				for _, commit := range c.commits {
					app := db.AppenderV2(t.Context())
					_, err := app.Append(0, lset, 0, base+commit.offset, 1, nil, nil, storage.AOptions{Metadata: metadata.Metadata{Type: model.MetricTypeGauge, Help: commit.help}})
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					expected[base+commit.offset] = commit.help
				}

				received := make(chan *writev2.Request, 32)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err == nil {
						body, err = compression.Decode(compression.Snappy, body, nil)
					}
					req := new(writev2.Request)
					if err == nil {
						err = req.Unmarshal(body)
					}
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					samples := 0
					for _, series := range req.Timeseries {
						samples += len(series.Samples)
					}
					w.Header().Set("X-Prometheus-Remote-Write-Samples-Written", strconv.Itoa(samples))
					received <- req
					w.WriteHeader(http.StatusNoContent)
				}))
				defer server.Close()
				s := NewStorage(nil, nil, db.StartTime, dir, defaultFlushDeadline, nil, false, true)
				defer s.Close()
				rw := baseRemoteWriteConfig(server.URL)
				rw.ProtobufMessage = remoteapi.WriteV2MessageType
				rw.QueueConfig = testDefaultQueueConfig()
				rw.MetadataConfig.Send = false
				require.NoError(t, s.ApplyConfig(&config.Config{RemoteWriteConfigs: []*config.RemoteWriteConfig{rw}}))
				deadline := time.After(5 * time.Second)
				notify := time.NewTicker(10 * time.Millisecond)
				defer notify.Stop()
				for len(expected) > 0 {
					select {
					case <-notify.C:
						s.Notify()
					case req := <-received:
						for _, series := range req.Timeseries {
							m, err := series.ToMetadata(req.Symbols)
							require.NoError(t, err)
							for _, sample := range series.Samples {
								want, ok := expected[sample.Timestamp]
								require.True(t, ok, "unexpected or duplicate sample at %d", sample.Timestamp)
								require.Equal(t, want, m.Help, "sample at %d", sample.Timestamp-base)
								delete(expected, sample.Timestamp)
							}
						}
					case <-deadline:
						t.Fatalf("missing deliveries: %v", expected)
					}
				}
			})
		}
	})

	// A repair override written before its series' retirement is read by a
	// delayed watcher after the sender has reset the series, through a real
	// Head, watcher and queue. Samples wait queued while the destination is
	// held back, and a failed send is retried. Every delivery must match the
	// WAL-prefix oracle, and the orphan state the override creates goes with
	// the first checkpoint beyond it that doesn't keep the series.
	//
	// The watcher processes a checkpoint asynchronously, after one select
	// between two ready tickers, so whether the queue's reset precedes its
	// application of the override varies. The recorder serializes the two, so
	// the recorded order is the order of the state updates. Attempts repeat,
	// within a bound, until both orders have occurred; each one is checked in
	// full.
	t.Run("a pre-retirement override read after a sender reset", func(t *testing.T) {
		orders := map[bool]int{}
		for attempt := 0; orders[true] == 0 || orders[false] == 0; attempt++ {
			require.Less(t, attempt, 40, "both orders must occur: %v", orders)
			synctest.Test(t, func(t *testing.T) {
				orders[deliverNativeMetadataAcrossSenderReset(t)]++
			})
		}
		t.Logf("attempts: %d with the reset before the override's application, %d after", orders[true], orders[false])
	})

	// A ref remapped at a restart is handed off to the sender, through a real
	// DB, watcher and queue: the restarted Head writes the surviving ref's
	// history, and queues started later, one from a checkpoint, label the
	// ref's samples with it. Every delivery must match the WAL-prefix oracle.
	t.Run("a remapped ref across restarts", func(t *testing.T) {
		synctest.Test(t, deliverNativeMetadataAcrossRestarts)
	})
}

// deliverNativeMetadataAcrossRestarts runs the remapped-ref scenario once, in a
// synctest bubble.
func deliverNativeMetadataAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	oracleDir := t.TempDir()
	opts := tsdb.DefaultOptions()
	opts.EnableNativeMetadata = true
	opts.EnableNativeMetadataPersistence = true
	open := func() *tsdb.DB {
		db, err := tsdb.Open(dir, nil, nil, opts, nil)
		require.NoError(t, err)
		db.DisableCompactions()
		return db
	}
	client := &heldBackClient{}
	start := func() *QueueManager {
		cfg := testDefaultQueueConfig()
		cfg.MinShards, cfg.MaxShards = 1, 1
		qm := newNativeTestQueueManager(t, cfg, client)
		qm.watcher = wlog.NewWatcher(wlog.NewWatcherMetrics(nil), wlog.NewLiveReaderMetrics(nil), promslog.NewNopLogger(), "test", nativeMetadataWriter{qm}, dir, false, false, true, record.NewBuffersPool())
		qm.Start()
		// The watcher replays the segments that exist, without sending
		// their samples, and tails the last.
		synctest.Wait()
		return qm
	}
	// catchUp gives the watcher time to read everything, and the queue to send.
	catchUp := func(qm *QueueManager) {
		for range 30 {
			qm.watcher.Notify()
			time.Sleep(100 * time.Millisecond)
		}
		synctest.Wait()
	}

	base := time.Now().Add(time.Hour).UnixMilli()
	l := labels.FromStrings(labels.MetricName, "l")
	mA := metadata.Metadata{Type: model.MetricTypeCounter, Help: "A"}
	mB := metadata.Metadata{Type: model.MetricTypeCounter, Help: "B"}
	mC := metadata.Metadata{Type: model.MetricTypeCounter, Help: "C"}
	mD := metadata.Metadata{Type: model.MetricTypeCounter, Help: "D"}
	commit := func(db *tsdb.DB, ts int64, m metadata.Metadata) chunks.HeadSeriesRef {
		app := db.AppenderV2(t.Context())
		ref, err := app.Append(0, l, 0, ts, 1, nil, nil, storage.AOptions{Metadata: m})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
		return chunks.HeadSeriesRef(ref)
	}
	// truncate truncates the Head, which starts a new WAL segment, and may
	// checkpoint and delete earlier ones; the oracle keeps copies.
	truncate := func(db *tsdb.DB, mint int64) {
		copyWALSegments(t, walDir, oracleDir)
		require.NoError(t, db.Head().Truncate(mint))
	}
	// requireDelivered requires every delivery so far to match the WAL-prefix
	// oracle, and these samples to have been delivered.
	requireDelivered := func(samples ...string) {
		t.Helper()
		copyWALSegments(t, walDir, oracleDir)
		expected := nativeMetadataWALPrefixOracle(t, oracleDir)
		client.mtx.Lock()
		defer client.mtx.Unlock()
		delivered := map[string]bool{}
		for _, a := range client.attempts {
			require.False(t, a.failed)
			for _, d := range a.samples {
				require.True(t, expected[d], "unexpected delivery %s", d)
				delivered[d] = true
			}
		}
		for _, d := range samples {
			require.True(t, delivered[d], "delivery %s", d)
		}
	}

	// 1. l is r1 with A, retired by a truncation, and then r2 with B.
	db := open()
	r1 := commit(db, base+1, mA)
	truncate(db, base+100)
	r2 := commit(db, base+200, mB)
	require.NotEqual(t, r1, r2)
	require.NoError(t, db.Close())

	// 2. The restart remaps r2 onto r1, whose handoff carries B, and a queue
	// started afterwards labels r1's samples with B, then with C.
	db = open()
	qm := start()
	require.Equal(t, r1, commit(db, base+300, mB))
	require.Equal(t, r1, commit(db, base+400, mC))
	catchUp(qm)
	qm.Stop()
	requireDelivered(nativeMetadataDelivery(l, base+300, mB), nativeMetadataDelivery(l, base+400, mC))

	// 3. Checkpoints keeping r2, whose samples last until base+200, hold
	// r1's handoff. A queue started from them labels r1's samples with C,
	// then with D.
	for i := range int64(4) {
		truncate(db, base+101+i)
		commit(db, base+410+10*i, mC)
	}
	truncate(db, base+150)
	checkpoint := mustLastCheckpoint(t, walDir)
	require.Contains(t, nativeMetadataCheckpointRefs(t, checkpoint), r2, "the checkpoint keeps the duplicate")
	require.True(t, nativeMetadataCheckpointHandoffs(t, checkpoint)[r1], "the checkpoint keeps r1 current")
	qm = start()
	commit(db, base+500, mC)
	commit(db, base+600, mD)
	catchUp(qm)
	qm.Stop()
	requireDelivered(nativeMetadataDelivery(l, base+500, mC), nativeMetadataDelivery(l, base+600, mD))

	// 4. Replay from the checkpoint keeps r1 current, and so its history.
	require.NoError(t, db.Close())
	db = open()
	series, _, err := db.NativeMetricMetadata(t.Context(), [][]*labels.Matcher{{labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "l")}}, 0)
	require.NoError(t, err)
	require.Len(t, series, 1)
	require.Equal(t, []tsdb.NativeMetricMetadataVersion{{EffectiveFrom: base + 200, Metadata: mB}, {EffectiveFrom: base + 400, Metadata: mC}, {EffectiveFrom: base + 600, Metadata: mD}}, series[0].Versions)
	qm = start()
	commit(db, base+700, mD)
	catchUp(qm)
	qm.Stop()
	requireDelivered(nativeMetadataDelivery(l, base+700, mD))
	require.NoError(t, db.Close())
}

// nativeMetadataCheckpointHandoffs returns the refs of a checkpoint's handoffs.
func nativeMetadataCheckpointHandoffs(t *testing.T, dir string) map[chunks.HeadSeriesRef]bool {
	t.Helper()
	sr, err := wlog.NewSegmentsReader(dir)
	require.NoError(t, err)
	defer sr.Close()
	var dec record.Decoder
	out := map[chunks.HeadSeriesRef]bool{}
	r := wlog.NewReader(sr)
	for r.Next() {
		if typ := dec.Type(r.Record()); typ != record.Metadata && typ != record.NativeMetadataCompact {
			continue
		}
		for _, e := range decodeNativeMetadataRecordForTest(t, r.Record()) {
			if e.Handoff {
				out[e.Ref] = true
			}
		}
	}
	require.NoError(t, r.Err())
	return out
}

// mustLastCheckpoint returns the last checkpoint's directory.
func mustLastCheckpoint(t *testing.T, walDir string) string {
	t.Helper()
	checkpoint, _, err := wlog.LastCheckpoint(walDir)
	require.NoError(t, err)
	return checkpoint
}

// nativeMetadataResetRecorder passes the watcher's calls to a queue. It
// records the queue's resets, and its applications of one ref's overrides,
// each with the ref's sender state right after it. One mutex serializes each
// call with its record, so the recorded order is the order of the state
// updates.
type nativeMetadataResetRecorder struct {
	nativeMetadataWriter
	mtx    sync.Mutex
	ref    chunks.HeadSeriesRef
	events []nativeMetadataResetEvent
}

// nativeMetadataResetEvent is a reset, or the application of an override, with
// the watched ref's sender state right after it.
type nativeMetadataResetEvent struct {
	what     string
	state    bool
	segment  int
	labelled bool
}

func (r *nativeMetadataResetRecorder) watch(ref chunks.HeadSeriesRef) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.ref = ref
}

// recordLocked records what happened, and the watched ref's state after it.
func (r *nativeMetadataResetRecorder) recordLocked(what string) {
	t := r.QueueManager
	t.seriesMtx.Lock()
	defer t.seriesMtx.Unlock()
	state, ok := t.seriesNativeMetadata[r.ref]
	_, labelled := t.seriesLabels[r.ref]
	r.events = append(r.events, nativeMetadataResetEvent{what: what, state: ok, segment: state.segment, labelled: labelled})
}

func (r *nativeMetadataResetRecorder) SeriesReset(index int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.nativeMetadataWriter.SeriesReset(index)
	r.recordLocked(fmt.Sprintf("reset %d", index))
}

// StoreNativeMetadataRecord records the record's overrides of the watched ref.
func (r *nativeMetadataResetRecorder) StoreNativeMetadataRecord(rec *record.CompactNativeMetadata, segment int) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	overrides := 0
	for _, e := range rec.Entries {
		if e.Ref == r.ref && e.Kind == record.NativeMetadataOverride {
			overrides++
		}
	}
	r.nativeMetadataWriter.StoreNativeMetadataRecord(rec, segment)
	for range overrides {
		r.recordLocked("override")
	}
}

// heldBackClient is a destination that can hold sends back, and fail the
// first send after its release once.
type heldBackClient struct {
	mtx      sync.Mutex
	gate     chan struct{}
	failOnce bool
	attempts []heldBackAttempt
}

type heldBackAttempt struct {
	samples []string
	failed  bool
}

func (c *heldBackClient) hold() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.gate = make(chan struct{})
}

func (c *heldBackClient) release() {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	close(c.gate)
	c.gate, c.failOnce = nil, true
}

func (*heldBackClient) Name() string     { return "held back" }
func (*heldBackClient) Endpoint() string { return "http://held.back" }

// Store records each sample as its labels, timestamp and metadata.
func (c *heldBackClient) Store(ctx context.Context, compressed []byte, _ int) (WriteResponseStats, error) {
	c.mtx.Lock()
	gate := c.gate
	c.mtx.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return WriteResponseStats{}, ctx.Err()
		}
	}
	decoded, err := compression.Decode(compression.Snappy, compressed, nil)
	if err != nil {
		return WriteResponseStats{}, err
	}
	var req writev2.Request
	if err := req.Unmarshal(decoded); err != nil {
		return WriteResponseStats{}, err
	}
	var attempt heldBackAttempt
	builder := labels.NewScratchBuilder(0)
	for _, series := range req.Timeseries {
		lset, err := series.ToLabels(&builder, req.Symbols)
		if err != nil {
			return WriteResponseStats{}, err
		}
		m, err := series.ToMetadata(req.Symbols)
		if err != nil {
			return WriteResponseStats{}, err
		}
		for _, sample := range series.Samples {
			attempt.samples = append(attempt.samples, nativeMetadataDelivery(lset, sample.Timestamp, m))
		}
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	attempt.failed, c.failOnce = c.failOnce, false
	c.attempts = append(c.attempts, attempt)
	if attempt.failed {
		return WriteResponseStats{}, RecoverableError{errors.New("injected send failure"), 0}
	}
	return WriteResponseStats{Samples: len(attempt.samples), Confirmed: true}, nil
}

func nativeMetadataDelivery(lset labels.Labels, ts int64, m metadata.Metadata) string {
	return fmt.Sprintf("%s@%d: %s/%s/%s", lset, ts, m.Type, m.Unit, m.Help)
}

// copyWALSegments copies the segments in dir into to, replacing earlier copies,
// so that to keeps every segment that truncation deletes.
func copyWALSegments(t *testing.T, dir, to string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil || e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(to, e.Name()), b, 0o666))
	}
}

// nativeMetadataWALPrefixOracle returns, for every sample in the segments of
// dir in WAL order, the delivery the WAL-prefix contract expects: the version
// at the sample's timestamp of its series' history reduced from the WAL
// before it.
func nativeMetadataWALPrefixOracle(t *testing.T, dir string) map[string]bool {
	t.Helper()
	first, last, err := wlog.Segments(dir)
	require.NoError(t, err)
	sr, err := wlog.NewSegmentsRangeReader(wlog.SegmentRange{Dir: dir, First: first, Last: last})
	require.NoError(t, err)
	defer sr.Close()
	var dec record.Decoder
	series := map[chunks.HeadSeriesRef]labels.Labels{}
	states := map[chunks.HeadSeriesRef]*nativemetadata.State{}
	expected := map[string]bool{}
	r := wlog.NewReader(sr)
	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Series:
			refs, err := dec.Series(rec, nil)
			require.NoError(t, err)
			for _, s := range refs {
				series[s.Ref] = s.Labels
			}
		case record.Metadata, record.NativeMetadataCompact:
			// The segments of a WAL without a checkpoint are trusted from the
			// start, so markers change nothing.
			for _, e := range decodeNativeMetadataRecordForTest(t, rec) {
				if e.Kind == record.NativeMetadataProvenance || e.Kind == record.NativeMetadataBoundary {
					continue
				}
				if states[e.Ref] == nil {
					states[e.Ref] = &nativemetadata.State{}
				}
				states[e.Ref].Apply(e.Kind, e.Truncated, nativeMetadataPointsForTest(e.Points))
			}
		case record.Samples:
			samples, err := dec.Samples(rec, nil)
			require.NoError(t, err)
			for _, s := range samples {
				m := metadata.Metadata{Type: model.MetricTypeUnknown}
				if state := states[s.Ref]; state != nil {
					if v := state.At(s.T); v != nil {
						m = *v
					}
				}
				expected[nativeMetadataDelivery(series[s.Ref], s.T, m)] = true
			}
		}
	}
	require.NoError(t, r.Err())
	return expected
}

// deliverNativeMetadataAcrossSenderReset runs the pre-retirement override
// scenario once, in a synctest bubble, and reports whether the sender reset the
// series before it applied the override.
func deliverNativeMetadataAcrossSenderReset(t *testing.T) bool {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	oracleDir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, walDir, wlog.DefaultSegmentSize, compression.None)
	require.NoError(t, err)
	errInjected := errors.New("injected WAL flush failure")
	var failMetadata atomic.Bool
	// failedType is the type of the native metadata record whose flush failed.
	var failedType atomic.Uint32
	wal.SetFlushFault(func(rec []byte, at wlog.FlushPoint) error {
		// A failed final flush keeps the record in the page buffer.
		if at == wlog.FlushAfterRecord && len(rec) > 0 && isNativeMetadataRecordForTest(rec) && failMetadata.CompareAndSwap(true, false) {
			failedType.Store(uint32(rec[0]))
			return errInjected
		}
		return nil
	})
	opts := tsdb.DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.EnableNativeMetadata = true
	opts.EnableNativeMetadataPersistence = true
	reg := prometheus.NewRegistry()
	h, err := tsdb.NewHead(reg, nil, wal, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(math.MinInt64))
	gauge := func(name string) float64 {
		families, err := reg.Gather()
		require.NoError(t, err)
		for _, f := range families {
			if f.GetName() == name {
				return f.GetMetric()[0].GetGauge().GetValue()
			}
		}
		t.Fatalf("no metric %s", name)
		return 0
	}

	client := &heldBackClient{}
	cfg := testDefaultQueueConfig()
	cfg.MinShards, cfg.MaxShards = 1, 1
	cfg.Capacity, cfg.MaxSamplesPerSend = 2, 1
	qm := newNativeTestQueueManager(t, cfg, client)
	recorder := &nativeMetadataResetRecorder{nativeMetadataWriter: nativeMetadataWriter{qm}}
	qm.watcher = wlog.NewWatcher(wlog.NewWatcherMetrics(nil), wlog.NewLiveReaderMetrics(nil), promslog.NewNopLogger(), "test", recorder, dir, false, false, true, record.NewBuffersPool())
	qm.Start()
	// The watcher must tail segment 0 before anything is written: segments
	// that exist when it starts are replayed without sending their samples.
	synctest.Wait()
	// Everything stops before the bubble ends, a failed test too.
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			qm.Stop()
			copyWALSegments(t, walDir, oracleDir)
			require.NoError(t, h.Close())
		})
	}
	defer stop()

	base := time.Now().Add(time.Hour).UnixMilli()
	lr, lo := labels.FromStrings(labels.MetricName, "r"), labels.FromStrings(labels.MetricName, "o")
	mA := metadata.Metadata{Type: model.MetricTypeCounter, Help: "A"}
	mX := metadata.Metadata{Type: model.MetricTypeGauge, Help: "X"}
	commit := func(lset labels.Labels, ts int64, m metadata.Metadata) (chunks.HeadSeriesRef, error) {
		app := h.AppenderV2(t.Context())
		ref, err := app.Append(0, lset, 0, ts, 1, nil, nil, storage.AOptions{Metadata: m})
		require.NoError(t, err)
		return chunks.HeadSeriesRef(ref), app.Commit()
	}
	mustCommit := func(lset labels.Labels, ts int64, m metadata.Metadata) chunks.HeadSeriesRef {
		ref, err := commit(lset, ts, m)
		require.NoError(t, err)
		return ref
	}
	rotate := func() {
		_, err := wal.NextSegmentSync()
		require.NoError(t, err)
	}
	// catchUp gives the watcher time to read everything, through segment ticks
	// and notifications.
	catchUp := func(d time.Duration) {
		for elapsed := time.Duration(0); elapsed < d; elapsed += 100 * time.Millisecond {
			qm.watcher.Notify()
			time.Sleep(100 * time.Millisecond)
		}
		synctest.Wait()
	}
	truncate := func(mint int64, wantCheckpoint int) {
		copyWALSegments(t, walDir, oracleDir)
		require.NoError(t, h.Truncate(mint))
		_, index, err := wlog.LastCheckpoint(walDir)
		require.NoError(t, err)
		require.Equal(t, wantCheckpoint, index)
	}

	// 1. r and o in segment 0, then fillers for o up to segment 10, read as
	// they are written.
	refR := mustCommit(lr, base+1, mA)
	recorder.watch(refR)
	refO := mustCommit(lo, base+1, mX)
	for segment := int64(1); segment <= 10; segment++ {
		rotate()
		mustCommit(lo, base+segment*10, mX)
	}
	catchUp(3 * time.Second)

	// 2. The destination is held back: sends block, the queue fills, and the
	// watcher waits to enqueue in segment 10, with r's last samples queued.
	client.hold()
	mustCommit(lr, base+101, mA)
	mustCommit(lr, base+102, mA)
	for ts := base + 103; ts <= base+106; ts++ {
		mustCommit(lo, ts, mX)
	}
	qm.watcher.Notify()
	synctest.Wait()
	require.Positive(t, client_testutil.ToFloat64(qm.metrics.enqueueRetriesTotal), "the watcher waits for capacity")

	// 3. In segment 11, r's failed commit leaves a phantom group, then the
	// repair worker writes r's override after it.
	rotate()
	failMetadata.Store(true)
	_, err = commit(lr, base+111, metadata.Metadata{Type: model.MetricTypeCounter, Help: "phantom"})
	require.ErrorIs(t, err, errInjected)
	require.False(t, failMetadata.Load(), "the fault fired")
	require.Equal(t, uint32(record.NativeMetadataCompact), failedType.Load(), "on a compact record")
	synctest.Wait()
	require.Zero(t, gauge("prometheus_tsdb_native_metric_metadata_repairs_pending"))
	mustCommit(lo, base+500, mX)

	// 4. GC retires r, now unpinned, and checkpoints before segment 11 drop it.
	truncate(base+112, 6)
	mustCommit(lo, base+1000, mX)
	truncate(base+600, 9)
	checkpoint, _, err := wlog.LastCheckpoint(walDir)
	require.NoError(t, err)
	for _, ref := range nativeMetadataCheckpointRefs(t, checkpoint) {
		require.NotEqual(t, refR, ref, "the checkpoint keeps r")
	}

	// 5. The destination recovers after the watcher's checkpoint tick is due.
	// The first send fails and is retried. The watcher resumes after its
	// enqueue backoff, and the catch-up spans a checkpoint tick of the tail
	// segment it then reaches.
	time.Sleep(6 * time.Second)
	client.release()
	catchUp(15 * time.Second)
	qm.seriesMtx.Lock()
	_, labelled := qm.seriesLabels[refR]
	state, ok := qm.seriesNativeMetadata[refR]
	qm.seriesMtx.Unlock()
	require.False(t, labelled, "the reset dropped r's labels")
	require.True(t, ok, "r's state is kept by its mark, or recreated by the override")
	require.Equal(t, 11, state.segment)

	// 6. r's labels return as a new series, and a checkpoint beyond segment 11,
	// keeping neither r nor its state, drops the state.
	refR2 := mustCommit(lr, base+2000, metadata.Metadata{Type: model.MetricTypeCounter, Help: "C"})
	require.NotEqual(t, refR, refR2)
	rotate()
	rotate()
	catchUp(time.Second)
	truncate(base+700, 12)
	catchUp(6 * time.Second)
	qm.seriesMtx.Lock()
	_, kept := qm.seriesNativeMetadata[refR]
	_, keptR2 := qm.seriesNativeMetadata[refR2]
	_, keptO := qm.seriesNativeMetadata[refO]
	qm.seriesMtx.Unlock()
	require.False(t, kept, "the checkpoint beyond the override drops r's state")
	require.True(t, keptR2)
	require.True(t, keptO)

	stop()

	// Every send carries the WAL-prefix oracle's metadata, and every sample is
	// delivered once, a failed send's again by its retry.
	expected := nativeMetadataWALPrefixOracle(t, oracleDir)
	client.mtx.Lock()
	defer client.mtx.Unlock()
	delivered := map[string]int{}
	failed := 0
	for _, a := range client.attempts {
		for _, d := range a.samples {
			require.True(t, expected[d], "unexpected delivery %s", d)
			if !a.failed {
				delivered[d]++
			}
		}
		if a.failed {
			failed++
		}
	}
	require.Equal(t, 1, failed)
	for d := range expected {
		require.Equal(t, 1, delivered[d], "delivery %s", d)
	}
	require.Contains(t, expected, nativeMetadataDelivery(lr, base+101, mA), "r's queued samples keep r's metadata")

	// The reset and the override's application happened in the recorded
	// order, each leaving r's state as that order implies. Reset first, the
	// reset drops r's state, and the override recreates it as an orphan.
	// Applied first, the override leaves r labelled, and the reset keeps its
	// state by its mark while dropping its labels.
	recorder.mtx.Lock()
	defer recorder.mtx.Unlock()
	reset := slices.IndexFunc(recorder.events, func(e nativeMetadataResetEvent) bool { return e.what == "reset 9" })
	override := slices.IndexFunc(recorder.events, func(e nativeMetadataResetEvent) bool { return e.what == "override" })
	require.NotEqual(t, -1, reset, "events: %+v", recorder.events)
	require.NotEqual(t, -1, override, "events: %+v", recorder.events)
	afterReset, afterOverride := recorder.events[reset], recorder.events[override]
	require.False(t, afterReset.labelled, "the reset drops r's labels")
	require.Equal(t, nativeMetadataResetEvent{what: "override", state: true, segment: 11, labelled: reset > override}, afterOverride)
	if reset < override {
		require.False(t, afterReset.state, "a reset before the override drops r's state")
		return true
	}
	require.Equal(t, nativeMetadataResetEvent{what: "reset 9", state: true, segment: 11}, afterReset, "a reset after the override keeps r's state by its mark")
	return false
}

// nativeMetadataCheckpointRefs returns the refs of a checkpoint's series
// records and native metadata entries.
func nativeMetadataCheckpointRefs(t *testing.T, dir string) []chunks.HeadSeriesRef {
	t.Helper()
	sr, err := wlog.NewSegmentsReader(dir)
	require.NoError(t, err)
	defer sr.Close()
	var dec record.Decoder
	var refs []chunks.HeadSeriesRef
	r := wlog.NewReader(sr)
	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			for _, s := range series {
				refs = append(refs, s.Ref)
			}
		case record.Metadata, record.NativeMetadataCompact:
			for _, e := range decodeNativeMetadataRecordForTest(t, rec) {
				refs = append(refs, e.Ref)
			}
		}
	}
	require.NoError(t, r.Err())
	return refs
}
