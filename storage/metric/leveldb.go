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
	"flag"
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/coding/indexable"
	"github.com/prometheus/prometheus/model"
	dto "github.com/prometheus/prometheus/model/generated"
	"github.com/prometheus/prometheus/storage"
	index "github.com/prometheus/prometheus/storage/raw/index/leveldb"
	leveldb "github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility"
	"log"
	"sort"
	"sync"
	"time"
)

var (
	leveldbChunkSize = flag.Int("leveldbChunkSize", 200, "Maximum number of samples stored under one key.")

	sortConcurrency = 2
)

type LevelDBMetricPersistence struct {
	fingerprintToMetrics    *leveldb.LevelDBPersistence
	labelNameToFingerprints *leveldb.LevelDBPersistence
	labelSetToFingerprints  *leveldb.LevelDBPersistence
	metricHighWatermarks    *leveldb.LevelDBPersistence
	metricMembershipIndex   *index.LevelDBMembershipIndex
	metricSamples           *leveldb.LevelDBPersistence
}

var (
	// These flag values are back of the envelope, though they seem sensible.
	// Please re-evaluate based on your own needs.
	fingerprintsToLabelPairCacheSize = flag.Int("fingerprintsToLabelPairCacheSizeBytes", 100*1024*1024, "The size for the fingerprint to label pair index (bytes).")
	highWatermarkCacheSize           = flag.Int("highWatermarksByFingerprintSizeBytes", 50*1024*1024, "The size for the metric high watermarks (bytes).")
	samplesByFingerprintCacheSize    = flag.Int("samplesByFingerprintCacheSizeBytes", 500*1024*1024, "The size for the samples database (bytes).")
	labelNameToFingerprintsCacheSize = flag.Int("labelNameToFingerprintsCacheSizeBytes", 100*1024*1024, "The size for the label name to metric fingerprint index (bytes).")
	labelPairToFingerprintsCacheSize = flag.Int("labelPairToFingerprintsCacheSizeBytes", 100*1024*1024, "The size for the label pair to metric fingerprint index (bytes).")
	metricMembershipIndexCacheSize   = flag.Int("metricMembershipCacheSizeBytes", 50*1024*1024, "The size for the metric membership index (bytes).")
)

type leveldbOpener func()
type leveldbCloser interface {
	Close()
}

func (l *LevelDBMetricPersistence) Close() {
	var persistences = []leveldbCloser{
		l.fingerprintToMetrics,
		l.metricHighWatermarks,
		l.metricSamples,
		l.labelNameToFingerprints,
		l.labelSetToFingerprints,
		l.metricMembershipIndex,
	}

	closerGroup := sync.WaitGroup{}

	for _, closer := range persistences {
		closerGroup.Add(1)
		go func(closer leveldbCloser) {
			closer.Close()
			closerGroup.Done()
		}(closer)
	}

	closerGroup.Wait()
}

func NewLevelDBMetricPersistence(baseDirectory string) (persistence *LevelDBMetricPersistence, err error) {
	errorChannel := make(chan error, 6)

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
				errorChannel <- err
			},
		},
		{
			"Samples by Fingerprint",
			func() {
				var err error
				emission.metricSamples, err = leveldb.NewLevelDBPersistence(baseDirectory+"/samples_by_fingerprint", *samplesByFingerprintCacheSize, 10)
				errorChannel <- err
			},
		},
		{
			"High Watermarks by Fingerprint",
			func() {
				var err error
				emission.metricHighWatermarks, err = leveldb.NewLevelDBPersistence(baseDirectory+"/high_watermarks_by_fingerprint", *highWatermarkCacheSize, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name",
			func() {
				var err error
				emission.labelNameToFingerprints, err = leveldb.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name", *labelNameToFingerprintsCacheSize, 10)
				errorChannel <- err
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var err error
				emission.labelSetToFingerprints, err = leveldb.NewLevelDBPersistence(baseDirectory+"/fingerprints_by_label_name_and_value_pair", *labelPairToFingerprintsCacheSize, 10)
				errorChannel <- err
			},
		},
		{
			"Metric Membership Index",
			func() {
				var err error
				emission.metricMembershipIndex, err = index.NewLevelDBMembershipIndex(baseDirectory+"/metric_membership_index", *metricMembershipIndexCacheSize, 10)
				errorChannel <- err
			},
		},
	}

	for _, subsystem := range subsystemOpeners {
		opener := subsystem.opener
		go opener()
	}

	for i := 0; i < cap(errorChannel); i++ {
		err = <-errorChannel

		if err != nil {
			log.Printf("Could not open a LevelDBPersistence storage container: %q\n", err)

			return
		}
	}
	persistence = emission

	return
}

func (l *LevelDBMetricPersistence) AppendSample(sample model.Sample) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSample, result: success}, map[string]string{operation: appendSample, result: failure})
	}()

	err = l.AppendSamples(model.Samples{sample})

	return
}

