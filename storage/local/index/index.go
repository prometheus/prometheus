package index

import (
	"sync"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/utility"
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
		b.Put(codableFingerprint(fp), codableMetric(m))
	}

	return i.Commit(b)
}

// UnindexBatch unindexes a batch of mappings from fingerprints to metrics.
func (i *FingerprintMetricIndex) UnindexBatch(mapping FingerprintMetricMapping) error {
	b := i.NewBatch()

	for fp, _ := range mapping {
		b.Delete(codableFingerprint(fp))
	}

	return i.Commit(b)
}

// Lookup looks up a metric by fingerprint.
func (i *FingerprintMetricIndex) Lookup(fp clientmodel.Fingerprint) (m clientmodel.Metric, ok bool, err error) {
	m = clientmodel.Metric{}
	if ok, err := i.Get(codableFingerprint(fp), codableMetric(m)); !ok {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	return m, true, nil
}

// NewFingerprintMetricIndex returns a FingerprintMetricIndex
// object ready to use.
func NewFingerprintMetricIndex(db KeyValueStore) *FingerprintMetricIndex {
	return &FingerprintMetricIndex{
		KeyValueStore: db,
	}
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
			batch.Delete(codableLabelName(name))
		} else {
			batch.Put(codableLabelName(name), codableLabelValues(values))
		}
	}

	return i.Commit(batch)
}

// Lookup looks up all label values for a given label name.
func (i *LabelNameLabelValuesIndex) Lookup(l clientmodel.LabelName) (values clientmodel.LabelValues, ok bool, err error) {
	ok, err = i.Get(codableLabelName(l), (*codableLabelValues)(&values))
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
func NewLabelNameLabelValuesIndex(db KeyValueStore) *LabelNameLabelValuesIndex {
	return &LabelNameLabelValuesIndex{
		KeyValueStore: db,
	}
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
			batch.Delete(codableLabelPair(pair))
		} else {
			batch.Put(codableLabelPair(pair), codableFingerprints(fps))
		}
	}

	return i.Commit(batch)
}

// Lookup looks up all fingerprints for a given label pair.
func (i *LabelPairFingerprintIndex) Lookup(p *metric.LabelPair) (fps clientmodel.Fingerprints, ok bool, err error) {
	ok, err = i.Get((*codableLabelPair)(p), (*codableFingerprints)(&fps))
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
func NewLabelPairFingerprintIndex(db KeyValueStore) *LabelPairFingerprintIndex {
	return &LabelPairFingerprintIndex{
		KeyValueStore: db,
	}
}

// FingerprintMembershipIndex models a database tracking the existence
// of metrics by their fingerprints.
type FingerprintMembershipIndex struct {
	KeyValueStore
}

// IndexBatch indexes a batch of fingerprints.
func (i *FingerprintMembershipIndex) IndexBatch(b FingerprintMetricMapping) error {
	batch := i.NewBatch()

	for fp, _ := range b {
		batch.Put(codableFingerprint(fp), codableMembership{})
	}

	return i.Commit(batch)
}

// UnindexBatch unindexes a batch of fingerprints.
func (i *FingerprintMembershipIndex) UnindexBatch(b FingerprintMetricMapping) error {
	batch := i.NewBatch()

	for fp, _ := range b {
		batch.Delete(codableFingerprint(fp))
	}

	return i.Commit(batch)
}

// Has returns true if the given fingerprint is present.
func (i *FingerprintMembershipIndex) Has(fp clientmodel.Fingerprint) (ok bool, err error) {
	return i.KeyValueStore.Has(codableFingerprint(fp))
}

// NewFingerprintMembershipIndex returns a FingerprintMembershipIndex object
// ready to use.
func NewFingerprintMembershipIndex(db KeyValueStore) *FingerprintMembershipIndex {
	return &FingerprintMembershipIndex{
		KeyValueStore: db,
	}
}

// TODO(julius): Currently unused, is it needed?
// SynchronizedIndexer provides naive locking for any MetricIndexer.
type SynchronizedIndexer struct {
	mu sync.Mutex
	i  MetricIndexer
}

// IndexMetrics calls IndexMetrics of the wrapped MetricIndexer after acquiring
// a lock.
func (i *SynchronizedIndexer) IndexMetrics(b FingerprintMetricMapping) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.i.IndexMetrics(b)
}

// TotalIndexer is a MetricIndexer that indexes all standard facets of a metric
// that a user or the Prometheus subsystem would want to query against:
//
//    <Fingerprint> -> <existence marker>
//    <Label Name> -> {<Label Value>, ...}
//    <Label Name> <Label Value> -> {<Fingerprint>, ...}
//    <Fingerprint> -> <Metric>
//
// This type supports concurrent queries, but only single writes, and it has no
// locking semantics to enforce this.
type TotalIndexer struct {
	FingerprintToMetric     *FingerprintMetricIndex
	LabelNameToLabelValues  *LabelNameLabelValuesIndex
	LabelPairToFingerprints *LabelPairFingerprintIndex
	FingerprintMembership   *FingerprintMembershipIndex
}

func findUnindexed(i *FingerprintMembershipIndex, b FingerprintMetricMapping) (FingerprintMetricMapping, error) {
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

func findIndexed(i *FingerprintMembershipIndex, b FingerprintMetricMapping) (FingerprintMetricMapping, error) {
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
	collection := map[metric.LabelPair]utility.Set{}

	for fp, m := range b {
		for n, v := range m {
			pair := metric.LabelPair{
				Name:  n,
				Value: v,
			}
			set, ok := collection[pair]
			if !ok {
				baseFps, _, err := i.Lookup(&pair)
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

// IndexMetrics adds the facets of all unindexed metrics found in the given
// FingerprintMetricMapping to the corresponding indices.
func (i *TotalIndexer) IndexMetrics(b FingerprintMetricMapping) error {
	unindexed, err := findUnindexed(i.FingerprintMembership, b)
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

	return i.FingerprintMembership.IndexBatch(unindexed)
}

// UnindexMetrics removes the facets of all indexed metrics found in the given
// FingerprintMetricMapping to the corresponding indices.
func (i *TotalIndexer) UnindexMetrics(b FingerprintMetricMapping) error {
	indexed, err := findIndexed(i.FingerprintMembership, b)
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

	return i.FingerprintMembership.UnindexBatch(indexed)
}

// GetMetricForFingerprint returns the metric associated with the provided fingerprint.
func (i *TotalIndexer) GetMetricForFingerprint(fp clientmodel.Fingerprint) (clientmodel.Metric, error) {
	m, _, err := i.FingerprintToMetric.Lookup(fp)
	return m, err
}

// GetFingerprintsForLabelPair returns all fingerprints for the provided label pair.
func (i *TotalIndexer) GetFingerprintsForLabelPair(ln clientmodel.LabelName, lv clientmodel.LabelValue) (clientmodel.Fingerprints, error) {
	fps, _, err := i.LabelPairToFingerprints.Lookup(&metric.LabelPair{
		Name:  ln,
		Value: lv,
	})
	return fps, err
}

// GetLabelValuesForLabelName returns all label values associated with a given label name.
func (i *TotalIndexer) GetLabelValuesForLabelName(ln clientmodel.LabelName) (clientmodel.LabelValues, error) {
	lvs, _, err := i.LabelNameToLabelValues.Lookup(ln)
	return lvs, err
}

// HasFingerprint returns true if a metric with the given fingerprint has been indexed.
func (i *TotalIndexer) HasFingerprint(fp clientmodel.Fingerprint) (bool, error) {
	return i.FingerprintMembership.Has(fp)
}
