// Copyright 2024 The Prometheus Authors
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

package tsdbv2

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/cockroachdb/pebble"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdbv2/batch"
	"github.com/prometheus/prometheus/tsdbv2/cache"
	"github.com/prometheus/prometheus/tsdbv2/compat"
	"github.com/prometheus/prometheus/tsdbv2/cursor"
	"github.com/prometheus/prometheus/tsdbv2/data"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
	"github.com/prometheus/prometheus/tsdbv2/keys"
	"github.com/prometheus/prometheus/tsdbv2/query"
	"github.com/prometheus/prometheus/tsdbv2/tenants"
	"github.com/prometheus/prometheus/tsdbv2/translate"
)

type DB struct {
	config config
	db     *pebble.DB
	*translate.Transtale
	dbPath string
}

type config struct {
	pebble              *pebble.Options
	defaultToRootTenant bool
}

type Option interface {
	apply(*config)
}

type optionFunc func(*config)

func (f optionFunc) apply(db *config) {
	f(db)
}

// WithRootTenant treats absence of a tenant ID the same as using toot Tenant ID.
// Defaults to true.
func WithRootTenant(yes bool) Option {
	return optionFunc(func(d *config) {
		d.defaultToRootTenant = true
	})
}

func (co *config) defaults() {
	co.defaultToRootTenant = true
}

func New(path string, opts ...Option) (*DB, error) {
	var co config
	co.defaults()
	for i := range opts {
		opts[i].apply(&co)
	}
	pdb, err := data.Open(path, co.pebble)
	if err != nil {
		return nil, err
	}
	db := &DB{
		db:        pdb,
		Transtale: translate.New(pdb),
		config:    co,
	}
	return db, nil
}

var (
	_ storage.Storage         = (*DB)(nil)
	_ storage.ExemplarStorage = (*DB)(nil)
	_ translate.Translator    = (*DB)(nil)
	_ tenants.Sounce          = (*DB)(nil)
)

// Appender returnns storage.Appender implementation that uses columnar roaring bitmap
// based index on top of pebble key value store.
//
// Appender can be called multiple times and is safe for concurrent use, however the
// returned  storage.Appender  is not safe for concurrent use. Call [storage.Appender.Commit]
// to persist the data or [storage.Appender.Rollback] to discard the changes.
//
// Under the hood [batch.Batch] is returned. We document here the storage layout instead of on
// [batch.Batch] because on the [batch.Batch] it is considered an implementation detail.
// Over here however it gives a high lever overview on how things actually work.
//
// A sample is broken down into the following columns, we will use full  symbol reference on
// where the columns are defined inside fields package.
//  1. [fields.Series]
//  2. [fields.Kind]
//  3. [fields.Labels]
//  4. [fields.Timestamp]
//  5. [fields.Value]
//
// Columns are grouped into the following data types
//  1. Equality
//  2. Bit sliced indexed (BSI)
//
// Sample able columns and their data types
//
//	Name      DataType
//	series    BSI
//	kind      Equality
//	labels    Equality
//	timestamp BSI
//	value     BSI
//
// Each sample is assigned a unique uint64 ID that is monotomically increasing, ie (1, 2, 3...).
// Throughout this doc,sample ID will somethime be referred to as column_id.
//
// # Equality encoded data type
//
// Used to record low cardinality uint64 values. We explained on main documantation that
// data is organized in 1 million samples  called shard.
//
//	Shardwidth = 1<<20
//
// Given a column position column_id and a row value row_id. We compute position in a roaring
// bitmap in such a way row_id will become a container key and column_id  bits will be set in
// its matching container.
//
// The formula used to compute the position is,
//
//	Position = ( row_id * ShardWidth ) + ( column_id % ShardWidth )
//
// This formula was designed by nice people from Pilosa. The computed position is stored
// in a roaring bitmap that is persisted in the undeerlying key value store. See the example
// ExampleDB_Appender_equality_encoded showcasing the formunla.
//
// With this formula we can now represent low cardinality columns in a roaring bitmap and
// efficiently find which sample record they belong to by  seeking to the row_id and
// read the container.
//
// Search for equality encoded columns only takes three steps.
//
//  1. Identify the row_id
//  2. Seek to the row_id container
//  3. Read the container ( which will contains all sample ID's that the searched low belongs to)
//
// # Encoding text columns
//
// Up to now we know equality encoding only works with uint64 and they must be of low cardinality
// which means we can't use random numbers or hashes.
//
// What we do is maintain a separate storage space dedicated to mapping  text to sequence of uint64
//
//	translateText func( text ) uint64
//	translateTD func( uint64 ) text
//
// We use the translated sequence to build the labels index.See [translate.Translate] for actual
// implementation that stores data in pebble.
//
// # BSI index
//
// Used to store high cardinality values that fits into int64. It is very similar to equality encoded
// except we store each bit as a separate rowID
//
//   - row 0 : stores existence bits (contains all colum_id present in this field)
//   - row 1 : stores signs ( which column_id is negative)
//   - row 4: first bit of the value
//   - row 5: second bit of the value
//   - ...: ...
//   - ...: 64 th  bit of the value
//
// This way we will only have at most 66 bitmaps to cover all possible int64 values.
//
// Helper functions to work with both encodings are found in [encodings/bitmaps] package.
//
// # Data layout
//
// Roaring bitmaps are stored as individual containers and [cursor.Cursor] is used to traverse them
//
// Data keys have the following components
//
//	[ prefix ][ tenant ][ shard ][ field ][ container ]
//
// prefix can either be [keys.Data] or [keys.Exustence] to differentiate between data only containers
// with containers that only check if sample ID was observed in the particular field.
func (db *DB) Appender(ctx context.Context) storage.Appender {
	ba := db.db.NewBatch()
	return batch.New(ba, db, db.config.defaultToRootTenant)
}

