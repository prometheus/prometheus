package metric

import (
	"math/rand"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"
)

const numTestValues = 5000

func TestValuesMarshalAndUnmarshal(t *testing.T) {
	values := randomValues(numTestValues)

	marshalled := values.marshal(nil)
	unmarshalled := unmarshalValues(marshalled, nil)

	for i, expected := range values {
		actual := unmarshalled[i]
		if !actual.Equal(&expected) {
			t.Fatalf("%d. got: %v, expected: %v", i, actual, expected)
		}
	}
}

func randomValues(numSamples int) Values {
	v := make(Values, 0, numSamples)
	for i := 0; i < numSamples; i++ {
		v = append(v, SamplePair{
			Timestamp: clientmodel.Timestamp(rand.Int63()),
			Value:     clientmodel.SampleValue(rand.NormFloat64()),
		})
	}

	return v
}

func benchmarkMarshal(b *testing.B, n int) {
	v := randomValues(n)
	b.ResetTimer()

	// TODO: Reuse buffer to compare performance.
	//       - Delta is -30 percent time overhead.
	for i := 0; i < b.N; i++ {
		v.marshal(nil)
	}
}

func BenchmarkMarshal1(b *testing.B) {
	benchmarkMarshal(b, 1)
}

func BenchmarkMarshal10(b *testing.B) {
	benchmarkMarshal(b, 10)
}

func BenchmarkMarshal100(b *testing.B) {
	benchmarkMarshal(b, 100)
}

func BenchmarkMarshal1000(b *testing.B) {
	benchmarkMarshal(b, 1000)
}

func BenchmarkMarshal10000(b *testing.B) {
	benchmarkMarshal(b, 10000)
}

func benchmarkUnmarshal(b *testing.B, n int) {
	v := randomValues(numTestValues)
	marshalled := v.marshal(nil)
	b.ResetTimer()

	// TODO: Reuse buffer to compare performance.
	//       - Delta is -15 percent time overhead.
	for i := 0; i < b.N; i++ {
		unmarshalValues(marshalled, nil)
	}
}

func BenchmarkUnmarshal1(b *testing.B) {
	benchmarkUnmarshal(b, 1)
}

func BenchmarkUnmarshal10(b *testing.B) {
	benchmarkUnmarshal(b, 10)
}

func BenchmarkUnmarshal100(b *testing.B) {
	benchmarkUnmarshal(b, 100)
}

func BenchmarkUnmarshal1000(b *testing.B) {
	benchmarkUnmarshal(b, 1000)
}

func BenchmarkUnmarshal10000(b *testing.B) {
	benchmarkUnmarshal(b, 10000)
}