// groupByFingerprint collects all of the provided samples, groups them
// together by their respective metric fingerprint, and finally sorts
// them chronologically.
func groupByFingerprint(samples model.Samples) map[model.Fingerprint]model.Samples {
	var (
		fingerprintToSamples = map[model.Fingerprint]model.Samples{}
	)

	for _, sample := range samples {
		fingerprint := model.NewFingerprintFromMetric(sample.Metric)
		samples := fingerprintToSamples[fingerprint]
		samples = append(samples, sample)
		fingerprintToSamples[fingerprint] = samples
	}

	var (
		sortingSemaphore = make(chan bool, sortConcurrency)
		doneSorting      sync.WaitGroup
	)

	for i := 0; i < sortConcurrency; i++ {
		sortingSemaphore <- true
	}

	for _, samples := range fingerprintToSamples {
		doneSorting.Add(1)

		go func(samples model.Samples) {
			<-sortingSemaphore
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
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: findUnindexedMetrics, result: success}, map[string]string{operation: findUnindexedMetrics, result: failure})
	}()

	unindexed = make(map[model.Fingerprint]model.Metric)

	// Determine which metrics are unknown in the database.
	for fingerprint, metric := range candidates {
		var (
			dto           = model.MetricToDTO(metric)
			indexHas, err = l.hasIndexMetric(dto)
		)
		if err != nil {
			panic(err)
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
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: indexLabelNames, result: success}, map[string]string{operation: indexLabelNames, result: failure})
	}()

	labelNameFingerprints := map[model.LabelName]utility.Set{}

	for fingerprint, metric := range metrics {
		for labelName := range metric {
			fingerprintSet, ok := labelNameFingerprints[labelName]
			if !ok {
				fingerprintSet = utility.Set{}

				fingerprints, err := l.GetFingerprintsForLabelName(labelName)
				if err != nil {
					panic(err)
					return err
				}

				for _, fingerprint := range fingerprints {
					fingerprintSet.Add(fingerprint)
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
		for fingerprint := range fingerprintSet {
			fingerprints = append(fingerprints, fingerprint.(model.Fingerprint))
		}

		sort.Sort(fingerprints)

		key := &dto.LabelName{
			Name: proto.String(string(labelName)),
		}
		value := &dto.FingerprintCollection{}
		for _, fingerprint := range fingerprints {
			value.Member = append(value.Member, fingerprint.ToDTO())
		}

		batch.Put(coding.NewProtocolBuffer(key), coding.NewProtocolBuffer(value))
	}

	err = l.labelNameToFingerprints.Commit(batch)
	if err != nil {
		panic(err)
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
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: indexLabelPairs, result: success}, map[string]string{operation: indexLabelPairs, result: failure})
	}()

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
					panic(err)
					return err
				}

				for _, fingerprint := range fingerprints {
					fingerprintSet.Add(fingerprint)
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
		for fingerprint := range fingerprintSet {
			fingerprints = append(fingerprints, fingerprint.(model.Fingerprint))
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

		batch.Put(coding.NewProtocolBuffer(key), coding.NewProtocolBuffer(value))
	}

	err = l.labelSetToFingerprints.Commit(batch)
	if err != nil {
		panic(err)
		return
	}

	return
}

// indexFingerprints updates all of the Fingerprint to Metric reverse lookups
// in the index and then bulk updates.
//
// This operation is idempotent.
func (l *LevelDBMetricPersistence) indexFingerprints(metrics map[model.Fingerprint]model.Metric) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: indexFingerprints, result: success}, map[string]string{operation: indexFingerprints, result: failure})
	}()

	batch := leveldb.NewBatch()
	defer batch.Close()

	for fingerprint, metric := range metrics {
		key := coding.NewProtocolBuffer(fingerprint.ToDTO())
		value := coding.NewProtocolBuffer(model.MetricToDTO(metric))
		batch.Put(key, value)
	}

	err = l.fingerprintToMetrics.Commit(batch)
	if err != nil {
		panic(err)
	}

	return
}

