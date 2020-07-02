// Copyright 2020 The Prometheus Authors
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

package v1

import (
	"context"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/storage"
)

// metadataProvider is used by the apiv1 and the langServer to retrieve metrics, labels ...etc.
type metadataProvider struct {
	queryable       storage.Queryable
	targetRetriever func(context.Context) TargetRetriever
}

func (w *metadataProvider) metadata(ctx context.Context, metric string, limit int) map[string][]metadata {
	metrics := map[string]map[metadata]struct{}{}
	for _, tt := range w.targetRetriever(ctx).TargetsActive() {
		for _, t := range tt {
			if len(metric) == 0 {
				for _, mm := range t.MetadataList() {
					m := metadata{Type: mm.Type, Help: mm.Help, Unit: mm.Unit}
					ms, ok := metrics[mm.Metric]

					if !ok {
						ms = map[metadata]struct{}{}
						metrics[mm.Metric] = ms
					}
					ms[m] = struct{}{}
				}
				continue
			}

			if md, ok := t.Metadata(metric); ok {
				m := metadata{Type: md.Type, Help: md.Help, Unit: md.Unit}
				ms, ok := metrics[md.Metric]

				if !ok {
					ms = map[metadata]struct{}{}
					metrics[md.Metric] = ms
				}
				ms[m] = struct{}{}
			}
		}
	}

	// Put the elements from the pseudo-set into a slice for marshaling.
	res := map[string][]metadata{}

	for name, set := range metrics {
		if limit >= 0 && len(res) >= limit {
			break
		}

		var s []metadata
		for metadata := range set {
			s = append(s, metadata)
		}
		res[name] = s
	}
	return res
}

func (w *metadataProvider) labelNames(ctx context.Context, start time.Time, end time.Time) ([]string, storage.Warnings, error) {
	q, err := w.queryable.Querier(ctx, timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return nil, nil, err
	}
	defer q.Close()

	return q.LabelNames()
}

func (w *metadataProvider) labelValues(ctx context.Context, labelName string, start time.Time, end time.Time) ([]string, storage.Warnings, error) {
	q, err := w.queryable.Querier(ctx, timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return nil, nil, err
	}
	defer q.Close()

	return q.LabelValues(labelName)
}

func (w *metadataProvider) series(ctx context.Context, matcherSets [][]*labels.Matcher, start time.Time, end time.Time) ([]labels.Labels, storage.Warnings, error) {
	q, err := w.queryable.Querier(ctx, timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return nil, nil, err
	}
	defer q.Close()

	var sets []storage.SeriesSet
	for _, mset := range matcherSets {
		s := q.Select(false, nil, mset...)
		sets = append(sets, s)
	}

	set := storage.NewMergeSeriesSet(sets, storage.ChainedSeriesMerge)
	metrics := []labels.Labels{}
	for set.Next() {
		metrics = append(metrics, set.At().Labels())
	}
	if set.Err() != nil {
		return nil, set.Warnings(), set.Err()
	}

	return metrics, set.Warnings(), nil
}
