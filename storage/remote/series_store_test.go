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
	"context"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

func TestSeriesStore_DenseSparseAndLifecycle(t *testing.T) {
	t.Parallel()

	dropRule := []*relabel.Config{{
		SourceLabels:         model.LabelNames{"__name__"},
		Separator:            ";",
		Regex:                relabel.MustNewRegexp("drop_.*"),
		Action:               relabel.Drop,
		NameValidationScheme: model.UTF8Validation,
	}}
	extLabels := []labels.Label{{Name: "cluster", Value: "eu-1"}}

	var store seriesStore

	// Store metadata before StoreSeries for both dense and sparse refs.
	const (
		densePreMetaKeep chunks.HeadSeriesRef = 7
		densePreMetaDrop chunks.HeadSeriesRef = 8
		denseMetaOnly    chunks.HeadSeriesRef = 9
		sparseRef        chunks.HeadSeriesRef = 1 << 40
		maxRef           chunks.HeadSeriesRef = math.MaxUint64
	)

	store.storeMetadata([]record.RefMetadata{
		{Ref: densePreMetaKeep, Type: uint8(record.Counter), Unit: "seconds", Help: "pre-keep"},
		{Ref: densePreMetaDrop, Type: uint8(record.Gauge), Unit: "bytes", Help: "pre-drop"},
		{Ref: denseMetaOnly, Type: uint8(record.Gauge), Unit: "", Help: "meta-only"},
		{Ref: sparseRef, Type: uint8(record.Counter), Unit: "requests", Help: "sparse-help"},
	})

	// Metadata-only entries are not active series yet.
	require.Equal(t, 0, store.activeLen())
	require.Equal(t, 4, store.metadataLen())
	require.Equal(t, seriesStateMetadataOnly, store.lookup(denseMetaOnly).state)

	// Store series across page and stripe boundaries, plus sparse refs.
	boundaryRefs := []chunks.HeadSeriesRef{
		0, 1, densePreMetaKeep, seriesPageSize - 1, seriesPageSize,
		seriesPageSize*numSeriesStripes - 1, seriesPageSize * numSeriesStripes,
		sparseRef, maxRef,
	}
	var seg0 []record.RefSeries
	for i, ref := range boundaryRefs {
		seg0 = append(seg0, record.RefSeries{
			Ref:    ref,
			Labels: labels.FromStrings("__name__", "metric_"+strconv.Itoa(i)),
		})
	}
	seg0 = append(seg0, record.RefSeries{
		Ref:    densePreMetaDrop,
		Labels: labels.FromStrings("__name__", "drop_me"),
	})
	store.storeSeries(seg0, 0, extLabels, dropRule)

	require.Equal(t, len(boundaryRefs), store.activeLen())

	// Pre-stored metadata is preserved when StoreSeries activates the ref.
	keptEntry := store.lookup(densePreMetaKeep)
	require.Equal(t, seriesStateActive, keptEntry.state)
	require.Equal(t, "eu-1", keptEntry.labels.Get("cluster"))
	require.False(t, metadata.IsZeroHandle(keptEntry.metadata))
	require.Equal(t, "pre-keep", keptEntry.metadata.Value().Help)

	// Dropped series transition to seriesStateDropped.
	droppedEntry := store.lookup(densePreMetaDrop)
	require.Equal(t, seriesStateDropped, droppedEntry.state)
	require.True(t, droppedEntry.labels.IsEmpty())

	// Batch lookup across >16 samples (exercising radix stripe bucketing with mixed dense + sparse + unknown).
	samples := make([]record.RefSample, 0, 32)
	for range 3 {
		for _, ref := range boundaryRefs {
			samples = append(samples, record.RefSample{Ref: ref})
		}
		samples = append(samples,
			record.RefSample{Ref: densePreMetaDrop},
			record.RefSample{Ref: denseMetaOnly},
			record.RefSample{Ref: 999_999}, // Unknown.
		)
	}
	dst := make([]seriesEntry, len(samples))
	store.lookupSamples(samples, dst)
	for i, s := range samples {
		require.Equal(t, store.lookup(s.Ref), dst[i], "mismatch at sample %d (ref=%d)", i, s.Ref)
	}

	// Promote half of boundaryRefs to segment 2 via updateSeriesSegment, then reset(<2).
	var promoted []record.RefSeries
	for i, ref := range boundaryRefs {
		if i%2 == 0 {
			promoted = append(promoted, record.RefSeries{Ref: ref})
		}
	}
	store.updateSeriesSegment(promoted, 2)

	store.reset(2)
	// Unpromoted boundaryRefs + densePreMetaDrop are evicted; denseMetaOnly (no segment) remains.
	for i, ref := range boundaryRefs {
		if i%2 == 0 {
			require.Equal(t, seriesStateActive, store.lookup(ref).state, "promoted ref=%d should remain active", ref)
		} else {
			require.Equal(t, seriesStateUnknown, store.lookup(ref).state, "unpromoted ref=%d should be evicted", ref)
		}
	}
	require.Equal(t, seriesStateUnknown, store.lookup(densePreMetaDrop).state)
	require.Equal(t, len(promoted), store.activeLen())
	require.Equal(t, seriesStateMetadataOnly, store.lookup(denseMetaOnly).state)

	// Lower minSegment on a promoted dense page entry (seg=1 < p.minSegment=2) and verify reset(2) evicts it.
	store.updateSeriesSegment([]record.RefSeries{{Ref: boundaryRefs[0]}}, 1)
	store.reset(2)
	require.Equal(t, seriesStateUnknown, store.lookup(boundaryRefs[0]).state)
	require.Equal(t, len(promoted)-1, store.activeLen())

	// Resetting at segment 3 removes the remaining promoted series.
	store.reset(3)
	require.Equal(t, 0, store.activeLen())

	// Exercise sparse-to-dense page migration and interior nil page re-allocation on a fresh stripe.
	const (
		stripe1Base                      = chunks.HeadSeriesRef(seriesPageSize) // stIdx = 1, pIdx = 0, slot = 0.
		pageStride                       = chunks.HeadSeriesRef(seriesPageSize * numSeriesStripes)
		gapPageIdx                       = maxDensePageGap + 1
		gapRefSlot0                      = stripe1Base + gapPageIdx*pageStride     // stIdx = 1, pIdx = 65, slot = 0.
		gapRefSlot1                      = stripe1Base + gapPageIdx*pageStride + 1 // stIdx = 1, pIdx = 65, slot = 1.
		page1Ref    chunks.HeadSeriesRef = stripe1Base + pageStride                // stIdx = 1, pIdx = 1, slot = 0.
	)
	// Stripe 1 currently has len(pages) == 0 (its pIdx=0 entry was cleared by reset(3)),
	// so gapRefSlot0 (pIdx = 65 > maxDensePageGap) is stored in stripe.sparse.
	store.storeSeries([]record.RefSeries{
		{Ref: gapRefSlot0, Labels: labels.FromStrings("__name__", "sparse_first")},
	}, 5, nil, nil)
	require.NotNil(t, store.stripes[1].sparse)
	require.Equal(t, seriesStateActive, store.lookup(gapRefSlot0).state)

	// Allocate pIdx = 1 on stripe 1 (advancing len(pages) to 2, with pages[0] == nil),
	// then write gapRefSlot1 on pIdx = 65 (now within 2 + maxDensePageGap = 66).
	// Allocating pages[65] must migrate gapRefSlot0 out of sparse so both slots remain visible.
	store.storeSeries([]record.RefSeries{
		{Ref: page1Ref, Labels: labels.FromStrings("__name__", "page_one")},
		{Ref: gapRefSlot1, Labels: labels.FromStrings("__name__", "dense_second")},
	}, 5, nil, nil)
	require.Nil(t, store.stripes[1].sparse, "sparse entry on pIdx=65 must migrate into newly allocated dense page")
	require.Equal(t, "sparse_first", store.lookup(gapRefSlot0).labels.Get("__name__"))
	require.Equal(t, "dense_second", store.lookup(gapRefSlot1).labels.Get("__name__"))

	var migDst [2]seriesEntry
	store.lookupSamples([]record.RefSample{{Ref: gapRefSlot0}, {Ref: gapRefSlot1}}, migDst[:])
	require.Equal(t, "sparse_first", migDst[0].labels.Get("__name__"))
	require.Equal(t, "dense_second", migDst[1].labels.Get("__name__"))

	// Re-allocate interior nil page (stripe 1, pIdx = 0 while len(pages) > 0) and verify noTransform keeps empty labels active.
	require.Nil(t, store.stripes[1].pages[0])
	store.storeSeries([]record.RefSeries{
		{Ref: stripe1Base, Labels: labels.EmptyLabels()},
	}, 5, nil, nil)
	require.NotNil(t, store.stripes[1].pages[0])
	require.Equal(t, seriesStateActive, store.lookup(stripe1Base).state)
}

