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
	"log"
	"sort"
	"sync"
	"time"

	"code.google.com/p/goprotobuf/proto"

	dto "github.com/prometheus/prometheus/model/generated"
	index "github.com/prometheus/prometheus/storage/raw/index/leveldb"

	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility"
)

const sortConcurrency = 2

type LevelDBMetricPersistence struct {
	CurationRemarks         *leveldb.LevelDBPersistence
	fingerprintToMetrics    *leveldb.LevelDBPersistence
	labelNameToFingerprints *leveldb.LevelDBPersistence
	labelSetToFingerprints  *leveldb.LevelDBPersistence
	MetricHighWatermarks    *leveldb.LevelDBPersistence
	metricMembershipIndex   *index.LevelDBMembershipIndex
	MetricSamples           *leveldb.LevelDBPersistence
}

var (
	leveldbChunkSize = flag.Int("leveldbChunkSize", 200, "Maximum number of samples stored under one key.")

	// These flag values are back of the envelope, though they seem sensible.
	// Please re-evaluate based on your own needs.
	curationRemarksCacheSize         = flag.Int("curationRemarksCacheSize", 50*1024*1024, "The size for the curation remarks cache (bytes).")
	fingerprintsToLabelPairCacheSize = flag.Int("fingerprintsToLabelPairCacheSizeBytes", 100*1024*1024, "The size for the fingerprint to label pair index (bytes).")
	highWatermarkCacheSize           = flag.Int("highWatermarksByFingerprintSizeBytes", 50*1024*1024, "The size for the metric high watermarks (bytes).")
	labelNameToFingerprintsCacheSize = flag.Int("labelNameToFingerprintsCacheSizeBytes", 100*1024*1024, "The size for the label name to metric fingerprint index (bytes).")
	labelPairToFingerprintsCacheSize = flag.Int("labelPairToFingerprintsCacheSizeBytes", 100*1024*1024, "The size for the label pair to metric fingerprint index (bytes).")
	metricMembershipIndexCacheSize   = flag.Int("metricMembershipCacheSizeBytes", 50*1024*1024, "The size for the metric membership index (bytes).")
	samplesByFingerprintCacheSize    = flag.Int("samplesByFingerprintCacheSizeBytes", 500*1024*1024, "The size for the samples database (bytes).")
)

type leveldbOpener func()
type leveldbCloser interface {
	Close()
}

func (l *LevelDBMetricPersistence) Close() {
	var persistences = []leveldbCloser{
		l.CurationRemarks,
		l.fingerprintToMetrics,
		l.labelNameToFingerprints,
		l.labelSetToFingerprints,
		l.MetricHighWatermarks,
		l.metricMembershipIndex,
		l.MetricSamples,
	}

	closerGroup := sync.WaitGroup{}

	for _, closer := range persistences {
		closerGroup.Add(1)
		go func(closer leveldbCloser) {
			if closer != nil {
				closer.Close()
			}
			closerGroup.Done()
		}(closer)
	}

	closerGroup.Wait()
}

func NewLevelDBMetricPersistence(baseDirectory string) (*LevelDBMetricPersistence, error) {
	workers := utility.NewUncertaintyGroup(7)

	emission := &LevelDBMetricPersistence{}

	var subsystemOpeners = []struct {
		name   string
		opener leveldbOpener
	}{
		{
			"Label Names and Value Pairs by Fingerprint",
			func() {
				var err error
				emission.fingerprintToMetrics, err = leveldb.NewLevelDBPersistence(baseDirectory+"/label_name_and_value_pairs_by_fingerprint", *fingerprintsToLabelPairCacheSize, 10)
				workers.MayFail(err)
			},
		},
		{
			"Samples by Fingerprint",
			func() {
				var err error
				emission.MetricSamples, err = leveldb.NewLevelDBPersistence(baseDirectory+"/samples_by_fingerprint", *samplesByFingerprintCacheSize, 10)
				workers.MayFail(err)
			},
		},
		{
			"High Watermarks by Fingerprint",
			func() {
				var err error
				emission.MetricHighWatermarks, err = leveldb.NewLevelDBPersistence(baseDirectory+"/high_watermarks_by_fingerprint", *highWatermarkCacheSize, 10)
				workers.MayFail(err)
			},
		},
		{
			"Fingerprints by Label Name",
			func() {
				var err error
				emission.labelNameToFingerprints, err = leveldb.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name", *labelNameToFingerprintsCacheSize, 10)
				workers.MayFail(err)
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var err error
				emission.labelSetToFingerprints, err = leveldb.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name_and_value_pair", *labelPairToFingerprintsCacheSize, 10)
				workers.MayFail(err)
			},
		},
		{
			"Metric Membership Index",
			func() {
				var err error
				emission.metricMembershipIndex, err = index.NewLevelDBMembershipIndex(baseDirectory+"/metric_membership_index", *metricMembershipIndexCacheSize, 10)
				workers.MayFail(err)
			},
		},
		{
			"Sample Curation Remarks",
			func() {
				var err error
				emission.CurationRemarks, err = leveldb.NewLevelDBPersistence(baseDirectory+"/curation_remarks", *curationRemarksCacheSize, 10)
				workers.MayFail(err)
			},
		},
	}

	for _, subsystem := range subsystemOpeners {
		opener := subsystem.opener
		go opener()
	}

	if !workers.Wait() {
		for _, err := range workers.Errors() {
			log.Printf("Could not open storage due to %s", err)
		}

		return nil, fmt.Errorf("Unable to open metric persistence.")
	}

	return emission, nil
}

