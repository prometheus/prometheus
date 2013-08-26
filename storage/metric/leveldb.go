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
	"sort"
	"sync"
	"time"

	"code.google.com/p/goprotobuf/proto"
	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility"

	dto "github.com/prometheus/prometheus/model/generated"
)

const sortConcurrency = 2

type LevelDBMetricPersistence struct {
	CurationRemarks         CurationRemarker
	FingerprintToMetrics    FingerprintMetricIndex
	LabelNameToFingerprints LabelNameFingerprintIndex
	LabelSetToFingerprints  LabelPairFingerprintIndex
	MetricHighWatermarks    HighWatermarker
	MetricMembershipIndex   MetricMembershipIndex

	Indexer MetricIndexer

	MetricSamples *leveldb.LevelDBPersistence

	// The remaining indices will be replaced with generalized interface resolvers:
	//
	// type FingerprintResolver interface {
	// 	GetFingerprintForMetric(clientmodel.Metric) (*clientmodel.Fingerprint, bool, error)
	// 	GetFingerprintsForLabelName(clientmodel.LabelName) (clientmodel.Fingerprints, bool, error)
	// 	GetFingerprintsForLabelSet(LabelPair) (clientmodel.Fingerprints, bool, error)
	// }

	// type MetricResolver interface {
	// 	GetMetricsForFingerprint(clientmodel.Fingerprints) (FingerprintMetricMapping, bool, error)
	// }
}

var (
	leveldbChunkSize = flag.Int("leveldbChunkSize", 200, "Maximum number of samples stored under one key.")

	// These flag values are back of the envelope, though they seem sensible.
	// Please re-evaluate based on your own needs.
	curationRemarksCacheSize         = flag.Int("curationRemarksCacheSize", 5*1024*1024, "The size for the curation remarks cache (bytes).")
	fingerprintsToLabelPairCacheSize = flag.Int("fingerprintsToLabelPairCacheSizeBytes", 25*1024*1024, "The size for the fingerprint to label pair index (bytes).")
	highWatermarkCacheSize           = flag.Int("highWatermarksByFingerprintSizeBytes", 5*1024*1024, "The size for the metric high watermarks (bytes).")
	labelNameToFingerprintsCacheSize = flag.Int("labelNameToFingerprintsCacheSizeBytes", 25*1024*1024, "The size for the label name to metric fingerprint index (bytes).")
	labelPairToFingerprintsCacheSize = flag.Int("labelPairToFingerprintsCacheSizeBytes", 25*1024*1024, "The size for the label pair to metric fingerprint index (bytes).")
	metricMembershipIndexCacheSize   = flag.Int("metricMembershipCacheSizeBytes", 5*1024*1024, "The size for the metric membership index (bytes).")
	samplesByFingerprintCacheSize    = flag.Int("samplesByFingerprintCacheSizeBytes", 50*1024*1024, "The size for the samples database (bytes).")
)

type leveldbOpener func()
type errorCloser interface {
	Close() error
}
type closer interface {
	Close()
}

func (l *LevelDBMetricPersistence) Close() {
	var persistences = []interface{}{
		l.CurationRemarks,
		l.FingerprintToMetrics,
		l.LabelNameToFingerprints,
		l.LabelSetToFingerprints,
		l.MetricHighWatermarks,
		l.MetricMembershipIndex,
		l.MetricSamples,
	}

	closerGroup := sync.WaitGroup{}

	for _, c := range persistences {
		closerGroup.Add(1)
		go func(c interface{}) {
			if c != nil {
				switch closer := c.(type) {
				case closer:
					closer.Close()
				case errorCloser:
					if err := closer.Close(); err != nil {
						glog.Error("Error closing persistence: ", err)
					}
				}
			}
			closerGroup.Done()
		}(c)
	}

	closerGroup.Wait()
}