func TestQueueManager_ConcurrentAppendBackpressureAndSeriesReset(t *testing.T) {
	t.Parallel()

	for _, maxBatch := range []int{1, 10} {
		t.Run("max_batch="+strconv.Itoa(maxBatch), func(t *testing.T) {
			t.Parallel()

			recClient := NewTestWriteClient(remoteapi.WriteV2MessageType)
			client := &MockWriteClient{
				StoreFunc: func(ctx context.Context, req []byte, attempt int) (WriteResponseStats, error) {
					// Small delay so shard queues fill and exercise drained wakeups.
					select {
					case <-time.After(time.Millisecond):
					case <-ctx.Done():
						return WriteResponseStats{}, ctx.Err()
					}
					return recClient.Store(ctx, req, attempt)
				},
				NameFunc:     recClient.Name,
				EndpointFunc: recClient.Endpoint,
			}

			cfg := config.DefaultQueueConfig
			cfg.MinShards = 4
			cfg.MaxShards = 4
			cfg.MaxSamplesPerSend = maxBatch
			cfg.Capacity = maxBatch * 2
			cfg.BatchSendDeadline = model.Duration(10 * time.Millisecond)

			qm := newTestQueueManager(t, cfg, config.DefaultMetadataConfig, 5*time.Second, client, remoteapi.WriteV2MessageType)

			const activeSeries = 200
			var initial []record.RefSeries
			var initialMeta []record.RefMetadata
			for i := range activeSeries {
				ref := chunks.HeadSeriesRef(i + 1)
				initial = append(initial, record.RefSeries{
					Ref:    ref,
					Labels: labels.FromStrings("__name__", "active_metric", "idx", strconv.Itoa(i)),
				})
				initialMeta = append(initialMeta, record.RefMetadata{
					Ref:  ref,
					Type: uint8(record.Counter),
					Help: "active counter",
				})
			}
			qm.StoreSeries(initial, 10)
			qm.StoreMetadata(initialMeta)

			qm.Start()

			// Run concurrent ephemeral series churn + SeriesReset alongside Append.
			var wg sync.WaitGroup
			stopChurn := make(chan struct{})
			wg.Go(func() {
				seg := 1
				for {
					select {
					case <-stopChurn:
						return
					default:
					}
					ephemeral := []record.RefSeries{
						{Ref: chunks.HeadSeriesRef(10_000 + seg), Labels: labels.FromStrings("__name__", "churn")},
					}
					qm.StoreSeries(ephemeral, seg)
					qm.UpdateSeriesSegment(initial, 10)
					qm.SeriesReset(seg + 1)
					seg++
					if seg >= 9 {
						seg = 1
					}
				}
			})

			samples := make([]record.RefSample, activeSeries)
			for round := range 5 {
				for i := range activeSeries {
					samples[i] = record.RefSample{
						Ref: chunks.HeadSeriesRef(i + 1),
						T:   int64(1_000_000 + round*1000 + i),
						V:   float64(round),
					}
				}
				require.True(t, qm.Append(samples))
			}

			close(stopChurn)
			wg.Wait()
			qm.Stop()

			require.Zero(t, client_testutil.ToFloat64(qm.metrics.droppedSamplesTotal.WithLabelValues(reasonUnintentionalDroppedSeries)))
			require.Zero(t, client_testutil.ToFloat64(qm.metrics.failedSamplesTotal))
			require.Zero(t, client_testutil.ToFloat64(qm.metrics.pendingSamples))
			require.Equal(t, 5*activeSeries, deepLen(recClient.receivedSamples))
			for i, s := range initial {
				got := recClient.receivedSamples[s.Labels.String()]
				require.Len(t, got, 5, "series %d (%s)", i, s.Labels)
				for round := range 5 {
					require.Equal(t, int64(1_000_000+round*1000+i), got[round].Timestamp)
					require.Equal(t, float64(round), got[round].Value)
				}
			}
		})
	}
}

