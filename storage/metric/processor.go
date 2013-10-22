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

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"

	dto "github.com/prometheus/prometheus/model/generated"
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
	Signature() []byte
	// Apply runs this processor against the sample set.  sampleIterator expects
	// to be pre-seeked to the initial starting position.  The processor will
	// run until up until stopAt has been reached.  It is imperative that the
	// provided stopAt is within the interval of the series frontier.
	//
	// Upon completion or error, the last time at which the processor finished
	// shall be emitted in addition to any errors.
	Apply(sampleIterator leveldb.Iterator, samplesPersistence raw.Persistence, stopAt time.Time, fingerprint *clientmodel.Fingerprint) (lastCurated time.Time, err error)
}

// CompactionProcessor combines sparse values in the database together such
// that at least MinimumGroupSize-sized chunks are grouped together.
type CompactionProcessor struct {
	maximumMutationPoolBatch int
	minimumGroupSize         int
	// signature is the byte representation of the CompactionProcessor's settings,
	// used for purely memoization purposes across an instance.
	signature []byte

	dtoSampleKeys *dtoSampleKeyList
	sampleKeys    *sampleKeyList
}

func (p *CompactionProcessor) Name() string {
	return "io.prometheus.CompactionProcessorDefinition"
}

func (p *CompactionProcessor) Signature() []byte {
	if len(p.signature) == 0 {
		out, err := proto.Marshal(&dto.CompactionProcessorDefinition{
			MinimumGroupSize: proto.Uint32(uint32(p.minimumGroupSize)),
		})
		if err != nil {
			panic(err)
		}

		p.signature = out
	}

	return p.signature
}

func (p *CompactionProcessor) String() string {
	return fmt.Sprintf("compactionProcessor for minimum group size %d", p.minimumGroupSize)
}

