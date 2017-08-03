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
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

func (s *Storage) Appender() (storage.Appender, error) {
	return s, nil
}

func (s *Storage) Add(l labels.Labels, t int64, v float64) (string, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	for _, q := range s.queues {
		q.Append(&model.Sample{
			Metric:    labelsToMetric(l),
			Timestamp: model.Time(t),
			Value:     model.SampleValue(v),
		})
	}
	return "", nil
}

func labelsToMetric(ls labels.Labels) model.Metric {
	metric := make(model.Metric, len(ls))
	for _, l := range ls {
		metric[model.LabelName(l.Name)] = model.LabelValue(l.Value)
	}
	return metric
}

func (s *Storage) AddFast(l labels.Labels, _ string, t int64, v float64) error {
	_, err := s.Add(l, t, v)
	return err
}

func (*Storage) Commit() error {
	return nil
}

func (*Storage) Rollback() error {
	return nil
}
