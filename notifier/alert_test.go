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

package notifier

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

func TestAlertName(t *testing.T) {
	a := &Alert{Labels: labels.FromStrings("alertname", "TestAlert", "severity", "critical")}
	require.Equal(t, "TestAlert", a.Name())
}

func TestAlertNameMissing(t *testing.T) {
	a := &Alert{Labels: labels.FromStrings("severity", "critical")}
	require.Empty(t, a.Name())
}

func TestAlertHash(t *testing.T) {
	a := &Alert{Labels: labels.FromStrings("alertname", "TestAlert")}
	b := &Alert{Labels: labels.FromStrings("alertname", "TestAlert")}
	require.Equal(t, a.Hash(), b.Hash())

	c := &Alert{Labels: labels.FromStrings("alertname", "DifferentAlert")}
	require.NotEqual(t, a.Hash(), c.Hash())
}

func TestAlertString(t *testing.T) {
	// An active alert (EndsAt is zero) should contain "[active]".
	a := &Alert{Labels: labels.FromStrings("alertname", "TestAlert")}
	require.Contains(t, a.String(), "TestAlert")
	require.Contains(t, a.String(), "[active]")

	// A resolved alert (EndsAt in the past) should contain "[resolved]".
	resolved := &Alert{
		Labels: labels.FromStrings("alertname", "TestAlert"),
		EndsAt: time.Now().Add(-time.Hour),
	}
	require.Contains(t, resolved.String(), "[resolved]")
}

func TestAlertResolved(t *testing.T) {
	// EndsAt is zero — alert is not resolved.
	a := &Alert{}
	require.False(t, a.Resolved())

	// EndsAt in the past — alert is resolved.
	a = &Alert{EndsAt: time.Now().Add(-time.Minute)}
	require.True(t, a.Resolved())

	// EndsAt in the future — alert is not resolved.
	a = &Alert{EndsAt: time.Now().Add(time.Hour)}
	require.False(t, a.Resolved())
}

func TestAlertResolvedAt(t *testing.T) {
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// EndsAt is zero — never resolved.
	a := &Alert{}
	require.False(t, a.ResolvedAt(ts))

	// EndsAt before ts — resolved at ts.
	a = &Alert{EndsAt: ts.Add(-time.Minute)}
	require.True(t, a.ResolvedAt(ts))

	// EndsAt equal to ts — resolved at ts.
	a = &Alert{EndsAt: ts}
	require.True(t, a.ResolvedAt(ts))

	// EndsAt after ts — not resolved at ts.
	a = &Alert{EndsAt: ts.Add(time.Minute)}
	require.False(t, a.ResolvedAt(ts))
}

