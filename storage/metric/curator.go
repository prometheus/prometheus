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
	"bytes"
	"fmt"
	"strings"
	"time"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"

	dto "github.com/prometheus/prometheus/model/generated"
)

// CurationStateUpdater receives updates about the curation state.
type CurationStateUpdater interface {
	UpdateCurationState(*CurationState)
}

// CurationState contains high-level curation state information for the
// heads-up-display.
type CurationState struct {
	Active      bool
	Name        string
	Limit       time.Duration
	Fingerprint *clientmodel.Fingerprint
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

// watermarkScanner converts (dto.Fingerprint, dto.MetricHighWatermark) doubles
// into (model.Fingerprint, model.Watermark) doubles.
//
// watermarkScanner determines whether to include or exclude candidate
// values from the curation process by virtue of how old the high watermark is.
//
// watermarkScanner scans over the curator.samples table for metrics whose
// high watermark has been determined to be allowable for curation.  This type
// is individually responsible for compaction.
//
// The scanning starts from CurationRemark.LastCompletionTimestamp and goes
// forward until the stop point or end of the series is reached.
type watermarkScanner struct {
	// curationState is the data store for curation remarks.
	curationState CurationRemarker
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

	// stop functions as the global stop channel for all future operations.
	stop chan bool
	// status is the outbound channel for notifying the status page of its state.
	status CurationStateUpdater

	seriesFrontierRes seriesFrontierResolver
}

// run facilitates the curation lifecycle.
//
// recencyThreshold represents the most recent time up to which values will be
// curated.
// curationState is the on-disk store where the curation remarks are made for
// how much progress has been made.
func (c *Curator) Run(ignoreYoungerThan time.Duration, instant time.Time, processor Processor, curationState CurationRemarker, samples *leveldb.LevelDBPersistence, watermarks HighWatermarker, status CurationStateUpdater) (err error) {
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

	defer status.UpdateCurationState(&CurationState{Active: false})

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

	scanner := &watermarkScanner{
		curationState:     curationState,
		ignoreYoungerThan: ignoreYoungerThan,
		processor:         processor,
		status:            status,
		stop:              c.Stop,
		stopAt:            instant.Add(-1 * ignoreYoungerThan),

		sampleIterator: iterator,
		samples:        samples,

		seriesFrontierRes: &levigoSeriesFrontierResolver{
			iterator: iterator,
			frontier: diskFrontier,
		},
	}

	// Right now, the ability to stop a curation is limited to the beginning of
	// each fingerprint cycle.  It is impractical to cease the work once it has
	// begun for a given series.
	_, err = watermarks.ForEach(scanner, scanner, scanner)

	return
}

// drain instructs the curator to stop at the next convenient moment as to not
// introduce data inconsistencies.
func (c *Curator) Drain() {
	if len(c.Stop) == 0 {
		c.Stop <- true
	}
}

func (w *watermarkScanner) DecodeKey(in interface{}) (interface{}, error) {
	key := new(dto.Fingerprint)
	bytes := in.([]byte)

	if err := proto.Unmarshal(bytes, key); err != nil {
		return nil, err
	}

	fingerprint := new(clientmodel.Fingerprint)
	loadFingerprint(fingerprint, key)

	return fingerprint, nil
}

func (w *watermarkScanner) DecodeValue(in interface{}) (interface{}, error) {
	value := new(dto.MetricHighWatermark)
	bytes := in.([]byte)

	if err := proto.Unmarshal(bytes, value); err != nil {
		return nil, err
	}

	watermark := new(watermarks)
	watermark.load(value)

	return watermark, nil
}

func (w *watermarkScanner) shouldStop() bool {
	return len(w.stop) != 0
}

func (w *watermarkScanner) Filter(key, value interface{}) (r storage.FilterResult) {
	fingerprint := key.(*clientmodel.Fingerprint)

	defer func() {
		labels := map[string]string{
			cutOff:        fmt.Sprint(w.ignoreYoungerThan),
			result:        strings.ToLower(r.String()),
			processorName: w.processor.Name(),
		}

		curationFilterOperations.Increment(labels)

		w.status.UpdateCurationState(&CurationState{
			Active:      true,
			Name:        w.processor.Name(),
			Limit:       w.ignoreYoungerThan,
			Fingerprint: fingerprint,
		})
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

	curationRemark, present, err := w.curationState.Get(k)
	if err != nil {
		return
	}
	if !present {
		return storage.ACCEPT
	}
	if !curationRemark.Before(w.stopAt) {
		return storage.SKIP
	}
	watermark := value.(*watermarks)
	if !curationRemark.Before(watermark.High) {
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
func (w *watermarkScanner) curationConsistent(f *clientmodel.Fingerprint, watermark *watermarks) (bool, error) {
	k := &curationKey{
		Fingerprint:              f,
		ProcessorMessageRaw:      w.processor.Signature(),
		ProcessorMessageTypeName: w.processor.Name(),
		IgnoreYoungerThan:        w.ignoreYoungerThan,
	}
	curationRemark, present, err := w.curationState.Get(k)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	if !curationRemark.Before(watermark.High) {
		return true, nil
	}

	return false, nil
}

func (w *watermarkScanner) Operate(key, _ interface{}) (oErr *storage.OperatorError) {
	fingerprint := key.(*clientmodel.Fingerprint)

	seriesFrontier, present, err := w.seriesFrontierRes.GetFrontier(fingerprint)
	if err != nil || !present {
		// An anomaly with the series frontier is severe in the sense that some sort
		// of an illegal state condition exists in the storage layer, which would
		// probably signify an illegal disk frontier.
		return &storage.OperatorError{error: err, Continuable: false}
	}

	curationState, present, err := w.curationState.Get(&curationKey{
		Fingerprint:              fingerprint,
		ProcessorMessageRaw:      w.processor.Signature(),
		ProcessorMessageTypeName: w.processor.Name(),
		IgnoreYoungerThan:        w.ignoreYoungerThan,
	})

	if err != nil {
		// An anomaly with the curation remark is likely not fatal in the sense that
		// there was a decoding error with the entity and shouldn't be cause to stop
		// work.  The process will simply start from a pessimistic work time and
		// work forward.  With an idempotent processor, this is safe.
		return &storage.OperatorError{error: err, Continuable: true}
	}
	var firstSeek time.Time
	switch {
	case !present, seriesFrontier.After(curationState):
		firstSeek = seriesFrontier.firstSupertime
	case !seriesFrontier.InSafeSeekRange(curationState):
		firstSeek = seriesFrontier.lastSupertime
	default:
		firstSeek = curationState
	}

	startKey := &SampleKey{
		Fingerprint:    fingerprint,
		FirstTimestamp: firstSeek,
	}
	dto := new(dto.SampleKey)

	startKey.Dump(dto)
	prospectiveKey := coding.NewPBEncoder(dto).MustEncode()
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

	err = w.curationState.Update(&curationKey{
		Fingerprint:              fingerprint,
		ProcessorMessageRaw:      w.processor.Signature(),
		ProcessorMessageTypeName: w.processor.Name(),
		IgnoreYoungerThan:        w.ignoreYoungerThan,
	}, lastTime)
	if err != nil {
		// Under the assumption that the processors are idempotent, they can be
		// re-run; thusly, the commitment of the curation remark is no cause
		// to cease further progress.
		return &storage.OperatorError{error: err, Continuable: true}
	}

	return nil
}

// curationKey provides a representation of dto.CurationKey with associated
// business logic methods attached to it to enhance code readability.
type curationKey struct {
	Fingerprint              *clientmodel.Fingerprint
	ProcessorMessageRaw      []byte
	ProcessorMessageTypeName string
	IgnoreYoungerThan        time.Duration
}

// Equal answers whether the two curationKeys are equivalent.
func (c *curationKey) Equal(o *curationKey) bool {
	switch {
	case !c.Fingerprint.Equal(o.Fingerprint):
		return false
	case bytes.Compare(c.ProcessorMessageRaw, o.ProcessorMessageRaw) != 0:
		return false
	case c.ProcessorMessageTypeName != o.ProcessorMessageTypeName:
		return false
	case c.IgnoreYoungerThan != o.IgnoreYoungerThan:
		return false
	}

	return true
}

func (c *curationKey) dump(d *dto.CurationKey) {
	d.Reset()

	// BUG(matt): Avenue for simplification.
	fingerprintDTO := &dto.Fingerprint{}

	dumpFingerprint(fingerprintDTO, c.Fingerprint)

	d.Fingerprint = fingerprintDTO
	d.ProcessorMessageRaw = c.ProcessorMessageRaw
	d.ProcessorMessageTypeName = proto.String(c.ProcessorMessageTypeName)
	d.IgnoreYoungerThan = proto.Int64(int64(c.IgnoreYoungerThan))
}

func (c *curationKey) load(d *dto.CurationKey) {
	// BUG(matt): Avenue for simplification.
	c.Fingerprint = &clientmodel.Fingerprint{}

	loadFingerprint(c.Fingerprint, d.Fingerprint)

	c.ProcessorMessageRaw = d.ProcessorMessageRaw
	c.ProcessorMessageTypeName = d.GetProcessorMessageTypeName()
	c.IgnoreYoungerThan = time.Duration(d.GetIgnoreYoungerThan())
}