func (l *LevelDBMetricPersistence) AppendSample(sample model.Sample) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSample, result: success}, map[string]string{operation: appendSample, result: failure})
	}(time.Now())

	err = l.AppendSamples(model.Samples{sample})

	return
}

// groupByFingerprint collects all of the provided samples, groups them
// together by their respective metric fingerprint, and finally sorts
// them chronologically.
func groupByFingerprint(samples model.Samples) map[model.Fingerprint]model.Samples {
	fingerprintToSamples := map[model.Fingerprint]model.Samples{}

	for _, sample := range samples {
		fingerprint := *model.NewFingerprintFromMetric(sample.Metric)
		samples := fingerprintToSamples[fingerprint]
		samples = append(samples, sample)
		fingerprintToSamples[fingerprint] = samples
	}

	sortingSemaphore := make(chan bool, sortConcurrency)
	doneSorting := sync.WaitGroup{}

	for i := 0; i < sortConcurrency; i++ {
		sortingSemaphore <- true
	}

	for _, samples := range fingerprintToSamples {
		doneSorting.Add(1)

		<-sortingSemaphore
		go func(samples model.Samples) {
			sort.Sort(samples)
			sortingSemaphore <- true
			doneSorting.Done()
		}(samples)
	}

	doneSorting.Wait()

	return fingerprintToSamples
}

// findUnindexedMetrics scours the metric membership index for each given Metric
// in the keyspace and returns a map of Fingerprint-Metric pairs that are
// absent.
func (l *LevelDBMetricPersistence) findUnindexedMetrics(candidates map[model.Fingerprint]model.Metric) (unindexed map[model.Fingerprint]model.Metric, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: findUnindexedMetrics, result: success}, map[string]string{operation: findUnindexedMetrics, result: failure})
	}(time.Now())

	unindexed = make(map[model.Fingerprint]model.Metric)

	// Determine which metrics are unknown in the database.
	for fingerprint, metric := range candidates {
		dto := model.MetricToDTO(metric)
		indexHas, err := l.hasIndexMetric(dto)
		if err != nil {
			return unindexed, err
		}
		if !indexHas {
			unindexed[fingerprint] = metric
		}
	}

	return
}

