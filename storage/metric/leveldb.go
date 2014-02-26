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

// LevelDBMetricPersistence is a leveldb-backed persistence layer for metrics.
type LevelDBMetricPersistence struct {
	CurationRemarks         CurationRemarker
	FingerprintToMetrics    FingerprintMetricIndex
	LabelPairToFingerprints LabelPairFingerprintIndex
	MetricHighWatermarks    HighWatermarker
	MetricMembershipIndex   MetricMembershipIndex

	Indexer MetricIndexer

	MetricSamples *leveldb.LevelDBPersistence

	// The remaining indices will be replaced with generalized interface resolvers:
	//
	// type FingerprintResolver interface {
	// 	GetFingerprintForMetric(clientmodel.Metric) (*clientmodel.Fingerprint, bool, error)
	// 	GetFingerprintsForLabelSet(LabelPair) (clientmodel.Fingerprints, bool, error)
	// }

	// type MetricResolver interface {
	// 	GetMetricsForFingerprint(clientmodel.Fingerprints) (FingerprintMetricMapping, bool, error)
	// }
}

var (
	leveldbChunkSize = flag.Int("leveldbChunkSize", 200, "Maximum number of samples stored under one key.")

	// These flag values are back of the envelope, though they seem
	// sensible.  Please re-evaluate based on your own needs.
	curationRemarksCacheSize         = flag.Int("curationRemarksCacheSize", 5*1024*1024, "The size for the curation remarks cache (bytes).")
	fingerprintsToLabelPairCacheSize = flag.Int("fingerprintsToLabelPairCacheSizeBytes", 25*1024*1024, "The size for the fingerprint to label pair index (bytes).")
	highWatermarkCacheSize           = flag.Int("highWatermarksByFingerprintSizeBytes", 5*1024*1024, "The size for the metric high watermarks (bytes).")
	labelPairToFingerprintsCacheSize = flag.Int("labelPairToFingerprintsCacheSizeBytes", 25*1024*1024, "The size for the label pair to metric fingerprint index (bytes).")
	metricMembershipIndexCacheSize   = flag.Int("metricMembershipCacheSizeBytes", 5*1024*1024, "The size for the metric membership index (bytes).")
	samplesByFingerprintCacheSize    = flag.Int("samplesByFingerprintCacheSizeBytes", 50*1024*1024, "The size for the samples database (bytes).")
)

type leveldbOpener func()

// Close closes all the underlying persistence layers. It implements the
// MetricPersistence interface.
func (l *LevelDBMetricPersistence) Close() {
	var persistences = []raw.Database{
		l.CurationRemarks,
		l.FingerprintToMetrics,
		l.LabelPairToFingerprints,
		l.MetricHighWatermarks,
		l.MetricMembershipIndex,
		l.MetricSamples,
	}

	closerGroup := sync.WaitGroup{}

	for _, c := range persistences {
		closerGroup.Add(1)
		go func(c raw.Database) {
			if c != nil {
				if err := c.Close(); err != nil {
					glog.Error("Error closing persistence: ", err)
				}
			}
			closerGroup.Done()
		}(c)
	}

	closerGroup.Wait()
}

