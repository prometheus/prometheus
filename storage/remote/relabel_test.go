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
	"strconv"
	"sync"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

var (
	relabelTestDropConfig = []*relabel.Config{{
		SourceLabels:         model.LabelNames{"__name__"},
		Regex:                relabel.MustNewRegexp("drop_me"),
		Action:               relabel.Drop,
		NameValidationScheme: model.UTF8Validation,
	}}
	relabelTestRewriteConfig = []*relabel.Config{{
		SourceLabels:         model.LabelNames{"env"},
		Regex:                relabel.MustNewRegexp("(.*)"),
		TargetLabel:          "environment",
		Replacement:          "$1",
		Action:               relabel.Replace,
		NameValidationScheme: model.UTF8Validation,
	}}
	relabelTestStripNameConfig = []*relabel.Config{{
		Regex:                relabel.MustNewRegexp("__name__"),
		Action:               relabel.LabelDrop,
		NameValidationScheme: model.UTF8Validation,
	}}
	// relabelTestLegacyDottedNameConfig rewrites __name__ to a value containing
	// a dot, valid under UTF-8 validation but not under legacy validation.
	relabelTestLegacyDottedNameConfig = []*relabel.Config{{
		SourceLabels:         model.LabelNames{"__name__"},
		Regex:                relabel.MustNewRegexp("(.*)"),
		TargetLabel:          "__name__",
		Replacement:          "${1}.suffix",
		Action:               relabel.Replace,
		NameValidationScheme: model.LegacyValidation,
	}}
)

func relabelTestConfigFunc(cfgs []*relabel.Config, validationScheme model.ValidationScheme) func() config.Config {
	return func() config.Config {
		return config.Config{
			GlobalConfig:          config.GlobalConfig{MetricNameValidationScheme: validationScheme},
			ReceiveRelabelConfigs: cfgs,
		}
	}
}

func TestNewRelabelingAppendable(t *testing.T) {
	keepLabels := labels.FromStrings("__name__", "keep_me", "env", "prod")
	dropLabels := labels.FromStrings("__name__", "drop_me")
	relabeledLabels := labels.FromStrings("__name__", "keep_me", "env", "prod", "environment", "prod")

	for _, tc := range []struct {
		name        string
		configs     []*relabel.Config
		scheme      model.ValidationScheme
		in          labels.Labels
		wantDropped bool
		wantLabels  labels.Labels
	}{
		{name: "no configs, passthrough", configs: nil, scheme: model.UTF8Validation, in: keepLabels, wantLabels: keepLabels},
		{name: "kept and relabeled", configs: relabelTestRewriteConfig, scheme: model.UTF8Validation, in: keepLabels, wantLabels: relabeledLabels},
		{name: "dropped", configs: relabelTestDropConfig, scheme: model.UTF8Validation, in: dropLabels, wantDropped: true},
		{name: "dropped because result loses __name__", configs: relabelTestStripNameConfig, scheme: model.UTF8Validation, in: keepLabels, wantDropped: true},
		{name: "dropped because result is invalid under the configured legacy validation scheme", configs: relabelTestLegacyDottedNameConfig, scheme: model.LegacyValidation, in: keepLabels, wantDropped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appendable := teststorage.NewAppendable()
			wrapped := NewRelabelingAppendable(appendable, relabelTestConfigFunc(tc.configs, tc.scheme), NewRelabelCache())
			app := wrapped.Appender(context.Background())

			ref, err := app.Append(0, tc.in, 10, 1)
			require.NoError(t, err)
			_, err = app.AppendExemplar(ref, tc.in, exemplar.Exemplar{Value: 1, Ts: 10})
			require.NoError(t, err)
			_, err = app.UpdateMetadata(ref, tc.in, metadata.Metadata{Type: model.MetricTypeCounter})
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			results := appendable.ResultSamples()
			if tc.wantDropped {
				require.Empty(t, results)
				return
			}

			require.Len(t, results, 1)
			require.True(t, labels.Equal(tc.wantLabels, results[0].L), "got labels %v, want %v", results[0].L, tc.wantLabels)
			require.Len(t, results[0].ES, 1)
			require.Equal(t, model.MetricTypeCounter, results[0].M.Type)
		})
	}
}

