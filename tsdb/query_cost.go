// Copyright The Prometheus Authors
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

package tsdb

import (
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/storage"
)

// CostSamplingQueryable returns a source restricted to incremental native readers.
func (db *DB) CostSamplingQueryable() (storage.SampleAndChunkQueryable, bool) {
	return costSamplingQueryable{db: db}, true
}

type costSamplingQueryable struct {
	db *DB
}

func (q costSamplingQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	base, err := q.blockQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	if base == nil {
		return storage.NoopQuerier(), nil
	}
	return &blockQuerier{blockBaseQuerier: base}, nil
}

func (q costSamplingQueryable) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	base, err := q.blockQuerier(mint, maxt)
	if err != nil {
		return nil, err
	}
	if base == nil {
		return storage.NoopChunkedQuerier(), nil
	}
	return &blockChunkQuerier{blockBaseQuerier: base}, nil
}

func (q costSamplingQueryable) blockQuerier(mint, maxt int64) (*blockBaseQuerier, error) {
	queriers, err := q.db.blockChunkQuerierForRange(mint, maxt)
	if err != nil {
		return nil, err
	}
	if len(queriers) == 0 {
		return nil, nil
	}
	if len(queriers) == 1 {
		if native, ok := queriers[0].(*blockChunkQuerier); ok {
			// Transfer ownership of the pinned readers to the returned querier.
			return native.blockBaseQuerier, nil
		}
	}
	// Check the actual pinned layout, rather than mutable configuration or block
	// metadata. OOO iterables and merged readers can encode a whole window eagerly.
	var closeErrs []error
	for _, querier := range queriers {
		closeErrs = append(closeErrs, querier.Close())
	}
	if err := errors.Join(closeErrs...); err != nil {
		return nil, fmt.Errorf("close unsupported cost sampling readers: %w", err)
	}
	return nil, storage.ErrCostSamplingUnsupported
}
