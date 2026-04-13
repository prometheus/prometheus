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

package seriesmetadata

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"testing"
)

func BenchmarkInsertVersion_SameContent(b *testing.B) {
	store := NewMemStore[*ResourceVersion](ResourceOps)
	rcd := ResourceCommitData{
		Identifying: map[string]string{"service.name": "svc", "k8s.namespace.name": "ns"},
		Descriptive: map[string]string{"host.name": "host1"},
		MinTime:     0,
		MaxTime:     0,
	}
	contentHash := hashResourceCommitData(rcd)
	buildFull := func() *ResourceVersion { return buildResourceVersion(rcd) }
	// Seed first insert.
	store.InsertVersion(1, contentHash, 0, 0, buildFull, false)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.InsertVersion(1, contentHash, 0, int64(i), buildFull, false)
	}
}

func BenchmarkInsertVersion_ManySeriesSameContent(b *testing.B) {
	store := NewMemStore[*ResourceVersion](ResourceOps)
	rcd := ResourceCommitData{
		Identifying: map[string]string{"service.name": "svc", "k8s.namespace.name": "ns"},
		Descriptive: map[string]string{"host.name": "host1"},
		MinTime:     0,
		MaxTime:     100,
	}
	contentHash := hashResourceCommitData(rcd)
	buildFull := func() *ResourceVersion { return buildResourceVersion(rcd) }
	// Seed canonical with first series.
	store.InsertVersion(0, contentHash, 0, 100, buildFull, false)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.InsertVersion(uint64(i+1), contentHash, 0, 100, buildFull, false)
	}
}

func BenchmarkMemStore_MemoryPerSeries(b *testing.B) {
	const N = 100_000
	store := NewMemStore[*ResourceVersion](ResourceOps)
	rcd := ResourceCommitData{
		Identifying: map[string]string{"service.name": "svc", "k8s.namespace.name": "ns"},
		Descriptive: map[string]string{"host.name": "host1"},
		MinTime:     0,
		MaxTime:     100,
	}
	contentHash := hashResourceCommitData(rcd)
	buildFull := func() *ResourceVersion { return buildResourceVersion(rcd) }

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	for i := range N {
		store.InsertVersion(uint64(i), contentHash, 0, 100, buildFull, false)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	bytesPerSeries := (after.HeapAlloc - before.HeapAlloc) / N
	b.ReportMetric(float64(bytesPerSeries), "bytes/series")

	// Prevent store from being GC'd before measurement.
	runtime.KeepAlive(store)
}

func BenchmarkCommitResourceToStore(b *testing.B) {
	store := NewMemStore[*ResourceVersion](ResourceOps)
	rcd := ResourceCommitData{
		Identifying: map[string]string{"service.name": "svc", "k8s.namespace.name": "ns"},
		Descriptive: map[string]string{"host.name": "host1"},
		MinTime:     0,
		MaxTime:     0,
	}
	// Seed first insert.
	CommitResourceToStore(store, 1, rcd)

	b.Run("without_reuse", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rcd.MaxTime = int64(i)
			CommitResourceToStore(store, 1, rcd)
		}
	})

	b.Run("with_reuse", func(b *testing.B) {
		b.ReportAllocs()
		var keysBuf []string
		for i := 0; i < b.N; i++ {
			rcd.MaxTime = int64(i)
			_, _, _, keysBuf = CommitResourceToStoreReusable(store, 1, rcd, keysBuf)
		}
	})
}

func BenchmarkIterVersionedFlatInline(b *testing.B) {
	const numSeries = 100_000
	m := populateBenchStore(numSeries, 1_000)
	ctx := context.Background()

	b.Run("IterVersionedFlatInline", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = m.ResourceStore().IterVersionedFlatInline(ctx, func(_ uint64, versions []*ResourceVersion, _, _ int64, _ bool) error {
				for range versions {
				}
				return nil
			})
		}
	})
}

// populateBenchStore creates a MemSeriesMetadata with numSeries entries
// and numResources unique resource contents.
func populateBenchStore(numSeries, numResources int) *MemSeriesMetadata {
	m := NewMemSeriesMetadata()
	rs := m.ResourceStore()

	for i := range numSeries {
		lh := uint64(i)
		resIdx := i % numResources
		rv := &ResourceVersion{
			Identifying: map[string]string{
				"service.name":       fmt.Sprintf("svc-%d", resIdx),
				"k8s.namespace.name": "ns",
			},
			Descriptive: map[string]string{"host.name": fmt.Sprintf("host-%d", resIdx)},
			MinTime:     0,
			MaxTime:     100,
		}
		rs.Set(lh, rv)
	}
	return m
}

func BenchmarkWriteFileWithOptions(b *testing.B) {
	const numSeries = 100_000
	m := populateBenchStore(numSeries, 1_000)
	dir := b.TempDir()
	logger := slog.New(slog.DiscardHandler)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := WriteFileWithOptions(logger, dir, m, WriterOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMergeAndWriteSeriesMetadata(b *testing.B) {
	const numSeries = 100_000
	m := populateBenchStore(numSeries, 1_000)
	dir := b.TempDir()
	logger := slog.New(slog.DiscardHandler)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Build needsResolve hashes via IterHashes.
		needsResolve := make(map[uint64]struct{}, numSeries)
		for _, kind := range AllKinds() {
			_ = m.IterHashes(context.Background(), kind.ID(), func(labelsHash uint64) error {
				needsResolve[labelsHash] = struct{}{}
				return nil
			})
		}

		// Build a simple identity ref resolver from needsResolve.
		labelsHashToRef := make(map[uint64]uint64, len(needsResolve))
		for lh := range needsResolve {
			labelsHashToRef[lh] = lh // identity mapping for benchmark
		}

		_, err := WriteFileWithOptions(logger, dir, m, WriterOptions{
			RefResolver: func(labelsHash uint64) (uint64, bool) {
				ref, ok := labelsHashToRef[labelsHash]
				return ref, ok
			},
			HashFilter: func(lh uint64) bool {
				_, ok := needsResolve[lh]
				return ok
			},
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}