func NewLevelDBMetricPersistence(baseDirectory string) (*LevelDBMetricPersistence, error) {
	workers := utility.NewUncertaintyGroup(7)

	emission := new(LevelDBMetricPersistence)

	var subsystemOpeners = []struct {
		name   string
		opener leveldbOpener
	}{
		{
			"Label Names and Value Pairs by Fingerprint",
			func() {
				var err error
				emission.FingerprintToMetrics, err = NewLevelDBFingerprintMetricIndex(LevelDBFingerprintMetricIndexOptions{
					LevelDBOptions: leveldb.LevelDBOptions{
						Name:           "Metrics by Fingerprint",
						Purpose:        "Index",
						Path:           baseDirectory + "/label_name_and_value_pairs_by_fingerprint",
						CacheSizeBytes: *fingerprintsToLabelPairCacheSize,
					},
				})
				workers.MayFail(err)
			},
		},
		{
			"Samples by Fingerprint",
			func() {
				var err error
				emission.MetricSamples, err = leveldb.NewLevelDBPersistence(leveldb.LevelDBOptions{
					Name:           "Samples",
					Purpose:        "Timeseries",
					Path:           baseDirectory + "/samples_by_fingerprint",
					CacheSizeBytes: *fingerprintsToLabelPairCacheSize,
				})
				workers.MayFail(err)
			},
		},
		{
			"High Watermarks by Fingerprint",
			func() {
				var err error
				emission.MetricHighWatermarks, err = NewLevelDBHighWatermarker(LevelDBHighWatermarkerOptions{
					LevelDBOptions: leveldb.LevelDBOptions{
						Name:           "High Watermarks",
						Purpose:        "The youngest sample in the database per metric.",
						Path:           baseDirectory + "/high_watermarks_by_fingerprint",
						CacheSizeBytes: *highWatermarkCacheSize,
					}})
				workers.MayFail(err)
			},
		},
		{
			"Fingerprints by Label Name",
			func() {
				var err error
				emission.LabelNameToFingerprints, err = NewLevelLabelNameFingerprintIndex(LevelDBLabelNameFingerprintIndexOptions{
					LevelDBOptions: leveldb.LevelDBOptions{
						Name:           "Fingerprints by Label Name",
						Purpose:        "Index",
						Path:           baseDirectory + "/fingerprints_by_label_name",
						CacheSizeBytes: *labelNameToFingerprintsCacheSize,
					},
				})
				workers.MayFail(err)
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var err error
				emission.LabelSetToFingerprints, err = NewLevelDBLabelSetFingerprintIndex(LevelDBLabelSetFingerprintIndexOptions{
					LevelDBOptions: leveldb.LevelDBOptions{
						Name:           "Fingerprints by Label Pair",
						Purpose:        "Index",
						Path:           baseDirectory + "/fingerprints_by_label_name_and_value_pair",
						CacheSizeBytes: *labelPairToFingerprintsCacheSize,
					},
				})
				workers.MayFail(err)
			},
		},
		{
			"Metric Membership Index",
			func() {
				var err error
				emission.MetricMembershipIndex, err = NewLevelDBMetricMembershipIndex(
					LevelDBMetricMembershipIndexOptions{
						LevelDBOptions: leveldb.LevelDBOptions{
							Name:           "Metric Membership",
							Purpose:        "Index",
							Path:           baseDirectory + "/metric_membership_index",
							CacheSizeBytes: *metricMembershipIndexCacheSize,
						},
					})
				workers.MayFail(err)
			},
		},
		{
			"Sample Curation Remarks",
			func() {
				var err error
				emission.CurationRemarks, err = NewLevelDBCurationRemarker(LevelDBCurationRemarkerOptions{
					LevelDBOptions: leveldb.LevelDBOptions{
						Name:           "Sample Curation Remarks",
						Purpose:        "Ledger of Progress for Various Curators",
						Path:           baseDirectory + "/curation_remarks",
						CacheSizeBytes: *curationRemarksCacheSize,
					},
				})
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
			glog.Error("Could not open storage: ", err)
		}

		return nil, fmt.Errorf("Unable to open metric persistence.")
	}

	emission.Indexer = &TotalIndexer{
		FingerprintToMetric:    emission.FingerprintToMetrics,
		LabelNameToFingerprint: emission.LabelNameToFingerprints,
		LabelPairToFingerprint: emission.LabelSetToFingerprints,
		MetricMembership:       emission.MetricMembershipIndex,
	}

	return emission, nil
}

func (l *LevelDBMetricPersistence) AppendSample(sample *clientmodel.Sample) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSample, result: success}, map[string]string{operation: appendSample, result: failure})
	}(time.Now())

	err = l.AppendSamples(clientmodel.Samples{sample})

	return
}

// groupByFingerprint collects all of the provided samples, groups them
// together by their respective metric fingerprint, and finally sorts
// them chronologically.
func groupByFingerprint(samples clientmodel.Samples) map[clientmodel.Fingerprint]clientmodel.Samples {
	fingerprintToSamples := map[clientmodel.Fingerprint]clientmodel.Samples{}

	for _, sample := range samples {
		fingerprint := &clientmodel.Fingerprint{}
		fingerprint.LoadFromMetric(sample.Metric)
		samples := fingerprintToSamples[*fingerprint]
		samples = append(samples, sample)
		fingerprintToSamples[*fingerprint] = samples
	}

	sortingSemaphore := make(chan bool, sortConcurrency)
	doneSorting := sync.WaitGroup{}

	for _, samples := range fingerprintToSamples {
		doneSorting.Add(1)

		sortingSemaphore <- true
		go func(samples clientmodel.Samples) {
			sort.Sort(samples)

			<-sortingSemaphore
			doneSorting.Done()
		}(samples)
	}

	doneSorting.Wait()

	return fingerprintToSamples
}

