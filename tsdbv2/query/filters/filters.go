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

package filters

import (
	"encoding/binary"

	"github.com/cockroachdb/pebble"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdbv2/encoding/bitmaps"
	ql "github.com/prometheus/prometheus/tsdbv2/query/labels"
)

// Compile process macthers and returns a bitmap filter. The returned filter can
// be callen multiple times and is safe for conurrent use.
func Compile(db *pebble.DB, tenantID uint64, matchers []*labels.Matcher) (filter bitmaps.Filter) {
	iter, err := db.NewIter(nil)
	if err != nil {
		return
	}
	defer iter.Close()

	for i := range matchers {
		m := matchers[i]
		var values []uint64
		var field uint64
		ql.Match(iter, tenantID, m, func(key, value []byte) bool {
			if field == 0 {
				field = binary.BigEndian.Uint64(key[1+8:])
			}
			values = append(values, binary.BigEndian.Uint64(value))
			return true
		})
		if field == 0 {
			continue
		}
		switch m.Type {
		case labels.MatchEqual, labels.MatchRegexp:
			filter = &bitmaps.And{Left: filter, Right: &bitmaps.Yes{Field: field, Values: values}}
		case labels.MatchNotEqual, labels.MatchNotRegexp:
			filter = &bitmaps.And{Left: filter, Right: &bitmaps.No{Field: field, Values: values}}
		}
	}
	return
}