// indexLabelNames accumulates all label name to fingerprint index entries for
// the dirty metrics, appends the new dirtied metrics, sorts, and bulk updates
// the index to reflect the new state.
//
// This operation is idempotent.
func (l *LevelDBMetricPersistence) indexLabelNames(metrics map[model.Fingerprint]model.Metric) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: indexLabelNames, result: success}, map[string]string{operation: indexLabelNames, result: failure})
	}(time.Now())

	labelNameFingerprints := map[model.LabelName]utility.Set{}

	for fingerprint, metric := range metrics {
		for labelName := range metric {
			fingerprintSet, ok := labelNameFingerprints[labelName]
			if !ok {
				fingerprintSet = utility.Set{}

				fingerprints, err := l.GetFingerprintsForLabelName(labelName)
				if err != nil {
					return err
				}

				for _, fingerprint := range fingerprints {
					fingerprintSet.Add(*fingerprint)
				}
			}

			fingerprintSet.Add(fingerprint)
			labelNameFingerprints[labelName] = fingerprintSet
		}
	}

	batch := leveldb.NewBatch()
	defer batch.Close()

	for labelName, fingerprintSet := range labelNameFingerprints {
		fingerprints := model.Fingerprints{}
		for e := range fingerprintSet {
			fingerprint := e.(model.Fingerprint)
			fingerprints = append(fingerprints, &fingerprint)
		}

		sort.Sort(fingerprints)

		key := &dto.LabelName{
			Name: proto.String(string(labelName)),
		}
		value := &dto.FingerprintCollection{}
		for _, fingerprint := range fingerprints {
			value.Member = append(value.Member, fingerprint.ToDTO())
		}

		batch.Put(key, value)
	}

	err = l.labelNameToFingerprints.Commit(batch)
	if err != nil {
		return
	}

	return
}

// indexLabelPairs accumulates all label pair to fingerprint index entries for
// the dirty metrics, appends the new dirtied metrics, sorts, and bulk updates
// the index to reflect the new state.
//
// This operation is idempotent.
func (l *LevelDBMetricPersistence) indexLabelPairs(metrics map[model.Fingerprint]model.Metric) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: indexLabelPairs, result: success}, map[string]string{operation: indexLabelPairs, result: failure})
	}(time.Now())

	labelPairFingerprints := map[model.LabelPair]utility.Set{}

	for fingerprint, metric := range metrics {
		for labelName, labelValue := range metric {
			labelPair := model.LabelPair{
				Name:  labelName,
				Value: labelValue,
			}
			fingerprintSet, ok := labelPairFingerprints[labelPair]
			if !ok {
				fingerprintSet = utility.Set{}

				fingerprints, err := l.GetFingerprintsForLabelSet(model.LabelSet{
					labelName: labelValue,
				})
				if err != nil {
					return err
				}

				for _, fingerprint := range fingerprints {
					fingerprintSet.Add(*fingerprint)
				}
			}

			fingerprintSet.Add(fingerprint)
			labelPairFingerprints[labelPair] = fingerprintSet
		}
	}

	batch := leveldb.NewBatch()
	defer batch.Close()

	for labelPair, fingerprintSet := range labelPairFingerprints {
		fingerprints := model.Fingerprints{}
		for e := range fingerprintSet {
			fingerprint := e.(model.Fingerprint)
			fingerprints = append(fingerprints, &fingerprint)
		}

		sort.Sort(fingerprints)

		key := &dto.LabelPair{
			Name:  proto.String(string(labelPair.Name)),
			Value: proto.String(string(labelPair.Value)),
		}
		value := &dto.FingerprintCollection{}
		for _, fingerprint := range fingerprints {
			value.Member = append(value.Member, fingerprint.ToDTO())
		}

		batch.Put(key, value)
	}

	err = l.labelSetToFingerprints.Commit(batch)
	if err != nil {
		return
	}

	return
}

// indexFingerprints updates all of the Fingerprint to Metric reverse lookups
// in the index and then bulk updates.
//
// This operation is idempotent.
func (l *LevelDBMetricPersistence) indexFingerprints(metrics map[model.Fingerprint]model.Metric) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: indexFingerprints, result: success}, map[string]string{operation: indexFingerprints, result: failure})
	}(time.Now())

	batch := leveldb.NewBatch()
	defer batch.Close()

	for fingerprint, metric := range metrics {
		batch.Put(fingerprint.ToDTO(), model.MetricToDTO(metric))
	}

	err = l.fingerprintToMetrics.Commit(batch)
	if err != nil {
		return
	}

	return
}

var existenceIdentity = &dto.MembershipIndexValue{}

