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

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestLabelsToOpenAPILabelSet(t *testing.T) {
	require.Equal(t, models.LabelSet{"aaa": "111", "bbb": "222"}, labelsToOpenAPILabelSet(labels.FromStrings("aaa", "111", "bbb", "222")))
}

// Edge case tests for utility functions

func TestLabelsToOpenAPILabelSetEmpty(t *testing.T) {
	result := labelsToOpenAPILabelSet(labels.EmptyLabels())
	require.Empty(t, result)
}

func TestLabelsToOpenAPILabelSetSpecialCharacters(t *testing.T) {
	result := labelsToOpenAPILabelSet(labels.FromStrings(
		"special/chars", "value with spaces",
		"unicode", "αβγ",
		"empty", "",
	))

	expected := models.LabelSet{
		"special/chars": "value with spaces",
		"unicode":       "αβγ",
		"empty":         "",
	}
	require.Equal(t, expected, result)
}

func TestAlertsToOpenAPIAlertsEmpty(t *testing.T) {
	result := alertsToOpenAPIAlerts([]*Alert{})
	require.Empty(t, result)
}

func TestAlertsToOpenAPIAlertsNil(t *testing.T) {
	result := alertsToOpenAPIAlerts(nil)
	require.Empty(t, result)
}

func TestAlertsToOpenAPIAlertsSingle(t *testing.T) {
	now := time.Now()
	alert := &Alert{
		Labels:       labels.FromStrings("alertname", "test", "severity", "critical"),
		Annotations:  labels.FromStrings("summary", "Test alert"),
		StartsAt:     now,
		EndsAt:       now.Add(time.Hour),
		GeneratorURL: "http://prometheus:9090/graph",
	}

	result := alertsToOpenAPIAlerts([]*Alert{alert})
	require.Len(t, result, 1)

	apiAlert := result[0]
	require.Equal(t, "test", apiAlert.Labels["alertname"])
	require.Equal(t, "critical", apiAlert.Labels["severity"])
	require.Equal(t, "Test alert", apiAlert.Annotations["summary"])
	require.Equal(t, "http://prometheus:9090/graph", string(apiAlert.GeneratorURL))
}

func TestAlertsToOpenAPIAlertsMultiple(t *testing.T) {
	now := time.Now()
	alerts := []*Alert{
		{
			Labels:      labels.FromStrings("alertname", "alert1"),
			Annotations: labels.FromStrings("desc", "First alert"),
			StartsAt:    now,
			EndsAt:      now.Add(time.Hour),
		},
		{
			Labels:      labels.FromStrings("alertname", "alert2"),
			Annotations: labels.FromStrings("desc", "Second alert"),
			StartsAt:    now.Add(time.Minute),
			EndsAt:      now.Add(2 * time.Hour),
		},
	}

	result := alertsToOpenAPIAlerts(alerts)
	require.Len(t, result, 2)

	require.Equal(t, "alert1", result[0].Labels["alertname"])
	require.Equal(t, "alert2", result[1].Labels["alertname"])
	require.Equal(t, "First alert", result[0].Annotations["desc"])
	require.Equal(t, "Second alert", result[1].Annotations["desc"])
}

func TestAlertsToOpenAPIAlertsEmptyFields(t *testing.T) {
	alert := &Alert{
		Labels:       labels.EmptyLabels(),
		Annotations:  labels.EmptyLabels(),
		StartsAt:     time.Time{},
		EndsAt:       time.Time{},
		GeneratorURL: "",
	}

	result := alertsToOpenAPIAlerts([]*Alert{alert})
	require.Len(t, result, 1)

	apiAlert := result[0]
	require.Empty(t, apiAlert.Labels)
	require.Empty(t, apiAlert.Annotations)
	require.Empty(t, string(apiAlert.GeneratorURL))
}
