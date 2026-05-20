// Copyright 2026 The Prometheus Authors
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

package promql

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func TestKVPairsToLabels_Valid(t *testing.T) {
	got, err := kvPairsToLabels([]string{"env", "prod", "job", "api"})
	require.NoError(t, err)
	require.Equal(t,
		labels.FromStrings("env", "prod", "job", "api"),
		got,
	)
}

func TestKVPairsToLabels_Empty(t *testing.T) {
	got, err := kvPairsToLabels(nil)
	require.NoError(t, err)
	require.Equal(t, labels.EmptyLabels(), got)
}

func TestKVPairsToLabels_OddLength(t *testing.T) {
	_, err := kvPairsToLabels([]string{"env", "prod", "job"})
	require.ErrorContains(t, err, "expected even number")
}

func TestKVPairsToLabels_InvalidName(t *testing.T) {
	_, err := kvPairsToLabels([]string{"1bad", "x"})
	require.ErrorContains(t, err, "invalid label name")
}

func TestKVPairsToLabels_RejectsNameLabel(t *testing.T) {
	_, err := kvPairsToLabels([]string{"__name__", "foo", "env", "prod"})
	require.ErrorContains(t, err, "__name__ must not be set")
}

func TestKVPairsToLabels_AllowsCommaInValue(t *testing.T) {
	got, err := kvPairsToLabels([]string{"tags", "a,b,c", "env", "prod"})
	require.NoError(t, err)
	require.Equal(t,
		labels.FromStrings("env", "prod", "tags", "a,b,c"),
		got,
	)
}

func TestSeriesBuilder_InjectsMetricName(t *testing.T) {
	b := newSeriesBuilder("my_metric", 1000, 7000)
	require.NoError(t, b.add(labels.FromStrings("env", "prod"), 1.0))

	require.Len(t, b.out, 1)
	require.Equal(t,
		labels.FromStrings("__name__", "my_metric", "env", "prod"),
		b.out[0].Metric,
	)
	require.Equal(t, 1.0, b.out[0].F)
	require.Equal(t, int64(7000), b.out[0].T)
}

func TestSeriesBuilder_OmitsMetricNameWhenEmpty(t *testing.T) {
	b := newSeriesBuilder("", 1000, 7000)
	require.NoError(t, b.add(labels.FromStrings("env", "prod"), 1.0))

	require.False(t, b.out[0].Metric.Has("__name__"))
}

func TestSeriesBuilder_Overflow(t *testing.T) {
	b := newSeriesBuilder("", 2, 0)
	require.NoError(t, b.add(labels.FromStrings("env", "prod"), 1))
	require.NoError(t, b.add(labels.FromStrings("env", "stage"), 1))
	err := b.add(labels.FromStrings("env", "dev"), 1)
	require.ErrorContains(t, err, "emitted series exceeds limit 2")
}

func TestSeriesBuilder_DuplicateLabelSet(t *testing.T) {
	b := newSeriesBuilder("f", 1000, 0)
	require.NoError(t, b.add(labels.FromStrings("env", "prod"), 1))
	err := b.add(labels.FromStrings("env", "prod"), 2)
	require.ErrorContains(t, err, "duplicate label set")
}

func TestTplEngine_ParseError(t *testing.T) {
	_, err := newTplEngine(`{{ this is not valid go template syntax`)
	require.ErrorContains(t, err, "template parse error")
}

func TestTplEngine_RejectsDefine(t *testing.T) {
	_, err := newTplEngine(`{{define "x"}}y{{end}}`)
	require.ErrorContains(t, err, "forbidden template action: define")
}

func TestTplEngine_RejectsTemplate(t *testing.T) {
	_, err := newTplEngine(`{{template "x"}}`)
	require.ErrorContains(t, err, "forbidden template action: template")
}

func TestTplEngine_AcceptsValidTemplate(t *testing.T) {
	_, err := newTplEngine(`hello {{.}}`)
	require.NoError(t, err)
}

func TestTplEngine_Build_SingleSeries(t *testing.T) {
	e, err := newTplEngine(`{{series 1.0 "env" "prod"}}`)
	require.NoError(t, err)

	v, err := e.build("f", 7000)
	require.NoError(t, err)
	require.Len(t, v, 1)
	require.Equal(t,
		labels.FromStrings("__name__", "f", "env", "prod"),
		v[0].Metric,
	)
	require.Equal(t, 1.0, v[0].F)
	require.Equal(t, int64(7000), v[0].T)
}

func TestTplEngine_Build_SeriesNoLabels(t *testing.T) {
	e, err := newTplEngine(`{{series 42.0}}`)
	require.NoError(t, err)

	v, err := e.build("f", 0)
	require.NoError(t, err)
	require.Len(t, v, 1)
	require.Equal(t, labels.FromStrings("__name__", "f"), v[0].Metric)
	require.Equal(t, 42.0, v[0].F)
}

func TestTplEngine_Build_RangeSeries(t *testing.T) {
	e, err := newTplEngine(`{{rangeSeries "env" "prod,stage,dev" 1.0}}`)
	require.NoError(t, err)

	v, err := e.build("", 0)
	require.NoError(t, err)
	require.Len(t, v, 3)
	envs := []string{}
	for _, s := range v {
		envs = append(envs, s.Metric.Get("env"))
	}
	require.ElementsMatch(t, []string{"prod", "stage", "dev"}, envs)
}

