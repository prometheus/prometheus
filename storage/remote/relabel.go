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
	"reflect"
	"slices"
	"sync"

	"github.com/prometheus/common/model"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/storage"
)

// relabelLabels applies cfgs to l and reports whether the series should be
// kept. A result missing __name__, or invalid under validationScheme, is
// also treated as dropped.
func relabelLabels(l labels.Labels, cfgs []*relabel.Config, validationScheme model.ValidationScheme) (labels.Labels, bool) {
	if len(cfgs) == 0 {
		return l, true
	}
	lb := labels.NewBuilder(l)
	if !relabel.ProcessBuilder(lb, cfgs...) {
		return labels.EmptyLabels(), false
	}
	result := lb.Labels()
	if !result.Has(labels.MetricName) || !result.IsValid(validationScheme) {
		return labels.EmptyLabels(), false
	}
	return result, true
}

// relabelCacheLowWatermark is the eviction target on overflow. The gap below
// relabelCacheMaxEntries gives touched marks room to accumulate across many
// puts before the next sweep judges them.
const (
	relabelCacheMaxEntries   = 100_000
	relabelCacheLowWatermark = 90_000
)

// RelabelCache memoizes receive-path relabeling decisions. Safe for
// concurrent use and for sharing between a v1 and v2 relabeling appendable.
type RelabelCache struct {
	mu sync.RWMutex

	entries map[uint64]*relabelCacheEntry
	// cfgs is the last-seen rule set.
	cfgs []*relabel.Config
}

type relabelCacheEntry struct {
	orig   labels.Labels // verifies against Hash() collisions.
	result labels.Labels
	keep   bool
	// touched marks the entry as used since the last sweep; sweep evicts
	// only entries left unmarked, so an actively reused entry survives
	// overflow instead of being wiped along with unrelated churn.
	touched atomic.Bool
}

// NewRelabelCache returns an empty RelabelCache.
func NewRelabelCache() *RelabelCache {
	return &RelabelCache{}
}

func (c *RelabelCache) relabel(l labels.Labels, cfgs []*relabel.Config, validationScheme model.ValidationScheme) (labels.Labels, bool) {
	if len(cfgs) == 0 {
		c.clear()
		return l, true
	}

	h := l.Hash()

	if result, keep, ok := c.get(h, l, cfgs); ok {
		return result, keep
	}

	result, keep := relabelLabels(l, cfgs, validationScheme)
	c.put(h, l, result, keep, cfgs)

	return result, keep
}

func (c *RelabelCache) get(h uint64, l labels.Labels, cfgs []*relabel.Config) (result labels.Labels, keep, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// An entry only reflects the rules it was computed under; serving it
	// for a different cfgs would risk returning a stale relabeling decision.
	if !slices.Equal(c.cfgs, cfgs) {
		return labels.EmptyLabels(), false, false
	}
	e, found := c.entries[h]
	if !found || !labels.Equal(e.orig, l) {
		return labels.EmptyLabels(), false, false
	}
	e.touched.Store(true)
	return e.result, e.keep, true
}

func (c *RelabelCache) put(h uint64, orig, result labels.Labels, keep bool, cfgs []*relabel.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !slices.Equal(c.cfgs, cfgs) {
		// A reload always allocates new *relabel.Config values even when
		// the rules are textually unchanged; fall back to a content
		// comparison so entries are dropped only on a genuine rule change.
		if !reflect.DeepEqual(c.cfgs, cfgs) {
			c.entries = make(map[uint64]*relabelCacheEntry)
		}
		c.cfgs = cfgs
	}

	if len(c.entries) >= relabelCacheMaxEntries {
		c.sweep()
		// Evict arbitrary entries down to the low watermark instead of
		// wiping the map, so a stampede doesn't force every hot entry to
		// recompute.
		for evict := range c.entries {
			if len(c.entries) <= relabelCacheLowWatermark {
				break
			}
			delete(c.entries, evict)
		}
	}
	// touched starts false: an entry only counts as "used" once something
	// looks it up again after this insert, so a sweep can tell a reused
	// entry apart from a one-off it never sees twice.
	c.entries[h] = &relabelCacheEntry{orig: orig, result: result, keep: keep}
}

