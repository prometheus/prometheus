// Copyright 2013 Prometheus Team
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

package metric

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/prometheus/storage"

	clientmodel "github.com/prometheus/client_golang/model"
)

type nopCurationStateUpdater struct{}

func (n *nopCurationStateUpdater) UpdateCurationState(*CurationState) {}

func generateTestSamples(endTime clientmodel.Timestamp, numTs int, samplesPerTs int, interval time.Duration) clientmodel.Samples {
	samples := make(clientmodel.Samples, 0, numTs*samplesPerTs)

	startTime := endTime.Add(-interval * time.Duration(samplesPerTs-1))
	for ts := 0; ts < numTs; ts++ {
		metric := clientmodel.Metric{}
		metric["name"] = clientmodel.LabelValue(fmt.Sprintf("metric_%d", ts))
		for i := 0; i < samplesPerTs; i++ {
			sample := &clientmodel.Sample{
				Metric:    metric,
				Value:     clientmodel.SampleValue(ts + 1000*i),
				Timestamp: startTime.Add(interval * time.Duration(i)),
			}
			samples = append(samples, sample)
		}
	}
	return samples
}

type compactionChecker struct {
	t               *testing.T
	sampleIdx       int
	numChunks       int
	expectedSamples clientmodel.Samples
}

func (c *compactionChecker) Operate(key, value interface{}) *storage.OperatorError {
	c.numChunks++
	sampleKey := key.(*SampleKey)
	if sampleKey.FirstTimestamp.After(sampleKey.LastTimestamp) {
		c.t.Fatalf("Chunk FirstTimestamp (%v) is after LastTimestamp (%v): %v", sampleKey.FirstTimestamp.Unix(), sampleKey.LastTimestamp.Unix(), sampleKey)
	}
	fp := &clientmodel.Fingerprint{}
	for _, sample := range value.(Values) {
		if sample.Timestamp.Before(sampleKey.FirstTimestamp) || sample.Timestamp.After(sampleKey.LastTimestamp) {
			c.t.Fatalf("Sample not within chunk boundaries: chunk FirstTimestamp (%v), chunk LastTimestamp (%v) vs. sample Timestamp (%v)", sampleKey.FirstTimestamp.Unix(), sampleKey.LastTimestamp.Unix(), sample.Timestamp)
		}

		expected := c.expectedSamples[c.sampleIdx]

		fp.LoadFromMetric(expected.Metric)
		if !sampleKey.Fingerprint.Equal(fp) {
			c.t.Fatalf("%d. Expected fingerprint %s, got %s", c.sampleIdx, fp, sampleKey.Fingerprint)
		}

		sp := &SamplePair{
			Value:     expected.Value,
			Timestamp: expected.Timestamp,
		}
		if !sample.Equal(sp) {
			c.t.Fatalf("%d. Expected sample %s, got %s", c.sampleIdx, sp, sample)
		}
		c.sampleIdx++
	}
	return nil
}

func checkStorageSaneAndEquivalent(t *testing.T, name string, ts *TieredStorage, samples clientmodel.Samples, expectedNumChunks int) {
	cc := &compactionChecker{
		expectedSamples: samples,
		t:               t,
	}
	entire, err := ts.DiskStorage.MetricSamples.ForEach(&MetricSamplesDecoder{}, &AcceptAllFilter{}, cc)
	if err != nil {
		t.Fatalf("%s: Error dumping samples: %s", name, err)
	}
	if !entire {
		t.Fatalf("%s: Didn't scan entire corpus", name)
	}
	if cc.numChunks != expectedNumChunks {
		t.Fatalf("%s: Expected %d chunks, got %d", name, expectedNumChunks, cc.numChunks)
	}
}

type compactionTestScenario struct {
	leveldbChunkSize int
	numTimeseries    int
	samplesPerTs     int

	ignoreYoungerThan        time.Duration
	maximumMutationPoolBatch int
	minimumGroupSize         int

	uncompactedChunks int
	compactedChunks   int
}

