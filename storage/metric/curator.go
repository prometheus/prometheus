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
	"strings"
	"time"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"

	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
)

// CurationState contains high-level curation state information for the
// heads-up-display.
type CurationState struct {
	Active      bool
	Name        string
	Limit       time.Duration
	Fingerprint *clientmodel.Fingerprint
}

// watermarkFilter determines whether to include or exclude candidate
// values from the curation process by virtue of how old the high watermark is.
type watermarkFilter struct {
	// curationState is the data store for curation remarks.
	curationState raw.Persistence
	// ignoreYoungerThan conveys this filter's policy of not working on elements
	// younger than a given relative time duration.  This is persisted to the
	// curation remark database (curationState) to indicate how far a given
	// policy of this type has progressed.
	ignoreYoungerThan time.Duration
	// processor is the post-processor that performs whatever action is desired on
	// the data that is deemed valid to be worked on.
	processor Processor
	// stop functions as the global stop channel for all future operations.
	stop chan bool
	// stopAt is used to determine the elegibility of series for compaction.
	stopAt time.Time
	// status is the outbound channel for notifying the status page of its state.
	status chan CurationState
}

// curator is responsible for effectuating a given curation policy across the
// stored samples on-disk.  This is useful to compact sparse sample values into
// single sample entities to reduce keyspace load on the datastore.
type Curator struct {
	// Stop functions as a channel that when empty allows the curator to operate.
	// The moment a value is ingested inside of it, the curator goes into drain
	// mode.
	Stop chan bool
}

// watermarkDecoder converts (dto.Fingerprint, dto.MetricHighWatermark) doubles
// into (model.Fingerprint, model.Watermark) doubles.
type watermarkDecoder struct {
	fingerprintDTOs   fingerprintDTOLessor
	fingerprints      fingerprintLessor
	highWatermarkDTOs highWatermarkDTOLessor
	highWatermarks    watermarksLessor
}

// watermarkOperator scans over the curator.samples table for metrics whose
// high watermark has been determined to be allowable for curation.  This type
// is individually responsible for compaction.
//
// The scanning starts from CurationRemark.LastCompletionTimestamp and goes
// forward until the stop point or end of the series is reached.
type watermarkOperator struct {
	// curationState is the data store for curation remarks.
	curationState raw.Persistence
	// diskFrontier models the available seekable ranges for the provided
	// sampleIterator.
	diskFrontier *diskFrontier
	// ignoreYoungerThan is passed into the curation remark for the given series.
	ignoreYoungerThan time.Duration
	// processor is responsible for executing a given stategy on the
	// to-be-operated-on series.
	processor Processor
	// sampleIterator is a snapshotted iterator for the time series.
	sampleIterator leveldb.Iterator
	// samples
	samples raw.Persistence
	// stopAt is a cue for when to stop mutating a given series.
	stopAt time.Time
}

// run facilitates the curation lifecycle.
//
// recencyThreshold represents the most recent time up to which values will be
// curated.
// curationState is the on-disk store where the curation remarks are made for
// how much progress has been made.
func (c Curator) Run(ignoreYoungerThan time.Duration, instant time.Time, processor Processor, curationState, samples, watermarks *leveldb.LevelDBPersistence, status chan CurationState) (err error) {
	defer func(t time.Time) {
		duration := float64(time.Since(t) / time.Millisecond)

		labels := map[string]string{
			cutOff:        fmt.Sprint(ignoreYoungerThan),
			processorName: processor.Name(),
			result:        success,
		}
		if err != nil {
			labels[result] = failure
		}

		curationDuration.IncrementBy(labels, duration)
		curationDurations.Add(labels, duration)
	}(time.Now())
	defer func() {
		select {
		case status <- CurationState{Active: false}:
		case <-status:
		default:
		}
	}()

	iterator := samples.NewIterator(true)
	defer iterator.Close()

	diskFrontier, present, err := newDiskFrontier(iterator)
	if err != nil {
		return
	}
	if !present {
		// No sample database exists; no work to do!
		return
	}

	decoder := &watermarkDecoder{}
	// defer decoder.close()

	filter := &watermarkFilter{
		curationState:     curationState,
		ignoreYoungerThan: ignoreYoungerThan,
		processor:         processor,
		status:            status,
		stop:              c.Stop,
		stopAt:            instant.Add(-1 * ignoreYoungerThan),
	}

	// Right now, the ability to stop a curation is limited to the beginning of
	// each fingerprint cycle.  It is impractical to cease the work once it has
	// begun for a given series.
	operator := watermarkOperator{
		curationState:     curationState,
		diskFrontier:      diskFrontier,
		processor:         processor,
		ignoreYoungerThan: ignoreYoungerThan,
		sampleIterator:    iterator,
		samples:           samples,
		stopAt:            instant.Add(-1 * ignoreYoungerThan),
	}

	_, err = watermarks.ForEach(decoder, filter, operator)

	return
}

