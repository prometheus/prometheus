// Copyright The Prometheus Authors
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
	"context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/storage"
)

// relabel applies cfgs to l and reports whether the series should be kept.
// It always starts from l, never from a previously relabeled result, so
// calling it multiple times for the same original series (e.g. once for a
// sample and once for an exemplar) yields consistent keep/drop decisions
// without needing any shared state between calls.
func relabelLabels(l labels.Labels, cfgs []*relabel.Config) (labels.Labels, bool) {
	if len(cfgs) == 0 {
		return l, true
	}
	lb := labels.NewBuilder(l)
	if !relabel.ProcessBuilder(lb, cfgs...) {
		return labels.EmptyLabels(), false
	}
	return lb.Labels(), true
}

// NewRelabelingAppendable wraps next so that every series appended through it
// is passed through the relabel_config rules configured in
// Config.ReceiveRelabelConfigs before reaching storage. Series dropped by
// relabeling are silently discarded, the same as scrape-time
// metric_relabel_configs.
//
// configFunc is called on every Appender call, so relabel_config changes take
// effect on config reload without a restart.
//
// NOTE: the returned Appender does not implement storage.GetRef. Since
// relabeling can change a series' labels, the ref/hash passthrough GetRef
// exists for would be semantically wrong while relabeling is active.
func NewRelabelingAppendable(next storage.Appendable, configFunc func() config.Config) storage.Appendable {
	return &relabelingAppendable{next: next, configFunc: configFunc}
}

type relabelingAppendable struct {
	next       storage.Appendable
	configFunc func() config.Config
}

func (a *relabelingAppendable) Appender(ctx context.Context) storage.Appender {
	return &relabelingAppender{
		Appender: a.next.Appender(ctx),
		configs:  a.configFunc().ReceiveRelabelConfigs,
	}
}

type relabelingAppender struct {
	storage.Appender

	configs []*relabel.Config
}

func (a *relabelingAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	nl, keep := relabelLabels(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.Append(ref, nl, t, v)
}

func (a *relabelingAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	nl, keep := relabelLabels(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendExemplar(ref, nl, e)
}

func (a *relabelingAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	nl, keep := relabelLabels(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendHistogram(ref, nl, t, h, fh)
}

func (a *relabelingAppender) AppendHistogramSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	nl, keep := relabelLabels(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendHistogramSTZeroSample(ref, nl, t, st, h, fh)
}

func (a *relabelingAppender) AppendSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64) (storage.SeriesRef, error) {
	nl, keep := relabelLabels(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendSTZeroSample(ref, nl, t, st)
}

func (a *relabelingAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	nl, keep := relabelLabels(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.UpdateMetadata(ref, nl, m)
}

// NewRelabelingAppendableV2 is the AppenderV2 equivalent of NewRelabelingAppendable.
// See NewRelabelingAppendable for the semantics.
func NewRelabelingAppendableV2(next storage.AppendableV2, configFunc func() config.Config) storage.AppendableV2 {
	return &relabelingAppendableV2{next: next, configFunc: configFunc}
}

type relabelingAppendableV2 struct {
	next       storage.AppendableV2
	configFunc func() config.Config
}

func (a *relabelingAppendableV2) AppenderV2(ctx context.Context) storage.AppenderV2 {
	return &relabelingAppenderV2{
		AppenderV2: a.next.AppenderV2(ctx),
		configs:    a.configFunc().ReceiveRelabelConfigs,
	}
}

type relabelingAppenderV2 struct {
	storage.AppenderV2

	configs []*relabel.Config
}

func (a *relabelingAppenderV2) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	nl, keep := relabelLabels(ls, a.configs)
	if !keep {
		return ref, nil
	}
	return a.AppenderV2.Append(ref, nl, st, t, v, h, fh, opts)
}