// indexMetrics takes groups of samples, determines which ones contain metrics
// that are unknown to the storage stack, and then proceeds to update all
// affected indices.
func (l *LevelDBMetricPersistence) indexMetrics(fingerprints map[model.Fingerprint]model.Metric) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: indexMetrics, result: success}, map[string]string{operation: indexMetrics, result: failure})
	}(time.Now())

	var (
		absentMetrics map[model.Fingerprint]model.Metric
	)

	absentMetrics, err = l.findUnindexedMetrics(fingerprints)
	if err != nil {
		return
	}

	if len(absentMetrics) == 0 {
		return
	}

	// TODO: For the missing fingerprints, determine what label names and pairs
	// are absent and act accordingly and append fingerprints.
	workers := utility.NewUncertaintyGroup(3)

	go func() {
		workers.MayFail(l.indexLabelNames(absentMetrics))
	}()

	go func() {
		workers.MayFail(l.indexLabelPairs(absentMetrics))
	}()

	go func() {
		workers.MayFail(l.indexFingerprints(absentMetrics))
	}()

	if !workers.Wait() {
		return fmt.Errorf("Could not index due to %s", workers.Errors())
	}

	// If any of the preceding operations failed, we will have inconsistent
	// indices.  Thusly, the Metric membership index should NOT be updated, as
	// its state is used to determine whether to bulk update the other indices.
	// Given that those operations are idempotent, it is OK to repeat them;
	// however, it will consume considerable amounts of time.
	batch := leveldb.NewBatch()
	defer batch.Close()

	for _, metric := range absentMetrics {
		batch.Put(model.MetricToDTO(metric), existenceIdentity)
	}

	err = l.metricMembershipIndex.Commit(batch)
	if err != nil {
		// Not critical but undesirable.
		log.Println(err)
	}

	return
}

func (l *LevelDBMetricPersistence) refreshHighWatermarks(groups map[model.Fingerprint]model.Samples) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: refreshHighWatermarks, result: success}, map[string]string{operation: refreshHighWatermarks, result: failure})
	}(time.Now())

	batch := leveldb.NewBatch()
	defer batch.Close()

	mutationCount := 0
	for fingerprint, samples := range groups {
		value := &dto.MetricHighWatermark{}
		newestSampleTimestamp := samples[len(samples)-1].Timestamp
		present, err := l.MetricHighWatermarks.Get(fingerprint.ToDTO(), value)
		if err != nil {
			return err
		}
		if !present {
			continue
		}

		// BUG(matt): Repace this with watermark management.
		if newestSampleTimestamp.Before(time.Unix(*value.Timestamp, 0)) {
			continue
		}

		value.Timestamp = proto.Int64(newestSampleTimestamp.Unix())
		batch.Put(fingerprint.ToDTO(), value)
		mutationCount++
	}

	err = l.MetricHighWatermarks.Commit(batch)
	if err != nil {
		return err
	}

	return nil
}

func (l *LevelDBMetricPersistence) AppendSamples(samples model.Samples) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSamples, result: success}, map[string]string{operation: appendSamples, result: failure})
	}(time.Now())

	fingerprintToSamples := groupByFingerprint(samples)
	indexErrChan := make(chan error, 1)
	watermarkErrChan := make(chan error, 1)

	go func(groups map[model.Fingerprint]model.Samples) {
		metrics := map[model.Fingerprint]model.Metric{}

		for fingerprint, samples := range groups {
			metrics[fingerprint] = samples[0].Metric
		}

		indexErrChan <- l.indexMetrics(metrics)
	}(fingerprintToSamples)

	go func(groups map[model.Fingerprint]model.Samples) {
		watermarkErrChan <- l.refreshHighWatermarks(groups)
	}(fingerprintToSamples)

	samplesBatch := leveldb.NewBatch()
	defer samplesBatch.Close()

	for fingerprint, group := range fingerprintToSamples {
		for {
			lengthOfGroup := len(group)

			if lengthOfGroup == 0 {
				break
			}

			take := *leveldbChunkSize
			if lengthOfGroup < take {
				take = lengthOfGroup
			}

			chunk := group[0:take]
			group = group[take:lengthOfGroup]

			key := model.SampleKey{
				Fingerprint:    &fingerprint,
				FirstTimestamp: chunk[0].Timestamp,
				LastTimestamp:  chunk[take-1].Timestamp,
				SampleCount:    uint32(take),
			}.ToDTO()

			value := &dto.SampleValueSeries{}
			for _, sample := range chunk {
				value.Value = append(value.Value, &dto.SampleValueSeries_Value{
					Timestamp: proto.Int64(sample.Timestamp.Unix()),
					Value:     sample.Value.ToDTO(),
				})
			}

			samplesBatch.Put(key, value)
		}
	}

	err = l.MetricSamples.Commit(samplesBatch)
	if err != nil {
		return
	}

	err = <-indexErrChan
	if err != nil {
		return
	}

	err = <-watermarkErrChan
	if err != nil {
		return
	}

	return
}

