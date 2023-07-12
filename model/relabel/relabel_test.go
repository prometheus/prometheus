// Copyright 2015 The Prometheus Authors
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

package relabel

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/model/labels"
)

func TestRelabel(t *testing.T) {
	tests := []struct {
		input   labels.Labels
		relabel []*Config
		output  labels.Labels
		drop    bool
	}{
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch${1}-ch${1}",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "choo-choo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a", "b"},
					Regex:        MustNewRegexp("f(.*);(.*)r"),
					TargetLabel:  "a",
					Separator:    ";",
					Replacement:  "b${1}${2}m", // boobam
					Action:       Replace,
				},
				{
					SourceLabels: model.LabelNames{"c", "a"},
					Regex:        MustNewRegexp("(b).*b(.*)ba(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1$2$2$3",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "boobam",
				"b": "bar",
				"c": "baz",
				"d": "boooom",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp(".*o.*"),
					Action:       Drop,
				}, {
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f(.*)"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "ch$1-ch$1",
					Action:       Replace,
				},
			},
			drop: true,
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp(".*o.*"),
					Action:       Drop,
				},
			},
			drop: true,
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "abc",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp(".*(b).*"),
					TargetLabel:  "d",
					Separator:    ";",
					Replacement:  "$1",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "abc",
				"d": "b",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("no-match"),
					Action:       Drop,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f|o"),
					Action:       Drop,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("no-match"),
					Action:       Keep,
				},
			},
			drop: true,
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f.*"),
					Action:       Keep,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			// No replacement must be applied if there is no match.
			input: labels.FromMap(map[string]string{
				"a": "boo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("f"),
					TargetLabel:  "b",
					Replacement:  "bar",
					Action:       Replace,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "boo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"c"},
					TargetLabel:  "d",
					Separator:    ";",
					Action:       HashMod,
					Modulus:      1000,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
				"b": "bar",
				"c": "baz",
				"d": "976",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a": "foo\nbar",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					TargetLabel:  "b",
					Separator:    ";",
					Action:       HashMod,
					Modulus:      1000,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo\nbar",
				"b": "734",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*Config{
				{
					Regex:       MustNewRegexp("(b.*)"),
					Replacement: "bar_${1}",
					Action:      LabelMap,
				},
			},
			output: labels.FromMap(map[string]string{
				"a":      "foo",
				"b1":     "bar",
				"b2":     "baz",
				"bar_b1": "bar",
				"bar_b2": "baz",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a":             "foo",
				"__meta_my_bar": "aaa",
				"__meta_my_baz": "bbb",
				"__meta_other":  "ccc",
			}),
			relabel: []*Config{
				{
					Regex:       MustNewRegexp("__meta_(my.*)"),
					Replacement: "${1}",
					Action:      LabelMap,
				},
			},
			output: labels.FromMap(map[string]string{
				"a":             "foo",
				"__meta_my_bar": "aaa",
				"__meta_my_baz": "bbb",
				"__meta_other":  "ccc",
				"my_bar":        "aaa",
				"my_baz":        "bbb",
			}),
		},
		{ // valid case
			input: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${2}",
					TargetLabel:  "${1}",
				},
			},
			output: labels.FromMap(map[string]string{
				"a":    "some-name-value",
				"name": "value",
			}),
		},
		{ // invalid replacement ""
			input: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${3}",
					TargetLabel:  "${1}",
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
		},
		{ // invalid target_labels
			input: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "0${3}",
				},
				{
					SourceLabels: model.LabelNames{"a"},
					Regex:        MustNewRegexp("some-([^-]+)-([^,]+)"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "-${3}",
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "some-name-value",
			}),
		},
		{ // more complex real-life like usecase
			input: labels.FromMap(map[string]string{
				"__meta_sd_tags": "path:/secret,job:some-job,label:foo=bar",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        MustNewRegexp("(?:.+,|^)path:(/[^,]+).*"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "__metrics_path__",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        MustNewRegexp("(?:.+,|^)job:([^,]+).*"),
					Action:       Replace,
					Replacement:  "${1}",
					TargetLabel:  "job",
				},
				{
					SourceLabels: model.LabelNames{"__meta_sd_tags"},
					Regex:        MustNewRegexp("(?:.+,|^)label:([^=]+)=([^,]+).*"),
					Action:       Replace,
					Replacement:  "${2}",
					TargetLabel:  "${1}",
				},
			},
			output: labels.FromMap(map[string]string{
				"__meta_sd_tags":   "path:/secret,job:some-job,label:foo=bar",
				"__metrics_path__": "/secret",
				"job":              "some-job",
				"foo":              "bar",
			}),
		},
		{ // From https://github.com/prometheus/prometheus/issues/12283
			input: labels.FromMap(map[string]string{
				"__meta_kubernetes_pod_container_port_name":         "foo",
				"__meta_kubernetes_pod_annotation_XXX_metrics_port": "9091",
			}),
			relabel: []*Config{
				{
					Regex:  MustNewRegexp("^__meta_kubernetes_pod_container_port_name$"),
					Action: LabelDrop,
				},
				{
					SourceLabels: model.LabelNames{"__meta_kubernetes_pod_annotation_XXX_metrics_port"},
					Regex:        MustNewRegexp("(.+)"),
					Action:       Replace,
					Replacement:  "metrics",
					TargetLabel:  "__meta_kubernetes_pod_container_port_name",
				},
				{
					SourceLabels: model.LabelNames{"__meta_kubernetes_pod_container_port_name"},
					Regex:        MustNewRegexp("^metrics$"),
					Action:       Keep,
				},
			},
			output: labels.FromMap(map[string]string{
				"__meta_kubernetes_pod_annotation_XXX_metrics_port": "9091",
				"__meta_kubernetes_pod_container_port_name":         "metrics",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*Config{
				{
					Regex:  MustNewRegexp("(b.*)"),
					Action: LabelKeep,
				},
			},
			output: labels.FromMap(map[string]string{
				"b1": "bar",
				"b2": "baz",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"a":  "foo",
				"b1": "bar",
				"b2": "baz",
			}),
			relabel: []*Config{
				{
					Regex:  MustNewRegexp("(b.*)"),
					Action: LabelDrop,
				},
			},
			output: labels.FromMap(map[string]string{
				"a": "foo",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"foo": "bAr123Foo",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"foo"},
					Action:       Uppercase,
					TargetLabel:  "foo_uppercase",
				},
				{
					SourceLabels: model.LabelNames{"foo"},
					Action:       Lowercase,
					TargetLabel:  "foo_lowercase",
				},
			},
			output: labels.FromMap(map[string]string{
				"foo":           "bAr123Foo",
				"foo_lowercase": "bar123foo",
				"foo_uppercase": "BAR123FOO",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       KeepEqual,
					TargetLabel:  "__port1",
				},
			},
			output: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       DropEqual,
					TargetLabel:  "__port1",
				},
			},
			drop: true,
		},
		{
			input: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       DropEqual,
					TargetLabel:  "__port2",
				},
			},
			output: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
		},
		{
			input: labels.FromMap(map[string]string{
				"__tmp_port": "1234",
				"__port1":    "1234",
				"__port2":    "5678",
			}),
			relabel: []*Config{
				{
					SourceLabels: model.LabelNames{"__tmp_port"},
					Action:       KeepEqual,
					TargetLabel:  "__port2",
				},
			},
			drop: true,
		},
	}

	for _, test := range tests {
		// Setting default fields, mimicking the behaviour in Prometheus.
		for _, cfg := range test.relabel {
			if cfg.Action == "" {
				cfg.Action = DefaultRelabelConfig.Action
			}
			if cfg.Separator == "" {
				cfg.Separator = DefaultRelabelConfig.Separator
			}
			if cfg.Regex.Regexp == nil || cfg.Regex.String() == "" {
				cfg.Regex = DefaultRelabelConfig.Regex
			}
			if cfg.Replacement == "" {
				cfg.Replacement = DefaultRelabelConfig.Replacement
			}
		}

		res, keep := Process(test.input, test.relabel...)
		require.Equal(t, !test.drop, keep)
		if keep {
			require.Equal(t, test.output, res)
		}
	}
}