func TestShards_TryEnqueueBatchPartialMultiShard(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultQueueConfig
	cfg.MinShards = 2
	cfg.MaxShards = 2
	cfg.MaxSamplesPerSend = 2
	cfg.Capacity = 2 // 1 batch of 2 in batchQueue + 1 partial batch of <2 in q.batch (max 3 buffered before blocking).

	qm := newTestQueueManager(t, cfg, config.DefaultMetadataConfig, defaultFlushDeadline, NewTestBlockedWriteClient(), remoteapi.WriteV2MessageType)
	// Populate 2 shard queues without starting background runShard consumers so we can
	// deterministically inspect partial acceptance and compaction.
	qm.shards.queues = []*queue{
		newQueue(cfg.MaxSamplesPerSend, cfg.Capacity),
		newQueue(cfg.MaxSamplesPerSend, cfg.Capacity),
	}

	// Pre-fill shard 1 (1 full batch of 2 in batchQueue + 1 item in partial batch = 3 items).
	// Next item on shard 1 will fill the partial batch to 2 and fail flushFullBatchLocked().
	require.Equal(t, 3, qm.shards.queues[1].appendShardedItems([]shardedItem{
		{ts: timeSeries{timestamp: 100}},
		{ts: timeSeries{timestamp: 101}},
		{ts: timeSeries{timestamp: 102}},
	}))

	sc := &appendScratch{
		items: []shardedItem{
			{ref: 0, ts: timeSeries{timestamp: 1000, value: 1, sType: tSample}}, // Shard 0 (even ref).
			{ref: 1, ts: timeSeries{timestamp: 5000, value: 2, sType: tSample}}, // Shard 1 (odd ref, blocked).
			{ref: 2, ts: timeSeries{timestamp: 2000, value: 3, sType: tSample}}, // Shard 0 (even ref).
			{ref: 3, ts: timeSeries{timestamp: 6000, value: 4, sType: tSample}}, // Shard 1 (odd ref, blocked).
		},
	}

	blocked, _ := qm.shards.tryEnqueueBatch(sc, tSample, 6000)
	require.Same(t, qm.shards.queues[1], blocked)
	// Only the 2 shard-0 items were accepted; the 2 shard-1 items must remain compacted in sc.items.
	require.Len(t, sc.items, 2)
	require.Equal(t, chunks.HeadSeriesRef(1), sc.items[0].ref)
	require.Equal(t, chunks.HeadSeriesRef(3), sc.items[1].ref)
	require.Equal(t, 2.0, client_testutil.ToFloat64(qm.metrics.pendingSamples))
	require.Equal(t, 2.0, qm.metrics.highestTimestamp.Get()) // 2000ms / 1000 = 2s (not 6000ms from unaccepted shard 1).

	// Drain 1 batch from shard 1 and retry: now the remaining 2 shard-1 items are accepted.
	_ = qm.shards.queues[1].Batch()
	blocked, _ = qm.shards.tryEnqueueBatch(sc, tSample, 6000)
	require.Nil(t, blocked)
	require.Empty(t, sc.items)
	require.Equal(t, 4.0, client_testutil.ToFloat64(qm.metrics.pendingSamples))
	require.Equal(t, 6.0, qm.metrics.highestTimestamp.Get()) // 6000ms / 1000 = 6s.
}