// NewLevelDBMetricPersistence returns a LevelDBMetricPersistence object ready
// to use.
func NewLevelDBMetricPersistence(baseDirectory string) (*LevelDBMetricPersistence, error) {
	workers := utility.NewUncertaintyGroup(6)

	emission := &LevelDBMetricPersistence{}

	var subsystemOpeners = []struct {
		name   string
		opener leveldbOpener
	}{
		{
			"Label Names and Value Pairs by Fingerprint",
			func() {
				var err error
				emission.FingerprintToMetrics, err = NewLevelDBFingerprintMetricIndex(
					leveldb.LevelDBOptions{
						Name:           "Metrics by Fingerprint",
						Purpose:        "Index",
						Path:           baseDirectory + "/label_name_and_value_pairs_by_fingerprint",
						CacheSizeBytes: *fingerprintsToLabelPairCacheSize,
					},
				)
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
				emission.MetricHighWatermarks, err = NewLevelDBHighWatermarker(
					leveldb.LevelDBOptions{
						Name:           "High Watermarks",
						Purpose:        "The youngest sample in the database per metric.",
						Path:           baseDirectory + "/high_watermarks_by_fingerprint",
						CacheSizeBytes: *highWatermarkCacheSize,
					},
				)
				workers.MayFail(err)
			},
		},
		{
			"Fingerprints by Label Name and Value Pair",
			func() {
				var err error
				emission.LabelPairToFingerprints, err = NewLevelDBLabelSetFingerprintIndex(
					leveldb.LevelDBOptions{
						Name:           "Fingerprints by Label Pair",
						Purpose:        "Index",
						Path:           baseDirectory + "/fingerprints_by_label_name_and_value_pair",
						CacheSizeBytes: *labelPairToFingerprintsCacheSize,
					},
				)
				workers.MayFail(err)
			},
		},
		{
			"Metric Membership Index",
			func() {
				var err error
				emission.MetricMembershipIndex, err = NewLevelDBMetricMembershipIndex(
					leveldb.LevelDBOptions{
						Name:           "Metric Membership",
						Purpose:        "Index",
						Path:           baseDirectory + "/metric_membership_index",
						CacheSizeBytes: *metricMembershipIndexCacheSize,
					},
				)
				workers.MayFail(err)
			},
		},
		{
			"Sample Curation Remarks",
			func() {
				var err error
				emission.CurationRemarks, err = NewLevelDBCurationRemarker(
					leveldb.LevelDBOptions{
						Name:           "Sample Curation Remarks",
						Purpose:        "Ledger of Progress for Various Curators",
						Path:           baseDirectory + "/curation_remarks",
						CacheSizeBytes: *curationRemarksCacheSize,
					},
				)
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

		return nil, fmt.Errorf("unable to open metric persistence")
	}

	emission.Indexer = &TotalIndexer{
		FingerprintToMetric:    emission.FingerprintToMetrics,
		LabelPairToFingerprint: emission.LabelPairToFingerprints,
		MetricMembership:       emission.MetricMembershipIndex,
	}

	return emission, nil
}

// AppendSample implements the MetricPersistence interface.
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

// AppendSamples appends the given Samples to the database and indexes them.
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

	key := &SampleKey{}
	keyDto := &dto.SampleKey{}
	values := make(Values, 0, *leveldbChunkSize)

	for fingerprint, group := range fingerprintToSamples {
		for {
			values := values[:0]
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

			key.Fingerprint = &fingerprint
			key.FirstTimestamp = chunk[0].Timestamp
			key.LastTimestamp = chunk[take-1].Timestamp
			key.SampleCount = uint32(take)

			key.Dump(keyDto)

			for _, sample := range chunk {
				values = append(values, &SamplePair{
					Timestamp: sample.Timestamp,
					Value:     sample.Value,
				})
			}
			val := values.marshal()
			samplesBatch.PutRaw(keyDto, val)
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

func (l *LevelDBMetricPersistence) hasIndexMetric(m clientmodel.Metric) (value bool, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasIndexMetric, result: success}, map[string]string{operation: hasIndexMetric, result: failure})
	}(time.Now())

	return l.MetricMembershipIndex.Has(m)
}

// HasLabelPair returns true if the given LabelPair is present in the underlying
// LabelPair index.
func (l *LevelDBMetricPersistence) HasLabelPair(p *LabelPair) (value bool, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: hasLabelPair, result: success}, map[string]string{operation: hasLabelPair, result: failure})
	}(time.Now())

	return l.LabelPairToFingerprints.Has(p)
}

