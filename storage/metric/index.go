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

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility"

	dto "github.com/prometheus/prometheus/model/generated"
)

type FingerprintMetricMapping map[clientmodel.Fingerprint]clientmodel.Metric

type FingerprintMetricIndex interface {
	raw.Pruner

	IndexBatch(FingerprintMetricMapping) error
	Lookup(*clientmodel.Fingerprint) (m clientmodel.Metric, ok bool, err error)
	State() *raw.DatabaseState
	Size() (s uint64, present bool, err error)
}

type LevelDBFingerprintMetricIndex struct {
	p *leveldb.LevelDBPersistence
}

type LevelDBFingerprintMetricIndexOptions struct {
	leveldb.LevelDBOptions
}

func (i *LevelDBFingerprintMetricIndex) Close() {
	i.p.Close()
}

func (i *LevelDBFingerprintMetricIndex) State() *raw.DatabaseState {
	return i.p.State()
}

func (i *LevelDBFingerprintMetricIndex) Size() (uint64, bool, error) {
	s, err := i.p.Size()
	return s, true, err
}

func (i *LevelDBFingerprintMetricIndex) IndexBatch(mapping FingerprintMetricMapping) error {
	b := leveldb.NewBatch()
	defer b.Close()

	for f, m := range mapping {
		k := new(dto.Fingerprint)
		dumpFingerprint(k, &f)
		v := new(dto.Metric)
		dumpMetric(v, m)

		b.Put(k, v)
	}

	return i.p.Commit(b)
}

