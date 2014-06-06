package storage_ng

import (
	"fmt"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
)

func TestChunk(t *testing.T) {
	samples := make(clientmodel.Samples, 500000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(i),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t)
	defer closer.Close()

	s.AppendSamples(samples)

	for _, s := range s.(*memorySeriesStorage).fingerprintToSeries {
		for i, v := range s.values() {
			if samples[i].Timestamp != v.Timestamp {
				t.Fatalf("%d. Got %v; want %v", i, v.Timestamp, samples[i].Timestamp)
			}
			if samples[i].Value != v.Value {
				t.Fatalf("%d. Got %v; want %v", i, v.Value, samples[i].Value)
			}
		}
	}
}

func TestGetValueAtTime(t *testing.T) {
	samples := make(clientmodel.Samples, 50000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(time.Duration(i) * time.Second),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t)
	defer closer.Close()

	s.AppendSamples(samples)

	fp := clientmodel.Metric{}.Fingerprint()

	it := s.NewIterator(fp)

	for i, expected := range samples {
		actual := it.GetValueAtTime(samples[i].Timestamp)

		if expected.Timestamp != actual[0].Timestamp {
			t.Fatalf("%d. Got %v; want %v", i, actual[0].Timestamp, expected.Timestamp)
		}
		if expected.Value != actual[0].Value {
			t.Fatalf("%d. Got %v; want %v", i, actual[0].Value, expected.Value)
		}
	}
}

func TestGetRangeValues(t *testing.T) {
	samples := make(clientmodel.Samples, 50000)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Timestamp: clientmodel.Timestamp(time.Duration(i) * time.Second),
			Value:     clientmodel.SampleValue(float64(i) * 0.2),
		}
	}
	s, closer := NewTestStorage(t)
	defer closer.Close()

	s.AppendSamples(samples)

	fp := clientmodel.Metric{}.Fingerprint()

	it := s.NewIterator(fp)

	for i, expected := range samples {
		actual := it.GetValueAtTime(samples[i].Timestamp)

		if expected.Timestamp != actual[0].Timestamp {
			t.Fatalf("%d. Got %v; want %v", i, actual[0].Timestamp, expected.Timestamp)
		}
		if expected.Value != actual[0].Value {
			t.Fatalf("%d. Got %v; want %v", i, actual[0].Value, expected.Value)
		}
	}
}

func BenchmarkAppend(b *testing.B) {
	samples := make(clientmodel.Samples, b.N)
	for i := range samples {
		samples[i] = &clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: clientmodel.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
				"label1":                    clientmodel.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
				"label2":                    clientmodel.LabelValue(fmt.Sprintf("test_metric_%d", i%10)),
			},
			Timestamp: clientmodel.Timestamp(i),
			Value:     clientmodel.SampleValue(i),
		}
	}
	b.ResetTimer()
	s, closer := NewTestStorage(b)
	defer closer.Close()

	s.AppendSamples(samples)
}
