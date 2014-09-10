package index

import (
	"flag"
	"path"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/local/codec"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
)

const (
	fingerprintToMetricDir     = "archived_fingerprint_to_metric"
	fingerprintTimeRangeDir    = "archived_fingerprint_to_timerange"
	labelNameToLabelValuesDir  = "labelname_to_labelvalues"
	labelPairToFingerprintsDir = "labelpair_to_fingerprints"
)

var (
	fingerprintToMetricCacheSize     = flag.Int("storage.fingerprintToMetricCacheSizeBytes", 25*1024*1024, "The size in bytes for the fingerprint to metric index cache.")
	labelNameToLabelValuesCacheSize  = flag.Int("storage.labelNameToLabelValuesCacheSizeBytes", 25*1024*1024, "The size in bytes for the label name to label values index cache.")
	labelPairToFingerprintsCacheSize = flag.Int("storage.labelPairToFingerprintsCacheSizeBytes", 25*1024*1024, "The size in bytes for the label pair to fingerprints index cache.")
	fingerprintTimeRangeCacheSize    = flag.Int("storage.fingerprintTimeRangeCacheSizeBytes", 5*1024*1024, "The size in bytes for the metric time range index cache.")
)

// FingerprintMetricMapping is an in-memory map of fingerprints to metrics.
type FingerprintMetricMapping map[clientmodel.Fingerprint]clientmodel.Metric

// FingerprintMetricIndex models a database mapping fingerprints to metrics.
type FingerprintMetricIndex struct {
	KeyValueStore
}

// IndexBatch indexes a batch of mappings from fingerprints to metrics.
func (i *FingerprintMetricIndex) IndexBatch(mapping FingerprintMetricMapping) error {
	b := i.NewBatch()

	for fp, m := range mapping {
		b.Put(codec.CodableFingerprint(fp), codec.CodableMetric(m))
	}

	return i.Commit(b)
}

// UnindexBatch unindexes a batch of mappings from fingerprints to metrics.
func (i *FingerprintMetricIndex) UnindexBatch(mapping FingerprintMetricMapping) error {
	b := i.NewBatch()

	for fp, _ := range mapping {
		b.Delete(codec.CodableFingerprint(fp))
	}

	return i.Commit(b)
}

