// Copyright 2013 The Prometheus Authors
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

package scrape

import (
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

type nopAppendable struct{}

func (a nopAppendable) Appender() (storage.Appender, error) {
	return nopAppender{}, nil
}

type nopAppender struct{}

func (a nopAppender) Add(labels.Labels, int64, float64) (uint64, error)   { return 0, nil }
func (a nopAppender) AddFast(labels.Labels, uint64, int64, float64) error { return nil }
func (a nopAppender) Commit() error                                       { return nil }
func (a nopAppender) Rollback() error                                     { return nil }

type sample struct {
	metric labels.Labels
	t      int64
	v      float64
}

// collectResultAppender records all samples that were added through the appender.
// It can be used as its zero value or be backed by another appender it writes samples through.
type collectResultAppender struct {
	next   storage.Appender
	result []sample
}

func (a *collectResultAppender) AddFast(m labels.Labels, ref uint64, t int64, v float64) error {
	if a.next == nil {
		return storage.ErrNotFound
	}
	err := a.next.AddFast(m, ref, t, v)
	if err != nil {
		return err
	}
	a.result = append(a.result, sample{
		metric: m,
		t:      t,
		v:      v,
	})
	return err
}

func (a *collectResultAppender) Add(m labels.Labels, t int64, v float64) (uint64, error) {
	a.result = append(a.result, sample{
		metric: m,
		t:      t,
		v:      v,
	})
	if a.next == nil {
		return 0, nil
	}
	return a.next.Add(m, t, v)
}

func (a *collectResultAppender) Commit() error   { return nil }
func (a *collectResultAppender) Rollback() error { return nil }
