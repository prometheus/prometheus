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
	"io"
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
	metricSamples           *leveldb.LevelDBPersistence
	labelNameToFingerprints *leveldb.LevelDBPersistence
	labelSetToFingerprints  *leveldb.LevelDBPersistence
	metricMembershipIndex   *index.LevelDBMembershipIndex
}

var (
	// These flag values are back of the envelope, though they seem sensible.
	// Please re-evaluate based on your own needs.
	fingerprintsToLabelPairCacheSize = flag.Int("fingerprintsToLabelPairCacheSizeBytes", 100*1024*1024, "The size for the fingerprint to label pair index (bytes).")
	samplesByFingerprintCacheSize    = flag.Int("samplesByFingerprintCacheSizeBytes", 500*1024*1024, "The size for the samples database (bytes).")
	labelNameToFingerprintsCacheSize = flag.Int("labelNameToFingerprintsCacheSizeBytes", 100*1024*1024, "The size for the label name to metric fingerprint index (bytes).")
	labelPairToFingerprintsCacheSize = flag.Int("labelPairToFingerprintsCacheSizeBytes", 100*1024*1024, "The size for the label pair to metric fingerprint index (bytes).")
	metricMembershipIndexCacheSize   = flag.Int("metricMembershipCacheSizeBytes", 50*1024*1024, "The size for the metric membership index (bytes).")
)

type leveldbOpener func()

func (l *LevelDBMetricPersistence) Close() error {
	var persistences = []struct {
		name   string
		closer io.Closer
	}{
		{
			"Fingerprint to Label Name and Value Pairs",
			l.fingerprintToMetrics,
		},
		{
			"Fingerprint Samples",
			l.metricSamples,
		},
		{
			"Label Name to Fingerprints",
			l.labelNameToFingerprints,
		},
		{
			"Label Name and Value Pairs to Fingerprints",
			l.labelSetToFingerprints,
		},
		{
			"Metric Membership Index",
			l.metricMembershipIndex,
		},
	}

	errorChannel := make(chan error, len(persistences))

	for _, persistence := range persistences {
		name := persistence.name
		closer := persistence.closer

		go func(name string, closer io.Closer) {
			if closer != nil {
				closingError := closer.Close()

				if closingError != nil {
					log.Printf("Could not close a LevelDBPersistence storage container; inconsistencies are possible: %q\n", closingError)
				}

				errorChannel <- closingError
			} else {
				errorChannel <- nil
			}
		}(name, closer)
	}

	for i := 0; i < cap(errorChannel); i++ {
		closingError := <-errorChannel

		if closingError != nil {
			return closingError
		}
	}

	return nil
}

func NewLevelDBMetricPersistence(baseDirectory string) (persistence *LevelDBMetricPersistence, err error) {
	errorChannel := make(chan error, 5)

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

		batch.Put(coding.NewProtocolBufferEncoder(key), coding.NewProtocolBufferEncoder(value))
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

		batch.Put(coding.NewProtocolBufferEncoder(key), coding.NewProtocolBufferEncoder(value))
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
		key := coding.NewProtocolBufferEncoder(fingerprint.ToDTO())
		value := coding.NewProtocolBufferEncoder(model.MetricToDTO(metric))
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
			key := coding.NewProtocolBufferEncoder(model.MetricToDTO(metric))
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

func (l *LevelDBMetricPersistence) AppendSamples(samples model.Samples) (err error) {
	begin := time.Now()
	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSamples, result: success}, map[string]string{operation: appendSamples, result: failure})
	}()

	var (
		fingerprintToSamples = groupByFingerprint(samples)
		indexErrChan         = make(chan error)
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
					Value:     proto.Float32(float32(sample.Value)),
				})
			}

			samplesBatch.Put(coding.NewProtocolBufferEncoder(key), coding.NewProtocolBufferEncoder(value))
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

	return
}

func extractSampleKey(i iterator) (k *dto.SampleKey, err error) {
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

func extractSampleValue(i iterator) (v *dto.SampleValueSeries, err error) {
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

	dtoKey := coding.NewProtocolBufferEncoder(dto)
	value, err = l.metricMembershipIndex.Has(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) HasLabelPair(dto *dto.LabelPair) (value bool, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelPair, result: success}, map[string]string{operation: hasLabelPair, result: failure})
	}()

	dtoKey := coding.NewProtocolBufferEncoder(dto)
	value, err = l.labelSetToFingerprints.Has(dtoKey)

	return
}

func (l *LevelDBMetricPersistence) HasLabelName(dto *dto.LabelName) (value bool, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelName, result: success}, map[string]string{operation: hasLabelName, result: failure})
	}()

	dtoKey := coding.NewProtocolBufferEncoder(dto)
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
		f, err := l.labelSetToFingerprints.Get(coding.NewProtocolBufferEncoder(labelSetDTO))
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

	raw, err := l.labelNameToFingerprints.Get(coding.NewProtocolBufferEncoder(model.LabelNameToDTO(&labelName)))
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

	raw, err := l.fingerprintToMetrics.Get(coding.NewProtocolBufferEncoder(model.FingerprintToDTO(f)))
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

