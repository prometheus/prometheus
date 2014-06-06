package storage_ng

import (
	"io/ioutil"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

func TestIndexPersistence(t *testing.T) {
	expected := Indexes{
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
				},
			},
		},
		LabelPairToFingerprints: map[metric.LabelPair]utility.Set{
			metric.LabelPair{
				Name:  clientmodel.MetricNameLabel,
				Value: "metric_0",
			}: {
				0: struct{}{},
				1: struct{}{},
			},
			metric.LabelPair{
				Name:  "label_1",
				Value: "value_1",
			}: {
				0: struct{}{},
			},
			metric.LabelPair{
				Name:  "label_2",
				Value: "value_2",
			}: {
				1: struct{}{},
			},
		},
		LabelNameToLabelValues: map[clientmodel.LabelName]utility.Set{
			clientmodel.MetricNameLabel: {
				clientmodel.LabelValue("metric_0"): struct{}{},
			},
			"label_1": {
				clientmodel.LabelValue("value_1"): struct{}{},
			},
			"label_2": {
				clientmodel.LabelValue("value_2"): struct{}{},
			},
		},
	}

	basePath, err := ioutil.TempDir("", "test_index_persistence")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewDiskPersistence(basePath, 1024)
	if err != nil {
		t.Fatal(err)
	}
	p.PersistIndexes(&expected)

	actual, err := p.LoadIndexes()
	if err != nil {
		t.Fatal(err)
	}

	if len(actual.FingerprintToSeries) != len(expected.FingerprintToSeries) {
		t.Fatalf("Count mismatch: Got %d; want %d", len(actual.FingerprintToSeries), len(expected.FingerprintToSeries))
	}
	for fp, actualSeries := range actual.FingerprintToSeries {
		expectedSeries := expected.FingerprintToSeries[fp]
		if !expectedSeries.metric.Equal(actualSeries.metric) {
			t.Fatalf("%s: Got %s; want %s", fp, actualSeries.metric, expectedSeries.metric)
		}
	}

	if len(actual.LabelPairToFingerprints) != len(expected.LabelPairToFingerprints) {
		t.Fatalf("Count mismatch: Got %d; want %d", len(actual.LabelPairToFingerprints), len(expected.LabelPairToFingerprints))
	}
	for lp, actualFps := range actual.LabelPairToFingerprints {
		expectedFps := expected.LabelPairToFingerprints[lp]
		if len(actualFps) != len(actualFps.Intersection(expectedFps)) {
			t.Fatalf("%s: Got %s; want %s", lp, actualFps, expectedFps)
		}
	}

	if len(actual.LabelNameToLabelValues) != len(expected.LabelNameToLabelValues) {
		t.Fatalf("Count mismatch: Got %d; want %d", len(actual.LabelNameToLabelValues), len(expected.LabelNameToLabelValues))
	}
	for name, actualVals := range actual.LabelNameToLabelValues {
		expectedVals := expected.LabelNameToLabelValues[name]
		if len(actualVals) != len(actualVals.Intersection(expectedVals)) {
			t.Fatalf("%s: Got %s; want %s", name, actualVals, expectedVals)
		}
	}
}
