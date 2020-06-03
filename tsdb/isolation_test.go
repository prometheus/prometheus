package tsdb

import (
	"sync"
	"testing"
)

func TestIsolation(t *testing.T) {
}

func BenchmarkIsolation_10(b *testing.B) {
	benchmarkIsolation(b, 10)
}

func BenchmarkIsolation_100(b *testing.B) {
	benchmarkIsolation(b, 100)
}

func BenchmarkIsolation_1000(b *testing.B) {
	benchmarkIsolation(b, 1000)
}

func BenchmarkIsolation_10000(b *testing.B) {
	benchmarkIsolation(b, 10000)
}

func benchmarkIsolation(b *testing.B, goroutines int) {
	iso := newIsolation()

	wg := sync.WaitGroup{}
	start := make(chan struct{})

	for g := 0; g < goroutines; g++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			for i := 0; i < b.N; i++ {
				appendID := iso.newAppendID()
				_ = iso.lowWatermark()

				iso.closeAppend(appendID)
			}
		}()
	}

	b.ResetTimer()
	close(start)
	wg.Wait()
}
