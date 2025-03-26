package semconv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	testdataElementsChanges = []change{
		{
			Forward:  metricGroupChange{MetricName: "my_app_custom_elements_changed_total", Unit: "", ValuePromQL: "", Attributes: []attribute{{Tag: "number"}, {Tag: "class", Members: []attributeMember{{Value: "FIRST"}, {Value: "SECOND"}, {Value: "OTHER"}}}}},
			Backward: metricGroupChange{MetricName: "my_app_custom_elements_total", Unit: "", ValuePromQL: "", Attributes: []attribute{{Tag: "integer"}, {Tag: "category", Members: []attributeMember{{Value: "first"}, {Value: "second"}, {Value: "other"}}}}},
		},
	}
	testdataLatencyChanges = []change{
		{
			Forward:  metricGroupChange{MetricName: "my_app_latency_seconds", Unit: "{second}", ValuePromQL: "value{} / 1000"},
			Backward: metricGroupChange{MetricName: "my_app_latency_milliseconds", Unit: "{millisecond}", ValuePromQL: "value{} * 1000"},
		},
	}
)

func TestFetchChangelog(t *testing.T) {
	expected := &changelog{
		Version: 1,
		MetricsChangelog: map[semanticMetricID][]change{
			"my_app_custom_elements":      testdataElementsChanges,
			"my_app_latency":              testdataLatencyChanges,
			"my_app_some_elements_totals": nil,
		},
	}

	t.Run("local", func(t *testing.T) {
		got, err := fetchChangelog("./testdata/changelog.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	// TODO(bwplotka): Move to something Prometheus owns e.g. internal Prometheus repo path.
	t.Run("http", func(t *testing.T) {
		got, err := fetchChangelog("https://raw.githubusercontent.com/bwplotka/metric-rename-demo/refs/heads/diff/my-org/semconv/changelog.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("http-custom", func(t *testing.T) {
		got, err := fetchChangelog("https://bwplotka.dev/semconv/changelog.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
}

func TestFetchIDs(t *testing.T) {
	expected := &ids{
		Version: 1,
		MetricsIDs: map[string][]versionedID{
			"my_app_latency_seconds~second.histogram": {
				{ID: "my_app_latency.2",
					IntroVersion: "1.1.0"},
			},
			"my_app_custom_elements_changed_total~element.counter": {
				{
					ID:           "my_app_custom_elements.2",
					IntroVersion: "1.1.0",
				},
			},
			"my_app_latency_milliseconds~millisecond.histogram": {
				{
					ID:           "my_app_latency",
					IntroVersion: "1.0.0",
				},
			},
			"my_app_custom_elements_total~element.counter": {
				{
					ID:           "my_app_custom_elements",
					IntroVersion: "1.0.0",
				},
			},
			"my_app_some_elements~element.gauge": {
				{
					ID:           "my_app_some_elements",
					IntroVersion: "1.0.0",
				},
			},
		},
		uniqueNameToIdentity: map[string]string{
			"my_app_custom_elements_changed_total": "my_app_custom_elements_changed_total~element.counter",
			"my_app_custom_elements_total":         "my_app_custom_elements_total~element.counter",
			"my_app_latency_milliseconds":          "my_app_latency_milliseconds~millisecond.histogram",
			"my_app_latency_seconds":               "my_app_latency_seconds~second.histogram",
			"my_app_some_elements":                 "my_app_some_elements~element.gauge",
		},
		uniqueNameTypeToIdentity: map[string]string{
			"my_app_custom_elements_changed_total.counter": "my_app_custom_elements_changed_total~element.counter",
			"my_app_custom_elements_total.counter":         "my_app_custom_elements_total~element.counter",
			"my_app_latency_milliseconds.histogram":        "my_app_latency_milliseconds~millisecond.histogram",
			"my_app_latency_seconds.histogram":             "my_app_latency_seconds~second.histogram",
			"my_app_some_elements.gauge":                   "my_app_some_elements~element.gauge",
		},
	}

	t.Run("local", func(t *testing.T) {
		got, err := fetchIDs("./testdata/ids.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("http", func(t *testing.T) {
		got, err := fetchIDs("https://raw.githubusercontent.com/bwplotka/metric-rename-demo/refs/heads/diff/my-org/semconv/ids.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("http-custom", func(t *testing.T) {
		got, err := fetchIDs("https://bwplotka.dev/semconv/ids.yaml")
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