func (db *DB) Querier(mint, maxt int64) (storage.Querier, error) {
	return query.NewSeries(db.db, db, mint, maxt), nil
}

func (db *DB) ExemplarQuerier(ctx context.Context) (storage.ExemplarQuerier, error) {
	return query.NewExemplar(db.db, db), nil
}

func (db *DB) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	return nil, errors.New("tsdbv2: chunk querier not supported")
}

func (db *DB) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (result storage.SeriesRef, err error) {
	err = db.batch(func(b *batch.Batch) error {
		result, err = b.AppendExemplar(ref, l, e)
		return err
	})
	return
}

// StartTime returs the timestamp of the first sample recorded by the store. It does not
// matter if the samples arrived out of order. We compute the minimum timestamp observed
// by the first tenant processed by the store.
func (db *DB) StartTime() (int64, error) {
	return StartTime(db.db, time.Now().Unix()*1000)
}

// StartTime finds the eariest timestamp recorded for a sample. Returns bas if tehere
// was no timeseries data in the db yet.
func StartTime(db *pebble.DB, base int64) (int64, error) {
	it, err := db.NewIter(nil)
	if err != nil {
		return 0, err
	}
	defer it.Close()
	// find the oldest possible view
	prefix := []byte{byte(keys.Data)}

	if !it.SeekGE(prefix) {
		return base, nil
	}
	key := it.Key()
	if !bytes.HasPrefix(key, prefix) {
		return base, nil
	}

	tenant, shard := encoding.DataComponents(key)
	cu := cursor.New(it, tenant)

	if !cu.ResetData(shard, fields.Timestamp.Hash()) {
		return base, nil
	}
	ts, _ := bitmaps.MinBSI(cu, shard)
	return ts, nil
}

func (db *DB) SetRef(tenant uint64, series cache.Series) error {
	key := encoding.MakeSeriesRefCacheKey(tenant, uint64(series.Ref()))
	err := db.db.Set(key, series.Encode(), nil)
	return err
}

func (db *DB) GetRef(tenant uint64, ref storage.SeriesRef, f func([]byte) error) error {
	key := encoding.MakeSeriesRefCacheKey(tenant, uint64(ref))
	value, done, err := db.db.Get(key)
	if err != nil {
		return compat.ErrInvalidSample
	}
	defer done.Close()
	return f(value)
}

func (db *DB) DefaultToRootTenant() bool {
	return db.config.defaultToRootTenant
}

func (db *DB) Close() error {
	return db.db.Close()
}

func (db *DB) batch(f func(*batch.Batch) error) error {
	ba := db.db.NewBatch()
	bs := batch.New(ba, db, db.config.defaultToRootTenant)
	err := f(bs)
	if err != nil {
		_ = bs.Rollback()
		return err
	}
	return bs.Commit()
}
