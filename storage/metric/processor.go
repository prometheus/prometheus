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
	"time"

	"code.google.com/p/goprotobuf/proto"

	dto "github.com/prometheus/prometheus/model/generated"

	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
)

// processor models a post-processing agent that performs work given a sample
// corpus.
type Processor interface {
	// Name emits the name of this processor's signature encoder.  It must be
	// fully-qualified in the sense that it could be used via a Protocol Buffer
	// registry to extract the descriptor to reassemble this message.
	Name() string
	// Signature emits a byte signature for this process for the purpose of
	// remarking how far along it has been applied to the database.
	Signature() (signature []byte, err error)
	// Apply runs this processor against the sample set.  sampleIterator expects
	// to be pre-seeked to the initial starting position.  The processor will
	// run until up until stopAt has been reached.  It is imperative that the
	// provided stopAt is within the interval of the series frontier.
	//
	// Upon completion or error, the last time at which the processor finished
	// shall be emitted in addition to any errors.
	Apply(sampleIterator leveldb.Iterator, samplesPersistence raw.Persistence, stopAt time.Time, fingerprint *model.Fingerprint) (lastCurated time.Time, err error)
}

// CompactionProcessor combines sparse values in the database together such
// that at least MinimumGroupSize-sized chunks are grouped together.
type CompactionProcessor struct {
	// MaximumMutationPoolBatch represents approximately the largest pending
	// batch of mutation operations for the database before pausing to
	// commit before resumption.
	//
	// A reasonable value would be (MinimumGroupSize * 2) + 1.
	MaximumMutationPoolBatch int
	// MinimumGroupSize represents the smallest allowed sample chunk size in the
	// database.
	MinimumGroupSize int
	// signature is the byte representation of the CompactionProcessor's settings,
	// used for purely memoization purposes across an instance.
	signature []byte
}

func (p CompactionProcessor) Name() string {
	return "io.prometheus.CompactionProcessorDefinition"
}

func (p *CompactionProcessor) Signature() (out []byte, err error) {
	if len(p.signature) == 0 {
		out, err = proto.Marshal(&dto.CompactionProcessorDefinition{
			MinimumGroupSize: proto.Uint32(uint32(p.MinimumGroupSize)),
		})

		p.signature = out
	}

	out = p.signature

	return
}

func (p CompactionProcessor) String() string {
	return fmt.Sprintf("compactionProcessor for minimum group size %d", p.MinimumGroupSize)
}

func (p CompactionProcessor) Apply(sampleIterator leveldb.Iterator, samplesPersistence raw.Persistence, stopAt time.Time, fingerprint *model.Fingerprint) (lastCurated time.Time, err error) {
	var pendingBatch raw.Batch = nil

	defer func() {
		if pendingBatch != nil {
			pendingBatch.Close()
		}
	}()

	var pendingMutations = 0
	var pendingSamples model.Values
	var sampleKey model.SampleKey
	var unactedSamples model.Values
	var lastTouchedTime time.Time
	var keyDropped bool

	sampleKey, err = extractSampleKey(sampleIterator)
	if err != nil {
		return
	}
	unactedSamples, err = extractSampleValues(sampleIterator)
	if err != nil {
		return
	}

	for lastCurated.Before(stopAt) && lastTouchedTime.Before(stopAt) {
		switch {
		// Furnish a new pending batch operation if none is available.
		case pendingBatch == nil:
			pendingBatch = leveldb.NewBatch()

		// If there are no sample values to extract from the datastore, let's
		// continue extracting more values to use.  We know that the time.Before()
		// block would prevent us from going into unsafe territory.
		case len(unactedSamples) == 0:
			if !sampleIterator.Next() {
				return lastCurated, fmt.Errorf("Illegal Condition: Invalid Iterator on Continuation")
			}

			keyDropped = false

			sampleKey, err = extractSampleKey(sampleIterator)
			if err != nil {
				return
			}
			unactedSamples, err = extractSampleValues(sampleIterator)
			if err != nil {
				return
			}

		// If the number of pending mutations exceeds the allowed batch amount,
		// commit to disk and delete the batch.  A new one will be recreated if
		// necessary.
		case pendingMutations >= p.MaximumMutationPoolBatch:
			err = samplesPersistence.Commit(pendingBatch)
			if err != nil {
				return
			}

			pendingMutations = 0

			pendingBatch.Close()
			pendingBatch = nil

		case len(pendingSamples) == 0 && len(unactedSamples) >= p.MinimumGroupSize:
			lastTouchedTime = unactedSamples[len(unactedSamples)-1].Timestamp
			unactedSamples = model.Values{}

		case len(pendingSamples)+len(unactedSamples) < p.MinimumGroupSize:
			if !keyDropped {
				pendingBatch.Drop(sampleKey.ToDTO())
				keyDropped = true
			}
			pendingSamples = append(pendingSamples, unactedSamples...)
			lastTouchedTime = unactedSamples[len(unactedSamples)-1].Timestamp
			unactedSamples = model.Values{}
			pendingMutations++

		// If the number of pending writes equals the target group size
		case len(pendingSamples) == p.MinimumGroupSize:
			newSampleKey := pendingSamples.ToSampleKey(fingerprint)
			pendingBatch.Put(newSampleKey.ToDTO(), pendingSamples.ToDTO())
			pendingMutations++
			lastCurated = newSampleKey.FirstTimestamp.In(time.UTC)
			if len(unactedSamples) > 0 {
				if !keyDropped {
					pendingBatch.Drop(sampleKey.ToDTO())
					keyDropped = true
				}

				if len(unactedSamples) > p.MinimumGroupSize {
					pendingSamples = unactedSamples[:p.MinimumGroupSize]
					unactedSamples = unactedSamples[p.MinimumGroupSize:]
					lastTouchedTime = unactedSamples[len(unactedSamples)-1].Timestamp
				} else {
					pendingSamples = unactedSamples
					lastTouchedTime = pendingSamples[len(pendingSamples)-1].Timestamp
					unactedSamples = model.Values{}
				}
			}

		case len(pendingSamples)+len(unactedSamples) >= p.MinimumGroupSize:
			if !keyDropped {
				pendingBatch.Drop(sampleKey.ToDTO())
				keyDropped = true
			}
			remainder := p.MinimumGroupSize - len(pendingSamples)
			pendingSamples = append(pendingSamples, unactedSamples[:remainder]...)
			unactedSamples = unactedSamples[remainder:]
			if len(unactedSamples) == 0 {
				lastTouchedTime = pendingSamples[len(pendingSamples)-1].Timestamp
			} else {
				lastTouchedTime = unactedSamples[len(unactedSamples)-1].Timestamp
			}
			pendingMutations++
		default:
			err = fmt.Errorf("Unhandled processing case.")
		}
	}

	if len(unactedSamples) > 0 || len(pendingSamples) > 0 {
		pendingSamples = append(pendingSamples, unactedSamples...)
		newSampleKey := pendingSamples.ToSampleKey(fingerprint)
		pendingBatch.Put(newSampleKey.ToDTO(), pendingSamples.ToDTO())
		pendingSamples = model.Values{}
		pendingMutations++
		lastCurated = newSampleKey.FirstTimestamp.In(time.UTC)
	}

	// This is not deferred due to the off-chance that a pre-existing commit
	// failed.
	if pendingBatch != nil && pendingMutations > 0 {
		err = samplesPersistence.Commit(pendingBatch)
		if err != nil {
			return
		}
	}

	return
}