func (s compactionTestScenario) test(t *testing.T) {
	defer flag.Set("leveldbChunkSize", flag.Lookup("leveldbChunkSize").Value.String())
	flag.Set("leveldbChunkSize", fmt.Sprintf("%d", s.leveldbChunkSize))

	ts, closer := NewTestTieredStorage(t)
	defer closer.Close()

	// 1. Store test values.
	samples := generateTestSamples(testInstant, s.numTimeseries, s.samplesPerTs, time.Minute)
	ts.AppendSamples(samples)
	ts.Flush()

	// 2. Check sanity of uncompacted values.
	checkStorageSaneAndEquivalent(t, "Before compaction", ts, samples, s.uncompactedChunks)

	// 3. Compact test storage.
	processor := NewCompactionProcessor(&CompactionProcessorOptions{
		MaximumMutationPoolBatch: s.maximumMutationPoolBatch,
		MinimumGroupSize:         s.minimumGroupSize,
	})
	defer processor.Close()

	curator := NewCurator(&CuratorOptions{
		Stop:      make(chan bool),
		ViewQueue: ts.ViewQueue,
	})
	defer curator.Close()

	err := curator.Run(s.ignoreYoungerThan, testInstant, processor, ts.DiskStorage.CurationRemarks, ts.DiskStorage.MetricSamples, ts.DiskStorage.MetricHighWatermarks, &nopCurationStateUpdater{})
	if err != nil {
		t.Fatalf("Failed to run curator: %s", err)
	}

	// 4. Check sanity of compacted values.
	checkStorageSaneAndEquivalent(t, "After compaction", ts, samples, s.compactedChunks)
}

func TestCompaction(t *testing.T) {
	scenarios := []compactionTestScenario{
		// BEFORE COMPACTION:
		//
		// Chunk size  |  Fingerprint  |  Samples
		//          5  |            A  |    1 ..  5
		//          5  |            A  |    6 .. 10
		//          5  |            A  |   11 .. 15
		//          5  |            B  |    1 ..  5
		//          5  |            B  |    6 .. 10
		//          5  |            B  |   11 .. 15
		//          5  |            C  |    1 ..  5
		//          5  |            C  |    6 .. 10
		//          5  |            C  |   11 .. 15
		//
		// AFTER COMPACTION:
		//
		// Chunk size  |  Fingerprint  |  Samples
		//         10  |            A  |    1 .. 10
		//          5  |            A  |   11 .. 15
		//         10  |            B  |    1 .. 10
		//          5  |            B  |   11 .. 15
		//         10  |            C  |    1 .. 10
		//          5  |            C  |   11 .. 15
		{
			leveldbChunkSize: 5,
			numTimeseries:    3,
			samplesPerTs:     15,

			ignoreYoungerThan:        time.Minute,
			maximumMutationPoolBatch: 30,
			minimumGroupSize:         10,

			uncompactedChunks: 9,
			compactedChunks:   6,
		},
		// BEFORE COMPACTION:
		//
		// Chunk size  |  Fingerprint  |  Samples
		//          5  |            A  |    1 ..  5
		//          5  |            A  |    6 .. 10
		//          5  |            A  |   11 .. 15
		//          5  |            B  |    1 ..  5
		//          5  |            B  |    6 .. 10
		//          5  |            B  |   11 .. 15
		//          5  |            C  |    1 ..  5
		//          5  |            C  |    6 .. 10
		//          5  |            C  |   11 .. 15
		//
		// AFTER COMPACTION:
		//
		// Chunk size  |  Fingerprint  |  Samples
		//         10  |            A  |    1 .. 15
		//         10  |            B  |    1 .. 15
		//         10  |            C  |    1 .. 15
		{
			leveldbChunkSize: 5,
			numTimeseries:    3,
			samplesPerTs:     15,

			ignoreYoungerThan:        time.Minute,
			maximumMutationPoolBatch: 30,
			minimumGroupSize:         30,

			uncompactedChunks: 9,
			compactedChunks:   3,
		},
		// BEFORE COMPACTION:
		//
		// Chunk size  |  Fingerprint  |  Samples
		//          5  |            A  |    1 ..  5
		//          5  |            A  |    6 .. 10
		//          5  |            A  |   11 .. 15
		//          5  |            A  |   16 .. 20
		//          5  |            B  |    1 ..  5
		//          5  |            B  |    6 .. 10
		//          5  |            B  |   11 .. 15
		//          5  |            B  |   16 .. 20
		//          5  |            C  |    1 ..  5
		//          5  |            C  |    6 .. 10
		//          5  |            C  |   11 .. 15
		//          5  |            C  |   16 .. 20
		//
		// AFTER COMPACTION:
		//
		// Chunk size  |  Fingerprint  |  Samples
		//         10  |            A  |    1 .. 15
		//         10  |            A  |   16 .. 20
		//         10  |            B  |    1 .. 15
		//         10  |            B  |   16 .. 20
		//         10  |            C  |    1 .. 15
		//         10  |            C  |   16 .. 20
		{
			leveldbChunkSize: 5,
			numTimeseries:    3,
			samplesPerTs:     20,

			ignoreYoungerThan:        time.Minute,
			maximumMutationPoolBatch: 30,
			minimumGroupSize:         10,

			uncompactedChunks: 12,
			compactedChunks:   6,
		},
	}

	for _, s := range scenarios {
		s.test(t)
	}
}
