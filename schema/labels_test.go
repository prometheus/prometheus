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
		Name: "metric_total",
		Type: model.MetricTypeCounter,
		Unit: "seconds",
	}

	for _, tcase := range []struct {
		emptyName, emptyType, emptyUnit bool
	}{
		{},
		{emptyName: true},
		{emptyType: true},
		{emptyUnit: true},
		{emptyName: true, emptyType: true, emptyUnit: true},
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
				lb.Add(model.MetricNameLabel, testMeta.Name)
				expectedMeta.Name = testMeta.Name
			}
			if !tcase.emptyType {
				lb.Add(model.MetricTypeLabel, string(testMeta.Type))
				expectedMeta.Type = testMeta.Type
			} else {
				expectedMeta.Type = model.MetricTypeUnknown
			}
			if !tcase.emptyUnit {
				lb.Add(model.MetricUnitLabel, testMeta.Unit)
				expectedMeta.Unit = testMeta.Unit
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
				require.Equal(t, tcase.emptyName, expectedMeta.IsEmptyFor(model.MetricNameLabel))
				require.Equal(t, tcase.emptyType, expectedMeta.IsEmptyFor(model.MetricTypeLabel))
				require.Equal(t, tcase.emptyType, expectedMeta.IsTypeEmpty())
				require.Equal(t, tcase.emptyUnit, expectedMeta.IsEmptyFor(model.MetricUnitLabel))
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
	incomingLabels := labels.FromStrings(model.MetricNameLabel, "different_name", model.MetricTypeLabel, string(model.MetricTypeSummary), model.MetricUnitLabel, "MB", "foo", "bar")
	for _, tcase := range []struct {
		highPrioMeta   Metadata
		expectedLabels labels.Labels
	}{
		{
			expectedLabels: incomingLabels,
		},
		{
			highPrioMeta: Metadata{
				Name: "metric_total",
				Type: model.MetricTypeCounter,
				Unit: "seconds",
			},
			expectedLabels: labels.FromStrings(model.MetricNameLabel, "metric_total", model.MetricTypeLabel, string(model.MetricTypeCounter), model.MetricUnitLabel, "seconds", "foo", "bar"),
		},
		{
			highPrioMeta: Metadata{
				Name: "metric_total",
				Type: model.MetricTypeCounter,
			},
			expectedLabels: labels.FromStrings(model.MetricNameLabel, "metric_total", model.MetricTypeLabel, string(model.MetricTypeCounter), model.MetricUnitLabel, "MB", "foo", "bar"),
		},
		{
			highPrioMeta: Metadata{
				Type: model.MetricTypeCounter,
				Unit: "seconds",
			},
			expectedLabels: labels.FromStrings(model.MetricNameLabel, "different_name", model.MetricTypeLabel, string(model.MetricTypeCounter), model.MetricUnitLabel, "seconds", "foo", "bar"),
		},
		{
			highPrioMeta: Metadata{
				Name: "metric_total",
				Type: model.MetricTypeUnknown,
				Unit: "seconds",
			},
			expectedLabels: labels.FromStrings(model.MetricNameLabel, "metric_total", model.MetricTypeLabel, string(model.MetricTypeSummary), model.MetricUnitLabel, "seconds", "foo", "bar"),
		},
	} {
		t.Run(fmt.Sprintf("meta=%#v", tcase.highPrioMeta), func(t *testing.T) {
			lb := labels.NewScratchBuilder(0)
			tcase.highPrioMeta.AddToLabels(&lb)
			wrapped := tcase.highPrioMeta.NewIgnoreOverriddenMetadataLabelScratchBuilder(&lb)
			incomingLabels.Range(func(l labels.Label) {
				wrapped.Add(l.Name, l.Value)
			})
			lb.Sort()
			require.Equal(t, tcase.expectedLabels, lb.Labels())
		})
	}
}