// GetFingerprintsForLabelSet returns the Fingerprints for the given LabelSet by
// querying the underlying LabelPairFingerprintIndex for each LabelPair
// contained in LabelSet.  It implements the MetricPersistence interface.
func (l *LevelDBMetricPersistence) GetFingerprintsForLabelSet(labelSet clientmodel.LabelSet) (fps clientmodel.Fingerprints, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getFingerprintsForLabelSet, result: success}, map[string]string{operation: getFingerprintsForLabelSet, result: failure})
	}(time.Now())

	sets := []utility.Set{}

	for name, value := range labelSet {
		fps, _, err := l.LabelPairToFingerprints.Lookup(&LabelPair{
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

// GetMetricForFingerprint returns the Metric for the given Fingerprint from the
// underlying FingerprintMetricIndex. It implements the MetricPersistence
// interface.
func (l *LevelDBMetricPersistence) GetMetricForFingerprint(f *clientmodel.Fingerprint) (m clientmodel.Metric, err error) {
	defer func(begin time.Time) {
		duration := time.Since(begin)

		recordOutcome(duration, err, map[string]string{operation: getMetricForFingerprint, result: success}, map[string]string{operation: getMetricForFingerprint, result: failure})
	}(time.Now())

	// TODO(matt): Update signature to work with ok.
	m, _, err = l.FingerprintToMetrics.Lookup(f)

	return m, nil
}

// GetAllValuesForLabel gets all label values that are associated with the
// provided label name.
func (l *LevelDBMetricPersistence) GetAllValuesForLabel(labelName clientmodel.LabelName) (values clientmodel.LabelValues, err error) {
	filter := &LabelNameFilter{
		labelName: labelName,
	}
	labelValuesOp := &CollectLabelValuesOp{}

	_, err = l.LabelPairToFingerprints.ForEach(&MetricKeyDecoder{}, filter, labelValuesOp)
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
	l.LabelPairToFingerprints.Prune()
	l.MetricHighWatermarks.Prune()
	l.MetricMembershipIndex.Prune()
	l.MetricSamples.Prune()
}

// Sizes returns the sum of all sizes of the underlying databases.
func (l *LevelDBMetricPersistence) Sizes() (total uint64, err error) {
	size := uint64(0)

	if size, err = l.CurationRemarks.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.FingerprintToMetrics.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.LabelPairToFingerprints.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.MetricHighWatermarks.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.MetricMembershipIndex.Size(); err != nil {
		return 0, err
	}
	total += size

	if size, err = l.MetricSamples.Size(); err != nil {
		return 0, err
	}
	total += size

	return total, nil
}

// States returns the DatabaseStates of all underlying databases.
func (l *LevelDBMetricPersistence) States() raw.DatabaseStates {
	return raw.DatabaseStates{
		l.CurationRemarks.State(),
		l.FingerprintToMetrics.State(),
		l.LabelPairToFingerprints.State(),
		l.MetricHighWatermarks.State(),
		l.MetricMembershipIndex.State(),
		l.MetricSamples.State(),
	}
}

// CollectLabelValuesOp implements storage.RecordOperator. It collects the
// encountered LabelValues in a slice.
type CollectLabelValuesOp struct {
	labelValues []clientmodel.LabelValue
}

// Operate implements storage.RecordOperator. 'key' is required to be a
// LabelPair. Its Value is appended to a slice of collected LabelValues.
func (op *CollectLabelValuesOp) Operate(key, value interface{}) (err *storage.OperatorError) {
	labelPair := key.(LabelPair)
	op.labelValues = append(op.labelValues, labelPair.Value)
	return
}

// MetricKeyDecoder implements storage.RecordDecoder for LabelPairs.
type MetricKeyDecoder struct{}

// DecodeKey implements storage.RecordDecoder. It requires 'in' to be a
// LabelPair protobuf. 'out' is a metric.LabelPair.
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

// DecodeValue implements storage.RecordDecoder. It is a no-op and always
// returns (nil, nil).
func (d *MetricKeyDecoder) DecodeValue(in interface{}) (out interface{}, err error) {
	return
}

// MetricSamplesDecoder implements storage.RecordDecoder for SampleKeys.
type MetricSamplesDecoder struct{}

// DecodeKey implements storage.RecordDecoder. It requires 'in' to be a
// SampleKey protobuf. 'out' is a metric.SampleKey.
func (d *MetricSamplesDecoder) DecodeKey(in interface{}) (interface{}, error) {
	key := &dto.SampleKey{}
	err := proto.Unmarshal(in.([]byte), key)
	if err != nil {
		return nil, err
	}

	sampleKey := &SampleKey{}
	sampleKey.Load(key)

	return sampleKey, nil
}

// DecodeValue implements storage.RecordDecoder. It requires 'in' to be a
// SampleValueSeries protobuf. 'out' is of type metric.Values.
func (d *MetricSamplesDecoder) DecodeValue(in interface{}) (interface{}, error) {
	return unmarshalValues(in.([]byte)), nil
}

// AcceptAllFilter implements storage.RecordFilter and accepts all records.
type AcceptAllFilter struct{}

// Filter implements storage.RecordFilter. It always returns ACCEPT.
func (d *AcceptAllFilter) Filter(_, _ interface{}) storage.FilterResult {
	return storage.Accept
}

// LabelNameFilter implements storage.RecordFilter and filters records matching
// a LabelName.
type LabelNameFilter struct {
	labelName clientmodel.LabelName
}

// Filter implements storage.RecordFilter. 'key' is expected to be a
// LabelPair. The result is ACCEPT if the Name of the LabelPair matches the
// LabelName of this LabelNameFilter.
func (f LabelNameFilter) Filter(key, value interface{}) (filterResult storage.FilterResult) {
	labelPair, ok := key.(LabelPair)
	if ok && labelPair.Name == f.labelName {
		return storage.Accept
	}
	return storage.Skip
}