func (i *LevelDBFingerprintMetricIndex) Lookup(f *clientmodel.Fingerprint) (m clientmodel.Metric, ok bool, err error) {
	k := new(dto.Fingerprint)
	dumpFingerprint(k, f)
	v := new(dto.Metric)
	if ok, err := i.p.Get(k, v); !ok {
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

func (i *LevelDBFingerprintMetricIndex) Prune() (bool, error) {
	i.p.Prune()

	return false, nil
}

func NewLevelDBFingerprintMetricIndex(o LevelDBFingerprintMetricIndexOptions) (*LevelDBFingerprintMetricIndex, error) {
	s, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBFingerprintMetricIndex{
		p: s,
	}, nil
}

type LabelNameFingerprintMapping map[clientmodel.LabelName]clientmodel.Fingerprints

type LabelNameFingerprintIndex interface {
	raw.Pruner

	IndexBatch(LabelNameFingerprintMapping) error
	Lookup(clientmodel.LabelName) (fps clientmodel.Fingerprints, ok bool, err error)
	Has(clientmodel.LabelName) (ok bool, err error)
	State() *raw.DatabaseState
	Size() (s uint64, present bool, err error)
}

type LevelDBLabelNameFingerprintIndex struct {
	p *leveldb.LevelDBPersistence
}

func (i *LevelDBLabelNameFingerprintIndex) IndexBatch(b LabelNameFingerprintMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for labelName, fingerprints := range b {
		sort.Sort(fingerprints)

		key := &dto.LabelName{
			Name: proto.String(string(labelName)),
		}
		value := new(dto.FingerprintCollection)
		for _, fingerprint := range fingerprints {
			f := new(dto.Fingerprint)
			dumpFingerprint(f, fingerprint)
			value.Member = append(value.Member, f)
		}

		batch.Put(key, value)
	}

	return i.p.Commit(batch)
}

func (i *LevelDBLabelNameFingerprintIndex) Lookup(l clientmodel.LabelName) (fps clientmodel.Fingerprints, ok bool, err error) {
	k := new(dto.LabelName)
	dumpLabelName(k, l)
	v := new(dto.FingerprintCollection)
	ok, err = i.p.Get(k, v)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	for _, m := range v.Member {
		fp := new(clientmodel.Fingerprint)
		loadFingerprint(fp, m)
		fps = append(fps, fp)
	}

	return fps, true, nil
}

func (i *LevelDBLabelNameFingerprintIndex) Has(l clientmodel.LabelName) (ok bool, err error) {
	return i.p.Has(&dto.LabelName{
		Name: proto.String(string(l)),
	})
}

func (i *LevelDBLabelNameFingerprintIndex) Prune() (bool, error) {
	i.p.Prune()

	return false, nil
}

func (i *LevelDBLabelNameFingerprintIndex) Close() {
	i.p.Close()
}

func (i *LevelDBLabelNameFingerprintIndex) Size() (uint64, bool, error) {
	s, err := i.p.Size()
	return s, true, err
}

func (i *LevelDBLabelNameFingerprintIndex) State() *raw.DatabaseState {
	return i.p.State()
}

type LevelDBLabelNameFingerprintIndexOptions struct {
	leveldb.LevelDBOptions
}

func NewLevelLabelNameFingerprintIndex(o LevelDBLabelNameFingerprintIndexOptions) (*LevelDBLabelNameFingerprintIndex, error) {
	s, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBLabelNameFingerprintIndex{
		p: s,
	}, nil
}

type LabelPairFingerprintMapping map[LabelPair]clientmodel.Fingerprints

type LabelPairFingerprintIndex interface {
	raw.ForEacher
	raw.Pruner

	IndexBatch(LabelPairFingerprintMapping) error
	Lookup(*LabelPair) (m clientmodel.Fingerprints, ok bool, err error)
	Has(*LabelPair) (ok bool, err error)
	State() *raw.DatabaseState
	Size() (s uint64, present bool, err error)
}

type LevelDBLabelPairFingerprintIndex struct {
	p *leveldb.LevelDBPersistence
}

type LevelDBLabelSetFingerprintIndexOptions struct {
	leveldb.LevelDBOptions
}

func (i *LevelDBLabelPairFingerprintIndex) IndexBatch(m LabelPairFingerprintMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for pair, fps := range m {
		sort.Sort(fps)

		key := &dto.LabelPair{
			Name:  proto.String(string(pair.Name)),
			Value: proto.String(string(pair.Value)),
		}
		value := new(dto.FingerprintCollection)
		for _, fp := range fps {
			f := new(dto.Fingerprint)
			dumpFingerprint(f, fp)
			value.Member = append(value.Member, f)
		}

		batch.Put(key, value)
	}

	return i.p.Commit(batch)
}

func (i *LevelDBLabelPairFingerprintIndex) Lookup(p *LabelPair) (m clientmodel.Fingerprints, ok bool, err error) {
	k := &dto.LabelPair{
		Name:  proto.String(string(p.Name)),
		Value: proto.String(string(p.Value)),
	}
	v := new(dto.FingerprintCollection)

	ok, err = i.p.Get(k, v)

	if !ok {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	for _, pair := range v.Member {
		fp := new(clientmodel.Fingerprint)
		loadFingerprint(fp, pair)
		m = append(m, fp)
	}

	return m, true, nil
}

func (i *LevelDBLabelPairFingerprintIndex) Has(p *LabelPair) (ok bool, err error) {
	k := &dto.LabelPair{
		Name:  proto.String(string(p.Name)),
		Value: proto.String(string(p.Value)),
	}

	return i.p.Has(k)
}

func (i *LevelDBLabelPairFingerprintIndex) ForEach(d storage.RecordDecoder, f storage.RecordFilter, o storage.RecordOperator) (bool, error) {
	return i.p.ForEach(d, f, o)
}

func (i *LevelDBLabelPairFingerprintIndex) Prune() (bool, error) {
	i.p.Prune()
	return false, nil
}

func (i *LevelDBLabelPairFingerprintIndex) Close() {
	i.p.Close()
}

func (i *LevelDBLabelPairFingerprintIndex) Size() (uint64, bool, error) {
	s, err := i.p.Size()
	return s, true, err
}

func (i *LevelDBLabelPairFingerprintIndex) State() *raw.DatabaseState {
	return i.p.State()
}

func NewLevelDBLabelSetFingerprintIndex(o LevelDBLabelSetFingerprintIndexOptions) (*LevelDBLabelPairFingerprintIndex, error) {
	s, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBLabelPairFingerprintIndex{
		p: s,
	}, nil
}

type MetricMembershipIndex interface {
	raw.Pruner

	IndexBatch(FingerprintMetricMapping) error
	Has(clientmodel.Metric) (ok bool, err error)
	State() *raw.DatabaseState
	Size() (s uint64, present bool, err error)
}

type LevelDBMetricMembershipIndex struct {
	p *leveldb.LevelDBPersistence
}

var existenceIdentity = new(dto.MembershipIndexValue)

func (i *LevelDBMetricMembershipIndex) IndexBatch(b FingerprintMetricMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for _, m := range b {
		k := new(dto.Metric)
		dumpMetric(k, m)
		batch.Put(k, existenceIdentity)
	}

	return i.p.Commit(batch)
}

func (i *LevelDBMetricMembershipIndex) Has(m clientmodel.Metric) (ok bool, err error) {
	k := new(dto.Metric)
	dumpMetric(k, m)

	return i.p.Has(k)
}

func (i *LevelDBMetricMembershipIndex) Close() {
	i.p.Close()
}

func (i *LevelDBMetricMembershipIndex) Size() (uint64, bool, error) {
	s, err := i.p.Size()
	return s, true, err
}

func (i *LevelDBMetricMembershipIndex) State() *raw.DatabaseState {
	return i.p.State()
}

func (i *LevelDBMetricMembershipIndex) Prune() (bool, error) {
	i.p.Prune()

	return false, nil
}

type LevelDBMetricMembershipIndexOptions struct {
	leveldb.LevelDBOptions
}

func NewLevelDBMetricMembershipIndex(o LevelDBMetricMembershipIndexOptions) (*LevelDBMetricMembershipIndex, error) {
	s, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBMetricMembershipIndex{
		p: s,
	}, nil
}

// MetricIndexer indexes facets of a clientmodel.Metric.
type MetricIndexer interface {
	// IndexMetric makes no assumptions about the concurrency safety of the
	// underlying implementer.
	IndexMetrics(FingerprintMetricMapping) error
}

// IndexObserver listens and receives changes to a given
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

// Close flushes the underlying index requests before closing.
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

func (i *SynchronizedIndexer) IndexMetrics(b FingerprintMetricMapping) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.i.IndexMetrics(b)
}

type flusher interface {
	Flush() error
}

func (i *SynchronizedIndexer) Flush() error {
	if flusher, ok := i.i.(flusher); ok {
		i.mu.Lock()
		defer i.mu.Unlock()

		return flusher.Flush()
	}

	return nil
}

func (i *SynchronizedIndexer) Close() error {
	if closer, ok := i.i.(io.Closer); ok {
		i.mu.Lock()
		defer i.mu.Unlock()

		return closer.Close()
	}

	return nil
}

// NewSynchronizedIndexer builds a new MetricIndexer.
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

func (i *BufferedIndexer) IndexMetrics(b FingerprintMetricMapping) error {
	if i.err != nil {
		return i.err
	}

	if len(i.buf) < i.limit {
		i.buf = append(i.buf, b)

		return nil
	}

	i.buf = append(i.buf)

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
	LabelNameToFingerprint LabelNameFingerprintIndex
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

func extendLabelNameIndex(i LabelNameFingerprintIndex, b FingerprintMetricMapping) (LabelNameFingerprintMapping, error) {
	collection := map[clientmodel.LabelName]utility.Set{}

	for fp, m := range b {
		for l := range m {
			set, ok := collection[l]
			if !ok {
				baseFps, _, err := i.Lookup(l)
				if err != nil {
					return nil, err
				}

				set = utility.Set{}

				for _, baseFp := range baseFps {
					set.Add(*baseFp)
				}

				collection[l] = set
			}

			set.Add(fp)
		}
	}

	batch := LabelNameFingerprintMapping{}
	for l, set := range collection {
		fps := clientmodel.Fingerprints{}
		for e := range set {
			fp := e.(clientmodel.Fingerprint)
			fps = append(fps, &fp)
		}

		batch[l] = fps
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

func (i *TotalIndexer) IndexMetrics(b FingerprintMetricMapping) error {
	unindexed, err := findUnindexed(i.MetricMembership, b)
	if err != nil {
		return err
	}

	labelNames, err := extendLabelNameIndex(i.LabelNameToFingerprint, unindexed)
	if err != nil {
		return err
	}
	if err := i.LabelNameToFingerprint.IndexBatch(labelNames); err != nil {
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
