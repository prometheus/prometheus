// Copyright 2017 The Prometheus Authors
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

package remote

import (
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

// Appender implements scrape.Appendable.
func (s *Storage) Appender() (storage.Appender, error) {
	return &timestampTracker{
		storage: s,
	}, nil
}

type timestampTracker struct {
	storage          *Storage
	samples          int64
	highestTimestamp int64
}

// Add implements storage.Appender.
func (t *timestampTracker) Add(_ labels.Labels, ts int64, v float64) (uint64, error) {
	t.samples++
	if ts > t.highestTimestamp {
		t.highestTimestamp = ts
	}
	return 0, nil
}

// AddFast implements storage.Appender.
func (t *timestampTracker) AddFast(l labels.Labels, _ uint64, ts int64, v float64) error {
	_, err := t.Add(l, ts, v)
	return err
}

// Commit implements storage.Appender.
func (t *timestampTracker) Commit() error {
	t.storage.samplesIn.incr(t.samples)
	t.storage.samplesInMetric.Add(float64(t.samples))

	t.storage.highestTimestampMtx.Lock()
	defer t.storage.highestTimestampMtx.Unlock()
	if t.highestTimestamp > t.storage.highestTimestamp {
		t.storage.highestTimestamp = t.highestTimestamp
		t.storage.highestTimestampMetric.Set(float64(t.highestTimestamp) / 1000.)
	}
	return nil
}

// Rollback implements storage.Appender.
func (*timestampTracker) Rollback() error {
	return nil
}
