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

package shards

import (
	"github.com/cockroachdb/pebble"

	"github.com/prometheus/prometheus/tsdbv2/cursor"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	"github.com/prometheus/prometheus/tsdbv2/fields"
)

// ReadShardsForTenant returns current shards for tenant. In order to scale the
// number of supported tenants we store this information as equality encoded bitmap
// where column ID is the shard and row ID is the tenant.
//
// This is possible because we know both shard and tenant ID are of low cardinality.
// It is guaranteeed to always be in the shard 0 because  there is no way to reach 1 million
// shards.
//
// A slice is returned instead of a bitmap to ease distributing the work on shards
// across available CPU.
func ReadShardsForTenant(db *pebble.DB, tenant uint64) ([]uint64, error) {
	it, err := db.NewIter(nil)
	if err != nil {
		return nil, err
	}
	defer it.Close()

	cu := cursor.New(it, tenant)
	if !cu.ResetData(0, fields.Shards.Hash()) {
		return nil, nil
	}
	ra := bitmaps.Row(cu, 0, tenant)
	return bitmaps.Slice(ra), nil
}