// DeletionProcessor deletes sample blocks older than a defined value.
type DeletionProcessor struct {
	// MaximumMutationPoolBatch represents approximately the largest pending
	// batch of mutation operations for the database before pausing to
	// commit before resumption.
	MaximumMutationPoolBatch int
	// signature is the byte representation of the DeletionProcessor's settings,
	// used for purely memoization purposes across an instance.
	signature []byte
}

func (p DeletionProcessor) Name() string {
	return "io.prometheus.DeletionProcessorDefinition"
}

func (p *DeletionProcessor) Signature() (out []byte, err error) {
	if len(p.signature) == 0 {
		out, err = proto.Marshal(&dto.DeletionProcessorDefinition{})

		p.signature = out
	}

	out = p.signature

	return
}

func (p DeletionProcessor) String() string {
	return "deletionProcessor"
}

func (p DeletionProcessor) Apply(sampleIterator leveldb.Iterator, samplesPersistence raw.Persistence, stopAt time.Time, fingerprint *model.Fingerprint) (lastCurated time.Time, err error) {
	var pendingBatch raw.Batch = nil

	defer func() {
		if pendingBatch != nil {
			pendingBatch.Close()
		}
	}()

	sampleKey, err := extractSampleKey(sampleIterator)
	if err != nil {
		return
	}
	sampleValues, err := extractSampleValues(sampleIterator)
	if err != nil {
		return
	}

	pendingMutations := 0

	for lastCurated.Before(stopAt) {
		switch {
		// Furnish a new pending batch operation if none is available.
		case pendingBatch == nil:
			pendingBatch = leveldb.NewBatch()

		// If there are no sample values to extract from the datastore, let's
		// continue extracting more values to use.  We know that the time.Before()
		// block would prevent us from going into unsafe territory.
		case len(sampleValues) == 0:
			if !sampleIterator.Next() {
				return lastCurated, fmt.Errorf("Illegal Condition: Invalid Iterator on Continuation")
			}

			sampleKey, err = extractSampleKey(sampleIterator)
			if err != nil {
				return
			}
			sampleValues, err = extractSampleValues(sampleIterator)
			if err != nil {
				return
			}

		// If the number of pending mutations exceeds the allowed batch amount,
		// commit to disk and delete the batch.  A new one will be recreated if
		// necessary.
		case pendingMutations >= p.MaximumMutationPoolBatch:
			err = samplesPersistence.Commit(pendingBatch)
			if err != nil {
				return
			}

			pendingMutations = 0

			pendingBatch.Close()
			pendingBatch = nil

		case !sampleKey.MayContain(stopAt):
			pendingBatch.Drop(sampleKey.ToDTO())
			lastCurated = sampleKey.LastTimestamp
			sampleValues = model.Values{}
			pendingMutations++

		case sampleKey.MayContain(stopAt):
			pendingBatch.Drop(sampleKey.ToDTO())
			pendingMutations++

			sampleValues = sampleValues.TruncateBefore(stopAt)
			if len(sampleValues) > 0 {
				sampleKey = sampleValues.ToSampleKey(fingerprint)
				lastCurated = sampleKey.FirstTimestamp
				pendingBatch.Put(sampleKey.ToDTO(), sampleValues.ToDTO())
				pendingMutations++
			} else {
				lastCurated = sampleKey.LastTimestamp
			}

		default:
			err = fmt.Errorf("Unhandled processing case.")
		}
	}

	// This is not deferred due to the off-chance that a pre-existing commit
	// failed.
	if pendingBatch != nil && pendingMutations > 0 {
		err = samplesPersistence.Commit(pendingBatch)
		if err != nil {
			return
		}
	}

	return
}
