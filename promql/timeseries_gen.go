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
	"errors"
	"fmt"
	"math"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

// maxTimeseriesGenSeries is the hard cap on how many series a single
// timeseries_gen call may emit.
const maxTimeseriesGenSeries = 10_000

// kvPairsToLabels converts a flat slice of alternating key,value strings into
// a sorted labels.Labels. The slice must have even length. When rejectName
// is true, setting __name__ is an error (because the caller has supplied a
// metric_name argument and the builder will inject it); when false, the
// template is allowed to set __name__ on a per-series basis. An empty input
// yields labels.EmptyLabels().
func kvPairsToLabels(kvPairs []string, rejectName bool) (labels.Labels, error) {
	if len(kvPairs) == 0 {
		return labels.EmptyLabels(), nil
	}
	if len(kvPairs)%2 != 0 {
		return labels.EmptyLabels(), fmt.Errorf("expected even number of key/value strings, got %d", len(kvPairs))
	}
	b := labels.NewScratchBuilder(len(kvPairs) / 2)
	for i := 0; i < len(kvPairs); i += 2 {
		name, val := kvPairs[i], kvPairs[i+1]
		if !labelNameValid(name) {
			return labels.EmptyLabels(), fmt.Errorf("invalid label name: %q", name)
		}
		if rejectName && name == model.MetricNameLabel {
			return labels.EmptyLabels(), errors.New("__name__ must not be set in template label arguments when metric_name is provided")
		}
		b.Add(name, val)
	}
	b.Sort()
	return b.Labels(), nil
}

// labelNameValid reports whether name matches the standard Prometheus
// label-name grammar: [a-zA-Z_][a-zA-Z0-9_]*.
func labelNameValid(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// add returns a + b, coercing each operand to float64 first.
func tplAdd(a, b any) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	return af + bf, nil
}

// tplSub returns a - b, coercing each operand to float64 first.
func tplSub(a, b any) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	return af - bf, nil
}

// tplMul returns a * b, coercing each operand to float64 first.
func tplMul(a, b any) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	return af * bf, nil
}

// tplDiv returns a / b, coercing each operand to float64 first.
// Dividing by zero is an error rather than producing ±Inf or NaN so
// authors notice the bug at template-execute time.
func tplDiv(a, b any) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	if bf == 0 {
		return 0, errors.New("division by zero")
	}
	return af / bf, nil
}

// tplMod returns math.Mod(a, b), coercing each operand to float64 first.
func tplMod(a, b any) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	if bf == 0 {
		return 0, errors.New("modulo by zero")
	}
	return math.Mod(af, bf), nil
}

// tplInt truncates v toward zero and returns the result as an int. It is
// the inverse of toFloat64 for the common case "I have a float result from
// add/sub/mul/div and need an int for printf %d".
func tplInt(v any) (int, error) {
	f, err := toFloat64(v)
	if err != nil {
		return 0, err
	}
	return int(math.Trunc(f)), nil
}

// tplAbs returns math.Abs(v), coercing v to float64 first.
func tplAbs(v any) (float64, error) {
	f, err := toFloat64(v)
	if err != nil {
		return 0, err
	}
	return math.Abs(f), nil
}

// tplFloor returns math.Floor(v), coercing v to float64 first.
func tplFloor(v any) (float64, error) {
	f, err := toFloat64(v)
	if err != nil {
		return 0, err
	}
	return math.Floor(f), nil
}

// tplCeil returns math.Ceil(v), coercing v to float64 first.
func tplCeil(v any) (float64, error) {
	f, err := toFloat64(v)
	if err != nil {
		return 0, err
	}
	return math.Ceil(f), nil
}

// tplRound returns math.Round(v), coercing v to float64 first. Returns a
// float64; chain with int to get an integer.
func tplRound(v any) (float64, error) {
	f, err := toFloat64(v)
	if err != nil {
		return 0, err
	}
	return math.Round(f), nil
}

// tplMin returns math.Min(a, b), coercing each operand to float64 first.
func tplMin(a, b any) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	return math.Min(af, bf), nil
}

// tplMax returns math.Max(a, b), coercing each operand to float64 first.
func tplMax(a, b any) (float64, error) {
	af, err := toFloat64(a)
	if err != nil {
		return 0, err
	}
	bf, err := toFloat64(b)
	if err != nil {
		return 0, err
	}
	return math.Max(af, bf), nil
}

// toFloat64 coerces any numeric value Go's text/template can produce
// (literals, loop indices from {{range}}, variables) into float64. Go
// text/template does not auto-convert int to float64 in function calls,
// so accepting `any` and converting inside the helper lets templates
// write `{{series $i ...}}` where $i comes from `seq`.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int8:
		return float64(n), nil
	case int16:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint:
		return float64(n), nil
	case uint8:
		return float64(n), nil
	case uint16:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("expected numeric value, got %T", v)
	}
}