func (l *LevelDBMetricPersistence) GetBoundaryValues(m model.Metric, i model.Interval, s StalenessPolicy) (open *model.Sample, end *model.Sample, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getBoundaryValues, result: success}, map[string]string{operation: getBoundaryValues, result: failure})
	}()

	// XXX: Maybe we will want to emit incomplete sets?
	open, err = l.GetValueAtTime(m, i.OldestInclusive, s)
	if err != nil {
		return
	} else if open == nil {
		return
	}

	end, err = l.GetValueAtTime(m, i.NewestInclusive, s)
	if err != nil {
		return
	} else if end == nil {
		open = nil
	}

	return
}

func interpolate(x1, x2 time.Time, y1, y2 float32, e time.Time) float32 {
	yDelta := y2 - y1
	xDelta := x2.Sub(x1)

	dDt := yDelta / float32(xDelta)
	offset := float32(e.Sub(x1))

	return y1 + (offset * dDt)
}

type iterator interface {
	Close()
	Key() []byte
	Next()
	Prev()
	Seek([]byte)
	SeekToFirst()
	SeekToLast()
	Valid() bool
	Value() []byte
}

func (l *LevelDBMetricPersistence) GetValueAtTime(m model.Metric, t time.Time, s StalenessPolicy) (sample *model.Sample, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getValueAtTime, result: success}, map[string]string{operation: getValueAtTime, result: failure})
	}()

	f := model.NewFingerprintFromMetric(m).ToDTO()

	// Candidate for Refactoring
	k := &dto.SampleKey{
		Fingerprint: f,
		Timestamp:   indexable.EncodeTime(t),
	}

	e, err := coding.NewProtocolBufferEncoder(k).Encode()
	if err != nil {
		return
	}

	iterator, closer, err := l.metricSamples.GetIterator()
	if err != nil {
		return
	}

	defer closer.Close()

	iterator.Seek(e)
	if !iterator.Valid() {
		/*
		 * Two cases for this:
		 * 1.) Corruption in LevelDB.
		 * 2.) Key seek after AND outside known range.
		 *
		 * Once a LevelDB iterator goes invalid, it cannot be recovered; thusly,
		 * we need to create a new in order to check if the last value in the
		 * database is sufficient for our purposes.  This is, in all reality, a
		 * corner case but one that could bring down the system.
		 */
		iterator, closer, err = l.metricSamples.GetIterator()
		if err != nil {
			return
		}
		defer closer.Close()
		iterator.SeekToLast()
		if !iterator.Valid() {
			/*
			 * For whatever reason, the LevelDB cannot be recovered.
			 */
			return
		}
	}

	var (
		firstKey   *dto.SampleKey
		firstValue *dto.SampleValueSeries
	)

	firstKey, err = extractSampleKey(iterator)
	if err != nil {
		return
	}

	peekAhead := false

	if !fingerprintsEqual(firstKey.Fingerprint, k.Fingerprint) {
		/*
		 * This allows us to grab values for metrics if our request time is after
		 * the last recorded time subject to the staleness policy due to the nuances
		 * of LevelDB storage:
		 *
		 * # Assumptions:
		 * - K0 < K1 in terms of sorting.
		 * - T0 < T1 in terms of sorting.
		 *
		 * # Data
		 *
		 * K0-T0
		 * K0-T1
		 * K0-T2
		 * K1-T0
		 * K1-T1
		 *
		 * # Scenario
		 * K0-T3, which does not exist, is requested.  LevelDB will thusly seek to
		 * K1-T1, when K0-T2 exists as a perfectly good candidate to check subject
		 * to the provided staleness policy and such.
		 */
		peekAhead = true
	}

	firstTime := indexable.DecodeTime(firstKey.Timestamp)
	if t.Before(firstTime) || peekAhead {
		iterator.Prev()
		if !iterator.Valid() {
			/*
			 * Two cases for this:
			 * 1.) Corruption in LevelDB.
			 * 2.) Key seek before AND outside known range.
			 *
			 * This is an explicit validation to ensure that if no previous values for
			 * the series are found, the query aborts.
			 */
			return
		}

		var (
			alternativeKey   *dto.SampleKey
			alternativeValue *dto.SampleValueSeries
		)

		alternativeKey, err = extractSampleKey(iterator)
		if err != nil {
			return
		}

		if !fingerprintsEqual(alternativeKey.Fingerprint, k.Fingerprint) {
			return
		}

		/*
		 * At this point, we found a previous value in the same series in the
		 * database.  LevelDB originally seeked to the subsequent element given
		 * the key, but we need to consider this adjacency instead.
		 */
		alternativeTime := indexable.DecodeTime(alternativeKey.Timestamp)

		firstKey = alternativeKey
		firstValue = alternativeValue
		firstTime = alternativeTime
	}

	firstDelta := firstTime.Sub(t)
	if firstDelta < 0 {
		firstDelta *= -1
	}
	if firstDelta > s.DeltaAllowance {
		return
	}

	firstValue, err = extractSampleValue(iterator)
	if err != nil {
		return
	}

	sample = model.SampleFromDTO(&m, &t, firstValue)

	if firstDelta == time.Duration(0) {
		return
	}

	iterator.Next()
	if !iterator.Valid() {
		/*
		 * Two cases for this:
		 * 1.) Corruption in LevelDB.
		 * 2.) Key seek after AND outside known range.
		 *
		 * This means that there are no more values left in the storage; and if this
		 * point is reached, we know that the one that has been found is within the
		 * allowed staleness limits.
		 */
		return
	}

	var secondKey *dto.SampleKey

	secondKey, err = extractSampleKey(iterator)
	if err != nil {
		return
	}

	if !fingerprintsEqual(secondKey.Fingerprint, k.Fingerprint) {
		return
	} else {
		/*
		 * At this point, current entry in the database has the same key as the
		 * previous.  For this reason, the validation logic will expect that the
		 * distance between the two points shall not exceed the staleness policy
		 * allowed limit to reduce interpolation errors.
		 *
		 * For this reason, the sample is reset in case of other subsequent
		 * validation behaviors.
		 */
		sample = nil
	}

	secondTime := indexable.DecodeTime(secondKey.Timestamp)

	totalDelta := secondTime.Sub(firstTime)
	if totalDelta > s.DeltaAllowance {
		return
	}

	var secondValue *dto.SampleValueSeries

	secondValue, err = extractSampleValue(iterator)
	if err != nil {
		return
	}

	fValue := *firstValue.Value[0].Value
	sValue := *secondValue.Value[0].Value

	interpolated := interpolate(firstTime, secondTime, fValue, sValue, t)

	sampleValue := &dto.SampleValueSeries{}
	sampleValue.Value = append(sampleValue.Value, &dto.SampleValueSeries_Value{Value: &interpolated})

	sample = model.SampleFromDTO(&m, &t, sampleValue)

	return
}

