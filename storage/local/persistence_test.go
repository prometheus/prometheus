package storage_ng

import (
	"io/ioutil"
	"os"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

var (
	indexes = Indexes{
		FingerprintToSeries: map[clientmodel.Fingerprint]*memorySeries{
			0: {
				metric: clientmodel.Metric{
					clientmodel.MetricNameLabel: "metric_0",
					"label_1":                   "value_1",
				},
			},
			1: {
				metric: clientmodel.Metric{
					clientmodel.MetricNameLabel: "metric_0",
					"label_2":                   "value_2",
					"label_3":                   "value_3",
				},
			},
			2: {
				metric: clientmodel.Metric{
					clientmodel.MetricNameLabel: "metric_1",
					"label_1":                   "value_2",
				},
			},
		},
		LabelPairToFingerprints: map[metric.LabelPair]utility.Set{
			metric.LabelPair{
				Name:  clientmodel.MetricNameLabel,
				Value: "metric_0",
			}: {
				clientmodel.Fingerprint(0): struct{}{},
				clientmodel.Fingerprint(1): struct{}{},
			},
			metric.LabelPair{
				Name:  clientmodel.MetricNameLabel,
				Value: "metric_1",
			}: {
				clientmodel.Fingerprint(2): struct{}{},
			},
			metric.LabelPair{
				Name:  "label_1",
				Value: "value_1",
			}: {
				clientmodel.Fingerprint(0): struct{}{},
			},
			metric.LabelPair{
				Name:  "label_1",
				Value: "value_2",
			}: {
				clientmodel.Fingerprint(2): struct{}{},
			},
			metric.LabelPair{
				Name:  "label_2",
				Value: "value_2",
			}: {
				clientmodel.Fingerprint(1): struct{}{},
			},
			metric.LabelPair{
				Name:  "label_3",
				Value: "value_2",
			}: {
				clientmodel.Fingerprint(1): struct{}{},
			},
		},
		LabelNameToLabelValues: map[clientmodel.LabelName]utility.Set{
			clientmodel.MetricNameLabel: {
				clientmodel.LabelValue("metric_0"): struct{}{},
				clientmodel.LabelValue("metric_1"): struct{}{},
			},
			"label_1": {
				clientmodel.LabelValue("value_1"): struct{}{},
				clientmodel.LabelValue("value_2"): struct{}{},
			},
			"label_2": {
				clientmodel.LabelValue("value_2"): struct{}{},
			},
			"label_3": {
				clientmodel.LabelValue("value_3"): struct{}{},
			},
		},
	}
)

func TestIndexPersistence(t *testing.T) {
	basePath, err := ioutil.TempDir("", "test_index_persistence")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(basePath)
	p, err := NewDiskPersistence(basePath, 1024)
	if err != nil {
		t.Fatal(err)
	}

	if err := p.PersistIndexes(&indexes); err != nil {
		t.Fatal(err)
	}

	actual, err := p.LoadIndexes()
	if err != nil {
		t.Fatal(err)
	}

	if len(actual.FingerprintToSeries) != len(indexes.FingerprintToSeries) {
		t.Fatalf("Count mismatch: Got %d; want %d", len(actual.FingerprintToSeries), len(indexes.FingerprintToSeries))
	}
	for fp, actualSeries := range actual.FingerprintToSeries {
		expectedSeries := indexes.FingerprintToSeries[fp]
		if !expectedSeries.metric.Equal(actualSeries.metric) {
			t.Fatalf("%s: Got %s; want %s", fp, actualSeries.metric, expectedSeries.metric)
		}
	}

	if len(actual.LabelPairToFingerprints) != len(indexes.LabelPairToFingerprints) {
		t.Fatalf("Count mismatch: Got %d; want %d", len(actual.LabelPairToFingerprints), len(indexes.LabelPairToFingerprints))
	}
	for lp, actualFps := range actual.LabelPairToFingerprints {
		expectedFps := indexes.LabelPairToFingerprints[lp]
		if len(actualFps) != len(actualFps.Intersection(expectedFps)) {
			t.Fatalf("%s: Got %s; want %s", lp, actualFps, expectedFps)
		}
	}

	if len(actual.LabelNameToLabelValues) != len(indexes.LabelNameToLabelValues) {
		t.Fatalf("Count mismatch: Got %d; want %d", len(actual.LabelNameToLabelValues), len(indexes.LabelNameToLabelValues))
	}
	for name, actualVals := range actual.LabelNameToLabelValues {
		expectedVals := indexes.LabelNameToLabelValues[name]
		if len(actualVals) != len(actualVals.Intersection(expectedVals)) {
			t.Fatalf("%s: Got %s; want %s", name, actualVals, expectedVals)
		}
	}
}

func BenchmarkPersistIndexes(b *testing.B) {
	basePath, err := ioutil.TempDir("", "test_index_persistence")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(basePath)
	p, err := NewDiskPersistence(basePath, 1024)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.PersistIndexes(&indexes); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

func BenchmarkLoadIndexes(b *testing.B) {
	basePath, err := ioutil.TempDir("", "test_index_persistence")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(basePath)
	p, err := NewDiskPersistence(basePath, 1024)
	if err != nil {
		b.Fatal(err)
	}
	if err := p.PersistIndexes(&indexes); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := p.LoadIndexes()
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}
