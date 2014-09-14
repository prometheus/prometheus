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
func (i *FingerprintMetricIndex) Lookup(fp clientmodel.Fingerprint) (metric clientmodel.Metric, ok bool, err error) {
	ok, err = i.Get(codec.CodableFingerprint(fp), (*codec.CodableMetric)(&metric))
	return
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
	return
}

func (i *LabelNameLabelValuesIndex) Extend(m clientmodel.Metric) error {
	b := make(LabelNameLabelValuesMapping, len(m))
	for ln, lv := range m {
		baseLVs, _, err := i.Lookup(ln)
		if err != nil {
			return err
		}
		lvSet := utility.Set{}
		for _, baseLV := range baseLVs {
			lvSet.Add(baseLV)
		}
		lvSet.Add(lv)
		if len(lvSet) == len(baseLVs) {
			continue
		}
		lvs := make(clientmodel.LabelValues, 0, len(lvSet))
		for v := range lvSet {
			lvs = append(lvs, v.(clientmodel.LabelValue))
		}
		b[ln] = lvs
	}
	return i.IndexBatch(b)
}

func (i *LabelNameLabelValuesIndex) Reduce(m LabelPairFingerprintsMapping) error {
	b := make(LabelNameLabelValuesMapping, len(m))
	for lp, fps := range m {
		if len(fps) != 0 {
			continue
		}
		ln := lp.Name
		lv := lp.Value
		baseValues, ok := b[ln]
		if !ok {
			var err error
			baseValues, _, err = i.Lookup(ln)
			if err != nil {
				return err
			}
		}
		lvSet := utility.Set{}
		for _, baseValue := range baseValues {
			lvSet.Add(baseValue)
		}
		lvSet.Remove(lv)
		if len(lvSet) == len(baseValues) {
			continue
		}
		lvs := make(clientmodel.LabelValues, 0, len(lvSet))
		for v := range lvSet {
			lvs = append(lvs, v.(clientmodel.LabelValue))
		}
		b[ln] = lvs
	}
	return i.IndexBatch(b)
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
	return
}

func (i *LabelPairFingerprintIndex) Extend(m clientmodel.Metric, fp clientmodel.Fingerprint) error {
	b := make(LabelPairFingerprintsMapping, len(m))
	for ln, lv := range m {
		lp := metric.LabelPair{Name: ln, Value: lv}
		baseFPs, _, err := i.Lookup(lp)
		if err != nil {
			return err
		}
		fpSet := utility.Set{}
		for _, baseFP := range baseFPs {
			fpSet.Add(baseFP)
		}
		fpSet.Add(fp)
		if len(fpSet) == len(baseFPs) {
			continue
		}
		fps := make(clientmodel.Fingerprints, 0, len(fpSet))
		for f := range fpSet {
			fps = append(fps, f.(clientmodel.Fingerprint))
		}
		b[lp] = fps

	}
	return i.IndexBatch(b)
}

func (i *LabelPairFingerprintIndex) Reduce(m clientmodel.Metric, fp clientmodel.Fingerprint) (LabelPairFingerprintsMapping, error) {
	b := make(LabelPairFingerprintsMapping, len(m))
	for ln, lv := range m {
		lp := metric.LabelPair{Name: ln, Value: lv}
		baseFPs, _, err := i.Lookup(lp)
		if err != nil {
			return nil, err
		}
		fpSet := utility.Set{}
		for _, baseFP := range baseFPs {
			fpSet.Add(baseFP)
		}
		fpSet.Remove(fp)
		if len(fpSet) == len(baseFPs) {
			continue
		}
		fps := make(clientmodel.Fingerprints, 0, len(fpSet))
		for f := range fpSet {
			fps = append(fps, f.(clientmodel.Fingerprint))
		}
		b[lp] = fps

	}
	return b, i.IndexBatch(b)
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

// Lookup returns the time range for the given fingerprint.
func (i *FingerprintTimeRangeIndex) Lookup(fp clientmodel.Fingerprint) (firstTime, lastTime clientmodel.Timestamp, ok bool, err error) {
	var tr codec.CodableTimeRange
	ok, err = i.Get(codec.CodableFingerprint(fp), &tr)
	return tr.First, tr.Last, ok, err
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