func extractSampleKey(i leveldb.Iterator) (key model.SampleKey, err error) {
	k := &dto.SampleKey{}
	err = proto.Unmarshal(i.Key(), k)
	if err != nil {
		return
	}

	key = model.NewSampleKeyFromDTO(k)

	return
}

func extractSampleValues(i leveldb.Iterator) (values model.Values, err error) {
	v := &dto.SampleValueSeries{}
	err = proto.Unmarshal(i.Value(), v)
	if err != nil {
		return
	}

	values = model.NewValuesFromDTO(v)

	return
}

func fingerprintsEqual(l *dto.Fingerprint, r *dto.Fingerprint) bool {
	if l == r {
		return true
	}

	if l == nil && r == nil {
		return true
	}

	if r.Signature == l.Signature {
		return true
	}

	if *r.Signature == *l.Signature {
		return true
	}

	return false
}

func (l *LevelDBMetricPersistence) hasIndexMetric(dto *dto.Metric) (value bool, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasIndexMetric, result: success}, map[string]string{operation: hasIndexMetric, result: failure})
	}(time.Now())

	value, err = l.metricMembershipIndex.Has(dto)

	return
}

func (l *LevelDBMetricPersistence) HasLabelPair(dto *dto.LabelPair) (value bool, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelPair, result: success}, map[string]string{operation: hasLabelPair, result: failure})
	}(time.Now())

	value, err = l.labelSetToFingerprints.Has(dto)

	return
}

func (l *LevelDBMetricPersistence) HasLabelName(dto *dto.LabelName) (value bool, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelName, result: success}, map[string]string{operation: hasLabelName, result: failure})
	}(time.Now())

	value, err = l.labelNameToFingerprints.Has(dto)

	return
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelSet(labelSet model.LabelSet) (fps model.Fingerprints, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getFingerprintsForLabelSet, result: success}, map[string]string{operation: getFingerprintsForLabelSet, result: failure})
	}(time.Now())

	sets := []utility.Set{}

	for _, labelSetDTO := range model.LabelSetToDTOs(&labelSet) {
		unmarshaled := &dto.FingerprintCollection{}
		present, err := l.labelSetToFingerprints.Get(labelSetDTO, unmarshaled)
		if err != nil {
			return fps, err
		}
		if !present {
			return nil, nil
		}

		set := utility.Set{}

		for _, m := range unmarshaled.Member {
			fp := model.NewFingerprintFromRowKey(*m.Signature)
			set.Add(*fp)
		}

		sets = append(sets, set)
	}

	numberOfSets := len(sets)
	if numberOfSets == 0 {
		return
	}

	base := sets[0]
	for i := 1; i < numberOfSets; i++ {
		base = base.Intersection(sets[i])
	}
	for _, e := range base.Elements() {
		fingerprint := e.(model.Fingerprint)
		fps = append(fps, &fingerprint)
	}

	return
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelName(labelName model.LabelName) (fps model.Fingerprints, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getFingerprintsForLabelName, result: success}, map[string]string{operation: getFingerprintsForLabelName, result: failure})
	}(time.Now())

	unmarshaled := &dto.FingerprintCollection{}
	present, err := l.labelNameToFingerprints.Get(model.LabelNameToDTO(&labelName), unmarshaled)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}

	for _, m := range unmarshaled.Member {
		fp := model.NewFingerprintFromRowKey(*m.Signature)
		fps = append(fps, fp)
	}

	return fps, nil
}

func (l *LevelDBMetricPersistence) GetMetricForFingerprint(f *model.Fingerprint) (m model.Metric, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getMetricForFingerprint, result: success}, map[string]string{operation: getMetricForFingerprint, result: failure})
	}(time.Now())

	unmarshaled := &dto.Metric{}
	present, err := l.fingerprintToMetrics.Get(model.FingerprintToDTO(f), unmarshaled)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}

	m = model.Metric{}

	for _, v := range unmarshaled.LabelPair {
		m[model.LabelName(*v.Name)] = model.LabelValue(*v.Value)
	}

	return m, nil
}

func (l LevelDBMetricPersistence) GetValueAtTime(f *model.Fingerprint, t time.Time) model.Values {
	panic("Not implemented")
}

