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
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"time"
)

// processor models a post-processing agent that performs work given a sample
// corpus.
type processor interface {
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
	Apply(sampleIterator leveldb.Iterator, samples raw.Persistence, stopAt time.Time, fingerprint model.Fingerprint) (lastCurated time.Time, err error)
}

// compactionProcessor combines sparse values in the database together such
// that at least MinimumGroupSize is fulfilled from the
type compactionProcessor struct {
	// MaximumMutationPoolBatch represents approximately the largest pending
	// batch of mutation operations for the database before pausing to
	// commit before resumption.
	//
	// A reasonable value would be (MinimumGroupSize * 2) + 1.
	MaximumMutationPoolBatch int
	// MinimumGroupSize represents the smallest allowed sample chunk size in the
	// database.
	MinimumGroupSize int
	// signature is the byte representation of the compactionProcessor's settings,
	// used for purely memoization purposes across an instance.
	signature []byte
}

func (p compactionProcessor) Name() string {
	return "io.prometheus.CompactionProcessorDefinition"
}

func (p *compactionProcessor) Signature() (out []byte, err error) {
	if len(p.signature) == 0 {
		out, err = proto.Marshal(&dto.CompactionProcessorDefinition{
			MinimumGroupSize: proto.Uint32(uint32(p.MinimumGroupSize)),
		})

		p.signature = out
	}

	out = p.signature

	return
}

func (p compactionProcessor) String() string {
	return fmt.Sprintf("compactionProcess for minimum group size %d", p.MinimumGroupSize)
}

func (p compactionProcessor) Apply(sampleIterator leveldb.Iterator, samples raw.Persistence, stopAt time.Time, fingerprint model.Fingerprint) (lastCurated time.Time, err error) {
	var pendingBatch raw.Batch = nil

	defer func() {
		if pendingBatch != nil {
			pendingBatch.Close()
		}
	}()

	var pendingMutations = 0
	var pendingSamples model.Values
	var sampleKey model.SampleKey
	var sampleValues model.Values
	var lastTouchedTime time.Time
	var keyDropped bool

	sampleKey, err = extractSampleKey(sampleIterator)
	if err != nil {
		return
	}
	sampleValues, err = extractSampleValues(sampleIterator)
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
		case len(sampleValues) == 0:
			if !sampleIterator.Next() {
				return lastCurated, fmt.Errorf("Illegal Condition: Invalid Iterator on Continuation")
			}

			keyDropped = false

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
			err = samples.Commit(pendingBatch)
			if err != nil {
				return
			}

			pendingMutations = 0

			pendingBatch.Close()
			pendingBatch = nil

		case len(pendingSamples) == 0 && len(sampleValues) >= p.MinimumGroupSize:
			lastTouchedTime = sampleValues[len(sampleValues)-1].Timestamp
			sampleValues = model.Values{}

		case len(pendingSamples)+len(sampleValues) < p.MinimumGroupSize:
			if !keyDropped {
				key := coding.NewProtocolBuffer(sampleKey.ToDTO())
				pendingBatch.Drop(key)
				keyDropped = true
			}
			pendingSamples = append(pendingSamples, sampleValues...)
			lastTouchedTime = sampleValues[len(sampleValues)-1].Timestamp
			sampleValues = model.Values{}
			pendingMutations++

		// If the number of pending writes equals the target group size
		case len(pendingSamples) == p.MinimumGroupSize:
			newSampleKey := pendingSamples.ToSampleKey(fingerprint)
			key := coding.NewProtocolBuffer(newSampleKey.ToDTO())
			value := coding.NewProtocolBuffer(pendingSamples.ToDTO())
			pendingBatch.Put(key, value)
			pendingMutations++
			lastCurated = newSampleKey.FirstTimestamp.In(time.UTC)
			if len(sampleValues) > 0 {
				if !keyDropped {
					key := coding.NewProtocolBuffer(sampleKey.ToDTO())
					pendingBatch.Drop(key)
					keyDropped = true
				}

				if len(sampleValues) > p.MinimumGroupSize {
					pendingSamples = sampleValues[:p.MinimumGroupSize]
					sampleValues = sampleValues[p.MinimumGroupSize:]
					lastTouchedTime = sampleValues[len(sampleValues)-1].Timestamp
				} else {
					pendingSamples = sampleValues
					lastTouchedTime = pendingSamples[len(pendingSamples)-1].Timestamp
					sampleValues = model.Values{}
				}
			}

		case len(pendingSamples)+len(sampleValues) >= p.MinimumGroupSize:
			if !keyDropped {
				key := coding.NewProtocolBuffer(sampleKey.ToDTO())
				pendingBatch.Drop(key)
				keyDropped = true
			}
			remainder := p.MinimumGroupSize - len(pendingSamples)
			pendingSamples = append(pendingSamples, sampleValues[:remainder]...)
			sampleValues = sampleValues[remainder:]
			if len(sampleValues) == 0 {
				lastTouchedTime = pendingSamples[len(pendingSamples)-1].Timestamp
			} else {
				lastTouchedTime = sampleValues[len(sampleValues)-1].Timestamp
			}
			pendingMutations++

		default:
			err = fmt.Errorf("Unhandled processing case.")
		}
	}

	if len(sampleValues) > 0 || len(pendingSamples) > 0 {
		pendingSamples = append(sampleValues, pendingSamples...)
		newSampleKey := pendingSamples.ToSampleKey(fingerprint)
		key := coding.NewProtocolBuffer(newSampleKey.ToDTO())
		value := coding.NewProtocolBuffer(pendingSamples.ToDTO())
		pendingBatch.Put(key, value)
		pendingSamples = model.Values{}
		pendingMutations++
		lastCurated = newSampleKey.FirstTimestamp.In(time.UTC)
	}

	// This is not deferred due to the off-chance that a pre-existing commit
	// failed.
	if pendingBatch != nil && pendingMutations > 0 {
		err = samples.Commit(pendingBatch)
		if err != nil {
			return
		}
	}

	return
}
