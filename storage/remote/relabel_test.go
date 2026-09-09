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
)

func relabelTestConfigFunc(cfgs []*relabel.Config) func() config.Config {
	return func() config.Config {
		return config.Config{ReceiveRelabelConfigs: cfgs}
	}
}

func TestNewRelabelingAppendable(t *testing.T) {
	keepLabels := labels.FromStrings("__name__", "keep_me", "env", "prod")
	dropLabels := labels.FromStrings("__name__", "drop_me")
	relabeledLabels := labels.FromStrings("__name__", "keep_me", "env", "prod", "environment", "prod")

	for _, tc := range []struct {
		name        string
		configs     []*relabel.Config
		in          labels.Labels
		wantDropped bool
		wantLabels  labels.Labels
	}{
		{name: "no configs, passthrough", configs: nil, in: keepLabels, wantLabels: keepLabels},
		{name: "kept and relabeled", configs: relabelTestRewriteConfig, in: keepLabels, wantLabels: relabeledLabels},
		{name: "dropped", configs: relabelTestDropConfig, in: dropLabels, wantDropped: true},
		{name: "dropped because result loses __name__", configs: relabelTestStripNameConfig, in: keepLabels, wantDropped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appendable := teststorage.NewAppendable()
			wrapped := NewRelabelingAppendable(appendable, relabelTestConfigFunc(tc.configs), NewRelabelCache())
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
		in          labels.Labels
		wantDropped bool
		wantLabels  labels.Labels
	}{
		{name: "no configs, passthrough", configs: nil, in: keepLabels, wantLabels: keepLabels},
		{name: "kept and relabeled", configs: relabelTestRewriteConfig, in: keepLabels, wantLabels: relabeledLabels},
		{name: "dropped", configs: relabelTestDropConfig, in: dropLabels, wantDropped: true},
		{name: "dropped because result loses __name__", configs: relabelTestStripNameConfig, in: keepLabels, wantDropped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appendable := teststorage.NewAppendable()
			wrapped := NewRelabelingAppendableV2(appendable, relabelTestConfigFunc(tc.configs), NewRelabelCache())
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

func TestRelabelCache(t *testing.T) {
	l := labels.FromStrings("__name__", "keep_me", "env", "prod")

	t.Run("caches and reuses result for the same config generation", func(t *testing.T) {
		cache := NewRelabelCache()
		result1, keep1 := cache.relabel(l, relabelTestRewriteConfig)
		require.True(t, keep1)

		cache.mu.RLock()
		entry, ok := cache.entries[l.Hash()]
		cache.mu.RUnlock()
		require.True(t, ok)
		require.True(t, labels.Equal(entry.orig, l))
		require.True(t, labels.Equal(entry.result, result1))

		result2, keep2 := cache.relabel(l, relabelTestRewriteConfig)
		require.Equal(t, keep1, keep2)
		require.True(t, labels.Equal(result1, result2))
	})

	t.Run("invalidates on a new config generation", func(t *testing.T) {
		cache := NewRelabelCache()
		result1, _ := cache.relabel(l, relabelTestRewriteConfig)

		reloaded := []*relabel.Config{{
			SourceLabels:         relabelTestRewriteConfig[0].SourceLabels,
			Regex:                relabelTestRewriteConfig[0].Regex,
			TargetLabel:          relabelTestRewriteConfig[0].TargetLabel,
			Replacement:          relabelTestRewriteConfig[0].Replacement,
			Action:               relabelTestRewriteConfig[0].Action,
			NameValidationScheme: relabelTestRewriteConfig[0].NameValidationScheme,
		}}
		result2, _ := cache.relabel(l, reloaded)
		require.True(t, labels.Equal(result1, result2))

		cache.mu.RLock()
		ident := cache.cfgsIdent
		cache.mu.RUnlock()
		require.Same(t, reloaded[0], ident)
	})

	t.Run("clears on overflow instead of growing unbounded", func(t *testing.T) {
		cache := NewRelabelCache()
		const overflowBy = 10
		for i := range relabelCacheMaxEntries + overflowBy {
			cache.relabel(labels.FromStrings("__name__", "m", "i", strconv.Itoa(i)), relabelTestRewriteConfig)
		}

		cache.mu.RLock()
		size := len(cache.entries)
		cache.mu.RUnlock()
		require.Equal(t, overflowBy, size)
	})

	t.Run("an entry reused before overflow survives it, one that wasn't does not", func(t *testing.T) {
		cache := NewRelabelCache()
		hot := labels.FromStrings("__name__", "m", "kind", "hot")
		cold := labels.FromStrings("__name__", "m", "kind", "cold")
		cache.relabel(hot, relabelTestRewriteConfig)
		cache.relabel(cold, relabelTestRewriteConfig)
		for i := range relabelCacheMaxEntries - 2 {
			cache.relabel(labels.FromStrings("__name__", "m", "pad", strconv.Itoa(i)), relabelTestRewriteConfig)
		}
		cache.relabel(hot, relabelTestRewriteConfig) // reused since insertion.

		cache.relabel(labels.FromStrings("__name__", "m", "kind", "trigger"), relabelTestRewriteConfig) // overflows, sweeps.

		cache.mu.RLock()
		_, hotSurvived := cache.entries[hot.Hash()]
		_, coldSurvived := cache.entries[cold.Hash()]
		cache.mu.RUnlock()
		require.True(t, hotSurvived)
		require.False(t, coldSurvived)
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
				result, keep := cache.relabel(l, relabelTestRewriteConfig)
				require.True(t, keep)
				require.True(t, result.Has("environment"))
			}
		})
	}
	wg.Wait()
}

func TestRelabelCache_SharedAcrossV1AndV2(t *testing.T) {
	cache := NewRelabelCache()
	configFunc := relabelTestConfigFunc(relabelTestRewriteConfig)

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
