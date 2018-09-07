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
	return s, nil
}

// Add implements storage.Appender.
func (s *Storage) Add(l labels.Labels, t int64, v float64) (uint64, error) {
	s.samplesIn.incr(1)
	s.samplesInMetric.Inc()
	if t > s.highestTimestamp {
		s.highestTimestamp = t
	}
	return 0, nil
}

// AddFast implements storage.Appender.
func (s *Storage) AddFast(l labels.Labels, _ uint64, t int64, v float64) error {
	_, err := s.Add(l, t, v)
	return err
}

// Commit implements storage.Appender.
func (s *Storage) Commit() error {
	s.highestTimestampMetric.Set(float64(s.highestTimestamp))
	return nil
}

// Rollback implements storage.Appender.
func (*Storage) Rollback() error {
	return nil
}