// indexMetrics takes groups of samples, determines which ones contain metrics
// that are unknown to the storage stack, and then proceeds to update all
// affected indices.
func (l *LevelDBMetricPersistence) indexMetrics(fingerprints map[model.Fingerprint]model.Metric) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: indexMetrics, result: success}, map[string]string{operation: indexMetrics, result: failure})
	}()

	var (
		absentMetrics map[model.Fingerprint]model.Metric
	)

	absentMetrics, err = l.findUnindexedMetrics(fingerprints)
	if err != nil {
		panic(err)
	}

	if len(absentMetrics) == 0 {
		return
	}

	// TODO: For the missing fingerprints, determine what label names and pairs
	// are absent and act accordingly and append fingerprints.
	var (
		doneBuildingLabelNameIndex   = make(chan error)
		doneBuildingLabelPairIndex   = make(chan error)
		doneBuildingFingerprintIndex = make(chan error)
	)

	go func() {
		doneBuildingLabelNameIndex <- l.indexLabelNames(absentMetrics)
	}()

	go func() {
		doneBuildingLabelPairIndex <- l.indexLabelPairs(absentMetrics)
	}()

	go func() {
		doneBuildingFingerprintIndex <- l.indexFingerprints(absentMetrics)
	}()

	makeTopLevelIndex := true

	err = <-doneBuildingLabelNameIndex
	if err != nil {
		panic(err)
		makeTopLevelIndex = false
	}
	err = <-doneBuildingLabelPairIndex
	if err != nil {
		panic(err)
		makeTopLevelIndex = false
	}
	err = <-doneBuildingFingerprintIndex
	if err != nil {
		panic(err)
		makeTopLevelIndex = false
	}

	// If any of the preceding operations failed, we will have inconsistent
	// indices.  Thusly, the Metric membership index should NOT be updated, as
	// its state is used to determine whether to bulk update the other indices.
	// Given that those operations are idempotent, it is OK to repeat them;
	// however, it will consume considerable amounts of time.
	if makeTopLevelIndex {
		batch := leveldb.NewBatch()
		defer batch.Close()

		// WART: We should probably encode simple fingerprints.
		for _, metric := range absentMetrics {
			key := coding.NewProtocolBuffer(model.MetricToDTO(metric))
			batch.Put(key, key)
		}

		err := l.metricMembershipIndex.Commit(batch)
		if err != nil {
			panic(err)
			// Not critical.
			log.Println(err)
		}
	}

	return
}

func (l *LevelDBMetricPersistence) refreshHighWatermarks(groups map[model.Fingerprint]model.Samples) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: refreshHighWatermarks, result: success}, map[string]string{operation: refreshHighWatermarks, result: failure})
	}()

	batch := leveldb.NewBatch()
	defer batch.Close()

	var (
		mutationCount = 0
	)
	for fingerprint, samples := range groups {
		var (
			key                   = &dto.Fingerprint{}
			value                 = &dto.MetricHighWatermark{}
			raw                   []byte
			newestSampleTimestamp = samples[len(samples)-1].Timestamp
			keyEncoded            = coding.NewProtocolBuffer(key)
		)

		key.Signature = proto.String(fingerprint.ToRowKey())
		raw, err = l.metricHighWatermarks.Get(keyEncoded)
		if err != nil {
			panic(err)
			return
		}

		if raw != nil {
			err = proto.Unmarshal(raw, value)
			if err != nil {
				panic(err)
				continue
			}

			if newestSampleTimestamp.Before(time.Unix(*value.Timestamp, 0)) {
				continue
			}
		}
		value.Timestamp = proto.Int64(newestSampleTimestamp.Unix())
		batch.Put(keyEncoded, coding.NewProtocolBuffer(value))
		mutationCount++
	}

	err = l.metricHighWatermarks.Commit(batch)
	if err != nil {
		panic(err)
	}

	return
}

func (l *LevelDBMetricPersistence) AppendSamples(samples model.Samples) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSamples, result: success}, map[string]string{operation: appendSamples, result: failure})
	}()

	var (
		fingerprintToSamples = groupByFingerprint(samples)
		indexErrChan         = make(chan error)
		watermarkErrChan     = make(chan error)
	)

	go func(groups map[model.Fingerprint]model.Samples) {
		var (
			metrics = map[model.Fingerprint]model.Metric{}
		)

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

			key := &dto.SampleKey{
				Fingerprint:   fingerprint.ToDTO(),
				Timestamp:     indexable.EncodeTime(chunk[0].Timestamp),
				LastTimestamp: proto.Int64(chunk[take-1].Timestamp.Unix()),
				SampleCount:   proto.Uint32(uint32(take)),
			}

			value := &dto.SampleValueSeries{}
			for _, sample := range chunk {
				value.Value = append(value.Value, &dto.SampleValueSeries_Value{
					Timestamp: proto.Int64(sample.Timestamp.Unix()),
					Value:     sample.Value.ToDTO(),
				})
			}

			samplesBatch.Put(coding.NewProtocolBuffer(key), coding.NewProtocolBuffer(value))
		}
	}

	err = l.metricSamples.Commit(samplesBatch)
	if err != nil {
		panic(err)
	}

	err = <-indexErrChan
	if err != nil {
		panic(err)
	}

	err = <-watermarkErrChan
	if err != nil {
		panic(err)
	}

	return
}

func extractSampleKey(i leveldb.Iterator) (k *dto.SampleKey, err error) {
	if i == nil {
		panic("nil iterator")
	}

	k = &dto.SampleKey{}
	rawKey := i.Key()
	if rawKey == nil {
		panic("illegal condition; got nil key...")
	}
	err = proto.Unmarshal(rawKey, k)

	return
}