// drain instructs the curator to stop at the next convenient moment as to not
// introduce data inconsistencies.
func (c Curator) Drain() {
	if len(c.Stop) == 0 {
		c.Stop <- true
	}
}

func (w *watermarkDecoder) DecodeKey(in interface{}) (interface{}, error) {
	key := w.fingerprintDTOs.lease()
	// defer w.fingerprintDTOs.credit(key)
	bytes := in.([]byte)

	if err := proto.Unmarshal(bytes, key); err != nil {
		return nil, err
	}

	fingerprint := w.fingerprints.lease()
	loadFingerprint(fingerprint, key)

	return fingerprint, nil
}

func (w *watermarkDecoder) DecodeValue(in interface{}) (interface{}, error) {
	value := w.highWatermarkDTOs.lease()
	defer w.highWatermarkDTOs.credit(value)
	bytes := in.([]byte)

	if err := proto.Unmarshal(bytes, value); err != nil {
		return nil, err
	}

	watermark := w.highWatermarks.lease()
	watermark.load(value)

	return watermark, nil
}

func (w *watermarkDecoder) close() {
	w.fingerprintDTOs.close()
	w.fingerprints.close()
	w.highWatermarks.close()
	w.highWatermarkDTOs.close()
}

func (w *watermarkFilter) shouldStop() bool {
	return len(w.stop) != 0
}

func getCurationRemark(states raw.Persistence, k *curationKey) (r *curationRemark, found bool, err error) {
	curationKey := &dto.CurationKey{}
	curationValue := &dto.CurationValue{}

	k.dump(curationKey)

	present, err := states.Get(curationKey, curationValue)
	if err != nil {
		return nil, false, err
	}
	if !present {
		return nil, false, nil
	}

	remark := &curationRemark{}
	remark.load(curationValue)

	return remark, true, nil
}

func (w *watermarkFilter) Filter(key, value interface{}) (r storage.FilterResult) {
	fingerprint := key.(*clientmodel.Fingerprint)

	defer func() {
		labels := map[string]string{
			cutOff:        fmt.Sprint(w.ignoreYoungerThan),
			result:        strings.ToLower(r.String()),
			processorName: w.processor.Name(),
		}

		curationFilterOperations.Increment(labels)

		select {
		case w.status <- CurationState{
			Active:      true,
			Name:        w.processor.Name(),
			Limit:       w.ignoreYoungerThan,
			Fingerprint: fingerprint,
		}:
		case <-w.status:
		default:
		}
	}()

	if w.shouldStop() {
		return storage.STOP
	}

	k := &curationKey{
		Fingerprint:              fingerprint,
		ProcessorMessageRaw:      w.processor.Signature(),
		ProcessorMessageTypeName: w.processor.Name(),
		IgnoreYoungerThan:        w.ignoreYoungerThan,
	}

	curationRemark, present, err := getCurationRemark(w.curationState, k)
	if err != nil {
		return
	}
	if !present {
		return storage.ACCEPT
	}
	if !curationRemark.OlderThan(w.stopAt) {
		return storage.SKIP
	}
	watermark := value.(watermarks)
	if !curationRemark.OlderThan(watermark.High) {
		return storage.SKIP
	}
	curationConsistent, err := w.curationConsistent(fingerprint, watermark)
	if err != nil {
		return
	}
	if curationConsistent {
		return storage.SKIP
	}

	return storage.ACCEPT
}

