// Copyright 2014 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package local

import (
	"reflect"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/local/codable"
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
				clientmodel.MetricNameLabel: codable.LabelValueSet{
					"metric_0": struct{}{},
					"metric_1": struct{}{},
				},
				"label_1": codable.LabelValueSet{
					"value_1": struct{}{},
					"value_2": struct{}{},
				},
				"label_2": codable.LabelValueSet{
					"value_2": struct{}{},
				},
				"label_3": codable.LabelValueSet{
					"value_3": struct{}{},
				},
			},
			expectedLpToFps: index.LabelPairFingerprintsMapping{
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_0",
				}: codable.FingerprintSet{0: struct{}{}, 1: struct{}{}},
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_1",
				}: codable.FingerprintSet{2: struct{}{}},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_1",
				}: codable.FingerprintSet{0: struct{}{}},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_2",
				}: codable.FingerprintSet{2: struct{}{}},
				metric.LabelPair{
					Name:  "label_2",
					Value: "value_2",
				}: codable.FingerprintSet{1: struct{}{}},
				metric.LabelPair{
					Name:  "label_3",
					Value: "value_3",
				}: codable.FingerprintSet{1: struct{}{}},
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
				clientmodel.MetricNameLabel: codable.LabelValueSet{
					"metric_0": struct{}{},
					"metric_1": struct{}{},
					"metric_2": struct{}{},
				},
				"label_1": codable.LabelValueSet{
					"value_1": struct{}{},
					"value_2": struct{}{},
					"value_3": struct{}{},
				},
				"label_2": codable.LabelValueSet{
					"value_2": struct{}{},
				},
				"label_3": codable.LabelValueSet{
					"value_1": struct{}{},
					"value_3": struct{}{},
				},
			},
			expectedLpToFps: index.LabelPairFingerprintsMapping{
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_0",
				}: codable.FingerprintSet{0: struct{}{}, 1: struct{}{}, 3: struct{}{}},
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_1",
				}: codable.FingerprintSet{2: struct{}{}, 5: struct{}{}},
				metric.LabelPair{
					Name:  clientmodel.MetricNameLabel,
					Value: "metric_2",
				}: codable.FingerprintSet{4: struct{}{}},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_1",
				}: codable.FingerprintSet{0: struct{}{}},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_2",
				}: codable.FingerprintSet{2: struct{}{}},
				metric.LabelPair{
					Name:  "label_1",
					Value: "value_3",
				}: codable.FingerprintSet{3: struct{}{}, 5: struct{}{}},
				metric.LabelPair{
					Name:  "label_2",
					Value: "value_2",
				}: codable.FingerprintSet{1: struct{}{}, 4: struct{}{}},
				metric.LabelPair{
					Name:  "label_3",
					Value: "value_1",
				}: codable.FingerprintSet{4: struct{}{}},
				metric.LabelPair{
					Name:  "label_3",
					Value: "value_3",
				}: codable.FingerprintSet{1: struct{}{}},
			},
		},
	}

	p, closer := newTestPersistence(t)
	defer closer.Close()

	indexedFpsToMetrics := index.FingerprintMetricMapping{}
	for i, b := range batches {
		for fp, m := range b.fpToMetric {
			p.IndexMetric(m, fp)
			if err := p.ArchiveMetric(fp, m, 1, 2); err != nil {
				t.Fatal(err)
			}
			indexedFpsToMetrics[fp] = m
		}
		// TODO: Find a better solution than sleeping.
		time.Sleep(2 * indexingBatchTimeout)
		verifyIndexedState(i, t, b, indexedFpsToMetrics, p.(*diskPersistence))
	}

	for i := len(batches) - 1; i >= 0; i-- {
		b := batches[i]
		// TODO: Find a better solution than sleeping.
		time.Sleep(2 * indexingBatchTimeout)
		verifyIndexedState(i, t, batches[i], indexedFpsToMetrics, p.(*diskPersistence))
		for fp, m := range b.fpToMetric {
			p.UnindexMetric(m, fp)
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

		outSet := codable.LabelValueSet{}
		for _, lv := range outLvs {
			outSet[lv] = struct{}{}
		}

		if !reflect.DeepEqual(lvs, outSet) {
			t.Errorf("%d. label values don't match. Got: %v; want %v", i, outSet, lvs)
		}
	}

	// Compare label pair -> fingerprints mappings.
	for lp, fps := range b.expectedLpToFps {
		outFps, err := p.GetFingerprintsForLabelPair(lp)
		if err != nil {
			t.Fatal(err)
		}

		outSet := codable.FingerprintSet{}
		for _, fp := range outFps {
			outSet[fp] = struct{}{}
		}

		if !reflect.DeepEqual(fps, outSet) {
			t.Errorf("%d. %v: fingerprints don't match. Got: %v; want %v", i, lp, outSet, fps)
		}
	}
}
