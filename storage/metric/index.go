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
	"io"
	"sort"
	"sync"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility"

	dto "github.com/prometheus/prometheus/model/generated"
)

// FingerprintMetricMapping is an in-memory map of Fingerprints to Metrics.
type FingerprintMetricMapping map[clientmodel.Fingerprint]clientmodel.Metric

// FingerprintMetricIndex models a database mapping Fingerprints to Metrics.
type FingerprintMetricIndex interface {
	raw.Database
	raw.Pruner

	IndexBatch(FingerprintMetricMapping) error
	Lookup(*clientmodel.Fingerprint) (m clientmodel.Metric, ok bool, err error)
}

// LevelDBFingerprintMetricIndex implements FingerprintMetricIndex using
// leveldb.
type LevelDBFingerprintMetricIndex struct {
	*leveldb.LevelDBPersistence
}

// IndexBatch implements FingerprintMetricIndex.
func (i *LevelDBFingerprintMetricIndex) IndexBatch(mapping FingerprintMetricMapping) error {
	b := leveldb.NewBatch()
	defer b.Close()

	for f, m := range mapping {
		k := &dto.Fingerprint{}
		dumpFingerprint(k, &f)
		v := &dto.Metric{}
		dumpMetric(v, m)

		b.Put(k, v)
	}

	return i.LevelDBPersistence.Commit(b)
}