func TestRelabelAlerts(t *testing.T) {
	tests := []struct {
		name           string
		relabelConfigs []*relabel.Config
		externalLabels labels.Labels
		alerts         []*Alert
		expected       []*Alert
	}{
		{
			name:           "nil configs and nil alerts returns nil",
			relabelConfigs: nil,
			externalLabels: labels.EmptyLabels(),
			alerts:         nil,
			expected:       nil,
		},
		{
			name:           "empty configs and empty alerts returns nil",
			relabelConfigs: []*relabel.Config{},
			externalLabels: labels.EmptyLabels(),
			alerts:         []*Alert{},
			expected:       nil,
		},
		{
			name: "no-op relabel config preserves all alerts",
			relabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"alertname"},
					TargetLabel:          "alertname",
					Action:               relabel.Replace,
					Regex:                relabel.MustNewRegexp("(.*)"),
					Replacement:          "$1",
					NameValidationScheme: model.UTF8Validation,
				},
			},
			externalLabels: labels.EmptyLabels(),
			alerts: []*Alert{
				{Labels: labels.FromStrings("alertname", "TestAlert")},
			},
			expected: []*Alert{
				{Labels: labels.FromStrings("alertname", "TestAlert")},
			},
		},
		{
			name: "drop action removes matching alerts",
			relabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"alertname"},
					Action:               relabel.Drop,
					Regex:                relabel.MustNewRegexp("drop_me"),
					NameValidationScheme: model.UTF8Validation,
				},
			},
			externalLabels: labels.EmptyLabels(),
			alerts: []*Alert{
				{Labels: labels.FromStrings("alertname", "drop_me")},
				{Labels: labels.FromStrings("alertname", "keep_me")},
			},
			expected: []*Alert{
				{Labels: labels.FromStrings("alertname", "keep_me")},
			},
		},
		{
			name: "drop action removes all alerts when all match",
			relabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"alertname"},
					Action:               relabel.Drop,
					Regex:                relabel.MustNewRegexp(".*"),
					NameValidationScheme: model.UTF8Validation,
				},
			},
			externalLabels: labels.EmptyLabels(),
			alerts: []*Alert{
				{Labels: labels.FromStrings("alertname", "alert1")},
				{Labels: labels.FromStrings("alertname", "alert2")},
			},
			expected: nil,
		},
		{
			name: "replace action modifies matching labels",
			relabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"alertname"},
					TargetLabel:          "alertname",
					Action:               relabel.Replace,
					Regex:                relabel.MustNewRegexp("old_name"),
					Replacement:          "new_name",
					NameValidationScheme: model.UTF8Validation,
				},
			},
			externalLabels: labels.EmptyLabels(),
			alerts: []*Alert{
				{Labels: labels.FromStrings("alertname", "old_name")},
				{Labels: labels.FromStrings("alertname", "unchanged")},
			},
			expected: []*Alert{
				{Labels: labels.FromStrings("alertname", "new_name")},
				{Labels: labels.FromStrings("alertname", "unchanged")},
			},
		},
		{
			name:           "external labels applied when absent from alert",
			relabelConfigs: []*relabel.Config{},
			externalLabels: labels.FromStrings("cluster", "us-east-1", "env", "prod"),
			alerts: []*Alert{
				{Labels: labels.FromStrings("alertname", "TestAlert")},
			},
			expected: []*Alert{
				{Labels: labels.FromStrings("alertname", "TestAlert", "cluster", "us-east-1", "env", "prod")},
			},
		},
		{
			name:           "external labels do not override existing alert labels",
			relabelConfigs: []*relabel.Config{},
			externalLabels: labels.FromStrings("severity", "warning"),
			alerts: []*Alert{
				{Labels: labels.FromStrings("alertname", "TestAlert", "severity", "critical")},
			},
			expected: []*Alert{
				{Labels: labels.FromStrings("alertname", "TestAlert", "severity", "critical")},
			},
		},
		{
			name: "external labels applied before relabeling",
			relabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"cluster"},
					TargetLabel:          "region",
					Action:               relabel.Replace,
					Regex:                relabel.MustNewRegexp("(.*)"),
					Replacement:          "$1",
					NameValidationScheme: model.UTF8Validation,
				},
			},
			externalLabels: labels.FromStrings("cluster", "us-west-2"),
			alerts: []*Alert{
				{Labels: labels.FromStrings("alertname", "TestAlert")},
			},
			expected: []*Alert{
				{Labels: labels.FromStrings("alertname", "TestAlert", "cluster", "us-west-2", "region", "us-west-2")},
			},
		},
		{
			name: "annotations and other fields are preserved after relabeling",
			relabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"alertname"},
					TargetLabel:          "extra",
					Action:               relabel.Replace,
					Regex:                relabel.MustNewRegexp("(.*)"),
					Replacement:          "added",
					NameValidationScheme: model.UTF8Validation,
				},
			},
			externalLabels: labels.EmptyLabels(),
			alerts: []*Alert{
				{
					Labels:       labels.FromStrings("alertname", "TestAlert"),
					Annotations:  labels.FromStrings("summary", "something broke"),
					GeneratorURL: "http://localhost:9090/graph",
					StartsAt:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					EndsAt:       time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				},
			},
			expected: []*Alert{
				{
					Labels:       labels.FromStrings("alertname", "TestAlert", "extra", "added"),
					Annotations:  labels.FromStrings("summary", "something broke"),
					GeneratorURL: "http://localhost:9090/graph",
					StartsAt:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					EndsAt:       time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := relabelAlerts(tc.relabelConfigs, tc.externalLabels, tc.alerts)

			if tc.expected == nil {
				require.Nil(t, result)
				return
			}

			require.Len(t, result, len(tc.expected), "unexpected number of alerts")
			for i, exp := range tc.expected {
				require.True(t, labels.Equal(exp.Labels, result[i].Labels),
					"labels mismatch at index %d: expected %s, got %s", i, exp.Labels, result[i].Labels)
			}
		})
	}
}

func TestRelabelAlertsImmutability(t *testing.T) {
	// Verify that relabeling does not mutate the original alert's labels.
	original := &Alert{
		Labels:       labels.FromStrings("alertname", "TestAlert"),
		Annotations:  labels.FromStrings("summary", "test"),
		GeneratorURL: "http://localhost:9090",
	}
	originalLabels := original.Labels.Copy()

	relabelConfigs := []*relabel.Config{
		{
			SourceLabels:         model.LabelNames{"alertname"},
			TargetLabel:          "mutated",
			Action:               relabel.Replace,
			Regex:                relabel.MustNewRegexp("(.*)"),
			Replacement:          "yes",
			NameValidationScheme: model.UTF8Validation,
		},
	}

	result := relabelAlerts(relabelConfigs, labels.EmptyLabels(), []*Alert{original})

	// The result should have the new label.
	require.Len(t, result, 1)
	require.Equal(t, "yes", result[0].Labels.Get("mutated"))

	// The original alert's labels must not be modified.
	require.True(t, labels.Equal(originalLabels, original.Labels),
		"original alert labels were mutated: expected %s, got %s", originalLabels, original.Labels)

	// The result should be a different pointer since labels changed.
	require.NotSame(t, original, result[0])

	// But annotations, GeneratorURL should be preserved.
	require.True(t, labels.Equal(original.Annotations, result[0].Annotations))
	require.Equal(t, original.GeneratorURL, result[0].GeneratorURL)
}

func TestRelabelAlertsNoMutationWhenLabelsUnchanged(t *testing.T) {
	// When relabeling does not alter labels, the same pointer should be returned.
	original := &Alert{
		Labels: labels.FromStrings("alertname", "TestAlert"),
	}

	// A no-op relabel config that matches but replaces with the same value.
	relabelConfigs := []*relabel.Config{
		{
			SourceLabels:         model.LabelNames{"alertname"},
			TargetLabel:          "alertname",
			Action:               relabel.Replace,
			Regex:                relabel.MustNewRegexp("(.*)"),
			Replacement:          "$1",
			NameValidationScheme: model.UTF8Validation,
		},
	}

	result := relabelAlerts(relabelConfigs, labels.EmptyLabels(), []*Alert{original})
	require.Len(t, result, 1)

	// Same pointer expected since labels were not altered.
	require.Same(t, original, result[0])
}
