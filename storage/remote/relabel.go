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
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/storage"
)

// relabelLabels applies cfgs to l and reports whether the series should be
// kept. An invalid result (e.g. missing __name__) is also treated as dropped.
func relabelLabels(l labels.Labels, cfgs []*relabel.Config) (labels.Labels, bool) {
	if len(cfgs) == 0 {
		return l, true
	}
	lb := labels.NewBuilder(l)
	if !relabel.ProcessBuilder(lb, cfgs...) {
		return labels.EmptyLabels(), false
	}
	result := lb.Labels()
	if !result.Has(labels.MetricName) || !result.IsValid(model.UTF8Validation) {
		return labels.EmptyLabels(), false
	}
	return result, true
}

// relabelCacheMaxEntries bounds RelabelCache size; overflow clears it entirely.
const relabelCacheMaxEntries = 100_000

// RelabelCache memoizes receive-path relabeling decisions. Safe for
// concurrent use and for sharing between a v1 and v2 relabeling appendable.
type RelabelCache struct {
	mu sync.RWMutex

	entries map[uint64]relabelCacheEntry
	// cfgsIdent: reload always allocates new *relabel.Config values, so a
	// mismatch here means the rules may have changed.
	cfgsIdent *relabel.Config
}

type relabelCacheEntry struct {
	orig   labels.Labels // verifies against Hash() collisions.
	result labels.Labels
	keep   bool
}

// NewRelabelCache returns an empty RelabelCache.
func NewRelabelCache() *RelabelCache {
	return &RelabelCache{}
}

// relabel is relabelLabels, memoized in c.
func (c *RelabelCache) relabel(l labels.Labels, cfgs []*relabel.Config) (labels.Labels, bool) {
	if len(cfgs) == 0 {
		return l, true
	}

	ident := cfgs[0]
	h := l.Hash()

	c.mu.RLock()
	if c.cfgsIdent == ident {
		if e, ok := c.entries[h]; ok && labels.Equal(e.orig, l) {
			c.mu.RUnlock()
			return e.result, e.keep
		}
	}
	c.mu.RUnlock()

	result, keep := relabelLabels(l, cfgs)

	c.mu.Lock()
	if c.cfgsIdent != ident || len(c.entries) >= relabelCacheMaxEntries {
		c.entries = make(map[uint64]relabelCacheEntry)
		c.cfgsIdent = ident
	}
	c.entries[h] = relabelCacheEntry{orig: l, result: result, keep: keep}
	c.mu.Unlock()

	return result, keep
}

// NewRelabelingAppendable wraps next to apply Config.ReceiveRelabelConfigs
// before samples reach storage, dropping series like metric_relabel_configs
// does at scrape time. Does not implement storage.GetRef, since relabeling
// can change a series' labels.
func NewRelabelingAppendable(next storage.Appendable, configFunc func() config.Config, cache *RelabelCache) storage.Appendable {
	return &relabelingAppendable{next: next, configFunc: configFunc, cache: cache}
}

type relabelingAppendable struct {
	next       storage.Appendable
	configFunc func() config.Config
	cache      *RelabelCache
}

func (a *relabelingAppendable) Appender(ctx context.Context) storage.Appender {
	return &relabelingAppender{
		Appender: a.next.Appender(ctx),
		configs:  a.configFunc().ReceiveRelabelConfigs,
		cache:    a.cache,
	}
}

type relabelingAppender struct {
	storage.Appender

	configs []*relabel.Config
	cache   *RelabelCache
}

func (a *relabelingAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.Append(ref, nl, t, v)
}

func (a *relabelingAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendExemplar(ref, nl, e)
}

func (a *relabelingAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendHistogram(ref, nl, t, h, fh)
}

func (a *relabelingAppender) AppendHistogramSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendHistogramSTZeroSample(ref, nl, t, st, h, fh)
}

func (a *relabelingAppender) AppendSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendSTZeroSample(ref, nl, t, st)
}

func (a *relabelingAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs)
	if !keep {
		return ref, nil
	}
	return a.Appender.UpdateMetadata(ref, nl, m)
}

// NewRelabelingAppendableV2 is the AppenderV2 equivalent of NewRelabelingAppendable.
// See NewRelabelingAppendable for the semantics.
func NewRelabelingAppendableV2(next storage.AppendableV2, configFunc func() config.Config, cache *RelabelCache) storage.AppendableV2 {
	return &relabelingAppendableV2{next: next, configFunc: configFunc, cache: cache}
}

type relabelingAppendableV2 struct {
	next       storage.AppendableV2
	configFunc func() config.Config
	cache      *RelabelCache
}

func (a *relabelingAppendableV2) AppenderV2(ctx context.Context) storage.AppenderV2 {
	return &relabelingAppenderV2{
		AppenderV2: a.next.AppenderV2(ctx),
		configs:    a.configFunc().ReceiveRelabelConfigs,
		cache:      a.cache,
	}
}

type relabelingAppenderV2 struct {
	storage.AppenderV2

	configs []*relabel.Config
	cache   *RelabelCache
}

func (a *relabelingAppenderV2) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(ls, a.configs)
	if !keep {
		return ref, nil
	}
	return a.AppenderV2.Append(ref, nl, st, t, v, h, fh, opts)
}