// Lookup implements FingerprintMetricIndex.
func (i *LevelDBFingerprintMetricIndex) Lookup(f *clientmodel.Fingerprint) (m clientmodel.Metric, ok bool, err error) {
	k := &dto.Fingerprint{}
	dumpFingerprint(k, f)
	v := &dto.Metric{}
	if ok, err := i.LevelDBPersistence.Get(k, v); !ok {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	m = clientmodel.Metric{}

	for _, pair := range v.LabelPair {
		m[clientmodel.LabelName(pair.GetName())] = clientmodel.LabelValue(pair.GetValue())
	}

	return m, true, nil
}

// NewLevelDBFingerprintMetricIndex returns a LevelDBFingerprintMetricIndex
// object ready to use.
func NewLevelDBFingerprintMetricIndex(o leveldb.LevelDBOptions) (*LevelDBFingerprintMetricIndex, error) {
	s, err := leveldb.NewLevelDBPersistence(o)
	if err != nil {
		return nil, err
	}

	return &LevelDBFingerprintMetricIndex{
		LevelDBPersistence: s,
	}, nil
}

// LabelNameLabelValuesMapping is an in-memory map of LabelNames to
// LabelValues.
type LabelNameLabelValuesMapping map[clientmodel.LabelName]clientmodel.LabelValues

// LabelNameLabelValuesIndex models a database mapping LabelNames to
// LabelValues.
type LabelNameLabelValuesIndex interface {
	raw.Database
	raw.Pruner

	IndexBatch(LabelNameLabelValuesMapping) error
	Lookup(clientmodel.LabelName) (values clientmodel.LabelValues, ok bool, err error)
	Has(clientmodel.LabelName) (ok bool, err error)
}

// LevelDBLabelNameLabelValuesIndex implements LabelNameLabelValuesIndex using
// leveldb.
type LevelDBLabelNameLabelValuesIndex struct {
	*leveldb.LevelDBPersistence
}

// IndexBatch implements LabelNameLabelValuesIndex.
func (i *LevelDBLabelNameLabelValuesIndex) IndexBatch(b LabelNameLabelValuesMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for labelName, labelValues := range b {
		sort.Sort(labelValues)

		key := &dto.LabelName{
			Name: proto.String(string(labelName)),
		}
		value := &dto.LabelValueCollection{}
		value.Member = make([]string, 0, len(labelValues))
		for _, labelValue := range labelValues {
			value.Member = append(value.Member, string(labelValue))
		}

		batch.Put(key, value)
	}

	return i.LevelDBPersistence.Commit(batch)
}

// Lookup implements LabelNameLabelValuesIndex.
func (i *LevelDBLabelNameLabelValuesIndex) Lookup(l clientmodel.LabelName) (values clientmodel.LabelValues, ok bool, err error) {
	k := &dto.LabelName{}
	dumpLabelName(k, l)
	v := &dto.LabelValueCollection{}
	ok, err = i.LevelDBPersistence.Get(k, v)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	for _, m := range v.Member {
		values = append(values, clientmodel.LabelValue(m))
	}

	return values, true, nil
}

// Has implements LabelNameLabelValuesIndex.
func (i *LevelDBLabelNameLabelValuesIndex) Has(l clientmodel.LabelName) (ok bool, err error) {
	return i.LevelDBPersistence.Has(&dto.LabelName{
		Name: proto.String(string(l)),
	})
}

// NewLevelDBLabelNameLabelValuesIndex returns a LevelDBLabelNameLabelValuesIndex
// ready to use.
func NewLevelDBLabelNameLabelValuesIndex(o leveldb.LevelDBOptions) (*LevelDBLabelNameLabelValuesIndex, error) {
	s, err := leveldb.NewLevelDBPersistence(o)
	if err != nil {
		return nil, err
	}

	return &LevelDBLabelNameLabelValuesIndex{
		LevelDBPersistence: s,
	}, nil
}

// LabelPairFingerprintMapping is an in-memory map of LabelPairs to
// Fingerprints.
type LabelPairFingerprintMapping map[LabelPair]clientmodel.Fingerprints

// LabelPairFingerprintIndex models a database mapping LabelPairs to
// Fingerprints.
type LabelPairFingerprintIndex interface {
	raw.Database
	raw.ForEacher
	raw.Pruner

	IndexBatch(LabelPairFingerprintMapping) error
	Lookup(*LabelPair) (m clientmodel.Fingerprints, ok bool, err error)
	Has(*LabelPair) (ok bool, err error)
}

// LevelDBLabelPairFingerprintIndex implements LabelPairFingerprintIndex using
// leveldb.
type LevelDBLabelPairFingerprintIndex struct {
	*leveldb.LevelDBPersistence
}

// IndexBatch implements LabelPairFingerprintMapping.
func (i *LevelDBLabelPairFingerprintIndex) IndexBatch(m LabelPairFingerprintMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for pair, fps := range m {
		sort.Sort(fps)

		key := &dto.LabelPair{
			Name:  proto.String(string(pair.Name)),
			Value: proto.String(string(pair.Value)),
		}
		value := &dto.FingerprintCollection{}
		for _, fp := range fps {
			f := &dto.Fingerprint{}
			dumpFingerprint(f, fp)
			value.Member = append(value.Member, f)
		}

		batch.Put(key, value)
	}

	return i.LevelDBPersistence.Commit(batch)
}

// Lookup implements LabelPairFingerprintMapping.
func (i *LevelDBLabelPairFingerprintIndex) Lookup(p *LabelPair) (m clientmodel.Fingerprints, ok bool, err error) {
	k := &dto.LabelPair{
		Name:  proto.String(string(p.Name)),
		Value: proto.String(string(p.Value)),
	}
	v := &dto.FingerprintCollection{}

	ok, err = i.LevelDBPersistence.Get(k, v)

	if !ok {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	for _, pair := range v.Member {
		fp := &clientmodel.Fingerprint{}
		loadFingerprint(fp, pair)
		m = append(m, fp)
	}

	return m, true, nil
}

// Has implements LabelPairFingerprintMapping.
func (i *LevelDBLabelPairFingerprintIndex) Has(p *LabelPair) (ok bool, err error) {
	k := &dto.LabelPair{
		Name:  proto.String(string(p.Name)),
		Value: proto.String(string(p.Value)),
	}

	return i.LevelDBPersistence.Has(k)
}

// NewLevelDBLabelSetFingerprintIndex returns a LevelDBLabelPairFingerprintIndex
// object ready to use.
func NewLevelDBLabelSetFingerprintIndex(o leveldb.LevelDBOptions) (*LevelDBLabelPairFingerprintIndex, error) {
	s, err := leveldb.NewLevelDBPersistence(o)
	if err != nil {
		return nil, err
	}

	return &LevelDBLabelPairFingerprintIndex{
		LevelDBPersistence: s,
	}, nil
}

// MetricMembershipIndex models a database tracking the existence of Metrics.
type MetricMembershipIndex interface {
	raw.Database
	raw.Pruner

	IndexBatch(FingerprintMetricMapping) error
	Has(clientmodel.Metric) (ok bool, err error)
}

// LevelDBMetricMembershipIndex implements MetricMembershipIndex using leveldb.
type LevelDBMetricMembershipIndex struct {
	*leveldb.LevelDBPersistence
}

var existenceIdentity = &dto.MembershipIndexValue{}

// IndexBatch implements MetricMembershipIndex.
func (i *LevelDBMetricMembershipIndex) IndexBatch(b FingerprintMetricMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for _, m := range b {
		k := &dto.Metric{}
		dumpMetric(k, m)
		batch.Put(k, existenceIdentity)
	}

	return i.LevelDBPersistence.Commit(batch)
}

// Has implements MetricMembershipIndex.
func (i *LevelDBMetricMembershipIndex) Has(m clientmodel.Metric) (ok bool, err error) {
	k := &dto.Metric{}
	dumpMetric(k, m)

	return i.LevelDBPersistence.Has(k)
}

// NewLevelDBMetricMembershipIndex returns a LevelDBMetricMembershipIndex object
// ready to use.
func NewLevelDBMetricMembershipIndex(o leveldb.LevelDBOptions) (*LevelDBMetricMembershipIndex, error) {
	s, err := leveldb.NewLevelDBPersistence(o)
	if err != nil {
		return nil, err
	}

	return &LevelDBMetricMembershipIndex{
		LevelDBPersistence: s,
	}, nil
}

// MetricIndexer indexes facets of a clientmodel.Metric.
type MetricIndexer interface {
	// IndexMetric makes no assumptions about the concurrency safety of the
	// underlying implementer.
	IndexMetrics(FingerprintMetricMapping) error
}

// IndexerObserver listens and receives changes to a given
// FingerprintMetricMapping.
type IndexerObserver interface {
	Observe(FingerprintMetricMapping) error
}

// IndexerProxy receives IndexMetric requests and proxies them to the underlying
// MetricIndexer.  Upon success of the underlying receiver, the registered
// IndexObservers are called serially.
//
// If an error occurs in the underlying MetricIndexer or any of the observers,
// this proxy will not work any further and return the offending error in this
// call or any subsequent ones.
type IndexerProxy struct {
	err error

	i         MetricIndexer
	observers []IndexerObserver
}

// IndexMetrics proxies the given FingerprintMetricMapping to the underlying
// MetricIndexer and calls all registered observers with it.
func (p *IndexerProxy) IndexMetrics(b FingerprintMetricMapping) error {
	if p.err != nil {
		return p.err
	}
	if p.err = p.i.IndexMetrics(b); p.err != nil {
		return p.err
	}

	for _, o := range p.observers {
		if p.err = o.Observe(b); p.err != nil {
			return p.err
		}
	}

	return nil
}

// Close closes the underlying indexer.
func (p *IndexerProxy) Close() error {
	if p.err != nil {
		return p.err
	}
	if closer, ok := p.i.(io.Closer); ok {
		p.err = closer.Close()
		return p.err
	}
	return nil
}

// Flush flushes the underlying index requests before closing.
func (p *IndexerProxy) Flush() error {
	if p.err != nil {
		return p.err
	}
	if flusher, ok := p.i.(flusher); ok {
		p.err = flusher.Flush()
		return p.err
	}
	return nil
}

// NewIndexerProxy builds an IndexerProxy for the given configuration.
func NewIndexerProxy(i MetricIndexer, o ...IndexerObserver) *IndexerProxy {
	return &IndexerProxy{
		i:         i,
		observers: o,
	}
}

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

type flusher interface {
	Flush() error
}

// Flush calls Flush of the wrapped MetricIndexer after acquiring a lock. If the
// wrapped MetricIndexer has no Flush method, this is a no-op.
func (i *SynchronizedIndexer) Flush() error {
	if flusher, ok := i.i.(flusher); ok {
		i.mu.Lock()
		defer i.mu.Unlock()

		return flusher.Flush()
	}

	return nil
}

// Close calls Close of the wrapped MetricIndexer after acquiring a lock. If the
// wrapped MetricIndexer has no Close method, this is a no-op.
func (i *SynchronizedIndexer) Close() error {
	if closer, ok := i.i.(io.Closer); ok {
		i.mu.Lock()
		defer i.mu.Unlock()

		return closer.Close()
	}

	return nil
}

// NewSynchronizedIndexer returns a SynchronizedIndexer wrapping the given
// MetricIndexer.
func NewSynchronizedIndexer(i MetricIndexer) *SynchronizedIndexer {
	return &SynchronizedIndexer{
		i: i,
	}
}

// BufferedIndexer provides unsynchronized index buffering.
//
// If an error occurs in the underlying MetricIndexer or any of the observers,
// this proxy will not work any further and return the offending error.
type BufferedIndexer struct {
	i MetricIndexer

	limit int

	buf []FingerprintMetricMapping

	err error
}

// IndexMetrics writes the entries in the given FingerprintMetricMapping to the
// index.
func (i *BufferedIndexer) IndexMetrics(b FingerprintMetricMapping) error {
	if i.err != nil {
		return i.err
	}

	if len(i.buf) < i.limit {
		i.buf = append(i.buf, b)

		return nil
	}

	i.err = i.Flush()

	return i.err
}

// Flush writes all pending entries to the index.
func (i *BufferedIndexer) Flush() error {
	if i.err != nil {
		return i.err
	}

	if len(i.buf) == 0 {
		return nil
	}

	superset := FingerprintMetricMapping{}
	for _, b := range i.buf {
		for fp, m := range b {
			if _, ok := superset[fp]; ok {
				continue
			}

			superset[fp] = m
		}
	}

	i.buf = make([]FingerprintMetricMapping, 0, i.limit)

	i.err = i.i.IndexMetrics(superset)

	return i.err
}

// Close flushes and closes the underlying buffer.
func (i *BufferedIndexer) Close() error {
	if err := i.Flush(); err != nil {
		return err
	}

	if closer, ok := i.i.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}

// NewBufferedIndexer returns a BufferedIndexer ready to use.
func NewBufferedIndexer(i MetricIndexer, limit int) *BufferedIndexer {
	return &BufferedIndexer{
		i:     i,
		limit: limit,
		buf:   make([]FingerprintMetricMapping, 0, limit),
	}
}

// TotalIndexer is a MetricIndexer that indexes all standard facets of a metric
// that a user or the Prometheus subsystem would want to query against:
//
//    "<Label Name>" -> {Fingerprint, ...}
//    "<Label Name> <Label Value>" -> {Fingerprint, ...}
//
//    "<Fingerprint>" -> Metric
//
//    "<Metric>" -> Existence Value
//
// This type supports concrete queries but only single writes, and it has no
// locking semantics to enforce this.
type TotalIndexer struct {
	FingerprintToMetric    FingerprintMetricIndex
	LabelNameToLabelValues LabelNameLabelValuesIndex
	LabelPairToFingerprint LabelPairFingerprintIndex
	MetricMembership       MetricMembershipIndex
}

func findUnindexed(i MetricMembershipIndex, b FingerprintMetricMapping) (FingerprintMetricMapping, error) {
	out := FingerprintMetricMapping{}

	for fp, m := range b {
		has, err := i.Has(m)
		if err != nil {
			return nil, err
		}
		if !has {
			out[fp] = m
		}
	}

	return out, nil
}

func extendLabelNameToLabelValuesIndex(i LabelNameLabelValuesIndex, b FingerprintMetricMapping) (LabelNameLabelValuesMapping, error) {
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

func extendLabelPairIndex(i LabelPairFingerprintIndex, b FingerprintMetricMapping) (LabelPairFingerprintMapping, error) {
	collection := map[LabelPair]utility.Set{}

	for fp, m := range b {
		for n, v := range m {
			pair := LabelPair{
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
					set.Add(*baseFp)
				}

				collection[pair] = set
			}

			set.Add(fp)
		}
	}

	batch := LabelPairFingerprintMapping{}

	for pair, set := range collection {
		fps := batch[pair]
		for element := range set {
			fp := element.(clientmodel.Fingerprint)
			fps = append(fps, &fp)
		}
		batch[pair] = fps
	}

	return batch, nil
}

// IndexMetrics adds the facets of all unindexed metrics found in the given
// FingerprintMetricMapping to the corresponding indices.
func (i *TotalIndexer) IndexMetrics(b FingerprintMetricMapping) error {
	unindexed, err := findUnindexed(i.MetricMembership, b)
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

	labelPairs, err := extendLabelPairIndex(i.LabelPairToFingerprint, unindexed)
	if err != nil {
		return err
	}
	if err := i.LabelPairToFingerprint.IndexBatch(labelPairs); err != nil {
		return err
	}

	if err := i.FingerprintToMetric.IndexBatch(unindexed); err != nil {
		return err
	}

	return i.MetricMembership.IndexBatch(unindexed)
}
