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
	"fmt"
	"testing"
	"time"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/raw/leveldb"

	dto "github.com/prometheus/prometheus/model/generated"
	fixture "github.com/prometheus/prometheus/storage/raw/leveldb/test"
)

type curationState struct {
	fingerprint       string
	ignoreYoungerThan time.Duration
	lastCurated       clientmodel.Timestamp
	processor         Processor
}

type watermarkState struct {
	fingerprint  string
	lastAppended clientmodel.Timestamp
}

type sampleGroup struct {
	fingerprint string
	values      Values
}

type in struct {
	curationStates    fixture.Pairs
	watermarkStates   fixture.Pairs
	sampleGroups      fixture.Pairs
	ignoreYoungerThan time.Duration
	groupSize         uint32
	processor         Processor
}

type out struct {
	curationStates []curationState
	sampleGroups   []sampleGroup
}

func (c curationState) Get() (key proto.Message, value interface{}) {
	signature := c.processor.Signature()
	fingerprint := &clientmodel.Fingerprint{}
	fingerprint.LoadFromString(c.fingerprint)
	keyRaw := curationKey{
		Fingerprint:              fingerprint,
		ProcessorMessageRaw:      signature,
		ProcessorMessageTypeName: c.processor.Name(),
		IgnoreYoungerThan:        c.ignoreYoungerThan,
	}

	k := &dto.CurationKey{}
	keyRaw.dump(k)

	v := &dto.CurationValue{
		LastCompletionTimestamp: proto.Int64(c.lastCurated.Unix()),
	}

	return k, v
}

func (w watermarkState) Get() (key proto.Message, value interface{}) {
	fingerprint := &clientmodel.Fingerprint{}
	fingerprint.LoadFromString(w.fingerprint)
	k := &dto.Fingerprint{}
	dumpFingerprint(k, fingerprint)
	v := &dto.MetricHighWatermark{}
	rawValue := &watermarks{
		High: w.lastAppended,
	}
	rawValue.dump(v)

	return k, v
}

func (s sampleGroup) Get() (key proto.Message, value interface{}) {
	fingerprint := &clientmodel.Fingerprint{}
	fingerprint.LoadFromString(s.fingerprint)
	keyRaw := SampleKey{
		Fingerprint:    fingerprint,
		FirstTimestamp: s.values[0].Timestamp,
		LastTimestamp:  s.values[len(s.values)-1].Timestamp,
		SampleCount:    uint32(len(s.values)),
	}
	k := &dto.SampleKey{}
	keyRaw.Dump(k)

	return k, s.values.marshal()
}

type noopUpdater struct{}

func (noopUpdater) UpdateCurationState(*CurationState) {}

