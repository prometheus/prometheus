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

package semconv

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchChangelog(t *testing.T) {
	testdataElementsChanges := []change{
		{
			Forward: metricGroupChange{
				MetricName:  "",
				Unit:        "",
				ValuePromQL: "",
				Attributes:  []attribute{{Tag: "my_number"}},
			},
			Backward: metricGroupChange{
				MetricName:  "",
				Unit:        "",
				ValuePromQL: "",
				Attributes:  []attribute{{Tag: "number"}},
			},
		},
		{
			Forward: metricGroupChange{
				MetricName:  "my_app_custom_changed_elements_total",
				Unit:        "",
				ValuePromQL: "",
				Attributes:  []attribute{{Tag: "number"}, {Tag: "class", Members: []attributeMember{{Value: "FIRST"}, {Value: "SECOND"}, {Value: "OTHER"}}}},
			},
			Backward: metricGroupChange{
				MetricName:  "my_app_custom_elements_total",
				Unit:        "",
				ValuePromQL: "",
				Attributes:  []attribute{{Tag: "integer"}, {Tag: "category", Members: []attributeMember{{Value: "first"}, {Value: "second"}, {Value: "other"}}}},
			},
		},
	}
	testdataLatencyChanges := []change{
		{
			Forward:  metricGroupChange{MetricName: "my_app_latency_seconds", Unit: "{second}", ValuePromQL: "value{} / 1000"},
			Backward: metricGroupChange{MetricName: "my_app_latency_milliseconds", Unit: "{millisecond}", ValuePromQL: "value{} * 1000"},
		},
	}
	expected := &changelog{
		Version: 1,
		MetricsChangelog: map[semanticMetricID][]change{
			"my_app_custom_elements": testdataElementsChanges,
			"my_app_latency":         testdataLatencyChanges,
		},
	}

	t.Run("local", func(t *testing.T) {
		e := newSchemaEngine()
		got, err := e.fetchChangelog("./testdata/1.1.0")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("http", func(t *testing.T) {
		srv := httptest.NewServer(http.FileServer(http.Dir("./testdata")))
		t.Cleanup(srv.Close)

		e := newSchemaEngine()
		got, err := e.fetchChangelog(srv.URL + "/1.1.0")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("override", func(t *testing.T) {
		e := newSchemaEngine()
		e.schemaBaseOverride["https://bwplotka.dev/YOLO"] = "./testdata"

		got, err := e.fetchChangelog("https://bwplotka.dev/YOLO/1.1.0")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
}

func TestSchemaEngine_FetchIDs(t *testing.T) {
	expected := &ids{
		Version: 1,
		MetricsIDs: map[string][]versionedID{
			"my_app_latency_seconds~seconds.histogram": {
				{
					ID:           "my_app_latency.2",
					IntroVersion: "1.1.0",
				},
			},
			"my_app_custom_changed_elements_total~elements.counter": {
				{
					ID:           "my_app_custom_elements.3",
					IntroVersion: "1.2.0",
				},
				{
					ID:           "my_app_custom_elements.2",
					IntroVersion: "1.1.0",
				},
			},
			"my_app_latency_milliseconds~milliseconds.histogram": {
				{
					ID:           "my_app_latency",
					IntroVersion: "1.0.0",
				},
			},
			"my_app_custom_elements_total~elements.counter": {
				{
					ID:           "my_app_custom_elements",
					IntroVersion: "1.0.0",
				},
			},
			"my_app_some_elements~elements.gauge": {
				{
					ID:           "my_app_some_elements",
					IntroVersion: "1.0.0",
				},
			},
		},
		uniqueNameToIdentity: map[string]string{
			"my_app_custom_changed_elements_total": "my_app_custom_changed_elements_total~elements.counter",
			"my_app_custom_elements_total":         "my_app_custom_elements_total~elements.counter",
			"my_app_latency_milliseconds":          "my_app_latency_milliseconds~milliseconds.histogram",
			"my_app_latency_seconds":               "my_app_latency_seconds~seconds.histogram",
			"my_app_some_elements":                 "my_app_some_elements~elements.gauge",
		},
		uniqueNameTypeToIdentity: map[string]string{
			"my_app_custom_changed_elements_total.counter": "my_app_custom_changed_elements_total~elements.counter",
			"my_app_custom_elements_total.counter":         "my_app_custom_elements_total~elements.counter",
			"my_app_latency_milliseconds.histogram":        "my_app_latency_milliseconds~milliseconds.histogram",
			"my_app_latency_seconds.histogram":             "my_app_latency_seconds~seconds.histogram",
			"my_app_some_elements.gauge":                   "my_app_some_elements~elements.gauge",
		},
	}

	t.Run("local", func(t *testing.T) {
		e := newSchemaEngine()
		got, err := e.fetchIDs("./testdata/1.1.0")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("http", func(t *testing.T) {
		srv := httptest.NewServer(http.FileServer(http.Dir("./testdata")))
		t.Cleanup(srv.Close)

		e := newSchemaEngine()
		got, err := e.fetchIDs(srv.URL + "/1.1.0")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("override", func(t *testing.T) {
		e := newSchemaEngine()
		e.schemaBaseOverride["https://bwplotka.dev/YOLO"] = "./testdata"

		got, err := e.fetchIDs("https://bwplotka.dev/YOLO/1.1.0")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
}

func TestSemanticMetricID(t *testing.T) {
	gotID, gotRev := metricID("my_app_custom_elements").semanticID()
	require.Equal(t, semanticMetricID("my_app_custom_elements"), gotID)
	require.Equal(t, 0, gotRev)

	gotID, gotRev = metricID("my_app_custom_elements.yolo").semanticID()
	require.Equal(t, semanticMetricID("my_app_custom_elements.yolo"), gotID)
	require.Equal(t, 0, gotRev)

	gotID, gotRev = metricID("my_app_custom_elements.2").semanticID()
	require.Equal(t, semanticMetricID("my_app_custom_elements"), gotID)
	require.Equal(t, 2, gotRev)
}
