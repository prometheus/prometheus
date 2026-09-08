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

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/storage"
)

// relabelLabels applies cfgs to l and reports whether the series should be kept.
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

// relabelCacheMaxEntries bounds RelabelCache memory use. On overflow the
// whole cache is cleared rather than partially evicted: a config reload
// already requires a full clear (the rule set changed), so reusing that
// same clear-all path for overflow avoids adding separate LRU/eviction
// bookkeeping for what should be a rare event in practice.
const relabelCacheMaxEntries = 100_000

// RelabelCache caches receive-path relabeling decisions across requests, so
// that a series whose labels recur across many remote-write or OTLP requests
// (the common case: a given sender keeps shipping the same series) doesn't
// pay the relabel_config evaluation cost on every single append.
//
// A relabel result depends only on the input labels and the configured
// rules, not on which write protocol produced it, so one RelabelCache can
// safely be shared between the v1 (remote-write) and v2 (OTLP) relabeling
// appendables. It is safe for concurrent use.
type RelabelCache struct {
	mu sync.RWMutex

	entries map[uint64]relabelCacheEntry
	// cfgsIdent identifies the []*relabel.Config generation the entries
	// below were computed from (its first element's pointer -- cfgs is
	// always non-empty here, since an empty rule set never reaches the
	// cache; see relabel()). Every config reload allocates brand-new
	// *relabel.Config values, even if the rules are textually unchanged, so
	// a pointer mismatch reliably signals "rules may have changed, clear
	// the cache".
	cfgsIdent *relabel.Config
}

type relabelCacheEntry struct {
	orig   labels.Labels // original input labels, to verify against Hash() collisions.
	result labels.Labels
	keep   bool
}

// NewRelabelCache returns an empty RelabelCache.
func NewRelabelCache() *RelabelCache {
	return &RelabelCache{}
}

// relabel is relabelLabels, but memoized in c for the lifetime of cfgs'
// current generation (see RelabelCache.cfgsIdent).
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

// NewRelabelingAppendable wraps next so that every series appended through it
// is passed through the relabel_config rules configured in
// Config.ReceiveRelabelConfigs before reaching storage. Series dropped by
// relabeling are silently discarded, the same as scrape-time
// metric_relabel_configs.
//
// configFunc is called on every Appender call, so relabel_config changes take
// effect on config reload without a restart. cache memoizes relabel results
// across calls and across Appender instances; pass the same *RelabelCache
// used by NewRelabelingAppendableV2, if both are constructed, so a series
// seen on either write path benefits from the other's cache entries.
//
// NOTE: the returned Appender does not implement storage.GetRef. Since
// relabeling can change a series' labels, the ref/hash passthrough GetRef
// exists for would be semantically wrong while relabeling is active.
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