func TestCuratorCompactionProcessor(t *testing.T) {
	scenarios := []struct {
		in  in
		out out
	}{
		{
			in: in{
				processor: NewCompactionProcessor(&CompactionProcessorOptions{
					MinimumGroupSize:         5,
					MaximumMutationPoolBatch: 15,
				}),
				ignoreYoungerThan: 1 * time.Hour,
				groupSize:         5,
				curationStates: fixture.Pairs{
					curationState{
						fingerprint:       "0001-A-1-Z",
						ignoreYoungerThan: 1 * time.Hour,
						lastCurated:       testInstant.Add(-1 * 30 * time.Minute),
						processor: NewCompactionProcessor(&CompactionProcessorOptions{
							MinimumGroupSize:         5,
							MaximumMutationPoolBatch: 15,
						}),
					},
					curationState{
						fingerprint:       "0002-A-2-Z",
						ignoreYoungerThan: 1 * time.Hour,
						lastCurated:       testInstant.Add(-1 * 90 * time.Minute),
						processor: NewCompactionProcessor(&CompactionProcessorOptions{
							MinimumGroupSize:         5,
							MaximumMutationPoolBatch: 15,
						}),
					},
					// This rule should effectively be ignored.
					curationState{
						fingerprint: "0002-A-2-Z",
						processor: NewCompactionProcessor(&CompactionProcessorOptions{
							MinimumGroupSize:         2,
							MaximumMutationPoolBatch: 15,
						}),
						ignoreYoungerThan: 30 * time.Minute,
						lastCurated:       testInstant.Add(-1 * 90 * time.Minute),
					},
				},
				watermarkStates: fixture.Pairs{
					watermarkState{
						fingerprint:  "0001-A-1-Z",
						lastAppended: testInstant.Add(-1 * 15 * time.Minute),
					},
					watermarkState{
						fingerprint:  "0002-A-2-Z",
						lastAppended: testInstant.Add(-1 * 15 * time.Minute),
					},
				},
				sampleGroups: fixture.Pairs{
					sampleGroup{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 90 * time.Minute),
								Value:     0,
							},
							{
								Timestamp: testInstant.Add(-1 * 85 * time.Minute),
								Value:     1,
							},
							{
								Timestamp: testInstant.Add(-1 * 80 * time.Minute),
								Value:     2,
							},
							{
								Timestamp: testInstant.Add(-1 * 75 * time.Minute),
								Value:     3,
							},
							{
								Timestamp: testInstant.Add(-1 * 70 * time.Minute),
								Value:     4,
							},
						},
					},
					sampleGroup{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 65 * time.Minute),
								Value:     0.25,
							},
							{
								Timestamp: testInstant.Add(-1 * 60 * time.Minute),
								Value:     1.25,
							},
							{
								Timestamp: testInstant.Add(-1 * 55 * time.Minute),
								Value:     2.25,
							},
							{
								Timestamp: testInstant.Add(-1 * 50 * time.Minute),
								Value:     3.25,
							},
							{
								Timestamp: testInstant.Add(-1 * 45 * time.Minute),
								Value:     4.25,
							},
						},
					},
					sampleGroup{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 40 * time.Minute),
								Value:     0.50,
							},
							{
								Timestamp: testInstant.Add(-1 * 35 * time.Minute),
								Value:     1.50,
							},
							{
								Timestamp: testInstant.Add(-1 * 30 * time.Minute),
								Value:     2.50,
							},
						},
					},
					sampleGroup{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 25 * time.Minute),
								Value:     0.75,
							},
						},
					},
					sampleGroup{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 20 * time.Minute),
								Value:     -2,
							},
						},
					},
					sampleGroup{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 15 * time.Minute),
								Value:     -3,
							},
						},
					},
					sampleGroup{
						// Moved into Block 1
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 90 * time.Minute),
								Value:     0,
							},
						},
					},
					sampleGroup{
						// Moved into Block 1
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 89 * time.Minute),
								Value:     1,
							},
						},
					},
					sampleGroup{
						// Moved into Block 1
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 88 * time.Minute),
								Value:     2,
							},
						},
					},
					sampleGroup{
						// Moved into Block 1
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 87 * time.Minute),
								Value:     3,
							},
						},
					},
					sampleGroup{
						// Moved into Block 1
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 86 * time.Minute),
								Value:     4,
							},
						},
					},
					sampleGroup{
						// Moved into Block 2
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 85 * time.Minute),
								Value:     5,
							},
						},
					},
					sampleGroup{
						// Moved into Block 2
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 84 * time.Minute),
								Value:     6,
							},
						},
					},
					sampleGroup{
						// Moved into Block 2
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 83 * time.Minute),
								Value:     7,
							},
						},
					},
					sampleGroup{
						// Moved into Block 2
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 82 * time.Minute),
								Value:     8,
							},
						},
					},
					sampleGroup{
						// Moved into Block 2
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 81 * time.Minute),
								Value:     9,
							},
						},
					},
					sampleGroup{
						// Moved into Block 3
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 80 * time.Minute),
								Value:     10,
							},
						},
					},
					sampleGroup{
						// Moved into Block 3
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 79 * time.Minute),
								Value:     11,
							},
						},
					},
					sampleGroup{
						// Moved into Block 3
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 78 * time.Minute),
								Value:     12,
							},
						},
					},
					sampleGroup{
						// Moved into Block 3
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 77 * time.Minute),
								Value:     13,
							},
						},
					},
					sampleGroup{
						// Moved into Blocks 3 and 4 and 5
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								// Block 3
								Timestamp: testInstant.Add(-1 * 76 * time.Minute),
								Value:     14,
							},
							{
								// Block 4
								Timestamp: testInstant.Add(-1 * 75 * time.Minute),
								Value:     15,
							},
							{
								// Block 4
								Timestamp: testInstant.Add(-1 * 74 * time.Minute),
								Value:     16,
							},
							{
								// Block 4
								Timestamp: testInstant.Add(-1 * 73 * time.Minute),
								Value:     17,
							},
							{
								// Block 4
								Timestamp: testInstant.Add(-1 * 72 * time.Minute),
								Value:     18,
							},
							{
								// Block 4
								Timestamp: testInstant.Add(-1 * 71 * time.Minute),
								Value:     19,
							},
							{
								// Block 5
								Timestamp: testInstant.Add(-1 * 70 * time.Minute),
								Value:     20,
							},
						},
					},
					sampleGroup{
						// Moved into Block 5
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 69 * time.Minute),
								Value:     21,
							},
						},
					},
					sampleGroup{
						// Moved into Block 5
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 68 * time.Minute),
								Value:     22,
							},
						},
					},
					sampleGroup{
						// Moved into Block 5
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 67 * time.Minute),
								Value:     23,
							},
						},
					},
					sampleGroup{
						// Moved into Block 5
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 66 * time.Minute),
								Value:     24,
							},
						},
					},
					sampleGroup{
						// Moved into Block 6
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 65 * time.Minute),
								Value:     25,
							},
						},
					},
					sampleGroup{
						// Moved into Block 6
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 64 * time.Minute),
								Value:     26,
							},
						},
					},
					sampleGroup{
						// Moved into Block 6
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 63 * time.Minute),
								Value:     27,
							},
						},
					},
					sampleGroup{
						// Moved into Block 6
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 62 * time.Minute),
								Value:     28,
							},
						},
					},
					sampleGroup{
						// Moved into Block 6
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 61 * time.Minute),
								Value:     29,
							},
						},
					},
					sampleGroup{
						// Moved into Block 7
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 60 * time.Minute),
								Value:     30,
							},
						},
					},
				},
			},
			out: out{
				curationStates: []curationState{
					{
						fingerprint:       "0001-A-1-Z",
						ignoreYoungerThan: time.Hour,
						lastCurated:       testInstant.Add(-1 * 30 * time.Minute),
						processor: NewCompactionProcessor(&CompactionProcessorOptions{
							MinimumGroupSize:         5,
							MaximumMutationPoolBatch: 15,
						}),
					},
					{
						fingerprint:       "0002-A-2-Z",
						ignoreYoungerThan: 30 * time.Minute,
						lastCurated:       testInstant.Add(-1 * 90 * time.Minute),
						processor: NewCompactionProcessor(&CompactionProcessorOptions{
							MinimumGroupSize:         2,
							MaximumMutationPoolBatch: 15,
						}),
					},
					{
						fingerprint:       "0002-A-2-Z",
						ignoreYoungerThan: time.Hour,
						lastCurated:       testInstant.Add(-1 * 60 * time.Minute),
						processor: NewCompactionProcessor(&CompactionProcessorOptions{
							MinimumGroupSize:         5,
							MaximumMutationPoolBatch: 15,
						}),
					},
				},
				sampleGroups: []sampleGroup{
					{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 90 * time.Minute),
								Value:     0,
							},
							{
								Timestamp: testInstant.Add(-1 * 85 * time.Minute),
								Value:     1,
							},
							{
								Timestamp: testInstant.Add(-1 * 80 * time.Minute),
								Value:     2,
							},
							{
								Timestamp: testInstant.Add(-1 * 75 * time.Minute),
								Value:     3,
							},
							{
								Timestamp: testInstant.Add(-1 * 70 * time.Minute),
								Value:     4,
							},
						},
					},
					{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 65 * time.Minute),
								Value:     0.25,
							},
							{
								Timestamp: testInstant.Add(-1 * 60 * time.Minute),
								Value:     1.25,
							},
							{
								Timestamp: testInstant.Add(-1 * 55 * time.Minute),
								Value:     2.25,
							},
							{
								Timestamp: testInstant.Add(-1 * 50 * time.Minute),
								Value:     3.25,
							},
							{
								Timestamp: testInstant.Add(-1 * 45 * time.Minute),
								Value:     4.25,
							},
						},
					},
					{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 40 * time.Minute),
								Value:     0.50,
							},
							{
								Timestamp: testInstant.Add(-1 * 35 * time.Minute),
								Value:     1.50,
							},
							{
								Timestamp: testInstant.Add(-1 * 30 * time.Minute),
								Value:     2.50,
							},
						},
					},
					{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 25 * time.Minute),
								Value:     0.75,
							},
						},
					},
					{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 20 * time.Minute),
								Value:     -2,
							},
						},
					},
					{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 15 * time.Minute),
								Value:     -3,
							},
						},
					},
					{
						// Block 1
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 90 * time.Minute),
								Value:     0,
							},
							{
								Timestamp: testInstant.Add(-1 * 89 * time.Minute),
								Value:     1,
							},
							{
								Timestamp: testInstant.Add(-1 * 88 * time.Minute),
								Value:     2,
							},
							{
								Timestamp: testInstant.Add(-1 * 87 * time.Minute),
								Value:     3,
							},
							{
								Timestamp: testInstant.Add(-1 * 86 * time.Minute),
								Value:     4,
							},
						},
					},
					{
						// Block 2
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 85 * time.Minute),
								Value:     5,
							},
							{
								Timestamp: testInstant.Add(-1 * 84 * time.Minute),
								Value:     6,
							},
							{
								Timestamp: testInstant.Add(-1 * 83 * time.Minute),
								Value:     7,
							},
							{
								Timestamp: testInstant.Add(-1 * 82 * time.Minute),
								Value:     8,
							},
							{
								Timestamp: testInstant.Add(-1 * 81 * time.Minute),
								Value:     9,
							},
						},
					},
					{
						// Block 3
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 80 * time.Minute),
								Value:     10,
							},
							{
								Timestamp: testInstant.Add(-1 * 79 * time.Minute),
								Value:     11,
							},
							{
								Timestamp: testInstant.Add(-1 * 78 * time.Minute),
								Value:     12,
							},
							{
								Timestamp: testInstant.Add(-1 * 77 * time.Minute),
								Value:     13,
							},
							{
								Timestamp: testInstant.Add(-1 * 76 * time.Minute),
								Value:     14,
							},
						},
					},
					{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 75 * time.Minute),
								Value:     15,
							},
							{
								Timestamp: testInstant.Add(-1 * 74 * time.Minute),
								Value:     16,
							},
							{
								Timestamp: testInstant.Add(-1 * 73 * time.Minute),
								Value:     17,
							},
							{
								Timestamp: testInstant.Add(-1 * 72 * time.Minute),
								Value:     18,
							},
							{
								Timestamp: testInstant.Add(-1 * 71 * time.Minute),
								Value:     19,
							},
						},
					},
					{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 70 * time.Minute),
								Value:     20,
							},
							{
								Timestamp: testInstant.Add(-1 * 69 * time.Minute),
								Value:     21,
							},
							{
								Timestamp: testInstant.Add(-1 * 68 * time.Minute),
								Value:     22,
							},
							{
								Timestamp: testInstant.Add(-1 * 67 * time.Minute),
								Value:     23,
							},
							{
								Timestamp: testInstant.Add(-1 * 66 * time.Minute),
								Value:     24,
							},
						},
					},
					{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 65 * time.Minute),
								Value:     25,
							},
							{
								Timestamp: testInstant.Add(-1 * 64 * time.Minute),
								Value:     26,
							},
							{
								Timestamp: testInstant.Add(-1 * 63 * time.Minute),
								Value:     27,
							},
							{
								Timestamp: testInstant.Add(-1 * 62 * time.Minute),
								Value:     28,
							},
							{
								Timestamp: testInstant.Add(-1 * 61 * time.Minute),
								Value:     29,
							},
						},
					},
					{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 60 * time.Minute),
								Value:     30,
							},
						},
					},
				},
			},
		},
	}

	for i, scenario := range scenarios {
		curatorDirectory := fixture.NewPreparer(t).Prepare("curator", fixture.NewCassetteFactory(scenario.in.curationStates))
		defer curatorDirectory.Close()

		watermarkDirectory := fixture.NewPreparer(t).Prepare("watermark", fixture.NewCassetteFactory(scenario.in.watermarkStates))
		defer watermarkDirectory.Close()

		sampleDirectory := fixture.NewPreparer(t).Prepare("sample", fixture.NewCassetteFactory(scenario.in.sampleGroups))
		defer sampleDirectory.Close()

		curatorStates, err := NewLevelDBCurationRemarker(
			leveldb.LevelDBOptions{
				Path: curatorDirectory.Path(),
			},
		)
		if err != nil {
			t.Fatal(err)
		}

		watermarkStates, err := NewLevelDBHighWatermarker(
			leveldb.LevelDBOptions{
				Path: watermarkDirectory.Path(),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		defer watermarkStates.Close()

		samples, err := leveldb.NewLevelDBPersistence(leveldb.LevelDBOptions{
			Path: sampleDirectory.Path(),
		})
		if err != nil {
			t.Fatal(err)
		}
		defer samples.Close()

		updates := &noopUpdater{}

		stop := make(chan bool)
		defer close(stop)

		c := NewCurator(&CuratorOptions{
			Stop: stop,
		})
		defer c.Close()

		err = c.Run(scenario.in.ignoreYoungerThan, testInstant, scenario.in.processor, curatorStates, samples, watermarkStates, updates)
		if err != nil {
			t.Fatal(err)
		}

		iterator, err := curatorStates.LevelDBPersistence.NewIterator(true)
		if err != nil {
			t.Fatal(err)
		}
		defer iterator.Close()

		for j, expected := range scenario.out.curationStates {
			switch j {
			case 0:
				if !iterator.SeekToFirst() {
					t.Fatalf("%d.%d. could not seek to beginning.", i, j)
				}
			default:
				if !iterator.Next() {
					t.Fatalf("%d.%d. could not seek to next.", i, j)
				}
			}

			curationKeyDto := &dto.CurationKey{}
			err = iterator.Key(curationKeyDto)
			if err != nil {
				t.Fatalf("%d.%d. could not unmarshal: %s", i, j, err)
			}
			actualKey := &curationKey{}
			actualKey.load(curationKeyDto)

			actualValue, present, err := curatorStates.Get(actualKey)
			if !present {
				t.Fatalf("%d.%d. could not get key-value pair %s", i, j, actualKey)
			}
			if err != nil {
				t.Fatalf("%d.%d. could not get key-value pair %s", i, j, err)
			}

			expectedFingerprint := &clientmodel.Fingerprint{}
			expectedFingerprint.LoadFromString(expected.fingerprint)
			expectedKey := &curationKey{
				Fingerprint:              expectedFingerprint,
				IgnoreYoungerThan:        expected.ignoreYoungerThan,
				ProcessorMessageRaw:      expected.processor.Signature(),
				ProcessorMessageTypeName: expected.processor.Name(),
			}
			if !actualKey.Equal(expectedKey) {
				t.Fatalf("%d.%d. expected %s, got %s", i, j, expectedKey, actualKey)
			}
			if !actualValue.Equal(expected.lastCurated) {
				t.Fatalf("%d.%d. expected %s, got %s", i, j, expected.lastCurated, actualValue)
			}
		}

		iterator, err = samples.NewIterator(true)
		if err != nil {
			t.Fatal(err)
		}
		defer iterator.Close()

		for j, expected := range scenario.out.sampleGroups {
			switch j {
			case 0:
				if !iterator.SeekToFirst() {
					t.Fatalf("%d.%d. could not seek to beginning.", i, j)
				}
			default:
				if !iterator.Next() {
					t.Fatalf("%d.%d. could not seek to next, expected %s", i, j, expected)
				}
			}

			sampleKey, err := extractSampleKey(iterator)
			if err != nil {
				t.Fatalf("%d.%d. error %s", i, j, err)
			}
			sampleValues := unmarshalValues(iterator.RawValue())

			expectedFingerprint := &clientmodel.Fingerprint{}
			expectedFingerprint.LoadFromString(expected.fingerprint)
			if !expectedFingerprint.Equal(sampleKey.Fingerprint) {
				t.Fatalf("%d.%d. expected fingerprint %s, got %s", i, j, expected.fingerprint, sampleKey.Fingerprint)
			}

			if int(sampleKey.SampleCount) != len(expected.values) {
				t.Fatalf("%d.%d. expected %d values, got %d", i, j, len(expected.values), sampleKey.SampleCount)
			}

			if len(sampleValues) != len(expected.values) {
				t.Fatalf("%d.%d. expected %d values, got %d", i, j, len(expected.values), len(sampleValues))
			}

			if !sampleKey.FirstTimestamp.Equal(expected.values[0].Timestamp) {
				t.Fatalf("%d.%d. expected %s, got %s", i, j, expected.values[0].Timestamp, sampleKey.FirstTimestamp)
			}

			for k, actualValue := range sampleValues {
				if expected.values[k].Value != actualValue.Value {
					t.Fatalf("%d.%d.%d. expected %v, got %v", i, j, k, expected.values[k].Value, actualValue.Value)
				}
				if !expected.values[k].Timestamp.Equal(actualValue.Timestamp) {
					t.Fatalf("%d.%d.%d. expected %s, got %s", i, j, k, expected.values[k].Timestamp, actualValue.Timestamp)
				}
			}

			if !sampleKey.LastTimestamp.Equal(expected.values[len(expected.values)-1].Timestamp) {
				fmt.Println("last", sampleValues[len(expected.values)-1].Value, expected.values[len(expected.values)-1].Value)
				t.Errorf("%d.%d. expected %s, got %s", i, j, expected.values[len(expected.values)-1].Timestamp, sampleKey.LastTimestamp)
			}
		}
	}
}

func TestCuratorDeletionProcessor(t *testing.T) {
	scenarios := []struct {
		in  in
		out out
	}{
		{
			in: in{
				processor: NewDeletionProcessor(&DeletionProcessorOptions{
					MaximumMutationPoolBatch: 15,
				}),
				ignoreYoungerThan: 1 * time.Hour,
				groupSize:         5,
				curationStates: fixture.Pairs{
					curationState{
						fingerprint:       "0001-A-1-Z",
						ignoreYoungerThan: 1 * time.Hour,
						lastCurated:       testInstant.Add(-1 * 90 * time.Minute),
						processor: NewDeletionProcessor(&DeletionProcessorOptions{
							MaximumMutationPoolBatch: 15,
						}),
					},
					curationState{
						fingerprint:       "0002-A-2-Z",
						ignoreYoungerThan: 1 * time.Hour,
						lastCurated:       testInstant.Add(-1 * 90 * time.Minute),
						processor: NewDeletionProcessor(&DeletionProcessorOptions{
							MaximumMutationPoolBatch: 15,
						}),
					},
				},
				watermarkStates: fixture.Pairs{
					watermarkState{
						fingerprint:  "0001-A-1-Z",
						lastAppended: testInstant.Add(-1 * 15 * time.Minute),
					},
					watermarkState{
						fingerprint:  "0002-A-2-Z",
						lastAppended: testInstant.Add(-1 * 15 * time.Minute),
					},
				},
				sampleGroups: fixture.Pairs{
					sampleGroup{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 90 * time.Minute),
								Value:     90,
							},
							{
								Timestamp: testInstant.Add(-1 * 30 * time.Minute),
								Value:     30,
							},
						},
					},
					sampleGroup{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 15 * time.Minute),
								Value:     15,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 90 * time.Minute),
								Value:     0,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 89 * time.Minute),
								Value:     1,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 88 * time.Minute),
								Value:     2,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 87 * time.Minute),
								Value:     3,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 86 * time.Minute),
								Value:     4,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 85 * time.Minute),
								Value:     5,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 84 * time.Minute),
								Value:     6,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 83 * time.Minute),
								Value:     7,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 82 * time.Minute),
								Value:     8,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 81 * time.Minute),
								Value:     9,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 80 * time.Minute),
								Value:     10,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 79 * time.Minute),
								Value:     11,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 78 * time.Minute),
								Value:     12,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 77 * time.Minute),
								Value:     13,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 76 * time.Minute),
								Value:     14,
							},
							{
								Timestamp: testInstant.Add(-1 * 75 * time.Minute),
								Value:     15,
							},
							{
								Timestamp: testInstant.Add(-1 * 74 * time.Minute),
								Value:     16,
							},
							{
								Timestamp: testInstant.Add(-1 * 73 * time.Minute),
								Value:     17,
							},
							{
								Timestamp: testInstant.Add(-1 * 72 * time.Minute),
								Value:     18,
							},
							{
								Timestamp: testInstant.Add(-1 * 71 * time.Minute),
								Value:     19,
							},
							{
								Timestamp: testInstant.Add(-1 * 70 * time.Minute),
								Value:     20,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 69 * time.Minute),
								Value:     21,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 68 * time.Minute),
								Value:     22,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 67 * time.Minute),
								Value:     23,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 66 * time.Minute),
								Value:     24,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 65 * time.Minute),
								Value:     25,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 64 * time.Minute),
								Value:     26,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 63 * time.Minute),
								Value:     27,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 62 * time.Minute),
								Value:     28,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 61 * time.Minute),
								Value:     29,
							},
						},
					},
					sampleGroup{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 60 * time.Minute),
								Value:     30,
							},
						},
					},
				},
			},
			out: out{
				curationStates: []curationState{
					{
						fingerprint:       "0001-A-1-Z",
						ignoreYoungerThan: 1 * time.Hour,
						lastCurated:       testInstant.Add(-1 * 30 * time.Minute),
						processor: NewDeletionProcessor(&DeletionProcessorOptions{
							MaximumMutationPoolBatch: 15,
						}),
					},
					{
						fingerprint:       "0002-A-2-Z",
						ignoreYoungerThan: 1 * time.Hour,
						lastCurated:       testInstant.Add(-1 * 60 * time.Minute),
						processor: NewDeletionProcessor(&DeletionProcessorOptions{
							MaximumMutationPoolBatch: 15,
						}),
					},
				},
				sampleGroups: []sampleGroup{
					{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 30 * time.Minute),
								Value:     30,
							},
						},
					},
					{
						fingerprint: "0001-A-1-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 15 * time.Minute),
								Value:     15,
							},
						},
					},
					{
						fingerprint: "0002-A-2-Z",
						values: Values{
							{
								Timestamp: testInstant.Add(-1 * 60 * time.Minute),
								Value:     30,
							},
						},
					},
				},
			},
		},
	}

	for i, scenario := range scenarios {
		curatorDirectory := fixture.NewPreparer(t).Prepare("curator", fixture.NewCassetteFactory(scenario.in.curationStates))
		defer curatorDirectory.Close()

		watermarkDirectory := fixture.NewPreparer(t).Prepare("watermark", fixture.NewCassetteFactory(scenario.in.watermarkStates))
		defer watermarkDirectory.Close()

		sampleDirectory := fixture.NewPreparer(t).Prepare("sample", fixture.NewCassetteFactory(scenario.in.sampleGroups))
		defer sampleDirectory.Close()

		curatorStates, err := NewLevelDBCurationRemarker(
			leveldb.LevelDBOptions{
				Path: curatorDirectory.Path(),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		defer curatorStates.Close()

		watermarkStates, err := NewLevelDBHighWatermarker(
			leveldb.LevelDBOptions{
				Path: watermarkDirectory.Path(),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		defer watermarkStates.Close()

		samples, err := leveldb.NewLevelDBPersistence(leveldb.LevelDBOptions{Path: sampleDirectory.Path()})
		if err != nil {
			t.Fatal(err)
		}
		defer samples.Close()

		updates := &noopUpdater{}

		stop := make(chan bool)
		defer close(stop)

		c := NewCurator(&CuratorOptions{
			Stop: stop,
		})
		defer c.Close()

		err = c.Run(scenario.in.ignoreYoungerThan, testInstant, scenario.in.processor, curatorStates, samples, watermarkStates, updates)
		if err != nil {
			t.Fatal(err)
		}

		iterator, err := curatorStates.LevelDBPersistence.NewIterator(true)
		if err != nil {
			t.Fatal(err)
		}
		defer iterator.Close()

		for j, expected := range scenario.out.curationStates {
			switch j {
			case 0:
				if !iterator.SeekToFirst() {
					t.Fatalf("%d.%d. could not seek to beginning.", i, j)
				}
			default:
				if !iterator.Next() {
					t.Fatalf("%d.%d. could not seek to next.", i, j)
				}
			}

			curationKeyDto := &dto.CurationKey{}
			if err := iterator.Key(curationKeyDto); err != nil {
				t.Fatalf("%d.%d. could not unmarshal: %s", i, j, err)
			}

			actualKey := &curationKey{}
			actualKey.load(curationKeyDto)
			signature := expected.processor.Signature()

			actualValue, present, err := curatorStates.Get(actualKey)
			if !present {
				t.Fatalf("%d.%d. could not get key-value pair %s", i, j, actualKey)
			}
			if err != nil {
				t.Fatalf("%d.%d. could not get key-value pair %s", i, j, err)
			}

			expectedFingerprint := &clientmodel.Fingerprint{}
			expectedFingerprint.LoadFromString(expected.fingerprint)
			expectedKey := &curationKey{
				Fingerprint:              expectedFingerprint,
				IgnoreYoungerThan:        expected.ignoreYoungerThan,
				ProcessorMessageRaw:      signature,
				ProcessorMessageTypeName: expected.processor.Name(),
			}
			if !actualKey.Equal(expectedKey) {
				t.Fatalf("%d.%d. expected %s, got %s", i, j, expectedKey, actualKey)
			}
			if !actualValue.Equal(expected.lastCurated) {
				t.Fatalf("%d.%d. expected %s, got %s", i, j, expected.lastCurated, actualValue)
			}
		}

		iterator, err = samples.NewIterator(true)
		if err != nil {
			t.Fatal(err)
		}
		defer iterator.Close()

		for j, expected := range scenario.out.sampleGroups {
			switch j {
			case 0:
				if !iterator.SeekToFirst() {
					t.Fatalf("%d.%d. could not seek to beginning.", i, j)
				}
			default:
				if !iterator.Next() {
					t.Fatalf("%d.%d. could not seek to next, expected %s", i, j, expected)
				}
			}

			sampleKey, err := extractSampleKey(iterator)
			if err != nil {
				t.Fatalf("%d.%d. error %s", i, j, err)
			}
			sampleValues := unmarshalValues(iterator.RawValue())

			expectedFingerprint := &clientmodel.Fingerprint{}
			expectedFingerprint.LoadFromString(expected.fingerprint)
			if !expectedFingerprint.Equal(sampleKey.Fingerprint) {
				t.Fatalf("%d.%d. expected fingerprint %s, got %s", i, j, expected.fingerprint, sampleKey.Fingerprint)
			}

			if int(sampleKey.SampleCount) != len(expected.values) {
				t.Fatalf("%d.%d. expected %d values, got %d", i, j, len(expected.values), sampleKey.SampleCount)
			}

			if len(sampleValues) != len(expected.values) {
				t.Fatalf("%d.%d. expected %d values, got %d", i, j, len(expected.values), len(sampleValues))
			}

			if !sampleKey.FirstTimestamp.Equal(expected.values[0].Timestamp) {
				t.Fatalf("%d.%d. expected %s, got %s", i, j, expected.values[0].Timestamp, sampleKey.FirstTimestamp)
			}

			for k, actualValue := range sampleValues {
				if expected.values[k].Value != actualValue.Value {
					t.Fatalf("%d.%d.%d. expected %v, got %v", i, j, k, expected.values[k].Value, actualValue.Value)
				}
				if !expected.values[k].Timestamp.Equal(actualValue.Timestamp) {
					t.Fatalf("%d.%d.%d. expected %s, got %s", i, j, k, expected.values[k].Timestamp, actualValue.Timestamp)
				}
			}

			if !sampleKey.LastTimestamp.Equal(expected.values[len(expected.values)-1].Timestamp) {
				fmt.Println("last", sampleValues[len(expected.values)-1].Value, expected.values[len(expected.values)-1].Value)
				t.Errorf("%d.%d. expected %s, got %s", i, j, expected.values[len(expected.values)-1].Timestamp, sampleKey.LastTimestamp)
			}
		}
	}
}
