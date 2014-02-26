package metric

import (
	"math/rand"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"
)

func TestValuesMarshalAndUnmarshal(t *testing.T) {
	values := Values{
		&SamplePair{
			Timestamp: clientmodel.TimestampFromUnix(1),
			Value:     clientmodel.SampleValue(1.23),
		},
		&SamplePair{
			Timestamp: clientmodel.TimestampFromUnix(-1),
			Value:     clientmodel.SampleValue(3.21),
		},
		&SamplePair{
			Timestamp: clientmodel.TimestampFromUnix(1000),
			Value:     clientmodel.SampleValue(1000),
		},
	}

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
	v := randomValues(5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.marshal()
	}
}

func BenchmarkUnmarshal(b *testing.B) {
	v := randomValues(5000)
	marshalled := v.marshal()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		unmarshalValues(marshalled)
	}
}