func TestNewRelabelingAppendableV2(t *testing.T) {
	keepLabels := labels.FromStrings("__name__", "keep_me", "env", "prod")
	dropLabels := labels.FromStrings("__name__", "drop_me")
	relabeledLabels := labels.FromStrings("__name__", "keep_me", "env", "prod", "environment", "prod")

	for _, tc := range []struct {
		name        string
		configs     []*relabel.Config
		scheme      model.ValidationScheme
		in          labels.Labels
		wantDropped bool
		wantLabels  labels.Labels
	}{
		{name: "no configs, passthrough", configs: nil, scheme: model.UTF8Validation, in: keepLabels, wantLabels: keepLabels},
		{name: "kept and relabeled", configs: relabelTestRewriteConfig, scheme: model.UTF8Validation, in: keepLabels, wantLabels: relabeledLabels},
		{name: "dropped", configs: relabelTestDropConfig, scheme: model.UTF8Validation, in: dropLabels, wantDropped: true},
		{name: "dropped because result loses __name__", configs: relabelTestStripNameConfig, scheme: model.UTF8Validation, in: keepLabels, wantDropped: true},
		{name: "dropped because result is invalid under the configured legacy validation scheme", configs: relabelTestLegacyDottedNameConfig, scheme: model.LegacyValidation, in: keepLabels, wantDropped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appendable := teststorage.NewAppendable()
			wrapped := NewRelabelingAppendableV2(appendable, relabelTestConfigFunc(tc.configs, tc.scheme), NewRelabelCache())
			app := wrapped.AppenderV2(context.Background())

			_, err := app.Append(0, tc.in, 0, 10, 1, nil, nil, storage.AOptions{
				Metadata:  metadata.Metadata{Type: model.MetricTypeCounter},
				Exemplars: []exemplar.Exemplar{{Value: 1, Ts: 10}},
			})
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			results := appendable.ResultSamples()
			if tc.wantDropped {
				require.Empty(t, results)
				return
			}

			require.Len(t, results, 1)
			require.True(t, labels.Equal(tc.wantLabels, results[0].L), "got labels %v, want %v", results[0].L, tc.wantLabels)
			require.Len(t, results[0].ES, 1)
			require.Equal(t, model.MetricTypeCounter, results[0].M.Type)
		})
	}
}

func TestNewRelabelingAppendableV2_MetricFamilyName(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configs    []*relabel.Config
		in         labels.Labels
		wantFamily string
	}{
		{
			name:       "name unchanged, family name preserved",
			configs:    relabelTestRewriteConfig,
			in:         labels.FromStrings("__name__", "keep_me", "env", "prod"),
			wantFamily: "keep_me",
		},
		{
			name: "name changed, stale family name cleared",
			configs: []*relabel.Config{{
				Regex:                relabel.MustNewRegexp("keep_me"),
				SourceLabels:         model.LabelNames{"__name__"},
				TargetLabel:          "__name__",
				Replacement:          "renamed",
				Action:               relabel.Replace,
				NameValidationScheme: model.UTF8Validation,
			}},
			in:         labels.FromStrings("__name__", "keep_me", "env", "prod"),
			wantFamily: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appendable := teststorage.NewAppendable()
			wrapped := NewRelabelingAppendableV2(appendable, relabelTestConfigFunc(tc.configs, model.UTF8Validation), NewRelabelCache())
			app := wrapped.AppenderV2(context.Background())

			_, err := app.Append(0, tc.in, 0, 10, 1, nil, nil, storage.AOptions{MetricFamilyName: "keep_me"})
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			results := appendable.ResultSamples()
			require.Len(t, results, 1)
			require.Equal(t, tc.wantFamily, results[0].MF)
		})
	}
}