// Lookup looks up a metric by fingerprint.
func (i *FingerprintMetricIndex) Lookup(fp clientmodel.Fingerprint) (clientmodel.Metric, bool, error) {
	m := codec.CodableMetric{}
	if ok, err := i.Get(codec.CodableFingerprint(fp), &m); !ok {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	return clientmodel.Metric(m), true, nil
}

// NewFingerprintMetricIndex returns a FingerprintMetricIndex
// object ready to use.
func NewFingerprintMetricIndex(basePath string) (*FingerprintMetricIndex, error) {
	fingerprintToMetricDB, err := NewLevelDB(LevelDBOptions{
		Path:           path.Join(basePath, fingerprintToMetricDir),
		CacheSizeBytes: *fingerprintToMetricCacheSize,
	})
	if err != nil {
		return nil, err
	}
	return &FingerprintMetricIndex{
		KeyValueStore: fingerprintToMetricDB,
	}, nil
}

// LabelNameLabelValuesMapping is an in-memory map of label names to
// label values.
type LabelNameLabelValuesMapping map[clientmodel.LabelName]clientmodel.LabelValues

// LabelNameLabelValuesIndex models a database mapping label names to
// label values.
type LabelNameLabelValuesIndex struct {
	KeyValueStore
}

// IndexBatch implements LabelNameLabelValuesIndex.
func (i *LabelNameLabelValuesIndex) IndexBatch(b LabelNameLabelValuesMapping) error {
	batch := i.NewBatch()

	for name, values := range b {
		if len(values) == 0 {
			batch.Delete(codec.CodableLabelName(name))
		} else {
			batch.Put(codec.CodableLabelName(name), codec.CodableLabelValues(values))
		}
	}

	return i.Commit(batch)
}

// Lookup looks up all label values for a given label name.
func (i *LabelNameLabelValuesIndex) Lookup(l clientmodel.LabelName) (values clientmodel.LabelValues, ok bool, err error) {
	ok, err = i.Get(codec.CodableLabelName(l), (*codec.CodableLabelValues)(&values))
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	return values, true, nil
}

// NewLabelNameLabelValuesIndex returns a LabelNameLabelValuesIndex
// ready to use.
func NewLabelNameLabelValuesIndex(basePath string) (*LabelNameLabelValuesIndex, error) {
	labelNameToLabelValuesDB, err := NewLevelDB(LevelDBOptions{
		Path:           path.Join(basePath, labelNameToLabelValuesDir),
		CacheSizeBytes: *labelNameToLabelValuesCacheSize,
	})
	if err != nil {
		return nil, err
	}
	return &LabelNameLabelValuesIndex{
		KeyValueStore: labelNameToLabelValuesDB,
	}, nil
}

// LabelPairFingerprintsMapping is an in-memory map of label pairs to
// fingerprints.
type LabelPairFingerprintsMapping map[metric.LabelPair]clientmodel.Fingerprints

// LabelPairFingerprintIndex models a database mapping label pairs to
// fingerprints.
type LabelPairFingerprintIndex struct {
	KeyValueStore
}

// IndexBatch indexes a batch of mappings from label pairs to fingerprints.
func (i *LabelPairFingerprintIndex) IndexBatch(m LabelPairFingerprintsMapping) error {
	batch := i.NewBatch()

	for pair, fps := range m {
		if len(fps) == 0 {
			batch.Delete(codec.CodableLabelPair(pair))
		} else {
			batch.Put(codec.CodableLabelPair(pair), codec.CodableFingerprints(fps))
		}
	}

	return i.Commit(batch)
}

// Lookup looks up all fingerprints for a given label pair.
func (i *LabelPairFingerprintIndex) Lookup(p metric.LabelPair) (fps clientmodel.Fingerprints, ok bool, err error) {
	ok, err = i.Get((codec.CodableLabelPair)(p), (*codec.CodableFingerprints)(&fps))
	if !ok {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	return fps, true, nil
}

// NewLabelPairFingerprintIndex returns a LabelPairFingerprintIndex
// object ready to use.
func NewLabelPairFingerprintIndex(basePath string) (*LabelPairFingerprintIndex, error) {
	labelPairToFingerprintsDB, err := NewLevelDB(LevelDBOptions{
		Path:           path.Join(basePath, labelPairToFingerprintsDir),
		CacheSizeBytes: *labelPairToFingerprintsCacheSize,
	})
	if err != nil {
		return nil, err
	}
	return &LabelPairFingerprintIndex{
		KeyValueStore: labelPairToFingerprintsDB,
	}, nil
}

// FingerprintTimeRangeIndex models a database tracking the time ranges
// of metrics by their fingerprints.
type FingerprintTimeRangeIndex struct {
	KeyValueStore
}

// UnindexBatch unindexes a batch of fingerprints.
func (i *FingerprintTimeRangeIndex) UnindexBatch(b FingerprintMetricMapping) error {
	batch := i.NewBatch()

	for fp, _ := range b {
		batch.Delete(codec.CodableFingerprint(fp))
	}

	return i.Commit(batch)
}

// Has returns true if the given fingerprint is present.
func (i *FingerprintTimeRangeIndex) Has(fp clientmodel.Fingerprint) (ok bool, err error) {
	return i.KeyValueStore.Has(codec.CodableFingerprint(fp))
}

// NewFingerprintTimeRangeIndex returns a FingerprintTimeRangeIndex object
// ready to use.
func NewFingerprintTimeRangeIndex(basePath string) (*FingerprintTimeRangeIndex, error) {
	fingerprintTimeRangeDB, err := NewLevelDB(LevelDBOptions{
		Path:           path.Join(basePath, fingerprintTimeRangeDir),
		CacheSizeBytes: *fingerprintTimeRangeCacheSize,
	})
	if err != nil {
		return nil, err
	}
	return &FingerprintTimeRangeIndex{
		KeyValueStore: fingerprintTimeRangeDB,
	}, nil
}

func findUnindexed(i *FingerprintTimeRangeIndex, b FingerprintMetricMapping) (FingerprintMetricMapping, error) {
	// TODO: Move up? Need to include fp->ts map?
	out := FingerprintMetricMapping{}

	for fp, m := range b {
		has, err := i.Has(fp)
		if err != nil {
			return nil, err
		}
		if !has {
			out[fp] = m
		}
	}

	return out, nil
}

func findIndexed(i *FingerprintTimeRangeIndex, b FingerprintMetricMapping) (FingerprintMetricMapping, error) {
	// TODO: Move up? Need to include fp->ts map?
	out := FingerprintMetricMapping{}

	for fp, m := range b {
		has, err := i.Has(fp)
		if err != nil {
			return nil, err
		}
		if has {
			out[fp] = m
		}
	}

	return out, nil
}

func extendLabelNameToLabelValuesIndex(i *LabelNameLabelValuesIndex, b FingerprintMetricMapping) (LabelNameLabelValuesMapping, error) {
	// TODO: Move up? Need to include fp->ts map?
	collection := map[clientmodel.LabelName]utility.Set{}

	for _, m := range b {
		for l, v := range m {
			set, ok := collection[l]
			if !ok {
				baseValues, _, err := i.Lookup(l)
				if err != nil {
					return nil, err
				}

				set = utility.Set{}

				for _, baseValue := range baseValues {
					set.Add(baseValue)
				}

				collection[l] = set
			}

			set.Add(v)
		}
	}

	batch := LabelNameLabelValuesMapping{}
	for l, set := range collection {
		values := make(clientmodel.LabelValues, 0, len(set))
		for e := range set {
			val := e.(clientmodel.LabelValue)
			values = append(values, val)
		}

		batch[l] = values
	}

	return batch, nil
}

func reduceLabelNameToLabelValuesIndex(i *LabelNameLabelValuesIndex, m LabelPairFingerprintsMapping) (LabelNameLabelValuesMapping, error) {
	// TODO: Move up? Need to include fp->ts map?
	collection := map[clientmodel.LabelName]utility.Set{}

	for lp, fps := range m {
		if len(fps) != 0 {
			continue
		}

		set, ok := collection[lp.Name]
		if !ok {
			baseValues, _, err := i.Lookup(lp.Name)
			if err != nil {
				return nil, err
			}

			set = utility.Set{}

			for _, baseValue := range baseValues {
				set.Add(baseValue)
			}

			collection[lp.Name] = set
		}

		set.Remove(lp.Value)
	}

	batch := LabelNameLabelValuesMapping{}
	for l, set := range collection {
		values := make(clientmodel.LabelValues, 0, len(set))
		for e := range set {
			val := e.(clientmodel.LabelValue)
			values = append(values, val)
		}

		batch[l] = values
	}
	return batch, nil
}

func extendLabelPairIndex(i *LabelPairFingerprintIndex, b FingerprintMetricMapping, remove bool) (LabelPairFingerprintsMapping, error) {
	// TODO: Move up? Need to include fp->ts map?
	collection := map[metric.LabelPair]utility.Set{}

	for fp, m := range b {
		for n, v := range m {
			pair := metric.LabelPair{
				Name:  n,
				Value: v,
			}
			set, ok := collection[pair]
			if !ok {
				baseFps, _, err := i.Lookup(pair)
				if err != nil {
					return nil, err
				}

				set = utility.Set{}
				for _, baseFp := range baseFps {
					set.Add(baseFp)
				}

				collection[pair] = set
			}

			if remove {
				set.Remove(fp)
			} else {
				set.Add(fp)
			}
		}
	}

	batch := LabelPairFingerprintsMapping{}

	for pair, set := range collection {
		fps := batch[pair]
		for element := range set {
			fp := element.(clientmodel.Fingerprint)
			fps = append(fps, fp)
		}
		batch[pair] = fps
	}

	return batch, nil
}

// TODO: Move IndexMetrics and UndindexMetrics into storage.go.

/*
// IndexMetrics adds the facets of all unindexed metrics found in the given
// FingerprintMetricMapping to the corresponding indices.
func (i *diskIndexer) IndexMetrics(b FingerprintMetricMapping) error {
	unindexed, err := findUnindexed(i.FingerprintTimeRange, b)
	if err != nil {
		return err
	}

	labelNames, err := extendLabelNameToLabelValuesIndex(i.LabelNameToLabelValues, unindexed)
	if err != nil {
		return err
	}
	if err := i.LabelNameToLabelValues.IndexBatch(labelNames); err != nil {
		return err
	}

	labelPairs, err := extendLabelPairIndex(i.LabelPairToFingerprints, unindexed, false)
	if err != nil {
		return err
	}
	if err := i.LabelPairToFingerprints.IndexBatch(labelPairs); err != nil {
		return err
	}

	if err := i.FingerprintToMetric.IndexBatch(unindexed); err != nil {
		return err
	}

	return i.FingerprintTimeRange.IndexBatch(unindexed)
}

// UnindexMetrics implements MetricIndexer.
func (i *diskIndexer) UnindexMetrics(b FingerprintMetricMapping) error {
	indexed, err := findIndexed(i.FingerprintTimeRange, b)
	if err != nil {
		return err
	}

	labelPairs, err := extendLabelPairIndex(i.LabelPairToFingerprints, indexed, true)
	if err != nil {
		return err
	}
	if err := i.LabelPairToFingerprints.IndexBatch(labelPairs); err != nil {
		return err
	}

	labelNames, err := reduceLabelNameToLabelValuesIndex(i.LabelNameToLabelValues, labelPairs)
	if err := i.LabelNameToLabelValues.IndexBatch(labelNames); err != nil {
		return err
	}

	if err := i.FingerprintToMetric.UnindexBatch(indexed); err != nil {
		return err
	}

	return i.FingerprintTimeRange.UnindexBatch(indexed)
}
*/
