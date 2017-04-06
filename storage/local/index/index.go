// Copyright 2014 The Prometheus Authors
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

// Package index provides a number of indexes backed by persistent key-value
// stores.  The only supported implementation of a key-value store is currently
// goleveldb, but other implementations can easily be added.
package index

import (
	"os"
	"path"
	"path/filepath"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/local/codable"
)

// Directory names for LevelDB indices.
const (
	FingerprintToMetricDir     = "archived_fingerprint_to_metric"
	FingerprintTimeRangeDir    = "archived_fingerprint_to_timerange"
	LabelNameToLabelValuesDir  = "labelname_to_labelvalues"
	LabelPairToFingerprintsDir = "labelpair_to_fingerprints"
)

// LevelDB cache sizes, changeable via flags.
var (
	FingerprintMetricCacheSize     = 10 * 1024 * 1024
	FingerprintTimeRangeCacheSize  = 5 * 1024 * 1024
	LabelNameLabelValuesCacheSize  = 10 * 1024 * 1024
	LabelPairFingerprintsCacheSize = 20 * 1024 * 1024
)

// FingerprintMetricMapping is an in-memory map of fingerprints to metrics.
type FingerprintMetricMapping map[model.Fingerprint]model.Metric

// FingerprintMetricIndex models a database mapping fingerprints to metrics.
type FingerprintMetricIndex struct {
	KeyValueStore
}

// IndexBatch indexes a batch of mappings from fingerprints to metrics.
//
// This method is goroutine-safe, but note that no specific order of execution
// can be guaranteed (especially critical if IndexBatch and UnindexBatch are
// called concurrently for the same fingerprint).
func (i *FingerprintMetricIndex) IndexBatch(mapping FingerprintMetricMapping) error {
	b := i.NewBatch()

	for fp, m := range mapping {
		if err := b.Put(codable.Fingerprint(fp), codable.Metric(m)); err != nil {
			return err
		}
	}

	return i.Commit(b)
}

// UnindexBatch unindexes a batch of mappings from fingerprints to metrics.
//
// This method is goroutine-safe, but note that no specific order of execution
// can be guaranteed (especially critical if IndexBatch and UnindexBatch are
// called concurrently for the same fingerprint).
func (i *FingerprintMetricIndex) UnindexBatch(mapping FingerprintMetricMapping) error {
	b := i.NewBatch()

	for fp := range mapping {
		if err := b.Delete(codable.Fingerprint(fp)); err != nil {
			return err
		}
	}

	return i.Commit(b)
}

// Lookup looks up a metric by fingerprint. Looking up a non-existing
// fingerprint is not an error. In that case, (nil, false, nil) is returned.
//
// This method is goroutine-safe.
func (i *FingerprintMetricIndex) Lookup(fp model.Fingerprint) (metric model.Metric, ok bool, err error) {
	ok, err = i.Get(codable.Fingerprint(fp), (*codable.Metric)(&metric))
	return
}

