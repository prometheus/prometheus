package index

import (
	"sort"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
)

type incrementalBatch struct {
	fpToMetric      FingerprintMetricMapping
	expectedLnToLvs LabelNameLabelValuesMapping
	expectedLpToFps LabelPairFingerprintsMapping
}

func newTestDB(t *testing.T) (KeyValueStore, test.Closer) {
	dir := test.NewTemporaryDirectory("test_db", t)
	db, err := NewLevelDB(LevelDBOptions{
		Path: dir.Path(),
	})
	if err != nil {
		dir.Close()
		t.Fatal("failed to create test DB: ", err)
	}
	return db, test.NewCallbackCloser(func() {
		db.Close()
		dir.Close()
	})
}

func verifyIndexedState(i int, t *testing.T, b incrementalBatch, indexedFpsToMetrics FingerprintMetricMapping, indexer *DiskIndexer) {
	for fp, m := range indexedFpsToMetrics {
		// Compare indexed metrics with input metrics.
		mOut, ok, err := indexer.FingerprintToMetric.Lookup(fp)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%d. fingerprint %v not found", i, fp)
		}
		if !mOut.Equal(m) {
			t.Fatalf("%d. %v: Got: %s; want %s", i, fp, mOut, m)
		}

		// Check that indexed metrics are in membership index.
		ok, err = indexer.FingerprintMembership.Has(fp)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%d. fingerprint %v not found", i, fp)
		}
	}

	// Compare label name -> label values mappings.
	for ln, lvs := range b.expectedLnToLvs {
		outLvs, ok, err := indexer.LabelNameToLabelValues.Lookup(ln)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%d. label name %s not found", i, ln)
		}

		sort.Sort(lvs)
		sort.Sort(outLvs)

		if len(lvs) != len(outLvs) {
			t.Fatalf("%d. different number of label values. Got: %d; want %d", i, len(outLvs), len(lvs))
		}
		for j, _ := range lvs {
			if lvs[j] != outLvs[j] {
				t.Fatalf("%d.%d. label values don't match. Got: %s; want %s", i, j, outLvs[j], lvs[j])
			}
		}
	}

	// Compare label pair -> fingerprints mappings.
	for lp, fps := range b.expectedLpToFps {
		outFps, ok, err := indexer.LabelPairToFingerprints.Lookup(&lp)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%d. label pair %v not found", i, lp)
		}

		sort.Sort(fps)
		sort.Sort(outFps)

		if len(fps) != len(outFps) {
			t.Fatalf("%d. %v: different number of fingerprints. Got: %d; want %d", i, lp, len(outFps), len(fps))
		}
		for j, _ := range fps {
			if fps[j] != outFps[j] {
				t.Fatalf("%d.%d. %v: fingerprints don't match. Got: %d; want %d", i, j, lp, outFps[j], fps[j])
			}
		}
	}
}

func TestIndexing(t *testing.T) {
	batches := []incrementalBatch{
		{
			fpToMetric: FingerprintMetricMapping{
				0: {
					clientmodel.MetricNameLabel: "metric_0",
					"label_1":                   "value_1",
				},
				1: {
					clientmodel.MetricNameLabel: "metric_0",
					"label_2":                   "value_2",
					"label_3":                   "value_3",
				},
				2: {
					clientmodel.MetricNameLabel: "metric_1",
					"label_1":                   "value_2",
				},
			},
			expectedLnToLvs: LabelNameLabelValuesMapping{
				clientmodel.MetricNameLabel: clientmodel.LabelValues{"metric_0", "metric_1"},
				"label_1":                   clientmodel.LabelValues{"value_1", "value_2"},
				"label_2":                   clientmodel.LabelValues{"value_2"},
				"label_3":                   clientmodel.LabelValues{"value_3"},
			},
			expectedLpToFps: LabelPairFingerprintsMapping{
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_0",
				}: {0, 1},
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_1",
				}: {2},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_1",
				}: {0},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_2",
				}: {2},
				metric.LabelPair{
					Name:  "label_2",
					Value: "value_2",
				}: {1},
				metric.LabelPair{
					Name:  "label_3",
					Value: "value_3",
				}: {1},
			},
		}, {
			fpToMetric: FingerprintMetricMapping{
				3: {
					clientmodel.MetricNameLabel: "metric_0",
					"label_1":                   "value_3",
				},
				4: {
					clientmodel.MetricNameLabel: "metric_2",
					"label_2":                   "value_2",
					"label_3":                   "value_1",
				},
				5: {
					clientmodel.MetricNameLabel: "metric_1",
					"label_1":                   "value_3",
				},
			},
			expectedLnToLvs: LabelNameLabelValuesMapping{
				clientmodel.MetricNameLabel: clientmodel.LabelValues{"metric_0", "metric_1", "metric_2"},
				"label_1":                   clientmodel.LabelValues{"value_1", "value_2", "value_3"},
				"label_2":                   clientmodel.LabelValues{"value_2"},
				"label_3":                   clientmodel.LabelValues{"value_1", "value_3"},
			},
			expectedLpToFps: LabelPairFingerprintsMapping{
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_0",
				}: {0, 1, 3},
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_1",
				}: {2, 5},
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_2",
				}: {4},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_1",
				}: {0},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_2",
				}: {2},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_3",
				}: {3, 5},
				metric.LabelPair{
					Name:  "label_2",
					Value: "value_2",
				}: {1, 4},
				metric.LabelPair{
					Name:  "label_3",
					Value: "value_1",
				}: {4},
				metric.LabelPair{
					Name:  "label_3",
					Value: "value_3",
				}: {1},
			},
		},
	}

	fpToMetricDB, fpToMetricCloser := newTestDB(t)
	defer fpToMetricCloser.Close()
	lnToLvsDB, lnToLvsCloser := newTestDB(t)
	defer lnToLvsCloser.Close()
	lpToFpDB, lpToFpCloser := newTestDB(t)
	defer lpToFpCloser.Close()
	fpMsDB, fpMsCloser := newTestDB(t)
	defer fpMsCloser.Close()

	indexer := DiskIndexer{
		FingerprintToMetric:     NewFingerprintMetricIndex(fpToMetricDB),
		LabelNameToLabelValues:  NewLabelNameLabelValuesIndex(lnToLvsDB),
		LabelPairToFingerprints: NewLabelPairFingerprintIndex(lpToFpDB),
		FingerprintMembership:   NewFingerprintMembershipIndex(fpMsDB),
	}

	indexedFpsToMetrics := FingerprintMetricMapping{}
	for i, b := range batches {
		indexer.IndexMetrics(b.fpToMetric)
		for fp, m := range b.fpToMetric {
			indexedFpsToMetrics[fp] = m
		}

		verifyIndexedState(i, t, b, indexedFpsToMetrics, &indexer)
	}

	for i := len(batches) - 1; i >= 0; i-- {
		b := batches[i]
		verifyIndexedState(i, t, batches[i], indexedFpsToMetrics, &indexer)
		indexer.UnindexMetrics(b.fpToMetric)
		for fp, _ := range b.fpToMetric {
			delete(indexedFpsToMetrics, fp)
		}
	}
}
