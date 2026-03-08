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
	store.InsertVersion(1, contentHash, 0, 0, buildFull)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.InsertVersion(1, contentHash, 0, int64(i), buildFull)
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
	store.InsertVersion(0, contentHash, 0, 100, buildFull)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.InsertVersion(uint64(i+1), contentHash, 0, 100, buildFull)
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
		store.InsertVersion(uint64(i), contentHash, 0, 100, buildFull)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	bytesPerSeries := (after.HeapAlloc - before.HeapAlloc) / N
	b.ReportMetric(float64(bytesPerSeries), "bytes/series")

	// Prevent store from being GC'd before measurement.
	runtime.KeepAlive(store)
}