// seriesBuilder collects synthetic samples emitted by the timeseries_gen
// template helpers. It enforces the output cardinality cap, deduplicates
// identical label sets, and injects __name__ when a metric name is supplied.
type seriesBuilder struct {
	metric    string
	maxSeries int
	ts        int64
	out       Vector
	seen      map[uint64]struct{}
}

// newSeriesBuilder returns an empty builder pre-sized for maxSeries samples.
// metric is injected as the __name__ label on every emitted sample;
// pass "" to omit __name__.
func newSeriesBuilder(metric string, maxSeries int, ts int64) *seriesBuilder {
	return &seriesBuilder{
		metric:    metric,
		maxSeries: maxSeries,
		ts:        ts,
		out:       make(Vector, 0),
		seen:      make(map[uint64]struct{}),
	}
}

// add appends a Sample with label set ls and value v at the builder's fixed
// timestamp. ls may be modified by injecting __name__ when the builder has
// a non-empty metric name. Duplicate label sets are rejected.
func (b *seriesBuilder) add(ls labels.Labels, v float64) error {
	if len(b.out) >= b.maxSeries {
		return fmt.Errorf("emitted series exceeds limit %d", b.maxSeries)
	}
	if b.metric != "" {
		lb := labels.NewBuilder(ls)
		lb.Set(model.MetricNameLabel, b.metric)
		ls = lb.Labels()
	}
	h := ls.Hash()
	if _, dup := b.seen[h]; dup {
		return fmt.Errorf("duplicate label set: %s", ls.String())
	}
	b.seen[h] = struct{}{}
	b.out = append(b.out, Sample{
		Metric: ls,
		T:      b.ts,
		F:      v,
	})
	return nil
}

// tplEngine wraps a parsed Go text/template for a single timeseries_gen
// invocation. Real helper closures are bound per-execution by build().
type tplEngine struct {
	parsed *template.Template
}

// newTplEngine parses src with a minimal placeholder FuncMap (the real
// helpers are bound per-execution by build), then walks the parse tree
// rejecting forbidden actions {{define}} and {{template}}.
func newTplEngine(src string) (*tplEngine, error) {
	tpl, err := template.New("ts_gen").Funcs(template.FuncMap{
		"series":      func(any, ...string) string { return "" },
		"rangeSeries": func(string, string, any, ...string) string { return "" },
		"seq":         func(int, int) []int { return nil },
		"split":       strings.Split,
		"lower":       strings.ToLower,
		"upper":       strings.ToUpper,
		"replace":     strings.ReplaceAll,
		"trim":        strings.TrimSpace,
		"add":         tplAdd,
		"sub":         tplSub,
		"mul":         tplMul,
		"div":         tplDiv,
		"mod":         tplMod,
		"int":         tplInt,
		"abs":         tplAbs,
		"floor":       tplFloor,
		"ceil":        tplCeil,
		"round":       tplRound,
		"min":         tplMin,
		"max":         tplMax,
	}).Parse(src)
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}
	if err := scanForbiddenActions(tpl); err != nil {
		return nil, err
	}
	return &tplEngine{parsed: tpl}, nil
}

// scanForbiddenActions walks the parse trees of tpl and any associated
// templates, rejecting {{define}} and {{template}} actions.
func scanForbiddenActions(tpl *template.Template) error {
	for _, t := range tpl.Templates() {
		if t.Name() != tpl.Name() && t.Tree != nil {
			return errors.New("forbidden template action: define")
		}
		if t.Tree == nil || t.Root == nil {
			continue
		}
		if err := walkTplNodes(t.Root); err != nil {
			return err
		}
	}
	return nil
}

// build executes the template with helpers wired to a fresh seriesBuilder
// and returns the collected Vector. ts is the timestamp stamped on every
// emitted sample. metric, if non-empty, becomes the __name__ label.
func (e *tplEngine) build(metric string, ts int64) (Vector, error) {
	return e.buildWithCap(metric, ts, maxTimeseriesGenSeries)
}

