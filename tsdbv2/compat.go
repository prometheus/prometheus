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
	"context"
	"errors"
	"math"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdbv2/backup"
	"github.com/prometheus/prometheus/tsdbv2/drop"
	ql "github.com/prometheus/prometheus/tsdbv2/query/labels"
)

func (db *DB) ForceHeadMMap()          {}
func (db *DB) DisableCompactions()     {}
func (db *DB) EnableNativeHistograms() {}
func (db *DB) Dir() string             { return db.dbPath }

func (db *DB) Compact(ctx context.Context) error {
	lo := []byte{0}
	hi := []byte{math.MaxUint8}
	return db.db.Compact(lo, hi, true)
}

func (db *DB) Snapshot(dir string, _ bool) error {
	return backup.Backup(dir, db.dbPath, db.config.pebble.FS)
}

func (db *DB) Delete(ctx context.Context, mint, maxt int64, ms ...*labels.Matcher) error {
	tenantID, matchers, ok := ql.ParseTenantID(db.db, db, ms...)
	if !ok {
		return errors.New("tsdbv2: missing tenant id")
	}
	return drop.Matching(db.db, tenantID, mint, maxt, matchers)
}