func TestRelabelCache(t *testing.T) {
	l := labels.FromStrings("__name__", "keep_me", "env", "prod")

	t.Run("caches and reuses result for the same config generation", func(t *testing.T) {
		cache := NewRelabelCache()
		result1, keep1 := cache.relabel(l, relabelTestRewriteConfig, model.UTF8Validation)
		require.True(t, keep1)

		cache.mu.RLock()
		entry, ok := cache.entries[l.Hash()]
		cache.mu.RUnlock()
		require.True(t, ok)
		require.True(t, labels.Equal(entry.orig, l))
		require.True(t, labels.Equal(entry.result, result1))

		result2, keep2 := cache.relabel(l, relabelTestRewriteConfig, model.UTF8Validation)
		require.Equal(t, keep1, keep2)
		require.True(t, labels.Equal(result1, result2))
	})

	t.Run("reload with unchanged rule content adopts the new identity without evicting unrelated entries", func(t *testing.T) {
		cache := NewRelabelCache()
		other := labels.FromStrings("__name__", "keep_me_2", "env", "prod")
		cache.relabel(l, relabelTestRewriteConfig, model.UTF8Validation)
		cache.relabel(other, relabelTestRewriteConfig, model.UTF8Validation)

		cache.mu.RLock()
		otherBefore := cache.entries[other.Hash()]
		cache.mu.RUnlock()

		reloaded := []*relabel.Config{{
			SourceLabels:         relabelTestRewriteConfig[0].SourceLabels,
			Regex:                relabelTestRewriteConfig[0].Regex,
			TargetLabel:          relabelTestRewriteConfig[0].TargetLabel,
			Replacement:          relabelTestRewriteConfig[0].Replacement,
			Action:               relabelTestRewriteConfig[0].Action,
			NameValidationScheme: relabelTestRewriteConfig[0].NameValidationScheme,
		}}
		cache.relabel(l, reloaded, model.UTF8Validation)

		cache.mu.RLock()
		otherAfter := cache.entries[other.Hash()]
		cfgs := cache.cfgs
		cache.mu.RUnlock()
		require.Same(t, otherBefore, otherAfter, "content-identical reload must not evict unrelated cached entries")
		require.Same(t, reloaded[0], cfgs[0])

		// Once the new identity is established, further lookups for l hit the cache.
		cache.mu.RLock()
		entryAfterReload := cache.entries[l.Hash()]
		cache.mu.RUnlock()
		cache.relabel(l, reloaded, model.UTF8Validation)
		cache.mu.RLock()
		entryStillSame := cache.entries[l.Hash()]
		cache.mu.RUnlock()
		require.Same(t, entryAfterReload, entryStillSame)
	})

	t.Run("reload with changed rule content wipes stale entries", func(t *testing.T) {
		cache := NewRelabelCache()
		result1, _ := cache.relabel(l, relabelTestRewriteConfig, model.UTF8Validation)
		require.True(t, result1.Has("environment"))
		require.Equal(t, "prod", result1.Get("environment"))

		changed := []*relabel.Config{{
			SourceLabels:         relabelTestRewriteConfig[0].SourceLabels,
			Regex:                relabelTestRewriteConfig[0].Regex,
			TargetLabel:          relabelTestRewriteConfig[0].TargetLabel,
			Replacement:          "changed-$1",
			Action:               relabelTestRewriteConfig[0].Action,
			NameValidationScheme: relabelTestRewriteConfig[0].NameValidationScheme,
		}}
		result2, _ := cache.relabel(l, changed, model.UTF8Validation)
		require.Equal(t, "changed-prod", result2.Get("environment"))
	})

	t.Run("clears on overflow instead of growing unbounded", func(t *testing.T) {
		cache := NewRelabelCache()
		const overflowBy = 10
		for i := range relabelCacheMaxEntries + overflowBy {
			cache.relabel(labels.FromStrings("__name__", "m", "i", strconv.Itoa(i)), relabelTestRewriteConfig, model.UTF8Validation)
		}

		cache.mu.RLock()
		size := len(cache.entries)
		cache.mu.RUnlock()
		require.Equal(t, overflowBy, size)
	})

	t.Run("sweep freeing nothing evicts arbitrary entries instead of wiping the cache", func(t *testing.T) {
		cache := NewRelabelCache()
		for i := range relabelCacheMaxEntries {
			cache.relabel(labels.FromStrings("__name__", "m", "i", strconv.Itoa(i)), relabelTestRewriteConfig, model.UTF8Validation)
		}
		// Touch every entry so sweep finds nothing untouched to free.
		for i := range relabelCacheMaxEntries {
			cache.relabel(labels.FromStrings("__name__", "m", "i", strconv.Itoa(i)), relabelTestRewriteConfig, model.UTF8Validation)
		}

		cache.relabel(labels.FromStrings("__name__", "m", "kind", "trigger"), relabelTestRewriteConfig, model.UTF8Validation)

		cache.mu.RLock()
		size := len(cache.entries)
		cache.mu.RUnlock()
		require.Equal(t, relabelCacheLowWatermark+1, size, "sweep freeing nothing must evict down to the low watermark, not wipe the whole cache")
	})

	t.Run("consecutive misses with nothing touched in between don't collapse the cache", func(t *testing.T) {
		cache := NewRelabelCache()
		for i := range relabelCacheMaxEntries {
			cache.relabel(labels.FromStrings("__name__", "m", "i", strconv.Itoa(i)), relabelTestRewriteConfig, model.UTF8Validation)
		}
		// Touch every entry so the first miss below finds nothing to free.
		for i := range relabelCacheMaxEntries {
			cache.relabel(labels.FromStrings("__name__", "m", "i", strconv.Itoa(i)), relabelTestRewriteConfig, model.UTF8Validation)
		}

		cache.relabel(labels.FromStrings("__name__", "m", "kind", "trigger1"), relabelTestRewriteConfig, model.UTF8Validation)
		cache.relabel(labels.FromStrings("__name__", "m", "kind", "trigger2"), relabelTestRewriteConfig, model.UTF8Validation)

		cache.mu.RLock()
		size := len(cache.entries)
		cache.mu.RUnlock()
		require.Equal(t, relabelCacheLowWatermark+2, size, "a second miss right after the first must not re-sweep an already-shrunk cache")
	})

	t.Run("an entry reused before overflow survives it, one that wasn't does not", func(t *testing.T) {
		cache := NewRelabelCache()
		hot := labels.FromStrings("__name__", "m", "kind", "hot")
		cold := labels.FromStrings("__name__", "m", "kind", "cold")
		cache.relabel(hot, relabelTestRewriteConfig, model.UTF8Validation)
		cache.relabel(cold, relabelTestRewriteConfig, model.UTF8Validation)
		for i := range relabelCacheMaxEntries - 2 {
			cache.relabel(labels.FromStrings("__name__", "m", "pad", strconv.Itoa(i)), relabelTestRewriteConfig, model.UTF8Validation)
		}
		cache.relabel(hot, relabelTestRewriteConfig, model.UTF8Validation) // reused since insertion.

		cache.relabel(labels.FromStrings("__name__", "m", "kind", "trigger"), relabelTestRewriteConfig, model.UTF8Validation) // overflows, sweeps.

		cache.mu.RLock()
		_, hotSurvived := cache.entries[hot.Hash()]
		_, coldSurvived := cache.entries[cold.Hash()]
		cache.mu.RUnlock()
		require.True(t, hotSurvived)
		require.False(t, coldSurvived)
	})

	t.Run("clears entries once reloaded to no configs", func(t *testing.T) {
		cache := NewRelabelCache()
		cache.relabel(l, relabelTestRewriteConfig, model.UTF8Validation)
		require.False(t, cache.empty())

		cache.relabel(l, nil, model.UTF8Validation)
		require.True(t, cache.empty())
	})
}