func (l LevelDBMetricPersistence) GetBoundaryValues(f *model.Fingerprint, i model.Interval) model.Values {
	panic("Not implemented")
}

func (l *LevelDBMetricPersistence) GetRangeValues(f *model.Fingerprint, i model.Interval) model.Values {
	panic("Not implemented")
}

type MetricKeyDecoder struct{}

func (d *MetricKeyDecoder) DecodeKey(in interface{}) (out interface{}, err error) {
	unmarshaled := dto.LabelPair{}
	err = proto.Unmarshal(in.([]byte), &unmarshaled)
	if err != nil {
		return
	}

	out = model.LabelPair{
		Name:  model.LabelName(*unmarshaled.Name),
		Value: model.LabelValue(*unmarshaled.Value),
	}

	return
}

func (d *MetricKeyDecoder) DecodeValue(in interface{}) (out interface{}, err error) {
	return
}

type LabelNameFilter struct {
	labelName model.LabelName
}

func (f LabelNameFilter) Filter(key, value interface{}) (filterResult storage.FilterResult) {
	labelPair, ok := key.(model.LabelPair)
	if ok && labelPair.Name == f.labelName {
		return storage.ACCEPT
	}
	return storage.SKIP
}

type CollectLabelValuesOp struct {
	labelValues []model.LabelValue
}

func (op *CollectLabelValuesOp) Operate(key, value interface{}) (err *storage.OperatorError) {
	labelPair := key.(model.LabelPair)
	op.labelValues = append(op.labelValues, model.LabelValue(labelPair.Value))
	return
}

func (l *LevelDBMetricPersistence) GetAllValuesForLabel(labelName model.LabelName) (values model.LabelValues, err error) {
	filter := &LabelNameFilter{
		labelName: labelName,
	}
	labelValuesOp := &CollectLabelValuesOp{}

	_, err = l.labelSetToFingerprints.ForEach(&MetricKeyDecoder{}, filter, labelValuesOp)
	if err != nil {
		return
	}

	values = labelValuesOp.labelValues
	return
}

// CompactKeyspace compacts each database's keyspace serially.
//
// Beware that it would probably be imprudent to run this on a live user-facing
// server due to latency implications.
func (l *LevelDBMetricPersistence) CompactKeyspaces() {
	l.CurationRemarks.CompactKeyspace()
	l.fingerprintToMetrics.CompactKeyspace()
	l.labelNameToFingerprints.CompactKeyspace()
	l.labelSetToFingerprints.CompactKeyspace()
	l.MetricHighWatermarks.CompactKeyspace()
	l.metricMembershipIndex.CompactKeyspace()
	l.MetricSamples.CompactKeyspace()
}

func (l *LevelDBMetricPersistence) ApproximateSizes() (total uint64, err error) {
	size := uint64(0)

	if size, err = l.CurationRemarks.ApproximateSize(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.fingerprintToMetrics.ApproximateSize(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.labelNameToFingerprints.ApproximateSize(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.labelSetToFingerprints.ApproximateSize(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.MetricHighWatermarks.ApproximateSize(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.metricMembershipIndex.ApproximateSize(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.MetricSamples.ApproximateSize(); err != nil {
		return 0, err
	}
	total += size

	return total, nil
}

func (l *LevelDBMetricPersistence) States() []leveldb.DatabaseState {
	states := []leveldb.DatabaseState{}

	state := l.CurationRemarks.State()
	state.Name = "Curation Remarks"
	state.Type = "Watermark"
	states = append(states, state)

	state = l.fingerprintToMetrics.State()
	state.Name = "Fingerprints to Metrics"
	state.Type = "Index"
	states = append(states, state)

	state = l.labelNameToFingerprints.State()
	state.Name = "Label Name to Fingerprints"
	state.Type = "Inverted Index"
	states = append(states, state)

	state = l.labelSetToFingerprints.State()
	state.Name = "Label Pair to Fingerprints"
	state.Type = "Inverted Index"
	states = append(states, state)

	state = l.MetricHighWatermarks.State()
	state.Name = "Metric Last Write"
	state.Type = "Watermark"
	states = append(states, state)

	state = l.metricMembershipIndex.State()
	state.Name = "Metric Membership"
	state.Type = "Index"
	states = append(states, state)

	state = l.MetricSamples.State()
	state.Name = "Samples"
	state.Type = "Time Series"
	states = append(states, state)

	return states
}