// buildWithCap is build with an explicit per-call cap. Exposed for testing.
func (e *tplEngine) buildWithCap(metric string, ts int64, seriesCap int) (Vector, error) {
	b := newSeriesBuilder(metric, seriesCap, ts)

	tpl, err := e.parsed.Clone()
	if err != nil {
		return nil, fmt.Errorf("template clone error: %w", err)
	}
	tpl = tpl.Funcs(template.FuncMap{
		"series": func(v any, kvPairs ...string) (string, error) {
			f, err := toFloat64(v)
			if err != nil {
				return "", err
			}
			ls, err := kvPairsToLabels(kvPairs, b.metric != "")
			if err != nil {
				return "", err
			}
			if err := b.add(ls, f); err != nil {
				return "", err
			}
			return "", nil
		},
		"rangeSeries": func(label, csv string, v any, kvPairs ...string) (string, error) {
			if !labelNameValid(label) {
				return "", fmt.Errorf("invalid label name: %q", label)
			}
			if b.metric != "" && label == model.MetricNameLabel {
				return "", errors.New("__name__ must not be the fanout label when metric_name is provided")
			}
			f, err := toFloat64(v)
			if err != nil {
				return "", err
			}
			extras, err := kvPairsToLabels(kvPairs, b.metric != "")
			if err != nil {
				return "", err
			}
			for val := range strings.SplitSeq(csv, ",") {
				val = strings.TrimSpace(val)
				if val == "" {
					// Skip empty entries so an empty csv or a stray trailing
					// comma does not emit a phantom series.
					continue
				}
				lb := labels.NewBuilder(extras)
				lb.Set(label, val)
				if err := b.add(lb.Labels(), f); err != nil {
					return "", err
				}
			}
			return "", nil
		},
		"seq": func(start, end int) []int {
			if end < start {
				return nil
			}
			out := make([]int, 0, end-start+1)
			for i := start; i <= end; i++ {
				out = append(out, i)
			}
			return out
		},
	})

	if err := tpl.Execute(discardWriter{}, nil); err != nil {
		return nil, err
	}
	return b.out, nil
}

// discardWriter implements io.Writer by discarding all bytes. It is used
// because timeseries_gen helpers communicate via side effects on
// seriesBuilder, not via the template's stdout.
type discardWriter struct{}

// Write discards all bytes and reports them as written.
func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// walkTplNodes recursively walks a text/template parse tree rejecting
// {{template}} actions. {{define}} is detected at the Templates() level
// in scanForbiddenActions.
func walkTplNodes(n parse.Node) error {
	switch v := n.(type) {
	case *parse.ListNode:
		if v == nil {
			return nil
		}
		for _, c := range v.Nodes {
			if err := walkTplNodes(c); err != nil {
				return err
			}
		}
	case *parse.TemplateNode:
		return errors.New("forbidden template action: template")
	case *parse.IfNode:
		if err := walkTplNodes(v.List); err != nil {
			return err
		}
		if v.ElseList != nil {
			if err := walkTplNodes(v.ElseList); err != nil {
				return err
			}
		}
	case *parse.RangeNode:
		if err := walkTplNodes(v.List); err != nil {
			return err
		}
		if v.ElseList != nil {
			if err := walkTplNodes(v.ElseList); err != nil {
				return err
			}
		}
	case *parse.WithNode:
		if err := walkTplNodes(v.List); err != nil {
			return err
		}
		if v.ElseList != nil {
			if err := walkTplNodes(v.ElseList); err != nil {
				return err
			}
		}
	}
	return nil
}

// timeseriesGenCache is the value stored on EvalNodeHelper.NodeCache by
// funcTimeseriesGen. It holds the built samples (labels + values, without
// per-step timestamps) so subsequent steps can re-stamp them without
// re-running the template.
type timeseriesGenCache struct {
	samples []timeseriesGenCacheEntry
}

// timeseriesGenCacheEntry holds one cached series's labels and value.
type timeseriesGenCacheEntry struct {
	metric labels.Labels
	value  float64
}

// funcTimeseriesGen is the dispatch entry for the timeseries_gen PromQL
// function. The first call parses, executes, and caches the template
// output on enh.NodeCache. Subsequent calls re-stamp the cached samples
// at enh.Ts. Errors are wrapped with the timeseries_gen: prefix and
// raised via panic, matching the pattern used by other dispatch
// functions (e.g. funcLabelReplace).
func funcTimeseriesGen(_ []Vector, _ Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	if c, ok := enh.NodeCache.(*timeseriesGenCache); ok {
		out := make(Vector, 0, len(c.samples))
		for _, s := range c.samples {
			out = append(out, Sample{Metric: s.metric, T: enh.Ts, F: s.value})
		}
		return out, nil
	}

	tplSrc := stringFromArg(args[0])
	metric := ""
	if len(args) >= 2 {
		metric = stringFromArg(args[1])
	}

	e, err := newTplEngine(tplSrc)
	if err != nil {
		panic(fmt.Errorf("timeseries_gen: %w", err))
	}
	built, err := e.build(metric, enh.Ts)
	if err != nil {
		panic(fmt.Errorf("timeseries_gen: %w", err))
	}

	entries := make([]timeseriesGenCacheEntry, 0, len(built))
	out := make(Vector, 0, len(built))
	for _, s := range built {
		entries = append(entries, timeseriesGenCacheEntry{metric: s.Metric, value: s.F})
		out = append(out, Sample{Metric: s.Metric, T: enh.Ts, F: s.F})
	}
	enh.NodeCache = &timeseriesGenCache{samples: entries}
	return out, nil
}
