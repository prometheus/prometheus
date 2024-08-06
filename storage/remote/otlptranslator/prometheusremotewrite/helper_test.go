// Copyright 2024 The Prometheus Authors
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
package prometheusremotewrite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/prometheus/prometheus/prompb"
)

func TestCreateAttributes(t *testing.T) {
	resourceAttrs := map[string]string{
		"service.name":        "service name",
		"service.instance.id": "service ID",
		"existent-attr":       "resource value",
		// This one is for testing conflict with metric attribute.
		"metric-attr": "resource value",
		// This one is for testing conflict with auto-generated job attribute.
		"job": "resource value",
		// This one is for testing conflict with auto-generated instance attribute.
		"instance": "resource value",
	}

	resource := pcommon.NewResource()
	for k, v := range resourceAttrs {
		resource.Attributes().PutStr(k, v)
	}
	attrs := pcommon.NewMap()
	attrs.PutStr("__name__", "test_metric")
	attrs.PutStr("metric-attr", "metric value")

	testCases := []struct {
		name                      string
		promoteResourceAttributes []string
		expectedLabels            []prompb.Label
	}{
		{
			name:                      "Successful conversion without resource attribute promotion",
			promoteResourceAttributes: nil,
			expectedLabels: []prompb.Label{
				{
					Name:  "__name__",
					Value: "test_metric",
				},
				{
					Name:  "instance",
					Value: "service ID",
				},
				{
					Name:  "job",
					Value: "service name",
				},
				{
					Name:  "metric_attr",
					Value: "metric value",
				},
			},
		},
		{
			name:                      "Successful conversion with resource attribute promotion",
			promoteResourceAttributes: []string{"non-existent-attr", "existent-attr"},
			expectedLabels: []prompb.Label{
				{
					Name:  "__name__",
					Value: "test_metric",
				},
				{
					Name:  "instance",
					Value: "service ID",
				},
				{
					Name:  "job",
					Value: "service name",
				},
				{
					Name:  "metric_attr",
					Value: "metric value",
				},
				{
					Name:  "existent_attr",
					Value: "resource value",
				},
			},
		},
		{
			name:                      "Successful conversion with resource attribute promotion, conflicting resource attributes are ignored",
			promoteResourceAttributes: []string{"non-existent-attr", "existent-attr", "metric-attr", "job", "instance"},
			expectedLabels: []prompb.Label{
				{
					Name:  "__name__",
					Value: "test_metric",
				},
				{
					Name:  "instance",
					Value: "service ID",
				},
				{
					Name:  "job",
					Value: "service name",
				},
				{
					Name:  "existent_attr",
					Value: "resource value",
				},
				{
					Name:  "metric_attr",
					Value: "metric value",
				},
			},
		},
		{
			name:                      "Successful conversion with resource attribute promotion, attributes are only promoted once",
			promoteResourceAttributes: []string{"existent-attr", "existent-attr"},
			expectedLabels: []prompb.Label{
				{
					Name:  "__name__",
					Value: "test_metric",
				},
				{
					Name:  "instance",
					Value: "service ID",
				},
				{
					Name:  "job",
					Value: "service name",
				},
				{
					Name:  "existent_attr",
					Value: "resource value",
				},
				{
					Name:  "metric_attr",
					Value: "metric value",
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			settings := Settings{
				PromoteResourceAttributes: tc.promoteResourceAttributes,
			}
			lbls := createAttributes(resource, attrs, settings, nil, false)

			assert.ElementsMatch(t, lbls, tc.expectedLabels)
		})
	}
}
