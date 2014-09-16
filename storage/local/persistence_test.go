package local

import (
	"sort"
	"testing"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/local/index"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility/test"
)

func newTestPersistence(t *testing.T) (Persistence, test.Closer) {
	dir := test.NewTemporaryDirectory("test_persistence", t)
	p, err := NewDiskPersistence(dir.Path(), 1024)
	if err != nil {
		dir.Close()
		t.Fatal(err)
	}
	return p, test.NewCallbackCloser(func() {
		p.Close()
		dir.Close()
	})
}

func buildTestChunks() map[clientmodel.Fingerprint]chunks {
	fps := clientmodel.Fingerprints{
		clientmodel.Metric{
			"label": "value1",
		}.Fingerprint(),
		clientmodel.Metric{
			"label": "value2",
		}.Fingerprint(),
		clientmodel.Metric{
			"label": "value3",
		}.Fingerprint(),
	}
	fpToChunks := map[clientmodel.Fingerprint]chunks{}

	for _, fp := range fps {
		fpToChunks[fp] = make(chunks, 0, 10)
		for i := 0; i < 10; i++ {
			fpToChunks[fp] = append(fpToChunks[fp], newDeltaEncodedChunk(d1, d1, true).add(&metric.SamplePair{
				Timestamp: clientmodel.Timestamp(i),
				Value:     clientmodel.SampleValue(fp),
			})[0])
		}
	}
	return fpToChunks
}

func chunksEqual(c1, c2 chunk) bool {
	values2 := c2.values()
	for v1 := range c1.values() {
		v2 := <-values2
		if !v1.Equal(v2) {
			return false
		}
	}
	return true
}

func TestPersistChunk(t *testing.T) {
	p, closer := newTestPersistence(t)
	defer closer.Close()

	fpToChunks := buildTestChunks()

	for fp, chunks := range fpToChunks {
		for _, c := range chunks {
			if err := p.PersistChunk(fp, c); err != nil {
				t.Fatal(err)
			}
		}
	}

	for fp, expectedChunks := range fpToChunks {
		indexes := make([]int, 0, len(expectedChunks))
		for i := range expectedChunks {
			indexes = append(indexes, i)
		}
		actualChunks, err := p.LoadChunks(fp, indexes)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range indexes {
			if !chunksEqual(expectedChunks[i], actualChunks[i]) {
				t.Fatalf("%d. Chunks not equal.", i)
			}
		}
	}
}

type incrementalBatch struct {
	fpToMetric      index.FingerprintMetricMapping
	expectedLnToLvs index.LabelNameLabelValuesMapping
	expectedLpToFps index.LabelPairFingerprintsMapping
}

func TestIndexing(t *testing.T) {
	batches := []incrementalBatch{
		{
			fpToMetric: index.FingerprintMetricMapping{
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
			expectedLnToLvs: index.LabelNameLabelValuesMapping{
				clientmodel.MetricNameLabel: clientmodel.LabelValues{"metric_0", "metric_1"},
				"label_1":                   clientmodel.LabelValues{"value_1", "value_2"},
				"label_2":                   clientmodel.LabelValues{"value_2"},
				"label_3":                   clientmodel.LabelValues{"value_3"},
			},
			expectedLpToFps: index.LabelPairFingerprintsMapping{
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
			fpToMetric: index.FingerprintMetricMapping{
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
			expectedLnToLvs: index.LabelNameLabelValuesMapping{
				clientmodel.MetricNameLabel: clientmodel.LabelValues{"metric_0", "metric_1", "metric_2"},
				"label_1":                   clientmodel.LabelValues{"value_1", "value_2", "value_3"},
				"label_2":                   clientmodel.LabelValues{"value_2"},
				"label_3":                   clientmodel.LabelValues{"value_1", "value_3"},
			},
			expectedLpToFps: index.LabelPairFingerprintsMapping{
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

	p, closer := newTestPersistence(t)
	defer closer.Close()

	indexedFpsToMetrics := index.FingerprintMetricMapping{}
	for i, b := range batches {
		for fp, m := range b.fpToMetric {
			if err := p.IndexMetric(m, fp); err != nil {
				t.Fatal(err)
			}
			if err := p.ArchiveMetric(fp, m, 1, 2); err != nil {
				t.Fatal(err)
			}
			indexedFpsToMetrics[fp] = m
		}
		verifyIndexedState(i, t, b, indexedFpsToMetrics, p.(*diskPersistence))
	}

	for i := len(batches) - 1; i >= 0; i-- {
		b := batches[i]
		verifyIndexedState(i, t, batches[i], indexedFpsToMetrics, p.(*diskPersistence))
		for fp, m := range b.fpToMetric {
			if err := p.UnindexMetric(m, fp); err != nil {
				t.Fatal(err)
			}
			unarchived, err := p.UnarchiveMetric(fp)
			if err != nil {
				t.Fatal(err)
			}
			if !unarchived {
				t.Errorf("%d. metric not unarchived", i)
			}
			delete(indexedFpsToMetrics, fp)
		}
	}
}

func verifyIndexedState(i int, t *testing.T, b incrementalBatch, indexedFpsToMetrics index.FingerprintMetricMapping, p *diskPersistence) {
	for fp, m := range indexedFpsToMetrics {
		// Compare archived metrics with input metrics.
		mOut, err := p.GetArchivedMetric(fp)
		if err != nil {
			t.Fatal(err)
		}
		if !mOut.Equal(m) {
			t.Errorf("%d. %v: Got: %s; want %s", i, fp, mOut, m)
		}

		// Check that archived metrics are in membership index.
		has, first, last, err := p.HasArchivedMetric(fp)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Errorf("%d. fingerprint %v not found", i, fp)
		}
		if first != 1 || last != 2 {
			t.Errorf(
				"%d. %v: Got first: %d, last %d; want first: %d, last %d",
				i, fp, first, last, 1, 2,
			)
		}
	}

	// Compare label name -> label values mappings.
	for ln, lvs := range b.expectedLnToLvs {
		outLvs, err := p.GetLabelValuesForLabelName(ln)
		if err != nil {
			t.Fatal(err)
		}

		sort.Sort(lvs)
		sort.Sort(outLvs)

		if len(lvs) != len(outLvs) {
			t.Errorf("%d. different number of label values. Got: %d; want %d", i, len(outLvs), len(lvs))
		}
		for j := range lvs {
			if lvs[j] != outLvs[j] {
				t.Errorf("%d.%d. label values don't match. Got: %s; want %s", i, j, outLvs[j], lvs[j])
			}
		}
	}

	// Compare label pair -> fingerprints mappings.
	for lp, fps := range b.expectedLpToFps {
		outFps, err := p.GetFingerprintsForLabelPair(lp)
		if err != nil {
			t.Fatal(err)
		}

		sort.Sort(fps)
		sort.Sort(outFps)

		if len(fps) != len(outFps) {
			t.Errorf("%d. %v: different number of fingerprints. Got: %d; want %d", i, lp, len(outFps), len(fps))
		}
		for j := range fps {
			if fps[j] != outFps[j] {
				t.Errorf("%d.%d. %v: fingerprints don't match. Got: %d; want %d", i, j, lp, outFps[j], fps[j])
			}
		}
	}
}
