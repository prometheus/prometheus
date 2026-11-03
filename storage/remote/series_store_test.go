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
	"math"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

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

	deleted := store.reset(2, true)
	// Deleted should contain the unpromoted boundaryRefs + densePreMetaDrop, but NOT denseMetaOnly (no segment).
	expectedDeleted := len(boundaryRefs) - len(promoted) + 1
	require.Len(t, deleted, expectedDeleted)
	require.Equal(t, len(promoted), store.activeLen())
	require.Equal(t, seriesStateMetadataOnly, store.lookup(denseMetaOnly).state)

	// Resetting at segment 3 removes the remaining promoted series.
	store.reset(3, false)
	require.Equal(t, 0, store.activeLen())
}
