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
	"time"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/raw"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility"

	dto "github.com/prometheus/prometheus/model/generated"
)

type watermarks struct {
	High time.Time
}

func (w *watermarks) load(d *dto.MetricHighWatermark) {
	w.High = time.Unix(d.GetTimestamp(), 0).UTC()
}

func (w *watermarks) dump(d *dto.MetricHighWatermark) {
	d.Reset()

	d.Timestamp = proto.Int64(w.High.Unix())
}

type FingerprintHighWatermarkMapping map[clientmodel.Fingerprint]time.Time

type HighWatermarker interface {
	raw.ForEacher
	raw.Pruner

	UpdateBatch(FingerprintHighWatermarkMapping) error
	Get(*clientmodel.Fingerprint) (t time.Time, ok bool, err error)
	State() *raw.DatabaseState
	Size() (uint64, bool, error)
}

type LevelDBHighWatermarker struct {
	p *leveldb.LevelDBPersistence
}

func (w *LevelDBHighWatermarker) Get(f *clientmodel.Fingerprint) (t time.Time, ok bool, err error) {
	k := new(dto.Fingerprint)
	dumpFingerprint(k, f)
	v := new(dto.MetricHighWatermark)
	ok, err = w.p.Get(k, v)
	if err != nil {
		return t, ok, err
	}
	if !ok {
		return t, ok, nil
	}
	t = time.Unix(v.GetTimestamp(), 0)
	return t, true, nil
}

func (w *LevelDBHighWatermarker) UpdateBatch(m FingerprintHighWatermarkMapping) error {
	batch := leveldb.NewBatch()
	defer batch.Close()

	for fp, t := range m {
		existing, present, err := w.Get(&fp)
		if err != nil {
			return err
		}
		k := new(dto.Fingerprint)
		dumpFingerprint(k, &fp)
		v := new(dto.MetricHighWatermark)
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

	return w.p.Commit(batch)
}

func (w *LevelDBHighWatermarker) ForEach(d storage.RecordDecoder, f storage.RecordFilter, o storage.RecordOperator) (bool, error) {
	return w.p.ForEach(d, f, o)
}

func (w *LevelDBHighWatermarker) Prune() (bool, error) {
	w.p.Prune()

	return false, nil
}

func (w *LevelDBHighWatermarker) Close() {
	w.p.Close()
}

func (w *LevelDBHighWatermarker) State() *raw.DatabaseState {
	return w.p.State()
}

func (w *LevelDBHighWatermarker) Size() (uint64, bool, error) {
	s, err := w.p.Size()
	return s, true, err
}

type LevelDBHighWatermarkerOptions struct {
	leveldb.LevelDBOptions
}

func NewLevelDBHighWatermarker(o LevelDBHighWatermarkerOptions) (*LevelDBHighWatermarker, error) {
	s, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBHighWatermarker{
		p: s,
	}, nil
}

type CurationRemarker interface {
	raw.Pruner

	Update(*curationKey, time.Time) error
	Get(*curationKey) (t time.Time, ok bool, err error)
	State() *raw.DatabaseState
	Size() (uint64, bool, error)
}

type LevelDBCurationRemarker struct {
	p *leveldb.LevelDBPersistence
}

type LevelDBCurationRemarkerOptions struct {
	leveldb.LevelDBOptions
}

func (w *LevelDBCurationRemarker) State() *raw.DatabaseState {
	return w.p.State()
}

func (w *LevelDBCurationRemarker) Size() (uint64, bool, error) {
	s, err := w.p.Size()
	return s, true, err
}

func (w *LevelDBCurationRemarker) Close() {
	w.p.Close()
}

func (w *LevelDBCurationRemarker) Prune() (bool, error) {
	w.p.Prune()

	return false, nil
}

func (w *LevelDBCurationRemarker) Get(c *curationKey) (t time.Time, ok bool, err error) {
	k := new(dto.CurationKey)
	c.dump(k)
	v := new(dto.CurationValue)

	ok, err = w.p.Get(k, v)
	if err != nil || !ok {
		return t, ok, err
	}

	return time.Unix(v.GetLastCompletionTimestamp(), 0).UTC(), true, nil
}

func (w *LevelDBCurationRemarker) Update(pair *curationKey, t time.Time) error {
	k := new(dto.CurationKey)
	pair.dump(k)

	return w.p.Put(k, &dto.CurationValue{
		LastCompletionTimestamp: proto.Int64(t.Unix()),
	})
}

func NewLevelDBCurationRemarker(o LevelDBCurationRemarkerOptions) (*LevelDBCurationRemarker, error) {
	s, err := leveldb.NewLevelDBPersistence(o.LevelDBOptions)
	if err != nil {
		return nil, err
	}

	return &LevelDBCurationRemarker{
		p: s,
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
