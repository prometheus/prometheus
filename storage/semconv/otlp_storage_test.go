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

// This file contains AwareStorage tests that depend on the OTLP translation
// strategy fan-out, which lands together with this file. Tests that exercise
// the wrapper plumbing without needing OTLP variants live in storage_test.go.

package semconv_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage/semconv"
	"github.com/prometheus/prometheus/util/teststorage"
)

// TestAwareStorage_OTLP tests AwareStorage, covering the OTLP translation-strategy
// fan-out.
func TestAwareStorage_OTLP(t *testing.T) {
	t.Run("fans out and canonicalizes labels", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)

		// Append under the OTLP-translated (escaped + suffixed) name as a counter
		// would be stored after a UnderscoreEscapingWithSuffixes write. The
		// metric "test" in registry/1.1.0 is a counter with unit "By".
		appendSeries(t, wrapped, "test_bytes_total", 1, 7.0, "tenant", "alice")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "NoTranslation"),
		)

		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the escaped series to surface via OTLP fan-out")
		var foundCanonical bool
		for k := range got {
			if strings.Contains(k, `__name__="test"`) || strings.Contains(k, `__name__="test.counter"`) {
				foundCanonical = true
			}
		}
		require.True(t, foundCanonical, "expected canonical name in: %v", got)
	})

	t.Run("__schema_url__ surfaces the baseline version era", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)

		// A series exactly as a semconv 1.0.0 producer using
		// UnderscoreEscapingWithSuffixes would write the metric "test.counter" (a
		// counter with unit "By" in registry/1.0.0). The 1.1.0 anchor renamed
		// "test.counter" to "test", so this era is only reachable by walking the
		// schema's versions back to 1.0.0 via __schema_url__. The distinctive value
		// 418 ties the assertion to this specific era.
		appendSeries(t, wrapped, "test_counter_bytes_total", 1, 7.0, "http_response_status_code", "418")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "NoTranslation"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)

		// The baseline-version walk must load 1.0.0's metadata so the historical
		// "test.counter" variant escapes to "test_counter_bytes_total" and matches
		// the stored series. Without it that suffixed name is never probed and the
		// result is empty.
		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the semconv 1.0.0 era series to surface via the __schema_url__ walk")
		var found bool
		for k := range got {
			if strings.Contains(k, `__name__="test"`) && strings.Contains(k, "418") {
				found = true
			}
		}
		require.True(t, found, "expected canonical name with the 1.0.0-era value in: %v", got)
	})

	t.Run("LabelNames fans out and dedupes", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test_bytes_total", 1, 1.0, "tenant", "alice")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		names, _, err := q.LabelNames(context.Background(), nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "NoTranslation"),
		)
		require.NoError(t, err)
		require.Contains(t, names, "tenant")
		require.Contains(t, names, model.MetricNameLabel)
	})

	t.Run("ChunkQuerier fans out and canonicalizes labels", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test_bytes_total", 1, 3.5, "tenant", "alice")

		cq, err := wrapped.ChunkQuerier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = cq.Close() })

		set := cq.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "NoTranslation"),
		)

		var seen []string
		for set.Next() {
			seen = append(seen, set.At().Labels().String())
		}
		require.NoError(t, set.Err())
		require.NotEmpty(t, seen, "expected ChunkQuerier OTLP fan-out to surface the escaped series")
		var foundCanonical bool
		for _, l := range seen {
			if strings.Contains(l, `__name__="test"`) || strings.Contains(l, `__name__="test.counter"`) {
				foundCanonical = true
			}
		}
		require.True(t, foundCanonical, "expected canonical name in: %v", seen)
	})

	t.Run("LabelValues canonicalizes the metric name", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		// Ingest under the escaped form a UnderscoreEscapingWithSuffixes write
		// would produce; the canonical name in registry/1.1.0 is "test".
		appendSeries(t, wrapped, "test_bytes_total", 1, 1.0, "tenant", "alice")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		vals, _, err := q.LabelValues(context.Background(), model.MetricNameLabel, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "NoTranslation"),
		)
		require.NoError(t, err)
		// All variant escapings collapse to the single canonical metric name.
		require.ElementsMatch(t, []string{"test"}, vals)
	})

	t.Run("LabelNames aggregates all variant errors", func(t *testing.T) {
		// Three sentinels keyed on distinct OTLP escapings produced for
		// "test.counter" (a counter with unit "By" in registry/1.0.0). The
		// errgroup fan-out probes each variant; the wrapper must surface all
		// three underlying failures via errors.Is — the prior firstErr
		// implementation would have surfaced only one.
		errIdentity := errors.New("err-identity")
		errEscapedSuffixes := errors.New("err-escaped-suffixes")
		errEscapedNoSuffix := errors.New("err-escaped-no-suffix")

		underlying := teststorage.New(t)
		wrapped := semconv.AwareStorage(&erroringStorage{
			Storage: underlying,
			errsByMetric: map[string]error{
				"test.counter":             errIdentity,
				"test_counter_bytes_total": errEscapedSuffixes,
				"test_counter":             errEscapedNoSuffix,
			},
		})

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		_, _, err = q.LabelNames(context.Background(), nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test.counter"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.0.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "NoTranslation"),
		)
		require.Error(t, err)
		require.ErrorIs(t, err, errIdentity, "errors.Join should preserve every variant error")
		require.ErrorIs(t, err, errEscapedSuffixes, "errors.Join should preserve every variant error")
		require.ErrorIs(t, err, errEscapedNoSuffix, "errors.Join should preserve every variant error")
	})

	t.Run("resolves a translated query name", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		// A series exactly as an UnderscoreEscapingWithSuffixes producer wrote it.
		appendSeries(t, wrapped, "test_bytes_total", 1, 7.0, "http_response_status_code", "200")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		// Querying the *translated* name plus its strategy reverse-resolves it
		// to the canonical "test" and fans out.
		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_bytes_total"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "UnderscoreEscapingWithSuffixes"),
		)
		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the translated name to reverse-resolve and surface data")
		var found bool
		for k := range got {
			if strings.Contains(k, `__name__="test_bytes_total"`) {
				found = true
			}
		}
		require.True(t, found, "expected escaped-dialect output in: %v", got)
	})

	t.Run("dialect output unifies both eras under the escaped name", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		// Escaped 1.1.0 era and native 1.1.0 era of the same logical series.
		appendSeries(t, wrapped, "test_bytes_total", 1, 7.0, "http_response_status_code", "200")
		appendSeries(t, wrapped, "test", 2, 8.0, "http.response.status_code", "200")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_bytes_total"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "UnderscoreEscapingWithSuffixes"),
		)
		// Both eras collapse onto one escaped-dialect series carrying both samples.
		var series []string
		samples := 0
		for set.Next() {
			s := set.At()
			series = append(series, s.Labels().String())
			it := s.Iterator(nil)
			for it.Next() > 0 {
				samples++
			}
		}
		require.NoError(t, set.Err())
		require.Len(t, series, 1, "expected one unified series, got: %v", series)
		require.Contains(t, series[0], `__name__="test_bytes_total"`)
		require.Contains(t, series[0], `http_response_status_code="200"`)
		require.Equal(t, 2, samples, "expected samples from both the escaped and native eras")
	})

	t.Run("unresolved name with strategy warns and passes through", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "no_such_metric", 1, 1.0)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "no_such_metric"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "UnderscoreEscapingWithSuffixes"),
		)
		// The unresolvable name passes through with reserved labels stripped, so
		// the raw series is returned alongside the warning.
		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the raw series to pass through")
		var foundRaw bool
		for k := range got {
			if strings.Contains(k, `__name__="no_such_metric"`) {
				foundRaw = true
			}
		}
		require.True(t, foundRaw, "expected raw series in: %v", got)
		warnings := warningStrings(set.Warnings())
		require.NotEmpty(t, warnings)
		requireWarningsContain(t, warnings, "no matching metric")
	})

	t.Run("warns when only __semconv_url__ is set", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test", 1, 1.0)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
		)
		// Passthrough strips the reserved label, so the raw series is returned
		// (not an empty result from matching the never-present label).
		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the raw series to pass through")
		var foundRaw bool
		for k := range got {
			if strings.Contains(k, `__name__="test"`) {
				foundRaw = true
			}
		}
		require.True(t, foundRaw, "expected raw test series in: %v", got)
		warnings := warningStrings(set.Warnings())
		require.NotEmpty(t, warnings)
		requireWarningsContain(t, warnings, "has no effect")
	})

	t.Run("warns on __otlp_strategy__ without __semconv_url__", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test", 1, 1.0)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "NoTranslation"),
		)
		_ = collectSeries(t, set)
		warnings := warningStrings(set.Warnings())
		require.NotEmpty(t, warnings)
		requireWarningsContain(t, warnings, "requires __semconv_url__")
	})

	t.Run("warns on an unrecognized __otlp_strategy__", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test", 1, 1.0)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "Bogus"),
		)
		_ = collectSeries(t, set)
		warnings := warningStrings(set.Warnings())
		require.NotEmpty(t, warnings)
		requireWarningsContain(t, warnings, "not a recognized OTLP translation strategy")
	})

	t.Run("__schema_url__ alone fans out version names with canonical output", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		// Native current-era series under the canonical name.
		appendSeries(t, wrapped, "test", 1, 5.0, "http.response.status_code", "200")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		// Version fan-out only (no __otlp_strategy__): raw OTel names, canonical output.
		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__schema_url__", "registry/registry.yaml"),
		)
		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the native series to surface via the version axis")
		var found bool
		for k := range got {
			if strings.Contains(k, `__name__="test"`) {
				found = true
			}
		}
		require.True(t, found, "expected canonical name in: %v", got)
	})

	t.Run("LabelNames reports escaped-dialect names", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test_bytes_total", 1, 1.0, "http_response_status_code", "200")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		names, _, err := q.LabelNames(context.Background(), nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_bytes_total"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "UnderscoreEscapingWithSuffixes"),
		)
		require.NoError(t, err)
		// Names are reported in the requested escaped dialect, not canonical.
		require.Contains(t, names, "http_response_status_code")
		require.NotContains(t, names, "http.response.status_code")
	})

	t.Run("LabelValues reports the escaped-dialect metric name", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test_bytes_total", 1, 1.0, "http_response_status_code", "200")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		vals, _, err := q.LabelValues(context.Background(), model.MetricNameLabel, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_bytes_total"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "UnderscoreEscapingWithSuffixes"),
		)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"test_bytes_total"}, vals)
	})

	t.Run("rejects the canonical name under a mismatched strategy", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test", 1, 1.0)

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		// "test" is the NoTranslation name; under UnderscoreEscapingWithSuffixes
		// the metric is "test_bytes_total", so the canonical name does not match
		// the declared dialect and must not resolve — the query passes through
		// with a warning instead of fanning out.
		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "UnderscoreEscapingWithSuffixes"),
		)
		got := collectSeries(t, set)
		require.NotEmpty(t, got, "expected the raw series to pass through")
		for k := range got {
			require.NotContains(t, k, "test_bytes_total", "expected the raw series, not escaped-dialect fan-out")
		}
		warnings := warningStrings(set.Warnings())
		require.NotEmpty(t, warnings)
		requireWarningsContain(t, warnings, "no matching metric")
	})

	t.Run("warns on a label matcher in the wrong dialect", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test", 1, 1.0, "http.response.status_code", "200")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		// NoTranslation dialect, but the label is escaped
		// (http_response_status_code) — a likely mistake worth flagging.
		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test"),
			labels.MustNewMatcher(labels.MatchEqual, "http_response_status_code", "200"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "NoTranslation"),
		)
		_ = collectSeries(t, set)
		warnings := warningStrings(set.Warnings())
		requireWarningsContain(t, warnings, "not written in the declared __otlp_strategy__ dialect")
	})

	t.Run("no dialect warning for an in-dialect query with a plain label", func(t *testing.T) {
		wrapped, _ := newAwareStorage(t)
		appendSeries(t, wrapped, "test_bytes_total", 1, 1.0, "http_response_status_code", "200", "instance", "a")

		q, err := wrapped.Querier(0, 10)
		require.NoError(t, err)
		t.Cleanup(func() { _ = q.Close() })

		// Escaped label matches the UESuffixes dialect; "instance" is a plain
		// non-attribute label. Neither should trigger a dialect warning.
		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "test_bytes_total"),
			labels.MustNewMatcher(labels.MatchEqual, "http_response_status_code", "200"),
			labels.MustNewMatcher(labels.MatchEqual, "instance", "a"),
			labels.MustNewMatcher(labels.MatchEqual, "__semconv_url__", "registry/1.1.0"),
			labels.MustNewMatcher(labels.MatchEqual, "__otlp_strategy__", "UnderscoreEscapingWithSuffixes"),
		)
		_ = collectSeries(t, set)
		for _, w := range warningStrings(set.Warnings()) {
			require.NotContains(t, w, "dialect", "unexpected dialect warning: %s", w)
		}
	})
}
