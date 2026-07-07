package scrape

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

func genHistogramSeries(nBuckets int) [][]byte {
	buckets := []string{"0.05", "0.1", "0.2", "0.5", "1", "2.5", "5", "10", "+Inf"}
	var out [][]byte
	for i := range nBuckets {
		le := buckets[i%len(buckets)]
		buf := fmt.Appendf(
			nil,
			`http_request_duration_seconds_bucket{method="GET",path="/api/v%d/resource",le="%s"}`,
			i%5,
			le,
		)
		out = append(out, buf)
	}
	return out
}

func genKubeStateStyleSeries(nDeployments int) [][]byte {
	conditions := []string{"Progressing", "Available", "ReplicaFailure"}
	statuses := []string{"true", "false", "unknown"}
	var out [][]byte
	for i := range nDeployments {
		ns := fmt.Sprintf("redacted-namespace-%d", i%50) // 50 distinct namespaces reused
		dep := fmt.Sprintf("redacted-deployment-name-very-interesting-%d", i)
		for _, cond := range conditions {
			for _, status := range statuses {
				buf := fmt.Appendf(
					nil,
					`kube_deployment_status_condition{namespace=%q,deployment=%q,condition=%q,status=%q}`,
					ns,
					dep,
					cond,
					status,
				)
				out = append(out, buf)
			}
		}
	}
	return out
}

func BenchmarkScrapeCacheMemory(b *testing.B) {
	sizes := []int{1_000, 10_000, 100_000}
	for _, n := range sizes {
		series := genKubeStateStyleSeries(n / 9) // ~9 lines generated per deployment
		b.Run(fmt.Sprintf("trie/n=%d", len(series)), func(b *testing.B) {
			benchmarkCacheMemory(b, series, true)
		})
	}
}

func benchmarkCacheMemory(b *testing.B, series [][]byte, useTrie bool) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		runtime.GC()
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)

		if useTrie {
			cache := newScrapeCache(nil)
			lset := labels.FromStrings("job", "test")
			for j, met := range series {
				cache.addRef(met, storage.SeriesRef(j+1), lset, uint64(j))
			}
			runtime.GC()
			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)
			bytesPerSeries := float64(m2.HeapAlloc-m1.HeapAlloc) / float64(len(series))
			b.ReportMetric(bytesPerSeries, "B/series")
			runtime.KeepAlive(cache)
		} else {
			m := make(map[string]*cacheEntry, len(series))
			lset := labels.FromStrings("job", "test")
			for j, met := range series {
				m[string(met)] = &cacheEntry{ref: storage.SeriesRef(j + 1), lset: lset, hash: uint64(j)}
			}
			runtime.GC()
			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)
			bytesPerSeries := float64(m2.HeapAlloc-m1.HeapAlloc) / float64(len(series))
			b.ReportMetric(bytesPerSeries, "B/series")
			runtime.KeepAlive(m)
		}
	}
}

func BenchmarkScrapeCacheGet(b *testing.B) {
	series := genHistogramSeries(5_000)
	cache := newScrapeCache(nil)
	lset := labels.FromStrings("job", "test", "instance", "localhost:9090")
	for i, met := range series {
		cache.addRef(met, storage.SeriesRef(i+1), lset, uint64(i))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.get(series[i%len(series)])
	}
}

func BenchmarkScrapeCacheAddRef(b *testing.B) {
	series := genHistogramSeries(5_000)
	lset := labels.FromStrings("job", "test", "instance", "localhost:9090")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache := newScrapeCache(nil) // fresh cache per op: AddRef is a first-insert path
		for j, met := range series {
			cache.addRef(met, storage.SeriesRef(j+1), lset, uint64(j))
		}
	}
}