func extractSampleValues(i leveldb.Iterator) (v *dto.SampleValueSeries, err error) {
	if i == nil {
		panic("nil iterator")
	}

	v = &dto.SampleValueSeries{}
	err = proto.Unmarshal(i.Value(), v)

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

type sampleKeyPredicate func(k *dto.SampleKey) bool

func keyIsOlderThan(t time.Time) sampleKeyPredicate {
	return func(k *dto.SampleKey) bool {
		return indexable.DecodeTime(k.Timestamp).After(t)
	}
}

func keyIsAtMostOld(t time.Time) sampleKeyPredicate {
	return func(k *dto.SampleKey) bool {
		return !indexable.DecodeTime(k.Timestamp).After(t)
	}
}

func (l *LevelDBMetricPersistence) hasIndexMetric(dto *dto.Metric) (value bool, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasIndexMetric, result: success}, map[string]string{operation: hasIndexMetric, result: failure})
	}()

	dtoKey := coding.NewProtocolBuffer(dto)
	value, err = l.metricMembershipIndex.Has(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) HasLabelPair(dto *dto.LabelPair) (value bool, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelPair, result: success}, map[string]string{operation: hasLabelPair, result: failure})
	}()

	dtoKey := coding.NewProtocolBuffer(dto)
	value, err = l.labelSetToFingerprints.Has(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) HasLabelName(dto *dto.LabelName) (value bool, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelName, result: success}, map[string]string{operation: hasLabelName, result: failure})
	}()

	dtoKey := coding.NewProtocolBuffer(dto)
	value, err = l.labelNameToFingerprints.Has(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelSet(labelSet model.LabelSet) (fps model.Fingerprints, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getFingerprintsForLabelSet, result: success}, map[string]string{operation: getFingerprintsForLabelSet, result: failure})
	}()

	sets := []utility.Set{}

	for _, labelSetDTO := range model.LabelSetToDTOs(&labelSet) {
		f, err := l.labelSetToFingerprints.Get(coding.NewProtocolBuffer(labelSetDTO))
		if err != nil {
			return fps, err
		}

		unmarshaled := &dto.FingerprintCollection{}
		err = proto.Unmarshal(f, unmarshaled)
		if err != nil {
			return fps, err
		}

		set := utility.Set{}

		for _, m := range unmarshaled.Member {
			fp := model.NewFingerprintFromRowKey(*m.Signature)
			set.Add(fp)
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
		fps = append(fps, fingerprint)
	}

	return
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelName(labelName model.LabelName) (fps model.Fingerprints, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getFingerprintsForLabelName, result: success}, map[string]string{operation: getFingerprintsForLabelName, result: failure})
	}()

	raw, err := l.labelNameToFingerprints.Get(coding.NewProtocolBuffer(model.LabelNameToDTO(&labelName)))
	if err != nil {
		return
	}

	unmarshaled := &dto.FingerprintCollection{}

	err = proto.Unmarshal(raw, unmarshaled)
	if err != nil {
		return
	}

	for _, m := range unmarshaled.Member {
		fp := model.NewFingerprintFromRowKey(*m.Signature)
		fps = append(fps, fp)
	}

	return
}

func (l *LevelDBMetricPersistence) GetMetricForFingerprint(f model.Fingerprint) (m *model.Metric, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getMetricForFingerprint, result: success}, map[string]string{operation: getMetricForFingerprint, result: failure})
	}()

	raw, err := l.fingerprintToMetrics.Get(coding.NewProtocolBuffer(model.FingerprintToDTO(f)))
	if err != nil {
		return
	}

	unmarshaled := &dto.Metric{}
	err = proto.Unmarshal(raw, unmarshaled)
	if err != nil {
		return
	}

	metric := model.Metric{}

	for _, v := range unmarshaled.LabelPair {
		metric[model.LabelName(*v.Name)] = model.LabelValue(*v.Value)
	}

	// Explicit address passing here shaves immense amounts of time off of the
	// code flow due to less tight-loop dereferencing.
	m = &metric

	return
}

func (l LevelDBMetricPersistence) GetValueAtTime(f model.Fingerprint, t time.Time) (samples model.Values) {
	panic("Not implemented")
}

func (l LevelDBMetricPersistence) GetBoundaryValues(f model.Fingerprint, i model.Interval) (first model.Values, second model.Values) {
	panic("Not implemented")
}

func (l *LevelDBMetricPersistence) GetRangeValues(f model.Fingerprint, i model.Interval) (samples model.Values) {
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

func (l *LevelDBMetricPersistence) ForEachSample(builder IteratorsForFingerprintBuilder) (err error) {
	panic("not implemented")
}