func TestTargetLabelValidity(t *testing.T) {
	tests := []struct {
		str   string
		valid bool
	}{
		{"-label", false},
		{"label", true},
		{"label${1}", true},
		{"${1}label", true},
		{"${1}", true},
		{"${1}label", true},
		{"${", false},
		{"$", false},
		{"${}", false},
		{"foo${", false},
		{"$1", true},
		{"asd$2asd", true},
		{"-foo${1}bar-", false},
		{"_${1}_", true},
		{"foo${bar}foo", true},
	}
	for _, test := range tests {
		require.Equal(t, test.valid, relabelTarget.Match([]byte(test.str)),
			"Expected %q to be %v", test.str, test.valid)
	}
}

func BenchmarkRelabel(b *testing.B) {
	tests := []struct {
		name   string
		lbls   labels.Labels
		config string
		cfgs   []*Config
	}{
		{
			name: "example", // From prometheus/config/testdata/conf.good.yml.
			config: `
            - source_labels: [job, __meta_dns_name]
              regex: "(.*)some-[regex]"
              target_label: job
              replacement: foo-${1}
              # action defaults to 'replace'
            - source_labels: [abc]
              target_label: cde
            - replacement: static
              target_label: abc
            - regex:
              replacement: static
              target_label: abc`,
			lbls: labels.FromStrings("__meta_dns_name", "example-some-x.com", "abc", "def", "job", "foo"),
		},
		{
			name: "kubernetes",
			config: `
        - source_labels:
            - __meta_kubernetes_pod_container_port_name
          regex: .*-metrics
          action: keep
        - source_labels:
            - __meta_kubernetes_pod_label_name
          action: drop
          regex: ""
        - source_labels:
            - __meta_kubernetes_pod_phase
          regex: Succeeded|Failed
          action: drop
        - source_labels:
            - __meta_kubernetes_pod_annotation_prometheus_io_scrape
          regex: "false"
          action: drop
        - source_labels:
            - __meta_kubernetes_pod_annotation_prometheus_io_scheme
          target_label: __scheme__
          regex: (https?)
          replacement: $1
          action: replace
        - source_labels:
            - __meta_kubernetes_pod_annotation_prometheus_io_path
          target_label: __metrics_path__
          regex: (.+)
          replacement: $1
          action: replace
        - source_labels:
            - __address__
            - __meta_kubernetes_pod_annotation_prometheus_io_port
          target_label: __address__
          regex: (.+?)(\:\d+)?;(\d+)
          replacement: $1:$3
          action: replace
        - regex: __meta_kubernetes_pod_annotation_prometheus_io_param_(.+)
          replacement: __param_$1
          action: labelmap
        - regex: __meta_kubernetes_pod_label_prometheus_io_label_(.+)
          action: labelmap
        - regex: __meta_kubernetes_pod_annotation_prometheus_io_label_(.+)
          action: labelmap
        - source_labels:
            - __meta_kubernetes_namespace
            - __meta_kubernetes_pod_label_name
          separator: /
          target_label: job
          replacement: $1
          action: replace
        - source_labels:
          - __meta_kubernetes_namespace
          target_label: namespace
          action: replace
        - source_labels:
            - __meta_kubernetes_pod_name
          target_label: pod
          action: replace
        - source_labels:
            - __meta_kubernetes_pod_container_name
          target_label: container
          action: replace
        - source_labels:
            - __meta_kubernetes_pod_name
            - __meta_kubernetes_pod_container_name
            - __meta_kubernetes_pod_container_port_name
          separator: ':'
          target_label: instance
          action: replace
        - target_label: cluster
          replacement: dev-us-central-0
        - source_labels:
            - __meta_kubernetes_namespace
          regex: hosted-grafana
          action: drop
        - source_labels:
            - __address__
          target_label: __tmp_hash
          modulus: 3
          action: hashmod
        - source_labels:
            - __tmp_hash
          regex: ^0$
          action: keep
        - regex: __tmp_hash
          action: labeldrop`,
			lbls: labels.FromStrings(
				"__address__", "10.132.183.40:80",
				"__meta_kubernetes_namespace", "loki-boltdb-shipper",
				"__meta_kubernetes_pod_annotation_promtail_loki_boltdb_shipper_hash", "50523b9759094a144adcec2eae0aa4ad",
				"__meta_kubernetes_pod_annotationpresent_promtail_loki_boltdb_shipper_hash", "true",
				"__meta_kubernetes_pod_container_init", "false",
				"__meta_kubernetes_pod_container_name", "promtail",
				"__meta_kubernetes_pod_container_port_name", "http-metrics",
				"__meta_kubernetes_pod_container_port_number", "80",
				"__meta_kubernetes_pod_container_port_protocol", "TCP",
				"__meta_kubernetes_pod_controller_kind", "DaemonSet",
				"__meta_kubernetes_pod_controller_name", "promtail-loki-boltdb-shipper",
				"__meta_kubernetes_pod_host_ip", "10.128.0.178",
				"__meta_kubernetes_pod_ip", "10.132.183.40",
				"__meta_kubernetes_pod_label_controller_revision_hash", "555b77cd7d",
				"__meta_kubernetes_pod_label_name", "promtail-loki-boltdb-shipper",
				"__meta_kubernetes_pod_label_pod_template_generation", "45",
				"__meta_kubernetes_pod_labelpresent_controller_revision_hash", "true",
				"__meta_kubernetes_pod_labelpresent_name", "true",
				"__meta_kubernetes_pod_labelpresent_pod_template_generation", "true",
				"__meta_kubernetes_pod_name", "promtail-loki-boltdb-shipper-jgtr7",
				"__meta_kubernetes_pod_node_name", "gke-dev-us-central-0-main-n2s8-2-14d53341-9hkr",
				"__meta_kubernetes_pod_phase", "Running",
				"__meta_kubernetes_pod_ready", "true",
				"__meta_kubernetes_pod_uid", "4c586419-7f6c-448d-aeec-ca4fa5b05e60",
				"__metrics_path__", "/metrics",
				"__scheme__", "http",
				"__scrape_interval__", "15s",
				"__scrape_timeout__", "10s",
				"job", "kubernetes-pods"),
		},
	}
	for i := range tests {
		err := yaml.UnmarshalStrict([]byte(tests[i].config), &tests[i].cfgs)
		require.NoError(b, err)
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = Process(tt.lbls, tt.cfgs...)
			}
		})
	}
}
