package tiered

import (
	"math/rand"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
)

const numTestValues = 5000

func TestValuesMarshalAndUnmarshal(t *testing.T) {
	values := randomValues(numTestValues)

	marshalled := marshalValues(values)
	unmarshalled := unmarshalValues(marshalled)

	for i, expected := range values {
		actual := unmarshalled[i]
		if !actual.Equal(&expected) {
			t.Fatalf("%d. got: %v, expected: %v", i, actual, expected)
		}
	}
}

func randomValues(numSamples int) metric.Values {
	v := make(metric.Values, 0, numSamples)
	for i := 0; i < numSamples; i++ {
		v = append(v, metric.SamplePair{
			Timestamp: clientmodel.Timestamp(rand.Int63()),
			Value:     clientmodel.SampleValue(rand.NormFloat64()),
		})
	}

	return v
}

func BenchmarkMarshal(b *testing.B) {
	v := randomValues(numTestValues)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		marshalValues(v)
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	v := randomValues(numTestValues)
	marshalled := marshalValues(v)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unmarshalValues(marshalled)
	}
}
