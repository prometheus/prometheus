package promql

// Test is a sequence of read and write commands that are run
import "testing"

// Benchmark runs a test multiple times as a built in benchmark
type Benchmark struct {
	b *testing.B
	t *Test
}

// NewBenchmark returns an initilized empty Benchmark
func NewBenchmark(b *testing.B, input string) *Benchmark {
	t, err := NewTest(b, input)
	if err != nil {
		b.Fatalf("Unable to run benchmark: %s", err)
	}
	return &Benchmark{
		b: b,
		t: t,
	}
}

// Run the benchmark a bunch of times
func (b *Benchmark) Run() {
	b.b.ReportAllocs()
	b.b.ResetTimer()
	for i := 0; i < b.b.N; i++ {
		b.t.RunAsBenchmark(b.b)
	}
}
