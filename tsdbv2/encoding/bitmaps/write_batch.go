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

package bitmaps

import (
	"github.com/cockroachdb/pebble"
	"github.com/gernest/roaring"

	"github.com/prometheus/prometheus/tsdbv2/encoding"
)

// Batch provides reusable data bitmaps writer. Bitmaps are broken down into containers
// and each container is stored in a batch using [*pebble.Batch.Merge].
//
// This is the only way to get bitmap data into pebble.
type Batch struct {
	key encoding.Key
}

// WriteData writes  normal bitmaps. Uses [keys.Data] prefix.
func (w *Batch) WriteData(ba *pebble.Batch, tenant, shard, field uint64, ra *roaring.Bitmap) error {
	w.key.SetData(tenant, shard, field, 0)
	return w.write(ba, ra)
}

// WriteExistence writes existence bitmap. Uses [keys.Existence] prefix.
func (w *Batch) WriteExistence(ba *pebble.Batch, tenant, shard, field uint64, ra *roaring.Bitmap) error {
	w.key.SetExistence(tenant, shard, field, 0)
	return w.write(ba, ra)
}

func (w *Batch) write(ba *pebble.Batch, ra *roaring.Bitmap) error {
	iter, _ := ra.Containers.Iterator(0)
	for iter.Next() {
		key, co := iter.Value()
		// only update container chunk. The rest of the key will not change during iteration.
		// It is assumed WriteData or WriteExistence is called before calling write.
		w.key.SetContainer(key)
		encoded := co.Encode()
		err := ba.Merge(w.key.Bytes(), encoded, nil)
		if err != nil {
			return err
		}
	}
	return nil
}
