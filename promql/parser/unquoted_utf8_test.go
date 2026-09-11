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

package parser

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/features"
)

func TestUnquotedUTF8Names(t *testing.T) {
	enabled := NewParser(Options{EnableUnquotedUTF8Names: true})
	disabled := NewParser(Options{})
	for _, name := range []string{
		"Björn", "温度", "θερμοκρασία", "температура", "حرارة", "𐐀", "a𐐀9",
		"http.server.request.duration", "foo..bar", "foo.", "_:温度.total",
		"rate.total", "Inf.total", "NaN.total", "on.total", "sum.total",
		"resource.k8s.namespace", "target.info", "histogram.count",
		"İnf", "а", "ö",
	} {
		t.Run(name, func(t *testing.T) {
			expr, err := enabled.ParseExpr(name)
			require.NoError(t, err)
			require.Equal(t, name, expr.(*VectorSelector).Name)
			require.Equal(t, name, expr.String())
			actual, err := enabled.ParseMetricSelector(name)
			require.NoError(t, err)
			expected, err := disabled.ParseMetricSelector(fmt.Sprintf("{%q}", name))
			require.NoError(t, err)
			require.Equal(t, expected, actual)
			// Alternate parser instances to also exercise reuse from the parser pool.
			_, err = disabled.ParseExpr(name)
			require.Error(t, err)
			_, err = enabled.ParseExpr(expr.String())
			require.NoError(t, err)
		})
	}

	for _, input := range []string{
		"foo{service.name=\"api\"}", "温度{場所=\"東京\"}",
		"sum by (service.name, équipe) (http.server.requests)",
		"sum without (service.name, équipe) (http.server.requests)",
		"requests_total + on (service.name) group_left (équipe) service_info",
		"requests_total + ignoring (service.name) group_right (équipe) service_info",
		"foo{a..=\"b\", __名=~\"東京.*\"}",
		"rate(http.server.requests[5m])", "http.server.requests[5m:1m]",
		"http.server.requests offset 5m", "http.server.requests @ 0.5",
		"http.server.requests + .5", "Inf.total + Inf", "NaN.total or foo",
	} {
		t.Run(input, func(t *testing.T) {
			expr, err := enabled.ParseExpr(input)
			require.NoError(t, err)
			printed := expr.String()
			roundTrip, err := enabled.ParseExpr(printed)
			require.NoError(t, err)
			require.Equal(t, printed, roundTrip.String())
			_, err = disabled.ParseExpr(input)
			require.Error(t, err)
		})
	}

	for _, input := range []string{
		".foo", "5foo", "foo{.label=\"x\"}", "foo{5label=\"x\"}",
		"foo{a:b=\"x\"}", "sum by (a:b) (foo)", "foo + on (a:b) bar",
		"foo😀", "foo١", "foo²", "o\u0308", "foo·bar", "foo\u200dbar", "foo\ufffd", "foo\xff",
		"sum by (o\u0308) (foo)", "foo{é😀=\"x\"}",
		"rate.total(foo)", "温度(foo)", "on", "bool", "foo[5m.foo]",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := enabled.ParseExpr(input)
			require.Error(t, err)
		})
	}

	for _, input := range []string{".5", "5.", "1.5", "1e3", "0x10", "Inf", "NaN", "5m"} {
		t.Run(input, func(t *testing.T) {
			expr, err := enabled.ParseExpr(input)
			require.NoError(t, err)
			require.IsType(t, &NumberLiteral{}, expr)
			expected, err := disabled.ParseExpr(input)
			require.NoError(t, err)
			require.Equal(t, expected.String(), expr.String())
		})
	}

	for _, name := range []string{".5", "5", "5foo", ".foo"} {
		_, err := enabled.ParseMetricSelector(name)
		require.Error(t, err)
		matchers, err := enabled.ParseMetricSelector(fmt.Sprintf("{%q}", name))
		require.NoError(t, err)
		require.Equal(t, name, matchers[0].Value)
	}

	metric, err := enabled.ParseMetric(`温度{場所="東京"}`)
	require.NoError(t, err)
	require.Equal(t, labels.FromStrings("__name__", "温度", "場所", "東京"), metric)
	series, values, err := enabled.ParseSeriesDesc(`温度{場所="東京"} 1+1x2`)
	require.NoError(t, err)
	require.Equal(t, metric, series)
	require.Equal(t, []SequenceValue{{Value: 1}, {Value: 2}, {Value: 3}}, values)

	for _, enabled := range []bool{false, true} {
		registry := features.NewRegistry()
		NewParser(Options{EnableUnquotedUTF8Names: enabled}).RegisterFeatures(registry)
		require.Equal(t, enabled, registry.Get()[features.PromQL]["unquoted_utf8_names"])
	}
}

func TestUnquotedUTF8NamesPrinting(t *testing.T) {
	for _, tc := range []struct{ input, enabled, disabled string }{
		{`foo{"service.name"="api"}`, `foo{service.name="api"}`, `foo{"service.name"="api"}`},
		{`sum by ("service.name", "équipe") (foo)`, `sum by (service.name, équipe) (foo)`, `sum by ("service.name", "équipe") (foo)`},
		{`foo + on ("service.name") group_left ("équipe") bar`, `foo + on (service.name) group_left (équipe) bar`, `foo + on ("service.name") group_left ("équipe") bar`},
		{`foo{".name"="x", "ö"="y", "😀"="z"}`, `foo{".name"="x","ö"="y","😀"="z"}`, `foo{".name"="x","ö"="y","😀"="z"}`},
	} {
		t.Run(tc.input, func(t *testing.T) {
			for _, enabled := range []bool{false, true} {
				p := NewParser(Options{EnableUnquotedUTF8Names: enabled})
				expr, err := p.ParseExpr(tc.input)
				require.NoError(t, err)
				expected := tc.disabled
				if enabled {
					expected = tc.enabled
				}
				require.Equal(t, expected, expr.String())
				roundTrip, err := p.ParseExpr(expr.String())
				require.NoError(t, err)
				require.Equal(t, expected, roundTrip.String())
			}
		})
	}
}

func BenchmarkParseNames(b *testing.B) {
	for _, tc := range []struct {
		name, query string
		extended    bool
	}{
		{"ASCII", `sum by (service_name) (rate(http_server_requests_total[5m]))`, false},
		{"quoted", `sum by ("service.name") (rate({"http.server.requests"}[5m]))`, false},
		{"dots", `sum by (service.name) (rate(http.server.requests[5m]))`, true},
		{"Unicode", `sum by (場所) (rate(温度[5m]))`, true},
	} {
		for _, enabled := range []bool{false, true} {
			if tc.extended && !enabled {
				continue
			}
			b.Run(fmt.Sprintf("%s/enabled=%t", tc.name, enabled), func(b *testing.B) {
				p := NewParser(Options{EnableUnquotedUTF8Names: enabled})
				b.ReportAllocs()
				for b.Loop() {
					if _, err := p.ParseExpr(tc.query); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