// clear drops all entries and resets cfgs.
func (c *RelabelCache) clear() {
	if c.empty() {
		return
	}
	c.mu.Lock()
	c.entries = nil
	c.cfgs = nil
	c.mu.Unlock()
}

func (c *RelabelCache) empty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfgs == nil
}

// sweep deletes entries not touched since the previous sweep and clears the
// mark on survivors. Called with c.mu held.
func (c *RelabelCache) sweep() {
	for h, e := range c.entries {
		if !e.touched.Swap(false) {
			delete(c.entries, h)
		}
	}
}

// NewRelabelingAppendable applies Config.ReceiveRelabelConfigs to samples
// before they reach next. Embedding storage.Appender does not promote
// storage.GetRef, so a ref lookup can't bypass relabeling.
func NewRelabelingAppendable(next storage.Appendable, configFunc func() config.Config, cache *RelabelCache) storage.Appendable {
	return &relabelingAppendable{next: next, configFunc: configFunc, cache: cache}
}

type relabelingAppendable struct {
	next       storage.Appendable
	configFunc func() config.Config
	cache      *RelabelCache
}

func (a *relabelingAppendable) Appender(ctx context.Context) storage.Appender {
	cfg := a.configFunc()
	return &relabelingAppender{
		Appender:         a.next.Appender(ctx),
		configs:          cfg.ReceiveRelabelConfigs,
		validationScheme: cfg.GlobalConfig.MetricNameValidationScheme,
		cache:            a.cache,
	}
}

type relabelingAppender struct {
	storage.Appender

	configs          []*relabel.Config
	validationScheme model.ValidationScheme
	cache            *RelabelCache
}

func (a *relabelingAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs, a.validationScheme)
	if !keep {
		return ref, nil
	}
	return a.Appender.Append(ref, nl, t, v)
}

func (a *relabelingAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs, a.validationScheme)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendExemplar(ref, nl, e)
}

func (a *relabelingAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs, a.validationScheme)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendHistogram(ref, nl, t, h, fh)
}

func (a *relabelingAppender) AppendHistogramSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs, a.validationScheme)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendHistogramSTZeroSample(ref, nl, t, st, h, fh)
}

func (a *relabelingAppender) AppendSTZeroSample(ref storage.SeriesRef, l labels.Labels, t, st int64) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs, a.validationScheme)
	if !keep {
		return ref, nil
	}
	return a.Appender.AppendSTZeroSample(ref, nl, t, st)
}

func (a *relabelingAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(l, a.configs, a.validationScheme)
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
	cfg := a.configFunc()
	return &relabelingAppenderV2{
		AppenderV2:       a.next.AppenderV2(ctx),
		configs:          cfg.ReceiveRelabelConfigs,
		validationScheme: cfg.GlobalConfig.MetricNameValidationScheme,
		cache:            a.cache,
	}
}

type relabelingAppenderV2 struct {
	storage.AppenderV2

	configs          []*relabel.Config
	validationScheme model.ValidationScheme
	cache            *RelabelCache
}

func (a *relabelingAppenderV2) Append(ref storage.SeriesRef, ls labels.Labels, st, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, opts storage.AOptions) (storage.SeriesRef, error) {
	nl, keep := a.cache.relabel(ls, a.configs, a.validationScheme)
	if !keep {
		return ref, nil
	}
	if nl.Get(labels.MetricName) != ls.Get(labels.MetricName) {
		// opts.MetricFamilyName was derived from the pre-relabel name.
		opts.MetricFamilyName = ""
	}
	return a.AppenderV2.Append(ref, nl, st, t, v, h, fh, opts)
}