func (l *LevelDBMetricPersistence) refreshHighWatermarks(groups map[clientmodel.Fingerprint]clientmodel.Samples) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: refreshHighWatermarks, result: success}, map[string]string{operation: refreshHighWatermarks, result: failure})
	}(time.Now())

	b := FingerprintHighWatermarkMapping{}
	for fp, ss := range groups {
		if len(ss) == 0 {
			continue
		}

		b[fp] = ss[len(ss)-1].Timestamp
	}

	return l.MetricHighWatermarks.UpdateBatch(b)
}

func (l *LevelDBMetricPersistence) AppendSamples(samples clientmodel.Samples) (err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: appendSamples, result: success}, map[string]string{operation: appendSamples, result: failure})
	}(time.Now())

	fingerprintToSamples := groupByFingerprint(samples)
	indexErrChan := make(chan error, 1)
	watermarkErrChan := make(chan error, 1)

	go func(groups map[clientmodel.Fingerprint]clientmodel.Samples) {
		metrics := FingerprintMetricMapping{}

		for fingerprint, samples := range groups {
			metrics[fingerprint] = samples[0].Metric
		}

		indexErrChan <- l.Indexer.IndexMetrics(metrics)
	}(fingerprintToSamples)

	go func(groups map[clientmodel.Fingerprint]clientmodel.Samples) {
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

			key := SampleKey{
				Fingerprint:    &fingerprint,
				FirstTimestamp: chunk[0].Timestamp,
				LastTimestamp:  chunk[take-1].Timestamp,
				SampleCount:    uint32(take),
			}

			value := &dto.SampleValueSeries{}
			for _, sample := range chunk {
				value.Value = append(value.Value, &dto.SampleValueSeries_Value{
					Timestamp: proto.Int64(sample.Timestamp.Unix()),
					Value:     proto.Float64(float64(sample.Value)),
				})
			}

			k := &dto.SampleKey{}
			key.Dump(k)
			samplesBatch.Put(k, value)
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

func extractSampleKey(i leveldb.Iterator) (*SampleKey, error) {
	k := &dto.SampleKey{}
	if err := i.Key(k); err != nil {
		return nil, err
	}

	key := &SampleKey{}
	key.Load(k)

	return key, nil
}

func extractSampleValues(i leveldb.Iterator) (Values, error) {
	v := &dto.SampleValueSeries{}
	if err := i.Value(v); err != nil {
		return nil, err
	}

	return NewValuesFromDTO(v), nil
}

func (l *LevelDBMetricPersistence) hasIndexMetric(m clientmodel.Metric) (value bool, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasIndexMetric, result: success}, map[string]string{operation: hasIndexMetric, result: failure})
	}(time.Now())

	return l.MetricMembershipIndex.Has(m)
}

func (l *LevelDBMetricPersistence) HasLabelPair(p *LabelPair) (value bool, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelPair, result: success}, map[string]string{operation: hasLabelPair, result: failure})
	}(time.Now())

	return l.LabelSetToFingerprints.Has(p)
}

func (l *LevelDBMetricPersistence) HasLabelName(n clientmodel.LabelName) (value bool, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelName, result: success}, map[string]string{operation: hasLabelName, result: failure})
	}(time.Now())

	value, err = l.LabelNameToFingerprints.Has(n)

	return
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelSet(labelSet clientmodel.LabelSet) (fps clientmodel.Fingerprints, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getFingerprintsForLabelSet, result: success}, map[string]string{operation: getFingerprintsForLabelSet, result: failure})
	}(time.Now())

	sets := []utility.Set{}

	for name, value := range labelSet {
		fps, _, err := l.LabelSetToFingerprints.Lookup(&LabelPair{
			Name:  name,
			Value: value,
		})
		if err != nil {
			return nil, err
		}

		set := utility.Set{}

		for _, fp := range fps {
			set.Add(*fp)
		}

		sets = append(sets, set)
	}

	numberOfSets := len(sets)
	if numberOfSets == 0 {
		return nil, nil
	}

	base := sets[0]
	for i := 1; i < numberOfSets; i++ {
		base = base.Intersection(sets[i])
	}
	for _, e := range base.Elements() {
		fingerprint := e.(clientmodel.Fingerprint)
		fps = append(fps, &fingerprint)
	}

	return fps, nil
}