func TestRelabelCache_sweep(t *testing.T) {
	cache := NewRelabelCache()
	hot := &relabelCacheEntry{orig: labels.FromStrings("__name__", "hot")}
	hot.touched.Store(true)
	cold := &relabelCacheEntry{orig: labels.FromStrings("__name__", "cold")}
	cache.entries = map[uint64]*relabelCacheEntry{1: hot, 2: cold}

	cache.sweep()

	require.Equal(t, map[uint64]*relabelCacheEntry{1: hot}, cache.entries)
	require.False(t, hot.touched.Load(), "a survivor's mark is cleared, so it must be reused again before the next sweep")
}

// Run with -race.
func TestRelabelCache_ConcurrentAccess(t *testing.T) {
	cache := NewRelabelCache()
	const goroutines = 50
	const iterations = 200

	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			for i := range iterations {
				l := labels.FromStrings("__name__", "keep_me", "env", "prod", "shard", strconv.Itoa(i%5))
				result, keep := cache.relabel(l, relabelTestRewriteConfig, model.UTF8Validation)
				require.True(t, keep)
				require.True(t, result.Has("environment"))
			}
		})
	}
	wg.Wait()
}

// Run with -race. Concurrent callers pass two genuinely different config
// generations for the same series; a call for one generation must never
// observe a result computed under the other, even while both race to
// establish their generation in the shared cache.
func TestRelabelCache_ConcurrentReload(t *testing.T) {
	cache := NewRelabelCache()
	l := labels.FromStrings("__name__", "keep_me", "env", "prod")

	cfgsA := []*relabel.Config{{
		SourceLabels:         model.LabelNames{"env"},
		Regex:                relabel.MustNewRegexp("(.*)"),
		TargetLabel:          "environment",
		Replacement:          "A-$1",
		Action:               relabel.Replace,
		NameValidationScheme: model.UTF8Validation,
	}}
	cfgsB := []*relabel.Config{{
		SourceLabels:         model.LabelNames{"env"},
		Regex:                relabel.MustNewRegexp("(.*)"),
		TargetLabel:          "environment",
		Replacement:          "B-$1",
		Action:               relabel.Replace,
		NameValidationScheme: model.UTF8Validation,
	}}

	const goroutines = 20
	const iterations = 1000

	var wg sync.WaitGroup
	for i := range goroutines {
		cfgs, want := cfgsA, "A-prod"
		if i%2 == 1 {
			cfgs, want = cfgsB, "B-prod"
		}
		wg.Go(func() {
			for range iterations {
				result, keep := cache.relabel(l, cfgs, model.UTF8Validation)
				require.True(t, keep)
				require.Equal(t, want, result.Get("environment"))
			}
		})
	}
	wg.Wait()
}

