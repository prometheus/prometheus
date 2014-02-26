package metric

import (
	"math/rand"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"
)

const numTestValues = 5000

func TestValuesMarshalAndUnmarshal(t *testing.T) {
	values := randomValues(numTestValues)

	marshalled := values.marshal()
	unmarshalled := unmarshalValues(marshalled)

	for i, expected := range values {
		actual := unmarshalled[i]
		if !actual.Equal(expected) {
			t.Fatalf("%d. got: %v, expected: %v", i, actual, expected)
		}
	}
}

func randomValues(numSamples int) Values {
	v := make(Values, 0, numSamples)
	for i := 0; i < numSamples; i++ {
		v = append(v, &SamplePair{
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
		v.marshal()
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	v := randomValues(numTestValues)
	marshalled := v.marshal()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unmarshalValues(marshalled)
	}
}