func (l *LevelDBMetricPersistence) GetFingerprintsForLabelName(labelName clientmodel.LabelName) (fps clientmodel.Fingerprints, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getFingerprintsForLabelName, result: success}, map[string]string{operation: getFingerprintsForLabelName, result: failure})
	}(time.Now())

	// TODO(matt): Update signature to work with ok.
	fps, _, err = l.LabelNameToFingerprints.Lookup(labelName)

	return fps, err
}

func (l *LevelDBMetricPersistence) GetMetricForFingerprint(f *clientmodel.Fingerprint) (m clientmodel.Metric, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getMetricForFingerprint, result: success}, map[string]string{operation: getMetricForFingerprint, result: failure})
	}(time.Now())

	// TODO(matt): Update signature to work with ok.
	m, _, err = l.FingerprintToMetrics.Lookup(f)

	return m, nil
}

func (l *LevelDBMetricPersistence) GetValueAtTime(f *clientmodel.Fingerprint, t time.Time) Values {
	panic("Not implemented")
}

func (l *LevelDBMetricPersistence) GetBoundaryValues(f *clientmodel.Fingerprint, i Interval) Values {
	panic("Not implemented")
}

func (l *LevelDBMetricPersistence) GetRangeValues(f *clientmodel.Fingerprint, i Interval) Values {
	panic("Not implemented")
}

type MetricKeyDecoder struct{}

func (d *MetricKeyDecoder) DecodeKey(in interface{}) (out interface{}, err error) {
	unmarshaled := dto.LabelPair{}
	err = proto.Unmarshal(in.([]byte), &unmarshaled)
	if err != nil {
		return
	}

	out = LabelPair{
		Name:  clientmodel.LabelName(*unmarshaled.Name),
		Value: clientmodel.LabelValue(*unmarshaled.Value),
	}

	return
}

func (d *MetricKeyDecoder) DecodeValue(in interface{}) (out interface{}, err error) {
	return
}

type LabelNameFilter struct {
	labelName clientmodel.LabelName
}

func (f LabelNameFilter) Filter(key, value interface{}) (filterResult storage.FilterResult) {
	labelPair, ok := key.(LabelPair)
	if ok && labelPair.Name == f.labelName {
		return storage.ACCEPT
	}
	return storage.SKIP
}

type CollectLabelValuesOp struct {
	labelValues []clientmodel.LabelValue
}

func (op *CollectLabelValuesOp) Operate(key, value interface{}) (err *storage.OperatorError) {
	labelPair := key.(LabelPair)
	op.labelValues = append(op.labelValues, clientmodel.LabelValue(labelPair.Value))
	return
}

func (l *LevelDBMetricPersistence) GetAllValuesForLabel(labelName clientmodel.LabelName) (values clientmodel.LabelValues, err error) {
	filter := &LabelNameFilter{
		labelName: labelName,
	}
	labelValuesOp := &CollectLabelValuesOp{}

	_, err = l.LabelSetToFingerprints.ForEach(&MetricKeyDecoder{}, filter, labelValuesOp)
	if err != nil {
		return
	}

	values = labelValuesOp.labelValues
	return
}

// Prune compacts each database's keyspace serially.
//
// Beware that it would probably be imprudent to run this on a live user-facing
// server due to latency implications.
func (l *LevelDBMetricPersistence) Prune() {
	l.CurationRemarks.Prune()
	l.FingerprintToMetrics.Prune()
	l.LabelNameToFingerprints.Prune()
	l.LabelSetToFingerprints.Prune()
	l.MetricHighWatermarks.Prune()
	l.MetricMembershipIndex.Prune()
	l.MetricSamples.Prune()
}

func (l *LevelDBMetricPersistence) Sizes() (total uint64, err error) {
	size := uint64(0)

	if size, _, err = l.CurationRemarks.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, _, err = l.FingerprintToMetrics.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, _, err = l.LabelNameToFingerprints.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, _, err = l.LabelSetToFingerprints.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, _, err = l.MetricHighWatermarks.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, _, err = l.MetricMembershipIndex.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.MetricSamples.Size(); err != nil {
		return 0, err
	}
	total += size

	return total, nil
}

func (l *LevelDBMetricPersistence) States() raw.DatabaseStates {
	return raw.DatabaseStates{
		l.CurationRemarks.State(),
		l.FingerprintToMetrics.State(),
		l.LabelNameToFingerprints.State(),
		l.LabelSetToFingerprints.State(),
		l.MetricHighWatermarks.State(),
		l.MetricMembershipIndex.State(),
		l.MetricSamples.State(),
	}
}