func TestRelabelCache_SharedAcrossV1AndV2(t *testing.T) {
	cache := NewRelabelCache()
	configFunc := relabelTestConfigFunc(relabelTestRewriteConfig, model.UTF8Validation)

	v1 := NewRelabelingAppendable(teststorage.NewAppendable(), configFunc, cache)
	v2Appendable := teststorage.NewAppendable()
	v2 := NewRelabelingAppendableV2(v2Appendable, configFunc, cache)

	l := labels.FromStrings("__name__", "keep_me", "env", "prod")
	wantLabels := labels.FromStrings("__name__", "keep_me", "env", "prod", "environment", "prod")

	app1 := v1.Appender(context.Background())
	_, err := app1.Append(0, l, 10, 1)
	require.NoError(t, err)
	require.NoError(t, app1.Commit())

	cache.mu.RLock()
	_, ok := cache.entries[l.Hash()]
	cache.mu.RUnlock()
	require.True(t, ok, "expected the v1 (remote-write) append to populate the shared cache")

	app2 := v2.AppenderV2(context.Background())
	_, err = app2.Append(0, l, 0, 20, 2, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app2.Commit())

	results := v2Appendable.ResultSamples()
	require.Len(t, results, 1)
	require.True(t, labels.Equal(wantLabels, results[0].L), "got labels %v, want %v", results[0].L, wantLabels)
}