// curationConsistent determines whether the given metric is in a dirty state
// and needs curation.
func (w *watermarkFilter) curationConsistent(f *clientmodel.Fingerprint, watermark watermarks) (bool, error) {
	k := &curationKey{
		Fingerprint:              f,
		ProcessorMessageRaw:      w.processor.Signature(),
		ProcessorMessageTypeName: w.processor.Name(),
		IgnoreYoungerThan:        w.ignoreYoungerThan,
	}
	curationRemark, present, err := getCurationRemark(w.curationState, k)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	if !curationRemark.OlderThan(watermark.High) {
		return true, nil
	}

	return false, nil
}

func (w watermarkOperator) Operate(key, _ interface{}) (oErr *storage.OperatorError) {
	fingerprint := key.(*clientmodel.Fingerprint)

	seriesFrontier, present, err := newSeriesFrontier(fingerprint, w.diskFrontier, w.sampleIterator)
	if err != nil || !present {
		// An anomaly with the series frontier is severe in the sense that some sort
		// of an illegal state condition exists in the storage layer, which would
		// probably signify an illegal disk frontier.
		return &storage.OperatorError{error: err, Continuable: false}
	}

	k := &curationKey{
		Fingerprint:              fingerprint,
		ProcessorMessageRaw:      w.processor.Signature(),
		ProcessorMessageTypeName: w.processor.Name(),
		IgnoreYoungerThan:        w.ignoreYoungerThan,
	}

	curationState, _, err := getCurationRemark(w.curationState, k)
	if err != nil {
		// An anomaly with the curation remark is likely not fatal in the sense that
		// there was a decoding error with the entity and shouldn't be cause to stop
		// work.  The process will simply start from a pessimistic work time and
		// work forward.  With an idempotent processor, this is safe.
		return &storage.OperatorError{error: err, Continuable: true}
	}

	startKey := sampleKey{
		Fingerprint:    fingerprint,
		FirstTimestamp: seriesFrontier.optimalStartTime(curationState),
	}

	prospectiveKey := coding.NewPBEncoder(startKey.ToDTO()).MustEncode()
	if !w.sampleIterator.Seek(prospectiveKey) {
		// LevelDB is picky about the seek ranges.  If an iterator was invalidated,
		// no work may occur, and the iterator cannot be recovered.
		return &storage.OperatorError{error: fmt.Errorf("Illegal Condition: Iterator invalidated due to seek range."), Continuable: false}
	}

	newestAllowedSample := w.stopAt
	if !newestAllowedSample.Before(seriesFrontier.lastSupertime) {
		newestAllowedSample = seriesFrontier.lastSupertime
	}

	lastTime, err := w.processor.Apply(w.sampleIterator, w.samples, newestAllowedSample, fingerprint)
	if err != nil {
		// We can't divine the severity of a processor error without refactoring the
		// interface.
		return &storage.OperatorError{error: err, Continuable: false}
	}

	err = w.refreshCurationRemark(fingerprint, lastTime)
	if err != nil {
		// Under the assumption that the processors are idempotent, they can be
		// re-run; thusly, the commitment of the curation remark is no cause
		// to cease further progress.
		return &storage.OperatorError{error: err, Continuable: true}
	}

	return
}

func (w watermarkOperator) refreshCurationRemark(f *clientmodel.Fingerprint, finished time.Time) (err error) {
	signature, err := w.processor.Signature()
	if err != nil {
		return
	}
	curationKey := curationKey{
		Fingerprint:              f,
		ProcessorMessageRaw:      signature,
		ProcessorMessageTypeName: w.processor.Name(),
		IgnoreYoungerThan:        w.ignoreYoungerThan,
	}.ToDTO()
	curationValue := curationRemark{
		LastCompletionTimestamp: finished,
	}.ToDTO()

	err = w.curationState.Put(curationKey, curationValue)

	return
}