func (p *CompactionProcessor) Apply(sampleIterator leveldb.Iterator, samplesPersistence raw.Persistence, stopAt time.Time, fingerprint *clientmodel.Fingerprint) (lastCurated time.Time, err error) {
	var pendingBatch raw.Batch = nil

	defer func() {
		if pendingBatch != nil {
			pendingBatch.Close()
		}
	}()

	var pendingMutations = 0
	var pendingSamples Values
	var unactedSamples Values
	var lastTouchedTime time.Time
	var keyDropped bool

	sampleKey, _ := p.sampleKeys.Get()
	defer p.sampleKeys.Give(sampleKey)

	sampleKeyDto, _ := p.dtoSampleKeys.Get()
	defer p.dtoSampleKeys.Give(sampleKeyDto)

	if err = sampleIterator.Key(sampleKeyDto); err != nil {
		return
	}

	sampleKey.Load(sampleKeyDto)

	unactedSamples, err = extractSampleValues(sampleIterator)
	if err != nil {
		return
	}

	for lastCurated.Before(stopAt) && lastTouchedTime.Before(stopAt) && sampleKey.Fingerprint.Equal(fingerprint) {
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

			if err = sampleIterator.Key(sampleKeyDto); err != nil {
				return
			}
			sampleKey.Load(sampleKeyDto)
			if !sampleKey.Fingerprint.Equal(fingerprint) {
				break
			}

			unactedSamples, err = extractSampleValues(sampleIterator)
			if err != nil {
				return
			}

		// If the number of pending mutations exceeds the allowed batch amount,
		// commit to disk and delete the batch.  A new one will be recreated if
		// necessary.
		case pendingMutations >= p.maximumMutationPoolBatch:
			err = samplesPersistence.Commit(pendingBatch)
			if err != nil {
				return
			}

			pendingMutations = 0

			pendingBatch.Close()
			pendingBatch = nil

		case len(pendingSamples) == 0 && len(unactedSamples) >= p.minimumGroupSize:
			lastTouchedTime = unactedSamples[len(unactedSamples)-1].Timestamp
			unactedSamples = Values{}

		case len(pendingSamples)+len(unactedSamples) < p.minimumGroupSize:
			if !keyDropped {
				k := new(dto.SampleKey)
				sampleKey.Dump(k)
				pendingBatch.Drop(k)

				keyDropped = true
			}
			pendingSamples = append(pendingSamples, unactedSamples...)
			lastTouchedTime = unactedSamples[len(unactedSamples)-1].Timestamp
			unactedSamples = Values{}
			pendingMutations++

		// If the number of pending writes equals the target group size
		case len(pendingSamples) == p.minimumGroupSize:
			k := new(dto.SampleKey)
			newSampleKey := pendingSamples.ToSampleKey(fingerprint)
			newSampleKey.Dump(k)
			b := new(dto.SampleValueSeries)
			pendingSamples.dump(b)
			pendingBatch.Put(k, b)

			pendingMutations++
			lastCurated = newSampleKey.FirstTimestamp.In(time.UTC)
			if len(unactedSamples) > 0 {
				if !keyDropped {
					sampleKey.Dump(k)
					pendingBatch.Drop(k)
					keyDropped = true
				}

				if len(unactedSamples) > p.minimumGroupSize {
					pendingSamples = unactedSamples[:p.minimumGroupSize]
					unactedSamples = unactedSamples[p.minimumGroupSize:]
					lastTouchedTime = unactedSamples[len(unactedSamples)-1].Timestamp
				} else {
					pendingSamples = unactedSamples
					lastTouchedTime = pendingSamples[len(pendingSamples)-1].Timestamp
					unactedSamples = Values{}
				}
			}

		case len(pendingSamples)+len(unactedSamples) >= p.minimumGroupSize:
			if !keyDropped {
				k := new(dto.SampleKey)
				sampleKey.Dump(k)
				pendingBatch.Drop(k)
				keyDropped = true
			}
			remainder := p.minimumGroupSize - len(pendingSamples)
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
		k := new(dto.SampleKey)
		newSampleKey := pendingSamples.ToSampleKey(fingerprint)
		newSampleKey.Dump(k)
		b := new(dto.SampleValueSeries)
		pendingSamples.dump(b)
		pendingBatch.Put(k, b)
		pendingSamples = Values{}
		pendingMutations++
		lastCurated = newSampleKey.FirstTimestamp.In(time.UTC)
	}

	// We might have arrived at a chunk which is too young to be curated (after
	// stopAt), but we still need to seek to the next fingerprint.
	for fingerprint.Equal(sampleKey.Fingerprint) && sampleIterator.Next() {
		if err = sampleIterator.Key(sampleKeyDto); err != nil {
			return
		}
		sampleKey.Load(sampleKeyDto)
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

func (p *CompactionProcessor) Close() {
	p.dtoSampleKeys.Close()
	p.sampleKeys.Close()
}

type CompactionProcessorOptions struct {
	// MaximumMutationPoolBatch represents approximately the largest pending
	// batch of mutation operations for the database before pausing to
	// commit before resumption.
	//
	// A reasonable value would be (MinimumGroupSize * 2) + 1.
	MaximumMutationPoolBatch int
	// MinimumGroupSize represents the smallest allowed sample chunk size in the
	// database.
	MinimumGroupSize int
}

func NewCompactionProcessor(o *CompactionProcessorOptions) *CompactionProcessor {
	return &CompactionProcessor{
		maximumMutationPoolBatch: o.MaximumMutationPoolBatch,
		minimumGroupSize:         o.MinimumGroupSize,

		dtoSampleKeys: newDtoSampleKeyList(10),
		sampleKeys:    newSampleKeyList(10),
	}
}

// DeletionProcessor deletes sample blocks older than a defined value.
type DeletionProcessor struct {
	maximumMutationPoolBatch int
	// signature is the byte representation of the DeletionProcessor's settings,
	// used for purely memoization purposes across an instance.
	signature []byte

	dtoSampleKeys *dtoSampleKeyList
	sampleKeys    *sampleKeyList
}

func (p *DeletionProcessor) Name() string {
	return "io.prometheus.DeletionProcessorDefinition"
}

func (p *DeletionProcessor) Signature() []byte {
	if len(p.signature) == 0 {
		out, err := proto.Marshal(&dto.DeletionProcessorDefinition{})

		if err != nil {
			panic(err)
		}

		p.signature = out
	}

	return p.signature
}

func (p *DeletionProcessor) String() string {
	return "deletionProcessor"
}

func (p *DeletionProcessor) Apply(sampleIterator leveldb.Iterator, samplesPersistence raw.Persistence, stopAt time.Time, fingerprint *clientmodel.Fingerprint) (lastCurated time.Time, err error) {
	var pendingBatch raw.Batch = nil

	defer func() {
		if pendingBatch != nil {
			pendingBatch.Close()
		}
	}()

	sampleKeyDto, _ := p.dtoSampleKeys.Get()
	defer p.dtoSampleKeys.Give(sampleKeyDto)

	sampleKey, _ := p.sampleKeys.Get()
	defer p.sampleKeys.Give(sampleKey)

	if err = sampleIterator.Key(sampleKeyDto); err != nil {
		return
	}
	sampleKey.Load(sampleKeyDto)

	sampleValues, err := extractSampleValues(sampleIterator)
	if err != nil {
		return
	}

	pendingMutations := 0

	for lastCurated.Before(stopAt) && sampleKey.Fingerprint.Equal(fingerprint) {
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

			if err = sampleIterator.Key(sampleKeyDto); err != nil {
				return
			}
			sampleKey.Load(sampleKeyDto)

			sampleValues, err = extractSampleValues(sampleIterator)
			if err != nil {
				return
			}

		// If the number of pending mutations exceeds the allowed batch amount,
		// commit to disk and delete the batch.  A new one will be recreated if
		// necessary.
		case pendingMutations >= p.maximumMutationPoolBatch:
			err = samplesPersistence.Commit(pendingBatch)
			if err != nil {
				return
			}

			pendingMutations = 0

			pendingBatch.Close()
			pendingBatch = nil

		case !sampleKey.MayContain(stopAt):
			k := &dto.SampleKey{}
			sampleKey.Dump(k)
			pendingBatch.Drop(k)
			lastCurated = sampleKey.LastTimestamp
			sampleValues = Values{}
			pendingMutations++

		case sampleKey.MayContain(stopAt):
			k := &dto.SampleKey{}
			sampleKey.Dump(k)
			pendingBatch.Drop(k)
			pendingMutations++

			sampleValues = sampleValues.TruncateBefore(stopAt)
			if len(sampleValues) > 0 {
				k := &dto.SampleKey{}
				sampleKey = sampleValues.ToSampleKey(fingerprint)
				sampleKey.Dump(k)
				v := &dto.SampleValueSeries{}
				sampleValues.dump(v)
				lastCurated = sampleKey.FirstTimestamp
				pendingBatch.Put(k, v)
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

func (p *DeletionProcessor) Close() {
	p.dtoSampleKeys.Close()
	p.sampleKeys.Close()
}

type DeletionProcessorOptions struct {
	// MaximumMutationPoolBatch represents approximately the largest pending
	// batch of mutation operations for the database before pausing to
	// commit before resumption.
	MaximumMutationPoolBatch int
}

func NewDeletionProcessor(o *DeletionProcessorOptions) *DeletionProcessor {
	return &DeletionProcessor{
		maximumMutationPoolBatch: o.MaximumMutationPoolBatch,

		dtoSampleKeys: newDtoSampleKeyList(10),
		sampleKeys:    newSampleKeyList(10),
	}
}
