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

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility"

	dto "github.com/prometheus/prometheus/model/generated"
)

type watermarks struct {
	High clientmodel.Timestamp
}

func (w *watermarks) load(d *dto.MetricHighWatermark) {
	w.High = clientmodel.TimestampFromUnix(d.GetTimestamp())
}

func (w *watermarks) dump(d *dto.MetricHighWatermark) {
	d.Reset()

	d.Timestamp = proto.Int64(w.High.Unix())
}

// A FingerprintHighWatermarkMapping is used for batch updates of many high
// watermarks in a database.
type FingerprintHighWatermarkMapping map[clientmodel.Fingerprint]clientmodel.Timestamp

// HighWatermarker models a high-watermark database.
type HighWatermarker interface {
	raw.Database
	raw.ForEacher
	raw.Pruner

	UpdateBatch(FingerprintHighWatermarkMapping) error
	Get(*clientmodel.Fingerprint) (t clientmodel.Timestamp, ok bool, err error)
}

// LevelDBHighWatermarker is an implementation of HighWatermarker backed by
// leveldb.
type LevelDBHighWatermarker struct {
	*leveldb.LevelDBPersistence
}

// Get implements HighWatermarker.
func (w *LevelDBHighWatermarker) Get(f *clientmodel.Fingerprint) (t clientmodel.Timestamp, ok bool, err error) {
	k := &dto.Fingerprint{}
	dumpFingerprint(k, f)
	v := &dto.MetricHighWatermark{}
	ok, err = w.LevelDBPersistence.Get(k, v)
	if err != nil {
		return t, ok, err
	}
	if !ok {
		return clientmodel.TimestampFromUnix(0), ok, nil
	}
	t = clientmodel.TimestampFromUnix(v.GetTimestamp())
	return t, true, nil
}

// UpdateBatch implements HighWatermarker.
func (w *LevelDBHighWatermarker) UpdateBatch(m FingerprintHighWatermarkMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for fp, t := range m {
		existing, present, err := w.Get(&fp)
		if err != nil {
			return err
		}
		k := &dto.Fingerprint{}
		dumpFingerprint(k, &fp)
		v := &dto.MetricHighWatermark{}
		if !present {
			v.Timestamp = proto.Int64(t.Unix())
			batch.Put(k, v)

			continue
		}

		// BUG(matt): Replace this with watermark management.
		if t.After(existing) {
			v.Timestamp = proto.Int64(t.Unix())
			batch.Put(k, v)
		}
	}

	return w.LevelDBPersistence.Commit(batch)
}

// LevelDBHighWatermarkerOptions just wraps leveldb.LevelDBOptions.
type LevelDBHighWatermarkerOptions struct {
	leveldb.LevelDBOptions
}

// NewLevelDBHighWatermarker returns a LevelDBHighWatermarker ready to use.
func NewLevelDBHighWatermarker(o LevelDBHighWatermarkerOptions) (*LevelDBHighWatermarker, error) {
	s, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBHighWatermarker{
		LevelDBPersistence: s,
	}, nil
}

// CurationRemarker models a curation remarker database.
type CurationRemarker interface {
	raw.Database
	raw.Pruner

	Update(*curationKey, clientmodel.Timestamp) error
	Get(*curationKey) (t clientmodel.Timestamp, ok bool, err error)
}

// LevelDBCurationRemarker is an implementation of CurationRemarker backed by
// leveldb.
type LevelDBCurationRemarker struct {
	*leveldb.LevelDBPersistence
}

// LevelDBCurationRemarkerOptions just wraps leveldb.LevelDBOptions.
type LevelDBCurationRemarkerOptions struct {
	leveldb.LevelDBOptions
}

// Get implements CurationRemarker.
func (w *LevelDBCurationRemarker) Get(c *curationKey) (t clientmodel.Timestamp, ok bool, err error) {
	k := &dto.CurationKey{}
	c.dump(k)
	v := &dto.CurationValue{}

	ok, err = w.LevelDBPersistence.Get(k, v)
	if err != nil || !ok {
		return clientmodel.TimestampFromUnix(0), ok, err
	}

	return clientmodel.TimestampFromUnix(v.GetLastCompletionTimestamp()), true, nil
}

// Update implements CurationRemarker.
func (w *LevelDBCurationRemarker) Update(pair *curationKey, t clientmodel.Timestamp) error {
	k := &dto.CurationKey{}
	pair.dump(k)

	return w.LevelDBPersistence.Put(k, &dto.CurationValue{
		LastCompletionTimestamp: proto.Int64(t.Unix()),
	})
}

// NewLevelDBCurationRemarker returns a LevelDBCurationRemarker ready to use.
func NewLevelDBCurationRemarker(o LevelDBCurationRemarkerOptions) (*LevelDBCurationRemarker, error) {
	s, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBCurationRemarker{
		LevelDBPersistence: s,
	}, nil
}

type watermarkCache struct {
	C utility.Cache
}

func (c *watermarkCache) Get(f *clientmodel.Fingerprint) (*watermarks, bool, error) {
	v, ok, err := c.C.Get(*f)
	if ok {
		return v.(*watermarks), ok, err
	}

	return nil, ok, err
}

func (c *watermarkCache) Put(f *clientmodel.Fingerprint, v *watermarks) (bool, error) {
	return c.C.Put(*f, v)
}

func (c *watermarkCache) Clear() (bool, error) {
	return c.C.Clear()
}