func TestTplEngine_Build_RangeSeriesWithExtras(t *testing.T) {
	e, err := newTplEngine(`{{rangeSeries "env" "prod,stage" 1.0 "job" "api"}}`)
	require.NoError(t, err)

	v, err := e.build("", 0)
	require.NoError(t, err)
	require.Len(t, v, 2)
	for _, s := range v {
		require.Equal(t, "api", s.Metric.Get("job"))
	}
}

func TestTplEngine_Build_SeqAndPrintf(t *testing.T) {
	e, err := newTplEngine(`{{range $i := seq 1 3}}{{series 1.0 "i" (printf "%d" $i)}}{{end}}`)
	require.NoError(t, err)

	v, err := e.build("f", 0)
	require.NoError(t, err)
	require.Len(t, v, 3)
}

func TestTplEngine_Build_CapEnforced(t *testing.T) {
	e, err := newTplEngine(`{{range $i := seq 1 5}}{{series 1.0 "i" (printf "%d" $i)}}{{end}}`)
	require.NoError(t, err)

	v, err := e.buildWithCap("f", 0, 3)
	require.Error(t, err)
	require.ErrorContains(t, err, "emitted series exceeds limit 3")
	require.Nil(t, v)
}

func TestTplEngine_Build_TemplateSetsNameRejected(t *testing.T) {
	e, err := newTplEngine(`{{series 1.0 "__name__" "foo" "env" "prod"}}`)
	require.NoError(t, err)

	_, err = e.build("", 0)
	require.ErrorContains(t, err, "__name__ must not be set")
}

func TestTplEngine_Build_RangeSeriesRejectsNameAsFanoutLabel(t *testing.T) {
	e, err := newTplEngine(`{{rangeSeries "__name__" "a,b" 1.0}}`)
	require.NoError(t, err)

	_, err = e.build("", 0)
	require.ErrorContains(t, err, "__name__ must not be set")
}

func TestTplEngine_Build_SeriesOddKVPairs(t *testing.T) {
	e, err := newTplEngine(`{{series 1.0 "env" "prod" "lonely"}}`)
	require.NoError(t, err)

	_, err = e.build("", 0)
	require.ErrorContains(t, err, "expected even number")
}

func TestEvalTimeseriesGen_FirstCallBuilds(t *testing.T) {
	enh := &EvalNodeHelper{Ts: 1000}
	args := mustParseArgs(t, `timeseries_gen("f", "{{series 1.0 \"env\" \"prod\"}}")`)
	v, _ := funcTimeseriesGen(nil, nil, args, enh)

	require.Len(t, v, 1)
	require.Equal(t,
		labels.FromStrings("__name__", "f", "env", "prod"),
		v[0].Metric,
	)
	require.Equal(t, int64(1000), v[0].T)
	require.NotNil(t, enh.NodeCache)
}

func TestEvalTimeseriesGen_SecondCallReusesCache(t *testing.T) {
	enh := &EvalNodeHelper{Ts: 1000}
	args := mustParseArgs(t, `timeseries_gen("f", "{{series 1.0 \"env\" \"prod\"}}")`)
	_, _ = funcTimeseriesGen(nil, nil, args, enh)
	first := enh.NodeCache

	enh.Ts = 2000
	v, _ := funcTimeseriesGen(nil, nil, args, enh)

	require.Same(t, first, enh.NodeCache, "second call must reuse the cache")
	require.Equal(t, int64(2000), v[0].T, "timestamp must be restamped per step")
}

func TestEvalTimeseriesGen_BadTemplatePanics(t *testing.T) {
	enh := &EvalNodeHelper{Ts: 1000}
	args := mustParseArgs(t, `timeseries_gen("", "{{define \"x\"}}y{{end}}")`)

	require.PanicsWithError(t, "timeseries_gen: forbidden template action: define", func() {
		_, _ = funcTimeseriesGen(nil, nil, args, enh)
	})
}

// mustParseArgs parses a PromQL call expression and returns its arguments.
func mustParseArgs(t *testing.T, expr string) parser.Expressions {
	t.Helper()
	p := parser.NewParser(parser.Options{EnableExperimentalFunctions: true})
	e, err := p.ParseExpr(expr)
	require.NoError(t, err)
	call, ok := e.(*parser.Call)
	require.True(t, ok)
	return call.Args
}

// BenchmarkTimeseriesGen_WarmPath measures the per-step cost of
// timeseries_gen after the first call has primed enh.NodeCache. The
// warm path must be O(N) in emitted series count and must NOT re-execute
// the underlying text/template.
func BenchmarkTimeseriesGen_WarmPath(b *testing.B) {
	enh := &EvalNodeHelper{Ts: 1000}
	args := mustParseArgsB(b, `timeseries_gen("f", "{{range $i := seq 1 100}}{{series 1.0 \"i\" (printf \"%d\" $i)}}{{end}}")`)

	// Prime the cache so the loop below exercises the warm path only.
	_, _ = funcTimeseriesGen(nil, nil, args, enh)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enh.Ts = int64(i)
		v, _ := funcTimeseriesGen(nil, nil, args, enh)
		if len(v) != 100 {
			b.Fatalf("unexpected len: %d", len(v))
		}
	}
}

// mustParseArgsB is the *testing.B variant of mustParseArgs.
func mustParseArgsB(b *testing.B, expr string) parser.Expressions {
	b.Helper()
	p := parser.NewParser(parser.Options{EnableExperimentalFunctions: true})
	e, err := p.ParseExpr(expr)
	if err != nil {
		b.Fatal(err)
	}
	call, ok := e.(*parser.Call)
	if !ok {
		b.Fatal("not a *parser.Call")
	}
	return call.Args
}
