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

package labels

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/cockroachdb/pebble"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/fields"
	"github.com/prometheus/prometheus/tsdbv2/hash"
	"github.com/prometheus/prometheus/tsdbv2/tenants"
	"github.com/prometheus/prometheus/util/annotations"
)

func LabelNames(ctx context.Context, db *pebble.DB, source tenants.Sounce, hints *storage.LabelHints, matchers ...*labels.Matcher) (result []string, _ annotations.Annotations, err error) {
	tenantID, matchers, ok := ParseTenantID(db, source, matchers...)
	if !ok {
		return
	}
	it, err := db.NewIter(nil)
	if err != nil {
		return nil, nil, err
	}
	defer it.Close()
	limit := int(math.MaxInt32)
	if hints != nil && hints.Limit > 0 {
		limit = hints.Limit
	}
	Search(it, tenantID, matchers, WithLimit(limit, func(key, value []byte, kind labels.MatchType) bool {
		chunk := key[encoding.LabelPrefixOfffset:]
		name, _, ok := bytes.Cut(chunk, encoding.SepBytes)
		if !ok {
			return false
		}
		result = append(result, string(name))
		return true
	}))
	return
}

func LabelValues(ctx context.Context, db *pebble.DB, source tenants.Sounce, name string, hints *storage.LabelHints, matchers ...*labels.Matcher) (result []string, _ annotations.Annotations, err error) {
	tenantID, matchers, ok := ParseTenantID(db, source, matchers...)
	if !ok {
		return
	}
	it, err := db.NewIter(nil)
	if err != nil {
		return nil, nil, err
	}
	defer it.Close()
	hasMatchers := len(matchers) > 0
	limit := int(math.MaxInt32)
	if hints != nil && hints.Limit > 0 {
		limit = hints.Limit
	}
	names(it, tenantID, name, func(key, _ []byte) bool {
		_, value, ok := bytes.Cut(key, encoding.SepBytes)
		if !ok {
			return false
		}
		if hasMatchers {
			sl := unsafe.String(unsafe.SliceData(value), len(value))
			for i := range matchers {
				if !matchers[i].Matches(sl) {
					return true
				}
			}
		}
		result = append(result, string(value))
		limit--
		return limit > 0
	})
	return
}

func WithLimit(limit int, m SearchFunc) SearchFunc {
	return func(key, value []byte, typ labels.MatchType) bool {
		ok := m(key, value, typ)
		if ok {
			limit--
		}
		return limit > 0
	}
}

type SearchFunc func(key, value []byte, kind labels.MatchType) bool

func Search(iter *pebble.Iterator, tenantID uint64, matchers []*labels.Matcher, f SearchFunc) {
	if len(matchers) == 0 {
		global(iter, tenantID, func(key, value []byte) bool {
			return f(key, value, labels.MatchEqual)
		})
		return
	}
	for i := range matchers {
		m := matchers[i]
		switch m.Type {
		case labels.MatchEqual, labels.MatchNotEqual:
			exact(iter, tenantID, m.Name, m.Value, func(key, value []byte) bool {
				return f(key, value, m.Type)
			})
		case labels.MatchRegexp, labels.MatchNotRegexp:
			names(iter, tenantID, m.Name, func(key, value []byte) bool {
				_, vb, ok := bytes.Cut(key, encoding.SepBytes)
				if !ok {
					return false
				}
				sl := unsafe.String(unsafe.SliceData(vb), len(vb))
				if m.Matches(sl) {
					return f(key, value, m.Type)
				}
				return true
			})
		}
	}
}

func Match(iter *pebble.Iterator, tenantID uint64, m *labels.Matcher, f func(key, value []byte) bool) {
	switch m.Type {
	case labels.MatchEqual, labels.MatchNotEqual:
		exact(iter, tenantID, m.Name, m.Value, func(key, value []byte) bool {
			return f(key, value)
		})
	case labels.MatchRegexp, labels.MatchNotRegexp:
		names(iter, tenantID, m.Name, func(key, value []byte) bool {
			_, vb, ok := bytes.Cut(key, encoding.SepBytes)
			if !ok {
				return false
			}
			sl := unsafe.String(unsafe.SliceData(vb), len(vb))
			if m.Matches(sl) {
				return f(key, value)
			}
			return true
		})
	}
}

func exact(iter *pebble.Iterator, tenantID uint64, name, value string, f func(key, value []byte) bool) {
	key := encoding.MakeLabelTranslationKey(tenantID, &labels.Label{
		Name: name, Value: value,
	})

	if iter.SeekGE(key) && iter.Valid() && bytes.Equal(iter.Key(), key) {
		_ = f(iter.Key(), iter.Value())
	}
}

func names(iter *pebble.Iterator, tenantID uint64, name string, f func(key, value []byte) bool) {
	prefix := encoding.MakeLabelTranslationKey(tenantID, &labels.Label{
		Name: name,
	})
	walk(iter, prefix, f)
}

func global(iter *pebble.Iterator, tenantID uint64, f func(key, value []byte) bool) {
	prefix := encoding.MakeLabelTranslationKeyPrefix(tenantID)
	walk(iter, prefix, f)
}

func walk(iter *pebble.Iterator, prefix []byte, f func(key, value []byte) bool) {
	for iter.SeekGE(prefix); iter.Valid() && bytes.HasPrefix(iter.Key(), prefix); iter.Next() {
		if !f(iter.Key(), iter.Value()) {
			return
		}
	}
}

func ParseTenantID(db *pebble.DB, source tenants.Sounce, match ...*labels.Matcher) (uint64, []*labels.Matcher, bool) {
	for i := range match {
		m := match[i]
		if m.Type == labels.MatchEqual && m.Name == encoding.TenantID {
			key := encoding.MakeBlobTranslationKey(encoding.RootTenant, fields.Tenant.Hash(), hash.String(m.Value))
			value, done, err := db.Get(key)
			if err != nil {
				return 0, nil, false
			}
			id := binary.BigEndian.Uint64(value)
			done.Close()
			return id, append(match[:i:i], match[i+1:]...), true
		}
	}
	if source.DefaultToRootTenant() {
		return encoding.RootTenant, match, true
	}
	return 0, nil, false
}

func ParseTenantIDFromMany(db *pebble.DB, source tenants.Sounce, matchers ...[]*labels.Matcher) (uint64, [][]*labels.Matcher, bool) {
	for _, match := range matchers {
		for i := range match {
			m := match[i]
			if m.Type == labels.MatchEqual && m.Name == encoding.TenantID {
				key := encoding.MakeBlobTranslationKey(encoding.RootTenant, fields.Tenant.Hash(), hash.String(m.Value))
				value, done, err := db.Get(key)
				if err != nil {
					return 0, nil, false
				}
				id := binary.BigEndian.Uint64(value)
				done.Close()
				matchers[i] = append(match[:i:i], match[i+1:]...)
				return id, matchers, true
			}
		}
	}
	if source.DefaultToRootTenant() {
		return encoding.RootTenant, matchers, true
	}
	return 0, nil, false
}
