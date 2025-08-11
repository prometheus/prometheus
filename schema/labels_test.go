// Copyright 2025 The Prometheus Authors
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

package schema

import (
	"fmt"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMetadata(t *testing.T) {
	testMeta := Metadata{
		Name:         "metric_total",
		Type:         model.MetricTypeCounter,
		Unit:         "seconds",
		Temporality:  "delta",
		Monotonicity: "true",
	}

	for _, tcase := range []struct {
		emptyName, emptyType, emptyUnit, emptyTemporality, emptyMonotonicity bool
	}{
		{},
		{emptyName: true},
		{emptyType: true},
		{emptyUnit: true},
		{emptyTemporality: true},
		{emptyMonotonicity: true},
		{emptyName: true, emptyType: true, emptyUnit: true, emptyTemporality: true},
	} {
		var (
			expectedMeta   Metadata
			expectedLabels labels.Labels
		)
		{
			// Setup expectations.
			lb := labels.NewScratchBuilder(0)
			lb.Add("foo", "bar")

			if !tcase.emptyName {
				lb.Add(metricName, testMeta.Name)
				expectedMeta.Name = testMeta.Name
			}
			if !tcase.emptyType {
				lb.Add(metricType, string(testMeta.Type))
				expectedMeta.Type = testMeta.Type
			} else {
				expectedMeta.Type = model.MetricTypeUnknown
			}
			if !tcase.emptyUnit {
				lb.Add(metricUnit, testMeta.Unit)
				expectedMeta.Unit = testMeta.Unit
			}
			if !tcase.emptyTemporality {
				lb.Add(metricTemporality, testMeta.Temporality)
				expectedMeta.Temporality = testMeta.Temporality
			}
			if !tcase.emptyMonotonicity {
				lb.Add(metricMonotonicity, testMeta.Monotonicity)
				expectedMeta.Monotonicity = testMeta.Monotonicity
			}
			lb.Sort()
			expectedLabels = lb.Labels()
		}

		t.Run(fmt.Sprintf("meta=%#v", expectedMeta), func(t *testing.T) {
			{
				// From labels to Metadata.
				got := NewMetadataFromLabels(expectedLabels)
				require.Equal(t, expectedMeta, got)
			}
			{
				// Empty methods.
				require.Equal(t, tcase.emptyName, expectedMeta.IsEmptyFor(metricName))
				require.Equal(t, tcase.emptyType, expectedMeta.IsEmptyFor(metricType))
				require.Equal(t, tcase.emptyType, expectedMeta.IsTypeEmpty())
				require.Equal(t, tcase.emptyUnit, expectedMeta.IsEmptyFor(metricUnit))
				require.Equal(t, tcase.emptyTemporality, expectedMeta.IsEmptyFor(metricTemporality))
				require.Equal(t, tcase.emptyMonotonicity, expectedMeta.IsEmptyFor(metricMonotonicity))
			}
			{
				// From Metadata to labels for various builders.
				slb := labels.NewScratchBuilder(0)
				slb.Add("foo", "bar")
				expectedMeta.AddToLabels(&slb)
				slb.Sort()
				testutil.RequireEqual(t, expectedLabels, slb.Labels())

				lb := labels.NewBuilder(labels.FromStrings("foo", "bar"))
				expectedMeta.SetToLabels(lb)
				testutil.RequireEqual(t, expectedLabels, lb.Labels())
			}
		})
	}
}

func TestIgnoreOverriddenMetadataLabelsScratchBuilder(t *testing.T) {
	// PROM-39 specifies that metadata labels should be sourced primarily from the metadata structures.
	// However, the original labels should be preserved IF the metadata structure does not set or support certain information.
	// Test those cases with common label interactions.
	incomingLabels := labels.FromStrings(metricName, "different_name", metricType, string(model.MetricTypeSummary), metricUnit, "MB", metricTemporality, "cumulative", "foo", "bar")
	for _, tcase := range []struct {
		highPrioMeta   Metadata
		expectedLabels labels.Labels
	}{
		{
			expectedLabels: incomingLabels,
		},
		{
			highPrioMeta: Metadata{
				Name:         "metric_total",
				Type:         model.MetricTypeCounter,
				Unit:         "seconds",
				Temporality:  "delta",
				Monotonicity: "true",
			},
			expectedLabels: labels.FromStrings(metricName, "metric_total", metricType, string(model.MetricTypeCounter), metricUnit, "seconds", metricTemporality, "delta", metricMonotonicity, "true", "foo", "bar"),
		},
		{
			highPrioMeta: Metadata{
				Name: "metric_total",
				Type: model.MetricTypeCounter,
			},
			expectedLabels: labels.FromStrings(metricName, "metric_total", metricType, string(model.MetricTypeCounter), metricUnit, "MB", metricTemporality, "cumulative", "foo", "bar"),
		},
		{
			highPrioMeta: Metadata{
				Type: model.MetricTypeCounter,
				Unit: "seconds",
			},
			expectedLabels: labels.FromStrings(metricName, "different_name", metricType, string(model.MetricTypeCounter), metricUnit, "seconds", metricTemporality, "cumulative", "foo", "bar"),
		},
		{
			highPrioMeta: Metadata{
				Name: "metric_total",
				Type: model.MetricTypeUnknown,
				Unit: "seconds",
			},
			expectedLabels: labels.FromStrings(metricName, "metric_total", metricType, string(model.MetricTypeSummary), metricUnit, "seconds", metricTemporality, "cumulative", "foo", "bar"),
		},
	} {
		t.Run(fmt.Sprintf("meta=%#v", tcase.highPrioMeta), func(t *testing.T) {
			lb := labels.NewScratchBuilder(0)
			tcase.highPrioMeta.AddToLabels(&lb)
			wrapped := &IgnoreOverriddenMetadataLabelsScratchBuilder{ScratchBuilder: &lb, Overwrite: tcase.highPrioMeta}
			incomingLabels.Range(func(l labels.Label) {
				wrapped.Add(l.Name, l.Value)
			})
			lb.Sort()
			require.Equal(t, tcase.expectedLabels, lb.Labels())
		})
	}
}

func TestIsMetadataLabel(t *testing.T) {
	testCases := []struct {
		label    string
		expected bool
	}{
		{"__name__", true},
		{"__type__", true},
		{"__unit__", true},
		{"__temporality__", true},
		{"__monotonicity__", true},
		{"foo", false},
		{"bar", false},
		{"__other__", false},
	}

	for _, tc := range testCases {
		t.Run(tc.label, func(t *testing.T) {
			result := IsMetadataLabel(tc.label)
			require.Equal(t, tc.expected, result)
		})
	}
}