// NewFingerprintMetricIndex returns a LevelDB-backed FingerprintMetricIndex
// ready to use.
func NewFingerprintMetricIndex(basePath string) (*FingerprintMetricIndex, error) {
	fingerprintToMetricDB, err := NewLevelDB(LevelDBOptions{
		Path:           filepath.Join(basePath, FingerprintToMetricDir),
		CacheSizeBytes: FingerprintMetricCacheSize,
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
type LabelNameLabelValuesMapping map[model.LabelName]codable.LabelValueSet

// LabelNameLabelValuesIndex is a KeyValueStore that maps existing label names
// to all label values stored for that label name.
type LabelNameLabelValuesIndex struct {
	KeyValueStore
}

// IndexBatch adds a batch of label name to label values mappings to the
// index. A mapping of a label name to an empty slice of label values results in
// a deletion of that mapping from the index.
//
// While this method is fundamentally goroutine-safe, note that the order of
// execution for multiple batches executed concurrently is undefined.
func (i *LabelNameLabelValuesIndex) IndexBatch(b LabelNameLabelValuesMapping) error {
	batch := i.NewBatch()

	for name, values := range b {
		if len(values) == 0 {
			if err := batch.Delete(codable.LabelName(name)); err != nil {
				return err
			}
		} else {
			if err := batch.Put(codable.LabelName(name), values); err != nil {
				return err
			}
		}
	}

	return i.Commit(batch)
}

// Lookup looks up all label values for a given label name and returns them as
// model.LabelValues (which is a slice). Looking up a non-existing label
// name is not an error. In that case, (nil, false, nil) is returned.
//
// This method is goroutine-safe.
func (i *LabelNameLabelValuesIndex) Lookup(l model.LabelName) (values model.LabelValues, ok bool, err error) {
	ok, err = i.Get(codable.LabelName(l), (*codable.LabelValues)(&values))
	return
}

// LookupSet looks up all label values for a given label name and returns them
// as a set. Looking up a non-existing label name is not an error. In that case,
// (nil, false, nil) is returned.
//
// This method is goroutine-safe.
func (i *LabelNameLabelValuesIndex) LookupSet(l model.LabelName) (values map[model.LabelValue]struct{}, ok bool, err error) {
	ok, err = i.Get(codable.LabelName(l), (*codable.LabelValueSet)(&values))
	if values == nil {
		values = map[model.LabelValue]struct{}{}
	}
	return
}

// NewLabelNameLabelValuesIndex returns a LevelDB-backed
// LabelNameLabelValuesIndex ready to use.
func NewLabelNameLabelValuesIndex(basePath string) (*LabelNameLabelValuesIndex, error) {
	labelNameToLabelValuesDB, err := NewLevelDB(LevelDBOptions{
		Path:           filepath.Join(basePath, LabelNameToLabelValuesDir),
		CacheSizeBytes: LabelNameLabelValuesCacheSize,
	})
	if err != nil {
		return nil, err
	}
	return &LabelNameLabelValuesIndex{
		KeyValueStore: labelNameToLabelValuesDB,
	}, nil
}

// DeleteLabelNameLabelValuesIndex deletes the LevelDB-backed
// LabelNameLabelValuesIndex. Use only for a not yet opened index.
func DeleteLabelNameLabelValuesIndex(basePath string) error {
	return os.RemoveAll(path.Join(basePath, LabelNameToLabelValuesDir))
}

// LabelPairFingerprintsMapping is an in-memory map of label pairs to
// fingerprints.
type LabelPairFingerprintsMapping map[model.LabelPair]codable.FingerprintSet

// LabelPairFingerprintIndex is a KeyValueStore that maps existing label pairs
// to the fingerprints of all metrics containing those label pairs.
type LabelPairFingerprintIndex struct {
	KeyValueStore
}

// IndexBatch indexes a batch of mappings from label pairs to fingerprints. A
// mapping to an empty slice of fingerprints results in deletion of that mapping
// from the index.
//
// While this method is fundamentally goroutine-safe, note that the order of
// execution for multiple batches executed concurrently is undefined.
func (i *LabelPairFingerprintIndex) IndexBatch(m LabelPairFingerprintsMapping) (err error) {
	batch := i.NewBatch()

	for pair, fps := range m {
		if len(fps) == 0 {
			err = batch.Delete(codable.LabelPair(pair))
		} else {
			err = batch.Put(codable.LabelPair(pair), fps)
		}

		if err != nil {
			return err
		}
	}

	return i.Commit(batch)
}

// Lookup looks up all fingerprints for a given label pair.  Looking up a
// non-existing label pair is not an error. In that case, (nil, false, nil) is
// returned.
//
// This method is goroutine-safe.
func (i *LabelPairFingerprintIndex) Lookup(p model.LabelPair) (fps model.Fingerprints, ok bool, err error) {
	ok, err = i.Get((codable.LabelPair)(p), (*codable.Fingerprints)(&fps))
	return
}

// LookupSet looks up all fingerprints for a given label pair.  Looking up a
// non-existing label pair is not an error. In that case, (nil, false, nil) is
// returned.
//
// This method is goroutine-safe.
func (i *LabelPairFingerprintIndex) LookupSet(p model.LabelPair) (fps map[model.Fingerprint]struct{}, ok bool, err error) {
	ok, err = i.Get((codable.LabelPair)(p), (*codable.FingerprintSet)(&fps))
	if fps == nil {
		fps = map[model.Fingerprint]struct{}{}
	}
	return
}

// NewLabelPairFingerprintIndex returns a LevelDB-backed
// LabelPairFingerprintIndex ready to use.
func NewLabelPairFingerprintIndex(basePath string) (*LabelPairFingerprintIndex, error) {
	labelPairToFingerprintsDB, err := NewLevelDB(LevelDBOptions{
		Path:           filepath.Join(basePath, LabelPairToFingerprintsDir),
		CacheSizeBytes: LabelPairFingerprintsCacheSize,
	})
	if err != nil {
		return nil, err
	}
	return &LabelPairFingerprintIndex{
		KeyValueStore: labelPairToFingerprintsDB,
	}, nil
}

// DeleteLabelPairFingerprintIndex deletes the LevelDB-backed
// LabelPairFingerprintIndex. Use only for a not yet opened index.
func DeleteLabelPairFingerprintIndex(basePath string) error {
	return os.RemoveAll(path.Join(basePath, LabelPairToFingerprintsDir))
}

// FingerprintTimeRangeIndex models a database tracking the time ranges
// of metrics by their fingerprints.
type FingerprintTimeRangeIndex struct {
	KeyValueStore
}

// Lookup returns the time range for the given fingerprint.  Looking up a
// non-existing fingerprint is not an error. In that case, (0, 0, false, nil) is
// returned.
//
// This method is goroutine-safe.
func (i *FingerprintTimeRangeIndex) Lookup(fp model.Fingerprint) (firstTime, lastTime model.Time, ok bool, err error) {
	var tr codable.TimeRange
	ok, err = i.Get(codable.Fingerprint(fp), &tr)
	return tr.First, tr.Last, ok, err
}

// NewFingerprintTimeRangeIndex returns a LevelDB-backed
// FingerprintTimeRangeIndex ready to use.
func NewFingerprintTimeRangeIndex(basePath string) (*FingerprintTimeRangeIndex, error) {
	fingerprintTimeRangeDB, err := NewLevelDB(LevelDBOptions{
		Path:           filepath.Join(basePath, FingerprintTimeRangeDir),
		CacheSizeBytes: FingerprintTimeRangeCacheSize,
	})
	if err != nil {
		return nil, err
	}
	return &FingerprintTimeRangeIndex{
		KeyValueStore: fingerprintTimeRangeDB,
	}, nil
}

// DeleteFingerprintTimeRangeIndex deletes the LevelDB-backed
// FingerprintTimeRangeIndex. Use only for a not yet opened index.
func DeleteFingerprintTimeRangeIndex(basePath string) error {
	return os.RemoveAll(path.Join(basePath, FingerprintTimeRangeDir))
}
