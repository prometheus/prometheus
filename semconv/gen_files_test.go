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
			Forward:  metricGroupChange{MetricName: "my_app_latency_seconds_total", Unit: "{seconds}", ValuePromQL: "$old / 1000"},
			Backward: metricGroupChange{MetricName: "my_app_latency_milliseconds_total", Unit: "{milliseconds}", ValuePromQL: "$new * 1000"},
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
		got, err := fetchChangelog("./testdata/latest/.gen/changelog.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	// TODO(bwplotka): Move to something Prometheus owns e.g. internal Prometheus repo path.
	t.Run("http", func(t *testing.T) {
		got, err := fetchChangelog("https://raw.githubusercontent.com/bwplotka/metric-rename-demo/refs/heads/diff/my-org/semconv/v1.1.0/.gen/changelog.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("http-custom", func(t *testing.T) {
		got, err := fetchChangelog("https://bwplotka.dev/semconv/latest/.gen/changelog.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
}

func TestFetchIDs(t *testing.T) {
	expected := &ids{
		Version: 1,
		MetricsIDs: map[string][]versionedID{
			"my_app_custom_elements_changed_total.counter": {
				{
					ID:           "my_app_custom_elements.2",
					IntroVersion: "v1.1.0",
				},
			},
			"my_app_custom_elements_total.counter": {
				{
					ID:           "my_app_custom_elements",
					IntroVersion: "v1.0.0",
				},
			},
			"my_app_latency_milliseconds_total~milliseconds.histogram": {
				{
					ID:           "my_app_latency",
					IntroVersion: "v1.0.0",
				},
			},
			"my_app_latency_seconds_total~seconds.histogram": {
				{ID: "my_app_latency.2",
					IntroVersion: "v1.1.0"},
			},
			"my_app_some_elements_totals~gauge": {
				{
					ID:           "my_app_some_elements",
					IntroVersion: "v1.0.0",
				},
			},
		},
		uniqueNameToIdentity: map[string]string{
			"my_app_custom_elements_changed_total": "my_app_custom_elements_changed_total.counter",
			"my_app_custom_elements_total":         "my_app_custom_elements_total.counter",
			"my_app_latency_milliseconds_total":    "my_app_latency_milliseconds_total~milliseconds.histogram",
			"my_app_latency_seconds_total":         "my_app_latency_seconds_total~seconds.histogram",
			"my_app_some_elements_totals":          "my_app_some_elements_totals.gauge",
		},
		uniqueNameTypeToIdentity: map[string]string{
			"my_app_custom_elements_changed_total.counter": "my_app_custom_elements_changed_total.counter",
			"my_app_custom_elements_total.counter":         "my_app_custom_elements_total.counter",
			"my_app_latency_milliseconds_total.histogram":  "my_app_latency_milliseconds_total~milliseconds.histogram",
			"my_app_latency_seconds_total.histogram":       "my_app_latency_seconds_total~seconds.histogram",
			"my_app_some_elements_totals.gauge":            "my_app_some_elements_totals.gauge",
		},
	}

	t.Run("local", func(t *testing.T) {
		got, err := fetchIDs("./testdata/latest/.gen/ids.yaml")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
	t.Run("http", func(t *testing.T) {
		got, err := fetchIDs("https://raw.githubusercontent.com/bwplotka/metric-rename-demo/refs/heads/diff/my-org/semconv/latest/.gen/ids.yaml")
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