func (l *LevelDBMetricPersistence) GetRangeValues(m model.Metric, i model.Interval) (v *model.SampleSet, err error) {
	begin := time.Now()

	defer func() {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getRangeValues, result: success}, map[string]string{operation: getRangeValues, result: failure})
	}()
	f := model.NewFingerprintFromMetric(m).ToDTO()

	k := &dto.SampleKey{
		Fingerprint: f,
		Timestamp:   indexable.EncodeTime(i.OldestInclusive),
	}

	e, err := coding.NewProtocolBufferEncoder(k).Encode()
	if err != nil {
		return
	}

	iterator, closer, err := l.metricSamples.GetIterator()
	if err != nil {
		return
	}
	defer closer.Close()

	iterator.Seek(e)

	predicate := keyIsOlderThan(i.NewestInclusive)

	for ; iterator.Valid(); iterator.Next() {
		retrievedKey := &dto.SampleKey{}

		retrievedKey, err = extractSampleKey(iterator)
		if err != nil {
			return
		}

		if predicate(retrievedKey) {
			break
		}

		if !fingerprintsEqual(retrievedKey.Fingerprint, k.Fingerprint) {
			break
		}

		retrievedValue, err := extractSampleValue(iterator)
		if err != nil {
			return nil, err
		}

		if v == nil {
			v = &model.SampleSet{}
		}

		v.Values = append(v.Values, model.SamplePair{
			Value:     model.SampleValue(*retrievedValue.Value[0].Value),
			Timestamp: indexable.DecodeTime(retrievedKey.Timestamp),
		})
	}

	// XXX: We should not explicitly sort here but rather rely on the datastore.
	//      This adds appreciable overhead.
	if v != nil {
		sort.Sort(v.Values)
	}

	return
}

type MetricKeyDecoder struct{}

func (d *MetricKeyDecoder) DecodeKey(in interface{}) (out interface{}, err error) {
	unmarshaled := &dto.LabelPair{}
	err = proto.Unmarshal(in.([]byte), unmarshaled)
	if err != nil {
		return
	}

	out = unmarshaled

	return
}

func (d *MetricKeyDecoder) DecodeValue(in interface{}) (out interface{}, err error) {
	return
}

type MetricNamesFilter struct{}

func (f *MetricNamesFilter) Filter(key, value interface{}) (filterResult storage.FilterResult) {
	unmarshaled, ok := key.(*dto.LabelPair)
	if ok && *unmarshaled.Name == "name" {
		return storage.ACCEPT
	}
	return storage.SKIP
}

type CollectMetricNamesOp struct {
	metricNames []string
}

func (op *CollectMetricNamesOp) Operate(key, value interface{}) (err *storage.OperatorError) {
	unmarshaled := key.(*dto.LabelPair)
	op.metricNames = append(op.metricNames, *unmarshaled.Value)
	return
}

func (l *LevelDBMetricPersistence) GetAllMetricNames() (metricNames []string, err error) {
	metricNamesOp := &CollectMetricNamesOp{}

	_, err = l.labelSetToFingerprints.ForEach(&MetricKeyDecoder{}, &MetricNamesFilter{}, metricNamesOp)
	if err != nil {
		return
	}

	metricNames = metricNamesOp.metricNames
	return
}

func (l *LevelDBMetricPersistence) ForEachSample(builder IteratorsForFingerprintBuilder) (err error) {
	panic("not implemented")
}
