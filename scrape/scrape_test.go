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

package scrape

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/atomic"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/pool"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	testutil.TolerantVerifyLeak(m)
}

func newTestRegistryAndScrapeMetrics(t testing.TB) (*prometheus.Registry, *scrapeMetrics) {
	reg := prometheus.NewRegistry()
	metrics, err := newScrapeMetrics(reg)
	require.NoError(t, err)
	return reg, metrics
}

func newTestScrapeMetrics(t testing.TB) *scrapeMetrics {
	_, metrics := newTestRegistryAndScrapeMetrics(t)
	return metrics
}

func TestNewScrapePool(t *testing.T) {
	var (
		app = teststorage.NewAppendable()
		cfg = &config.ScrapeConfig{
			MetricNameValidationScheme: model.UTF8Validation,
			MetricNameEscapingScheme:   model.AllowUTF8,
		}
		sp, err = newScrapePool(cfg, app, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
	)
	require.NoError(t, err)

	a, ok := sp.appendable.(*teststorage.Appendable)
	require.True(t, ok, "Failure to append.")
	require.Equal(t, app, a, "Wrong sample appender.")
	require.Equal(t, cfg, sp.config, "Wrong scrape config.")
}

func TestStorageHandlesOutOfOrderTimestamps(t *testing.T) {
	// Test with default OutOfOrderTimeWindow (0)
	t.Run("Out-Of-Order Sample Disabled", func(t *testing.T) {
		s := teststorage.New(t)
		t.Cleanup(func() { _ = s.Close() })

		runScrapeLoopTest(t, s, false)
	})

	// Test with specific OutOfOrderTimeWindow (600000)
	t.Run("Out-Of-Order Sample Enabled", func(t *testing.T) {
		s := teststorage.New(t, 600000)
		t.Cleanup(func() { _ = s.Close() })

		runScrapeLoopTest(t, s, true)
	})
}

func runScrapeLoopTest(t *testing.T, s *teststorage.TestStorage, expectOutOfOrder bool) {
	sl, _ := newTestScrapeLoop(t, withAppendable(s))

	// Current time for generating timestamps.
	now := time.Now()

	// Calculate timestamps for the samples based on the current time.
	now = now.Truncate(time.Minute) // round down the now timestamp to the nearest minute
	timestampInorder1 := now
	timestampOutOfOrder := now.Add(-5 * time.Minute)
	timestampInorder2 := now.Add(5 * time.Minute)

	app := sl.appender()
	_, _, _, err := app.append([]byte(`metric_total{a="1",b="1"} 1`), "text/plain", timestampInorder1)
	require.NoError(t, err)

	_, _, _, err = app.append([]byte(`metric_total{a="1",b="1"} 2`), "text/plain", timestampOutOfOrder)
	require.NoError(t, err)

	_, _, _, err = app.append([]byte(`metric_total{a="1",b="1"} 3`), "text/plain", timestampInorder2)
	require.NoError(t, err)

	require.NoError(t, app.Commit())

	// Query the samples back from the storage.
	q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	// Use a matcher to filter the metric name.
	series := q.Select(t.Context(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "metric_total"))

	var results []sample
	for series.Next() {
		it := series.At().Iterator(nil)
		for it.Next() == chunkenc.ValFloat {
			t, v := it.At()
			results = append(results, sample{
				L: series.At().Labels(),
				T: t,
				V: v,
			})
		}
		require.NoError(t, it.Err())
	}
	require.NoError(t, series.Err())

	// Define the expected results
	want := []sample{
		{
			L: labels.FromStrings("__name__", "metric_total", "a", "1", "b", "1"),
			T: timestamp.FromTime(timestampInorder1),
			V: 1,
		},
		{
			L: labels.FromStrings("__name__", "metric_total", "a", "1", "b", "1"),
			T: timestamp.FromTime(timestampInorder2),
			V: 3,
		},
	}

	if expectOutOfOrder {
		require.NotEqual(t, want, results, "Expected results to include out-of-order sample:\n%s", results)
	} else {
		require.Equal(t, want, results, "Appended samples not as expected:\n%s", results)
	}
}

// Regression test against https://github.com/prometheus/prometheus/issues/15831.
func TestScrapeAppend_MetadataUpdate(t *testing.T) {
	const (
		scrape1 = `# TYPE test_metric counter
# HELP test_metric some help text
# UNIT test_metric metric
test_metric_total 1
# TYPE test_metric2 gauge
# HELP test_metric2 other help text
test_metric2{foo="bar"} 2
# TYPE test_metric3 gauge
# HELP test_metric3 this represents tricky case of "broken" text that is not trivial to detect
test_metric3_metric4{foo="bar"} 2
# EOF`
		scrape2 = `# TYPE test_metric counter
# HELP test_metric different help text
test_metric_total 11
# TYPE test_metric2 gauge
# HELP test_metric2 other help text
# UNIT test_metric2 metric2
test_metric2{foo="bar"} 22
# EOF`
	)

	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))

	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte(scrape1), "application/openmetrics-text", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	testutil.RequireEqual(t, []sample{
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{L: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}},
	}, appTest.ResultMetadata())
	appTest.ResultReset()

	// Next (the same) scrape should not new metadata entries.
	app = sl.appender()
	_, _, _, err = app.append([]byte(scrape1), "application/openmetrics-text", now.Add(15*time.Second))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Empty(t, appTest.ResultMetadata())
	appTest.ResultReset()

	app = sl.appender()
	_, _, _, err = app.append([]byte(scrape2), "application/openmetrics-text", now.Add(15*time.Second))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	testutil.RequireEqual(t, []sample{
		{L: labels.FromStrings("__name__", "test_metric_total"), M: metadata.Metadata{Type: "counter", Unit: "metric", Help: "different help text"}}, // Here, technically we should have no unit, but it's a known limitation of the current implementation.
		{L: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), M: metadata.Metadata{Type: "gauge", Unit: "metric2", Help: "other help text"}},
	}, appTest.ResultMetadata())
	appTest.ResultReset()
}

func TestScrapeReportMetadata(t *testing.T) {
	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))
	app := sl.appender()

	now := time.Now()
	require.NoError(t, sl.report(app, now, 2*time.Second, 1, 1, 1, 512, nil))
	require.NoError(t, app.Commit())
	testutil.RequireEqual(t, []sample{
		{L: labels.FromStrings("__name__", "up"), M: scrapeHealthMetric.Metadata},
		{L: labels.FromStrings("__name__", "scrape_duration_seconds"), M: scrapeDurationMetric.Metadata},
		{L: labels.FromStrings("__name__", "scrape_samples_scraped"), M: scrapeSamplesMetric.Metadata},
		{L: labels.FromStrings("__name__", "scrape_samples_post_metric_relabeling"), M: samplesPostRelabelMetric.Metadata},
		{L: labels.FromStrings("__name__", "scrape_series_added"), M: scrapeSeriesAddedMetric.Metadata},
	}, appTest.ResultMetadata())
}

func TestIsSeriesPartOfFamily(t *testing.T) {
	t.Run("counter", func(t *testing.T) {
		require.True(t, isSeriesPartOfFamily("http_requests_total", []byte("http_requests_total"), model.MetricTypeCounter)) // Prometheus text style.
		require.True(t, isSeriesPartOfFamily("http_requests_total", []byte("http_requests"), model.MetricTypeCounter))       // OM text style.
		require.True(t, isSeriesPartOfFamily("http_requests_total", []byte("http_requests_total"), model.MetricTypeUnknown))

		require.False(t, isSeriesPartOfFamily("http_requests_total", []byte("http_requests"), model.MetricTypeUnknown)) // We don't know.
		require.False(t, isSeriesPartOfFamily("http_requests2_total", []byte("http_requests_total"), model.MetricTypeCounter))
		require.False(t, isSeriesPartOfFamily("http_requests_requests_total", []byte("http_requests"), model.MetricTypeCounter))
	})

	t.Run("gauge", func(t *testing.T) {
		require.True(t, isSeriesPartOfFamily("http_requests_count", []byte("http_requests_count"), model.MetricTypeGauge))
		require.True(t, isSeriesPartOfFamily("http_requests_count", []byte("http_requests_count"), model.MetricTypeUnknown))

		require.False(t, isSeriesPartOfFamily("http_requests_count2", []byte("http_requests_count"), model.MetricTypeCounter))
	})

	t.Run("histogram", func(t *testing.T) {
		require.True(t, isSeriesPartOfFamily("http_requests_seconds_sum", []byte("http_requests_seconds"), model.MetricTypeHistogram))
		require.True(t, isSeriesPartOfFamily("http_requests_seconds_count", []byte("http_requests_seconds"), model.MetricTypeHistogram))
		require.True(t, isSeriesPartOfFamily("http_requests_seconds_bucket", []byte("http_requests_seconds"), model.MetricTypeHistogram))
		require.True(t, isSeriesPartOfFamily("http_requests_seconds", []byte("http_requests_seconds"), model.MetricTypeHistogram))

		require.False(t, isSeriesPartOfFamily("http_requests_seconds_sum", []byte("http_requests_seconds"), model.MetricTypeUnknown)) // We don't know.
		require.False(t, isSeriesPartOfFamily("http_requests_seconds2_sum", []byte("http_requests_seconds"), model.MetricTypeHistogram))
	})

	t.Run("summary", func(t *testing.T) {
		require.True(t, isSeriesPartOfFamily("http_requests_seconds_sum", []byte("http_requests_seconds"), model.MetricTypeSummary))
		require.True(t, isSeriesPartOfFamily("http_requests_seconds_count", []byte("http_requests_seconds"), model.MetricTypeSummary))
		require.True(t, isSeriesPartOfFamily("http_requests_seconds", []byte("http_requests_seconds"), model.MetricTypeSummary))

		require.False(t, isSeriesPartOfFamily("http_requests_seconds_sum", []byte("http_requests_seconds"), model.MetricTypeUnknown)) // We don't know.
		require.False(t, isSeriesPartOfFamily("http_requests_seconds2_sum", []byte("http_requests_seconds"), model.MetricTypeSummary))
	})

	t.Run("info", func(t *testing.T) {
		require.True(t, isSeriesPartOfFamily("go_build_info", []byte("go_build_info"), model.MetricTypeInfo)) // Prometheus text style.
		require.True(t, isSeriesPartOfFamily("go_build_info", []byte("go_build"), model.MetricTypeInfo))      // OM text style.
		require.True(t, isSeriesPartOfFamily("go_build_info", []byte("go_build_info"), model.MetricTypeUnknown))

		require.False(t, isSeriesPartOfFamily("go_build_info", []byte("go_build"), model.MetricTypeUnknown)) // We don't know.
		require.False(t, isSeriesPartOfFamily("go_build2_info", []byte("go_build_info"), model.MetricTypeInfo))
		require.False(t, isSeriesPartOfFamily("go_build_build_info", []byte("go_build_info"), model.MetricTypeInfo))
	})
}

func TestDroppedTargetsList(t *testing.T) {
	var (
		app = teststorage.NewAppendable()
		cfg = &config.ScrapeConfig{
			JobName:                    "dropMe",
			ScrapeInterval:             model.Duration(1),
			MetricNameValidationScheme: model.UTF8Validation,
			MetricNameEscapingScheme:   model.AllowUTF8,
			RelabelConfigs: []*relabel.Config{
				{
					Action:               relabel.Drop,
					Regex:                relabel.MustNewRegexp("dropMe"),
					SourceLabels:         model.LabelNames{"job"},
					NameValidationScheme: model.UTF8Validation,
				},
			},
		}
		tgs = []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{model.AddressLabel: "127.0.0.1:9090"},
					{model.AddressLabel: "127.0.0.1:9091"},
				},
			},
		}
		sp, _                  = newScrapePool(cfg, app, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
		expectedLabelSetString = "{__address__=\"127.0.0.1:9090\", __scrape_interval__=\"0s\", __scrape_timeout__=\"0s\", job=\"dropMe\"}"
		expectedLength         = 2
	)
	sp.Sync(tgs)
	sp.Sync(tgs)
	require.Len(t, sp.droppedTargets, expectedLength)
	require.Equal(t, expectedLength, sp.droppedTargetsCount)
	lb := labels.NewBuilder(labels.EmptyLabels())
	require.Equal(t, expectedLabelSetString, sp.droppedTargets[0].DiscoveredLabels(lb).String())

	// Check that count is still correct when we don't retain all dropped targets.
	sp.config.KeepDroppedTargets = 1
	sp.Sync(tgs)
	require.Len(t, sp.droppedTargets, 1)
	require.Equal(t, expectedLength, sp.droppedTargetsCount)
}

// TestDiscoveredLabelsUpdate checks that DiscoveredLabels are updated
// even when new labels don't affect the target `hash`.
func TestDiscoveredLabelsUpdate(t *testing.T) {
	sp := newTestScrapePool(t, nil)

	// These are used when syncing so need this to avoid a panic.
	sp.config = &config.ScrapeConfig{
		ScrapeInterval:             model.Duration(1),
		ScrapeTimeout:              model.Duration(1),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
	}
	sp.activeTargets = make(map[uint64]*Target)
	t1 := &Target{
		tLabels:      model.LabelSet{"label": "name"},
		scrapeConfig: sp.config,
	}
	sp.activeTargets[t1.hash()] = t1

	t2 := &Target{
		tLabels:      model.LabelSet{"labelNew": "nameNew"},
		scrapeConfig: sp.config,
	}
	sp.sync([]*Target{t2})

	lb := labels.NewBuilder(labels.EmptyLabels())
	require.Equal(t, t2.DiscoveredLabels(lb), sp.activeTargets[t1.hash()].DiscoveredLabels(lb))
}

type testLoop struct {
	startFunc    func(interval, timeout time.Duration, errc chan<- error)
	stopFunc     func()
	forcedErr    error
	forcedErrMtx sync.Mutex
	runOnce      bool
	interval     time.Duration
	timeout      time.Duration
}

func (*testLoop) setScrapeFailureLogger(FailureLogger) {
}

func (l *testLoop) run(errc chan<- error) {
	if l.runOnce {
		panic("loop must be started only once")
	}
	l.runOnce = true
	l.startFunc(l.interval, l.timeout, errc)
}

func (*testLoop) disableEndOfRunStalenessMarkers() {
}

func (l *testLoop) setForcedError(err error) {
	l.forcedErrMtx.Lock()
	defer l.forcedErrMtx.Unlock()
	l.forcedErr = err
}

func (l *testLoop) getForcedError() error {
	l.forcedErrMtx.Lock()
	defer l.forcedErrMtx.Unlock()
	return l.forcedErr
}

func (l *testLoop) stop() {
	l.stopFunc()
}

func (*testLoop) getCache() *scrapeCache {
	return nil
}

func TestScrapePoolStop(t *testing.T) {
	t.Parallel()
	sp := newTestScrapePool(t, nil)

	var mtx sync.Mutex
	stopped := map[uint64]bool{}
	numTargets := 20

	// Stopping the scrape pool must call stop() on all scrape loops,
	// clean them and the respective targets up. It must wait until each loop's
	// stop function returned before returning itself.

	for i := range numTargets {
		t := &Target{
			labels:       labels.FromStrings(model.AddressLabel, fmt.Sprintf("example.com:%d", i)),
			scrapeConfig: &config.ScrapeConfig{},
		}
		l := &testLoop{}
		d := time.Duration((i+1)*20) * time.Millisecond
		l.stopFunc = func() {
			time.Sleep(d)

			mtx.Lock()
			stopped[t.hash()] = true
			mtx.Unlock()
		}

		sp.activeTargets[t.hash()] = t
		sp.loops[t.hash()] = l
	}

	done := make(chan struct{})
	stopTime := time.Now()

	go func() {
		sp.stop()
		close(done)
	}()

	select {
	case <-time.After(5 * time.Second):
		require.Fail(t, "scrapeLoop.stop() did not return as expected")
	case <-done:
		// This should have taken at least as long as the last target slept.
		require.GreaterOrEqual(t, time.Since(stopTime), time.Duration(numTargets*20)*time.Millisecond, "scrapeLoop.stop() exited before all targets stopped")
	}

	mtx.Lock()
	require.Len(t, stopped, numTargets, "Unexpected number of stopped loops")
	mtx.Unlock()

	require.Empty(t, sp.activeTargets, "Targets were not cleared on stopping: %d left", len(sp.activeTargets))
	require.Empty(t, sp.loops, "Loops were not cleared on stopping: %d left", len(sp.loops))
}

// TestScrapePoolReload tests reloading logic, so:
// * all loops are reloaded, reusing cache if scrape config changed.
// * reloaded loops are stopped before new ones are started.
// * new scrapeLoops are configured with the updated scrape config.
func TestScrapePoolReload(t *testing.T) {
	t.Parallel()

	var (
		mtx        sync.Mutex
		numTargets = 20
		stopped    = map[uint64]bool{}
	)

	cfg0 := &config.ScrapeConfig{}
	cfg1 := &config.ScrapeConfig{
		ScrapeInterval:             model.Duration(3 * time.Second),
		ScrapeTimeout:              model.Duration(2 * time.Second),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,

		// Test a few example options.
		SampleLimit:            123,
		ScrapeFallbackProtocol: "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited",
	}
	newLoopCfg1 := func(opts scrapeLoopOptions) loop {
		// Test cfg1 is being used.
		require.Equal(t, cfg1, opts.sp.config)

		// Inject out testLoop that allows mocking start and stop.
		l := &testLoop{interval: opts.interval, timeout: opts.timeout}

		// On start, expect previous loop instances for the same target to be stopped.
		l.startFunc = func(interval, timeout time.Duration, _ chan<- error) {
			// Ensure cfg1 interval and timeout are correctly configured.
			require.Equal(t, time.Duration(cfg1.ScrapeInterval), interval, "Unexpected scrape interval")
			require.Equal(t, time.Duration(cfg1.ScrapeTimeout), timeout, "Unexpected scrape timeout")

			mtx.Lock()
			targetScraper := opts.scraper.(*targetScraper)
			require.True(t, stopped[targetScraper.hash()], "Scrape loop for %v not stopped yet", targetScraper)
			mtx.Unlock()
		}
		return l
	}

	// Create test pool.
	reg, metrics := newTestRegistryAndScrapeMetrics(t)
	sp := newTestScrapePool(t, newLoopCfg1)
	sp.metrics = metrics

	// Prefill pool with 20 loops, simulating 20 scrape targets.
	for i := range numTargets {
		t := &Target{
			labels:       labels.FromStrings(model.AddressLabel, fmt.Sprintf("example.com:%d", i)),
			scrapeConfig: cfg0,
		}
		l := &testLoop{}
		d := time.Duration((i+1)*20) * time.Millisecond
		l.stopFunc = func() {
			time.Sleep(d) // Sleep uneven time on stop.

			mtx.Lock()
			stopped[t.hash()] = true
			mtx.Unlock()
		}

		sp.activeTargets[t.hash()] = t
		sp.loops[t.hash()] = l
	}

	beforeTargets := map[uint64]*Target{}
	maps.Copy(beforeTargets, sp.activeTargets)

	// Reloading a scrape pool with a new scrape configuration must stop all scrape
	// loops and start new ones. A new loop must not be started before the preceding
	// one terminated.
	require.NoError(t, sp.reload(cfg1))
	var stoppedCount int
	mtx.Lock()
	stoppedCount = len(stopped)
	mtx.Unlock()
	require.Equal(t, numTargets, stoppedCount, "Unexpected number of stopped loops")
	require.Equal(t, sp.activeTargets, beforeTargets, "Reloading affected target states unexpectedly")
	require.Len(t, sp.loops, numTargets, "Unexpected number of loops after reload")

	// Check if prometheus_target_reload_length_seconds points to cfg1.ScrapeInterval.
	got, err := gatherLabels(reg, "prometheus_target_reload_length_seconds")
	require.NoError(t, err)
	expectedName, expectedValue := "interval", cfg1.ScrapeInterval.String()
	require.Equal(t, [][]*dto.LabelPair{{{Name: &expectedName, Value: &expectedValue}}}, got)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(sp.metrics.targetScrapePoolReloads))
}

func TestScrapePoolReloadPreserveRelabeledIntervalTimeout(t *testing.T) {
	reloadCfg := &config.ScrapeConfig{
		ScrapeInterval:             model.Duration(3 * time.Second),
		ScrapeTimeout:              model.Duration(2 * time.Second),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
	}
	newLoop := func(opts scrapeLoopOptions) loop {
		l := &testLoop{interval: opts.interval, timeout: opts.timeout}
		l.startFunc = func(interval, timeout time.Duration, _ chan<- error) {
			require.Equal(t, 5*time.Second, interval, "Unexpected scrape interval")
			require.Equal(t, 3*time.Second, timeout, "Unexpected scrape timeout")
		}
		return l
	}
	reg, metrics := newTestRegistryAndScrapeMetrics(t)
	sp := newTestScrapePool(t, newLoop)
	sp.activeTargets[1] = &Target{
		labels: labels.FromStrings(model.ScrapeIntervalLabel, "5s", model.ScrapeTimeoutLabel, "3s"),
	}
	sp.metrics = metrics
	sp.loops[1] = noopLoop()

	err := sp.reload(reloadCfg)
	if err != nil {
		t.Fatalf("unable to reload configuration: %s", err)
	}
	// Check that the reload metric is labeled with the pool interval, not the overridden interval.
	got, err := gatherLabels(reg, "prometheus_target_reload_length_seconds")
	require.NoError(t, err)
	expectedName, expectedValue := "interval", "3s"
	require.Equal(t, [][]*dto.LabelPair{{{Name: &expectedName, Value: &expectedValue}}}, got)
}

// Gather metrics from the provided Gatherer with specified familyName,
// and return all sets of name/value pairs.
func gatherLabels(g prometheus.Gatherer, familyName string) ([][]*dto.LabelPair, error) {
	families, err := g.Gather()
	if err != nil {
		return nil, err
	}
	ret := make([][]*dto.LabelPair, 0)
	for _, f := range families {
		if f.GetName() == familyName {
			for _, m := range f.GetMetric() {
				ret = append(ret, m.GetLabel())
			}
			break
		}
	}
	return ret, nil
}

func TestScrapePoolTargetLimit(t *testing.T) {
	var wg sync.WaitGroup
	// On starting to run, new loops created on reload check whether their preceding
	// equivalents have been stopped.
	newLoop := func(scrapeLoopOptions) loop {
		wg.Add(1)
		l := &testLoop{
			startFunc: func(_, _ time.Duration, _ chan<- error) {
				wg.Done()
			},
			stopFunc: func() {},
		}
		return l
	}

	sp := newTestScrapePool(t, newLoop)

	var tgs []*targetgroup.Group
	for i := range 50 {
		tgs = append(tgs,
			&targetgroup.Group{
				Targets: []model.LabelSet{
					{model.AddressLabel: model.LabelValue(fmt.Sprintf("127.0.0.1:%d", 9090+i))},
				},
			},
		)
	}

	var limit uint
	reloadWithLimit := func(l uint) {
		limit = l
		require.NoError(t, sp.reload(&config.ScrapeConfig{
			ScrapeInterval:             model.Duration(3 * time.Second),
			ScrapeTimeout:              model.Duration(2 * time.Second),
			MetricNameValidationScheme: model.UTF8Validation,
			MetricNameEscapingScheme:   model.AllowUTF8,
			TargetLimit:                l,
		}))
	}

	var targets int
	loadTargets := func(n int) {
		targets = n
		sp.Sync(tgs[:n])
	}

	validateIsRunning := func() {
		wg.Wait()
		for _, l := range sp.loops {
			require.True(t, l.(*testLoop).runOnce, "loop should be running")
		}
	}

	validateErrorMessage := func(shouldErr bool) {
		for _, l := range sp.loops {
			lerr := l.(*testLoop).getForcedError()
			if shouldErr {
				require.Error(t, lerr, "error was expected for %d targets with a limit of %d", targets, limit)
				require.EqualError(t, lerr, fmt.Sprintf("target_limit exceeded (number of targets: %d, limit: %d)", targets, limit))
			} else {
				require.NoError(t, lerr)
			}
		}
	}

	reloadWithLimit(0)
	loadTargets(50)
	validateIsRunning()

	// Simulate an initial config with a limit.
	sp.config.TargetLimit = 30
	limit = 30
	loadTargets(50)
	validateIsRunning()
	validateErrorMessage(true)

	reloadWithLimit(50)
	validateIsRunning()
	validateErrorMessage(false)

	reloadWithLimit(40)
	validateIsRunning()
	validateErrorMessage(true)

	loadTargets(30)
	validateIsRunning()
	validateErrorMessage(false)

	loadTargets(40)
	validateIsRunning()
	validateErrorMessage(false)

	loadTargets(41)
	validateIsRunning()
	validateErrorMessage(true)

	reloadWithLimit(0)
	validateIsRunning()
	validateErrorMessage(false)

	reloadWithLimit(51)
	validateIsRunning()
	validateErrorMessage(false)

	tgs = append(tgs,
		&targetgroup.Group{
			Targets: []model.LabelSet{
				{model.AddressLabel: "127.0.0.1:1090"},
			},
		},
		&targetgroup.Group{
			Targets: []model.LabelSet{
				{model.AddressLabel: "127.0.0.1:1090"},
			},
		},
	)

	sp.Sync(tgs)
	validateIsRunning()
	validateErrorMessage(false)
}

func TestScrapePoolAppenderWithLimits(t *testing.T) {
	// Create a unique value, to validate the correct chain of appenders.
	baseAppender := struct{ storage.Appender }{}
	appendable := appendableFunc(func(context.Context) storage.Appender { return baseAppender })

	sl, _ := newTestScrapeLoop(t, withAppendable(appendable))
	wrapped := appenderWithLimits(sl.appendable.Appender(context.Background()), 0, 0, histogram.ExponentialSchemaMax)

	tl, ok := wrapped.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", wrapped)

	require.Equal(t, baseAppender, tl.Appender, "Expected base appender but got %T", tl.Appender)

	sampleLimit := 100
	sl, _ = newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appendable
		sl.sampleLimit = sampleLimit
	})
	wrapped = appenderWithLimits(sl.appendable.Appender(context.Background()), sampleLimit, 0, histogram.ExponentialSchemaMax)

	la, ok := wrapped.(*limitAppender)
	require.True(t, ok, "Expected limitAppender but got %T", wrapped)

	tl, ok = la.Appender.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", la.Appender)

	require.Equal(t, baseAppender, tl.Appender, "Expected base appender but got %T", tl.Appender)

	wrapped = appenderWithLimits(sl.appendable.Appender(context.Background()), sampleLimit, 100, histogram.ExponentialSchemaMax)

	bl, ok := wrapped.(*bucketLimitAppender)
	require.True(t, ok, "Expected bucketLimitAppender but got %T", wrapped)

	la, ok = bl.Appender.(*limitAppender)
	require.True(t, ok, "Expected limitAppender but got %T", bl)

	tl, ok = la.Appender.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", la.Appender)

	require.Equal(t, baseAppender, tl.Appender, "Expected base appender but got %T", tl.Appender)

	wrapped = appenderWithLimits(sl.appendable.Appender(context.Background()), sampleLimit, 100, 0)

	ml, ok := wrapped.(*maxSchemaAppender)
	require.True(t, ok, "Expected maxSchemaAppender but got %T", wrapped)

	bl, ok = ml.Appender.(*bucketLimitAppender)
	require.True(t, ok, "Expected bucketLimitAppender but got %T", wrapped)

	la, ok = bl.Appender.(*limitAppender)
	require.True(t, ok, "Expected limitAppender but got %T", bl)

	tl, ok = la.Appender.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", la.Appender)

	require.Equal(t, baseAppender, tl.Appender, "Expected base appender but got %T", tl.Appender)
}

func TestScrapePoolRaces(t *testing.T) {
	t.Parallel()
	interval, _ := model.ParseDuration("1s")
	timeout, _ := model.ParseDuration("500ms")
	newConfig := func() *config.ScrapeConfig {
		return &config.ScrapeConfig{
			ScrapeInterval:             interval,
			ScrapeTimeout:              timeout,
			MetricNameValidationScheme: model.UTF8Validation,
			MetricNameEscapingScheme:   model.AllowUTF8,
		}
	}
	sp, _ := newScrapePool(newConfig(), teststorage.NewAppendable(), 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
	tgts := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: "127.0.0.1:9090"},
				{model.AddressLabel: "127.0.0.2:9090"},
				{model.AddressLabel: "127.0.0.3:9090"},
				{model.AddressLabel: "127.0.0.4:9090"},
				{model.AddressLabel: "127.0.0.5:9090"},
				{model.AddressLabel: "127.0.0.6:9090"},
				{model.AddressLabel: "127.0.0.7:9090"},
				{model.AddressLabel: "127.0.0.8:9090"},
			},
		},
	}

	sp.Sync(tgts)
	active := sp.ActiveTargets()
	dropped := sp.DroppedTargets()
	expectedActive, expectedDropped := len(tgts[0].Targets), 0

	require.Len(t, active, expectedActive, "Invalid number of active targets")
	require.Len(t, dropped, expectedDropped, "Invalid number of dropped targets")

	for range 20 {
		time.Sleep(10 * time.Millisecond)
		_ = sp.reload(newConfig())
	}
	sp.stop()
}

func TestScrapePoolScrapeLoopsStarted(t *testing.T) {
	var wg sync.WaitGroup
	newLoop := func(scrapeLoopOptions) loop {
		wg.Add(1)
		l := &testLoop{
			startFunc: func(_, _ time.Duration, _ chan<- error) {
				wg.Done()
			},
			stopFunc: func() {},
		}
		return l
	}
	sp := newTestScrapePool(t, newLoop)

	tgs := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue("127.0.0.1:9090")},
			},
		},
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue("127.0.0.1:9090")},
			},
		},
	}

	require.NoError(t, sp.reload(&config.ScrapeConfig{
		ScrapeInterval:             model.Duration(3 * time.Second),
		ScrapeTimeout:              model.Duration(2 * time.Second),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
	}))
	sp.Sync(tgs)

	require.Len(t, sp.loops, 1)

	wg.Wait()
	for _, l := range sp.loops {
		require.True(t, l.(*testLoop).runOnce, "loop should be running")
	}
}

func TestScrapeLoopStopBeforeRun(t *testing.T) {
	t.Parallel()

	sl, scraper := newTestScrapeLoop(t)

	// The scrape pool synchronizes on stopping scrape loops. However, new scrape
	// loops are started asynchronously. Thus, it's possible, that a loop is stopped
	// again before having started properly.
	// Stopping not-yet-started loops must block until the run method was called and exited.
	// The run method must exit immediately.

	stopDone := make(chan struct{})
	go func() {
		sl.stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		require.FailNow(t, "Stopping terminated before run exited successfully.")
	case <-time.After(500 * time.Millisecond):
	}

	// Running the scrape loop must exit before calling the scraper even once.
	scraper.scrapeFunc = func(context.Context, io.Writer) error {
		require.FailNow(t, "Scraper was called for terminated scrape loop.")
		return nil
	}

	runDone := make(chan struct{})
	go func() {
		sl.run(nil)
		close(runDone)
	}()

	select {
	case <-runDone:
	case <-time.After(1 * time.Second):
		require.FailNow(t, "Running terminated scrape loop did not exit.")
	}

	select {
	case <-stopDone:
	case <-time.After(1 * time.Second):
		require.FailNow(t, "Stopping did not terminate after running exited.")
	}
}

func nopMutator(l labels.Labels) labels.Labels { return l }

func TestScrapeLoopStop(t *testing.T) {
	signal := make(chan struct{}, 1)

	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appTest
		// Since we're writing samples directly below we need to provide a protocol fallback.
		sl.fallbackScrapeProtocol = "text/plain"
	})

	// Terminate loop after 2 scrapes.
	numScrapes := 0
	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++
		if numScrapes == 2 {
			go sl.stop()
			<-sl.ctx.Done()
		}
		_, _ = w.Write([]byte("metric_a 42\n"))
		return ctx.Err()
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape wasn't stopped.")
	}

	got := appTest.ResultSamples()
	// We expected 1 actual sample for each scrape plus 5 for report samples.
	// At least 2 scrapes were made, plus the final stale markers.
	require.GreaterOrEqual(t, len(got), 6*3, "Expected at least 3 scrapes with 6 samples each.")
	require.Zero(t, len(got)%6, "There is a scrape with missing samples.")
	// All samples in a scrape must have the same timestamp.
	var ts int64
	for i, s := range got {
		switch {
		case i%6 == 0:
			ts = s.T
		case s.T != ts:
			t.Fatalf("Unexpected multiple timestamps within single scrape")
		}
	}
	// All samples from the last scrape must be stale markers.
	for _, s := range got[len(got)-5:] {
		require.True(t, value.IsStaleNaN(s.V), "Appended last sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(s.V))
	}
}

func TestScrapeLoopRun(t *testing.T) {
	t.Parallel()
	var (
		signal = make(chan struct{}, 1)
		errc   = make(chan error)
	)

	ctx, cancel := context.WithCancel(t.Context())
	sl, scraper := newTestScrapeLoop(t, withCtx(ctx))
	// The loop must terminate during the initial offset if the context
	// is canceled.
	scraper.offsetDur = time.Hour

	go func() {
		sl.run(errc)
		signal <- struct{}{}
	}()

	// Wait to make sure we are actually waiting on the offset.
	time.Sleep(1 * time.Second)

	cancel()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Cancellation during initial offset failed.")
	case err := <-errc:
		require.FailNow(t, "Unexpected error", "err: %s", err)
	}

	ctx, cancel = context.WithCancel(t.Context())
	sl, scraper = newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.timeout = 100 * time.Millisecond
	})
	// The provided timeout must cause cancellation of the context passed down to the
	// scraper. The scraper has to respect the context.
	scraper.offsetDur = 0

	blockCtx, blockCancel := context.WithCancel(t.Context())
	scraper.scrapeFunc = func(ctx context.Context, _ io.Writer) error {
		select {
		case <-blockCtx.Done():
			cancel()
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}

	go func() {
		sl.run(errc)
		signal <- struct{}{}
	}()

	select {
	case err := <-errc:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "Expected timeout error but got none.")
	}

	// We already caught the timeout error and are certainly in the loop.
	// Let the scrapes returns immediately to cause no further timeout errors
	// and check whether canceling the parent context terminates the loop.
	blockCancel()
	select {
	case <-signal:
		// Loop terminated as expected.
	case err := <-errc:
		require.FailNow(t, "Unexpected error", "err: %s", err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "Loop did not terminate on context cancellation")
	}
}

func TestScrapeLoopForcedErr(t *testing.T) {
	var (
		signal = make(chan struct{}, 1)
		errc   = make(chan error)
	)

	ctx, cancel := context.WithCancel(t.Context())
	sl, scraper := newTestScrapeLoop(t, withCtx(ctx))

	forcedErr := errors.New("forced err")
	sl.setForcedError(forcedErr)

	scraper.scrapeFunc = func(context.Context, io.Writer) error {
		require.FailNow(t, "Should not be scraped.")
		return nil
	}

	go func() {
		sl.run(errc)
		signal <- struct{}{}
	}()

	select {
	case err := <-errc:
		require.ErrorIs(t, err, forcedErr)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "Expected forced error but got none.")
	}
	cancel()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape not stopped.")
	}
}

func TestScrapeLoopRun_ContextCancelTerminatesBlockedSend(t *testing.T) {
	// Regression test for issue #17553
	defer goleak.VerifyNone(t)

	var (
		signal = make(chan struct{})
		errc   = make(chan error)
	)

	ctx, cancel := context.WithCancel(t.Context())
	sl, scraper := newTestScrapeLoop(t, withCtx(ctx))

	forcedErr := errors.New("forced err")
	sl.setForcedError(forcedErr)

	scraper.scrapeFunc = func(context.Context, io.Writer) error {
		return nil
	}

	go func() {
		sl.run(errc)
		close(signal)
	}()

	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case <-signal:
		// success case
	case <-time.After(3 * time.Second):
		require.FailNow(t, "Scrape loop failed to exit on context cancellation (goroutine leak detected)")
	}
}

func TestScrapeLoopMetadata(t *testing.T) {
	sl, _ := newTestScrapeLoop(t)

	app := sl.appender()
	total, _, _, err := app.append([]byte(`# TYPE test_metric counter
# HELP test_metric some help text
# UNIT test_metric metric
test_metric_total 1
# TYPE test_metric_no_help gauge
# HELP test_metric_no_type other help text
# EOF`), "application/openmetrics-text", time.Now())
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1, total)

	md, ok := sl.cache.GetMetadata("test_metric")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, model.MetricTypeCounter, md.Type, "unexpected metric type")
	require.Equal(t, "some help text", md.Help)
	require.Equal(t, "metric", md.Unit)

	md, ok = sl.cache.GetMetadata("test_metric_no_help")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, model.MetricTypeGauge, md.Type, "unexpected metric type")
	require.Empty(t, md.Help)
	require.Empty(t, md.Unit)

	md, ok = sl.cache.GetMetadata("test_metric_no_type")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, model.MetricTypeUnknown, md.Type, "unexpected metric type")
	require.Equal(t, "other help text", md.Help)
	require.Empty(t, md.Unit)
}

func TestScrapeLoopSeriesAdded(t *testing.T) {
	sl, _ := newTestScrapeLoop(t)

	app := sl.appender()
	total, added, seriesAdded, err := app.append([]byte("test_metric 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 1, seriesAdded)

	app = sl.appender()
	total, added, seriesAdded, err = app.append([]byte("test_metric 1\n"), "text/plain", time.Time{})
	require.NoError(t, app.Commit())
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 0, seriesAdded)
}

func TestScrapeLoopFailWithInvalidLabelsAfterRelabel(t *testing.T) {
	target := &Target{
		labels: labels.FromStrings("pod_label_invalid_012\xff", "test"),
	}
	relabelConfig := []*relabel.Config{{
		Action:               relabel.LabelMap,
		Regex:                relabel.MustNewRegexp("pod_label_invalid_(.+)"),
		Separator:            ";",
		Replacement:          "$1",
		NameValidationScheme: model.UTF8Validation,
	}}
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.sampleMutator = func(l labels.Labels) labels.Labels {
			return mutateSampleLabels(l, target, true, relabelConfig)
		}
	})

	app := sl.appender()
	total, added, seriesAdded, err := app.append([]byte("test_metric 1\n"), "text/plain", time.Time{})
	require.ErrorContains(t, err, "invalid metric name or label names")
	require.NoError(t, app.Rollback())
	require.Equal(t, 1, total)
	require.Equal(t, 0, added)
	require.Equal(t, 0, seriesAdded)
}

func TestScrapeLoopFailLegacyUnderUTF8(t *testing.T) {
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.validationScheme = model.LegacyValidation
	})

	app := sl.appender()
	total, added, seriesAdded, err := app.append([]byte("{\"test.metric\"} 1\n"), "text/plain", time.Time{})
	require.ErrorContains(t, err, "invalid metric name or label names")
	require.NoError(t, app.Rollback())
	require.Equal(t, 1, total)
	require.Equal(t, 0, added)
	require.Equal(t, 0, seriesAdded)

	// When scrapeloop has validation set to UTF-8, the metric is allowed.
	sl, _ = newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.validationScheme = model.UTF8Validation
	})

	app = sl.appender()
	total, added, seriesAdded, err = app.append([]byte("{\"test.metric\"} 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 1, seriesAdded)
}

func readTextParseTestMetrics(t testing.TB) []byte {
	t.Helper()

	b, err := os.ReadFile("../model/textparse/testdata/alltypes.237mfs.prom.txt")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func makeTestGauges(n int) []byte {
	sb := bytes.Buffer{}
	sb.WriteString("# TYPE metric_a gauge\n")
	sb.WriteString("# HELP metric_a help text\n")
	for i := range n {
		_, _ = fmt.Fprintf(&sb, "metric_a{foo=\"%d\",bar=\"%d\"} 1\n", i, i*100)
	}
	sb.WriteString("# EOF\n")
	return sb.Bytes()
}

func promTextToProto(tb testing.TB, text []byte) []byte {
	tb.Helper()

	p := expfmt.NewTextParser(model.UTF8Validation)
	fams, err := p.TextToMetricFamilies(bytes.NewReader(text))
	if err != nil {
		tb.Fatal(err)
	}
	// Order by name for the deterministic tests.
	var names []string
	for n := range fams {
		names = append(names, n)
	}
	sort.Strings(names)

	buf := bytes.Buffer{}
	for _, n := range names {
		o, err := proto.Marshal(fams[n])
		if err != nil {
			tb.Fatal(err)
		}

		// Write first length, then binary protobuf.
		varintBuf := binary.AppendUvarint(nil, uint64(len(o)))
		buf.Write(varintBuf)
		buf.Write(o)
	}
	return buf.Bytes()
}

func TestPromTextToProto(t *testing.T) {
	metricsText := readTextParseTestMetrics(t)
	// TODO(bwplotka): Windows adds \r for new lines which is
	// not handled correctly in the expfmt parser, fix it.
	metricsText = bytes.ReplaceAll(metricsText, []byte("\r"), nil)

	metricsProto := promTextToProto(t, metricsText)
	d := expfmt.NewDecoder(bytes.NewReader(metricsProto), expfmt.NewFormat(expfmt.TypeProtoDelim))

	var got []string
	for {
		mf := &dto.MetricFamily{}
		if err := d.Decode(mf); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		got = append(got, mf.GetName())
	}
	require.Len(t, got, 237)
	// Check a few to see if those are not dups.
	require.Equal(t, "go_gc_cycles_automatic_gc_cycles_total", got[0])
	require.Equal(t, "prometheus_sd_kuma_fetch_duration_seconds", got[128])
	require.Equal(t, "promhttp_metric_handler_requests_total", got[236])
}

// BenchmarkScrapeLoopAppend benchmarks a core append function in a scrapeLoop
// that creates a new parser and goes through a byte slice from a single scrape.
// Benchmark compares append function run across 2 dimensions:
// *`data`: different sizes of metrics scraped e.g. one big gauge metric family
//  with a thousand series and more realistic scenario with common types.
// *`fmt`: different scrape formats which will benchmark different parsers e.g.
//  promtext, omtext and promproto.
//
// Recommended CLI invocation:
/*
	export bench=append && go test ./scrape/... \
		-run '^$' -bench '^BenchmarkScrapeLoopAppend' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkScrapeLoopAppend(b *testing.B) {
	for _, data := range []struct {
		name         string
		parsableText []byte
	}{
		{name: "1Fam1000Gauges", parsableText: makeTestGauges(2000)},         // ~68.1 KB, ~77.9 KB in proto.
		{name: "237FamsAllTypes", parsableText: readTextParseTestMetrics(b)}, // ~185.7 KB, ~70.6 KB in proto.
	} {
		b.Run(fmt.Sprintf("data=%v", data.name), func(b *testing.B) {
			metricsProto := promTextToProto(b, data.parsableText)

			for _, bcase := range []struct {
				name        string
				contentType string
				parsable    []byte
			}{
				{name: "PromText", contentType: "text/plain", parsable: data.parsableText},
				{name: "OMText", contentType: "application/openmetrics-text", parsable: data.parsableText},
				{name: "PromProto", contentType: "application/vnd.google.protobuf", parsable: metricsProto},
			} {
				b.Run(fmt.Sprintf("fmt=%v", bcase.name), func(b *testing.B) {
					// Need a full storage for correct Add/AddFast semantics.
					s := teststorage.New(b)
					b.Cleanup(func() { _ = s.Close() })

					sl, _ := newTestScrapeLoop(b, withAppendable(s))
					app := sl.appender()
					ts := time.Time{}

					b.ReportAllocs()
					b.ResetTimer()
					for b.Loop() {
						ts = ts.Add(time.Second)
						_, _, _, err := app.append(bcase.parsable, bcase.contentType, ts)
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

func TestScrapeLoopScrapeAndReport(t *testing.T) {
	parsableText := readTextParseTestMetrics(t)
	// On windows \r is added when reading, but parsers do not support this. Kill it.
	parsableText = bytes.ReplaceAll(parsableText, []byte("\r"), nil)

	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appTest
		sl.fallbackScrapeProtocol = "application/openmetrics-text"
	})
	scraper.scrapeFunc = func(_ context.Context, writer io.Writer) error {
		_, err := writer.Write(parsableText)
		return err
	}

	ts := time.Time{}

	sl.scrapeAndReport(time.Time{}, ts, nil)
	require.NoError(t, scraper.lastError)

	require.Len(t, appTest.ResultSamples(), 1862)
	require.Len(t, appTest.ResultMetadata(), 1862)
}

// Recommended CLI invocation:
/*
	export bench=scrapeAndReport && go test ./scrape/... \
		-run '^$' -bench '^BenchmarkScrapeLoopScrapeAndReport' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkScrapeLoopScrapeAndReport(b *testing.B) {
	parsableText := readTextParseTestMetrics(b)

	s := teststorage.New(b)
	b.Cleanup(func() { _ = s.Close() })

	sl, scraper := newTestScrapeLoop(b, func(sl *scrapeLoop) {
		sl.appendable = s
		sl.fallbackScrapeProtocol = "application/openmetrics-text"
	})
	scraper.scrapeFunc = func(_ context.Context, writer io.Writer) error {
		_, err := writer.Write(parsableText)
		return err
	}

	ts := time.Time{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ts = ts.Add(time.Second)
		sl.scrapeAndReport(time.Time{}, ts, nil)
		require.NoError(b, scraper.lastError)
	}
}

func TestSetOptionsHandlingStaleness(t *testing.T) {
	s := teststorage.New(t, 600000)
	t.Cleanup(func() { _ = s.Close() })

	signal := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Function to run the scrape loop
	runScrapeLoop := func(ctx context.Context, t *testing.T, cue int, action func(*scrapeLoop)) {
		sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
			sl.ctx = ctx
			sl.appendable = s
		})

		numScrapes := 0
		scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
			numScrapes++
			if numScrapes == cue {
				action(sl)
			}
			_, _ = fmt.Fprintf(w, "metric_a{a=\"1\",b=\"1\"} %d\n", 42+numScrapes)
			return nil
		}
		sl.run(nil)
	}
	go func() {
		runScrapeLoop(ctx, t, 2, func(sl *scrapeLoop) {
			go sl.stop()
			// Wait a bit then start a new target.
			time.Sleep(100 * time.Millisecond)
			go func() {
				runScrapeLoop(ctx, t, 4, func(*scrapeLoop) {
					cancel()
				})
				signal <- struct{}{}
			}()
		})
	}()

	select {
	case <-signal:
	case <-time.After(10 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	ctx1, cancel := context.WithCancel(t.Context())
	defer cancel()

	q, err := s.Querier(0, time.Now().UnixNano())

	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	series := q.Select(ctx1, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "metric_a"))

	var results []sample
	for series.Next() {
		it := series.At().Iterator(nil)
		for it.Next() == chunkenc.ValFloat {
			t, v := it.At()
			results = append(results, sample{
				L: series.At().Labels(),
				T: t,
				V: v,
			})
		}
		require.NoError(t, it.Err())
	}
	require.NoError(t, series.Err())
	var c int
	for _, s := range results {
		if value.IsStaleNaN(s.V) {
			c++
		}
	}
	require.Equal(t, 0, c, "invalid count of staleness markers after stopping the engine")
}

func TestScrapeLoopRunCreatesStaleMarkersOnFailedScrape(t *testing.T) {
	signal := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.appendable = appTest
		// Since we're writing samples directly below we need to provide a protocol fallback.
		sl.fallbackScrapeProtocol = "text/plain"
	})

	// Succeed once, several failures, then stop.
	numScrapes := 0
	scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
		numScrapes++

		switch numScrapes {
		case 1:
			_, _ = w.Write([]byte("metric_a 42\n"))
			return nil
		case 5:
			cancel()
		}
		return errors.New("scrape failed")
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape wasn't stopped.")
	}

	got := appTest.ResultSamples()
	// 1 successfully scraped sample
	// 1 stale marker after first fail
	// 5x 5 report samples for each scrape successful or not.
	require.Len(t, got, 27, "Appended samples not as expected:\n%s", appTest)
	require.Equal(t, 42.0, got[0].V, "Appended first sample not as expected")
	require.True(t, value.IsStaleNaN(got[6].V),
		"Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(got[6].V))
}

func TestScrapeLoopRunCreatesStaleMarkersOnParseFailure(t *testing.T) {
	signal := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.appendable = appTest
		// Since we're writing samples directly below we need to provide a protocol fallback.
		sl.fallbackScrapeProtocol = "text/plain"
	})

	// Succeed once, several failures, then stop.
	numScrapes := 0
	scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
		numScrapes++

		switch numScrapes {
		case 1:
			_, _ = w.Write([]byte("metric_a 42\n"))
			return nil
		case 2:
			_, _ = w.Write([]byte("7&-\n"))
			return nil
		case 3:
			cancel()
		}
		return errors.New("scrape failed")
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
		// TODO(bwplotka): Prone to flakiness, depend on atomic numScrapes.
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape wasn't stopped.")
	}

	got := appTest.ResultSamples()
	// 1 successfully scraped sample
	// 1 stale marker after first fail
	// 3x 5 report samples for each scrape successful or not.
	require.Len(t, got, 17, "Appended samples not as expected:\n%s", appTest)
	require.Equal(t, 42.0, got[0].V, "Appended first sample not as expected")
	require.True(t, value.IsStaleNaN(got[6].V),
		"Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(got[6].V))
}

// If we have a target with sample_limit set and scrape initially works, but then we hit the sample_limit error,
// then we don't expect to see any StaleNaNs appended for the series that disappeared due to sample_limit error.
func TestScrapeLoopRunCreatesStaleMarkersOnSampleLimit(t *testing.T) {
	signal := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.appendable = appTest
		// Since we're writing samples directly below we need to provide a protocol fallback.
		sl.fallbackScrapeProtocol = "text/plain"
		sl.sampleLimit = 4
	})

	// Succeed once, several failures, then stop.
	numScrapes := 0
	scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
		numScrapes++
		switch numScrapes {
		case 1:
			_, _ = w.Write([]byte("metric_a 10\nmetric_b 10\nmetric_c 10\nmetric_d 10\n"))
			return nil
		case 2:
			_, _ = w.Write([]byte("metric_a 20\nmetric_b 20\nmetric_c 20\nmetric_d 20\nmetric_e 999\n"))
			return nil
		case 3:
			_, _ = w.Write([]byte("metric_a 30\nmetric_b 30\nmetric_c 30\nmetric_d 30\n"))
			return nil
		case 4:
			cancel()
		}
		return errors.New("scrape failed")
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape wasn't stopped.")
	}

	got := appTest.ResultSamples()

	// 4 scrapes in total:
	// #1 - success - 4 samples appended + 5 report series
	// #2 - sample_limit exceeded - no samples appended, only 5 report series
	// #3 - success - 4 samples appended + 5 report series
	// #4 - scrape canceled - 4 StaleNaNs appended because of scrape error + 5 report series
	require.Len(t, got, (4+5)+5+(4+5)+(4+5), "Appended samples not as expected:\n%s", appTest)
	// Expect first 4 samples to be metric_X [0-3].
	for i := range 4 {
		require.Equal(t, 10.0, got[i].V, "Appended %d sample not as expected", i)
	}
	// Next 5 samples are report series [4-8].
	// Next 5 samples are report series for the second scrape [9-13].
	// Expect first 4 samples to be metric_X from the third scrape [14-17].
	for i := 14; i <= 17; i++ {
		require.Equal(t, 30.0, got[i].V, "Appended %d sample not as expected", i)
	}
	// Next 5 samples are report series [18-22].
	// Next 5 samples are report series [23-26].
	for i := 23; i <= 26; i++ {
		require.True(t, value.IsStaleNaN(got[i].V),
			"Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(got[i].V))
	}
}

func TestScrapeLoopCache(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	signal := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	appTest := teststorage.NewAppendable().Then(s)
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.l = promslog.New(&promslog.Config{})
		sl.appendable = appTest
		// Since we're writing samples directly below we need to provide a protocol fallback.
		sl.fallbackScrapeProtocol = "text/plain"
		// Decreasing the scrape interval could make the test fail, as multiple scrapes might be initiated at identical millisecond timestamps.
		// See https://github.com/prometheus/prometheus/issues/12727.
		sl.interval = 100 * time.Millisecond
	})

	numScrapes := 0
	scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
		switch numScrapes {
		case 1, 2:
			_, ok := sl.cache.series["metric_a"]
			require.True(t, ok, "metric_a missing from cache after scrape %d", numScrapes)
			_, ok = sl.cache.series["metric_b"]
			require.True(t, ok, "metric_b missing from cache after scrape %d", numScrapes)
		case 3:
			_, ok := sl.cache.series["metric_a"]
			require.True(t, ok, "metric_a missing from cache after scrape %d", numScrapes)
			_, ok = sl.cache.series["metric_b"]
			require.False(t, ok, "metric_b present in cache after scrape %d", numScrapes)
		}

		numScrapes++
		switch numScrapes {
		case 1:
			_, _ = w.Write([]byte("metric_a 42\nmetric_b 43\n"))
			return nil
		case 3:
			_, _ = w.Write([]byte("metric_a 44\n"))
			return nil
		case 4:
			cancel()
		}
		return errors.New("scrape failed")
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape wasn't stopped.")
	}

	// 3 successfully scraped samples
	// 3 stale marker after samples were missing.
	// 4x 5 report samples for each scrape successful or not.
	require.Len(t, appTest.ResultSamples(), 26, "Appended samples not as expected:\n%s", appTest)
}

func TestScrapeLoopCacheMemoryExhaustionProtection(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	signal := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
	})
	numScrapes := 0
	scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
		numScrapes++
		if numScrapes < 5 {
			s := ""
			for i := range 500 {
				s = fmt.Sprintf("%smetric_%d_%d 42\n", s, i, numScrapes)
			}
			_, _ = w.Write([]byte(s + "&"))
		} else {
			cancel()
		}
		return nil
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape wasn't stopped.")
	}

	require.LessOrEqual(t, len(sl.cache.series), 2000, "More than 2000 series cached.")
}

func TestScrapeLoopAppend(t *testing.T) {
	tests := []struct {
		title           string
		honorLabels     bool
		scrapeLabels    string
		discoveryLabels []string
		expLset         labels.Labels
		expValue        float64
	}{
		{
			// When "honor_labels" is not set
			// label name collision is handler by adding a prefix.
			title:           "Label name collision",
			honorLabels:     false,
			scrapeLabels:    `metric{n="1"} 0`,
			discoveryLabels: []string{"n", "2"},
			expLset:         labels.FromStrings("__name__", "metric", "exported_n", "1", "n", "2"),
			expValue:        0,
		}, {
			// When "honor_labels" is not set
			// exported label from discovery don't get overwritten
			title:           "Label name collision",
			honorLabels:     false,
			scrapeLabels:    `metric 0`,
			discoveryLabels: []string{"n", "2", "exported_n", "2"},
			expLset:         labels.FromStrings("__name__", "metric", "n", "2", "exported_n", "2"),
			expValue:        0,
		}, {
			// Labels with no value need to be removed as these should not be ingested.
			title:           "Delete Empty labels",
			honorLabels:     false,
			scrapeLabels:    `metric{n=""} 0`,
			discoveryLabels: nil,
			expLset:         labels.FromStrings("__name__", "metric"),
			expValue:        0,
		}, {
			// Honor Labels should ignore labels with the same name.
			title:           "Honor Labels",
			honorLabels:     true,
			scrapeLabels:    `metric{n1="1", n2="2"} 0`,
			discoveryLabels: []string{"n1", "0"},
			expLset:         labels.FromStrings("__name__", "metric", "n1", "1", "n2", "2"),
			expValue:        0,
		}, {
			title:           "Stale - NaN",
			honorLabels:     false,
			scrapeLabels:    `metric NaN`,
			discoveryLabels: nil,
			expLset:         labels.FromStrings("__name__", "metric"),
			expValue:        math.Float64frombits(value.NormalNaN),
		},
	}

	for _, test := range tests {
		discoveryLabels := &Target{
			labels: labels.FromStrings(test.discoveryLabels...),
		}

		appTest := teststorage.NewAppendable()
		sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
			sl.appendable = appTest
			sl.sampleMutator = func(l labels.Labels) labels.Labels {
				return mutateSampleLabels(l, discoveryLabels, test.honorLabels, nil)
			}
			sl.reportSampleMutator = func(l labels.Labels) labels.Labels {
				return mutateReportSampleLabels(l, discoveryLabels)
			}
		})

		now := time.Now()

		app := sl.appender()
		_, _, _, err := app.append([]byte(test.scrapeLabels), "text/plain", now)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		expected := []sample{
			{
				L: test.expLset,
				T: timestamp.FromTime(now),
				V: test.expValue,
			},
		}

		t.Logf("Test:%s", test.title)
		requireEqual(t, expected, appTest.ResultSamples())
	}
}

func requireEqual(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	testutil.RequireEqualWithOptions(t, expected, actual,
		[]cmp.Option{
			cmp.Comparer(func(a, b sample) bool { return a.Equals(b) }),
			// StaleNaN samples are generated by iterating over a map, which means that the order
			// of samples might be different on every test run. Sort series by label to avoid
			// test failures because of that.
			cmpopts.SortSlices(func(a, b sample) int {
				return labels.Compare(a.L, b.L)
			}),
		},
		msgAndArgs...)
}

func TestScrapeLoopAppendForConflictingPrefixedLabels(t *testing.T) {
	testcases := map[string]struct {
		targetLabels  []string
		exposedLabels string
		expected      []string
	}{
		"One target label collides with existing label": {
			targetLabels:  []string{"foo", "2"},
			exposedLabels: `metric{foo="1"} 0`,
			expected:      []string{"__name__", "metric", "exported_foo", "1", "foo", "2"},
		},

		"One target label collides with existing label, plus target label already with prefix 'exported'": {
			targetLabels:  []string{"foo", "2", "exported_foo", "3"},
			exposedLabels: `metric{foo="1"} 0`,
			expected:      []string{"__name__", "metric", "exported_exported_foo", "1", "exported_foo", "3", "foo", "2"},
		},
		"One target label collides with existing label, plus existing label already with prefix 'exported": {
			targetLabels:  []string{"foo", "3"},
			exposedLabels: `metric{foo="1", exported_foo="2"} 0`,
			expected:      []string{"__name__", "metric", "exported_exported_foo", "1", "exported_foo", "2", "foo", "3"},
		},
		"One target label collides with existing label, both already with prefix 'exported'": {
			targetLabels:  []string{"exported_foo", "2"},
			exposedLabels: `metric{exported_foo="1"} 0`,
			expected:      []string{"__name__", "metric", "exported_exported_foo", "1", "exported_foo", "2"},
		},
		"Two target labels collide with existing labels, both with and without prefix 'exported'": {
			targetLabels:  []string{"foo", "3", "exported_foo", "4"},
			exposedLabels: `metric{foo="1", exported_foo="2"} 0`,
			expected: []string{
				"__name__", "metric", "exported_exported_foo", "1", "exported_exported_exported_foo",
				"2", "exported_foo", "4", "foo", "3",
			},
		},
		"Extreme example": {
			targetLabels:  []string{"foo", "0", "exported_exported_foo", "1", "exported_exported_exported_foo", "2"},
			exposedLabels: `metric{foo="3", exported_foo="4", exported_exported_exported_foo="5"} 0`,
			expected: []string{
				"__name__", "metric",
				"exported_exported_exported_exported_exported_foo", "5",
				"exported_exported_exported_exported_foo", "3",
				"exported_exported_exported_foo", "2",
				"exported_exported_foo", "1",
				"exported_foo", "4",
				"foo", "0",
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			appTest := teststorage.NewAppendable()
			sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
				sl.appendable = appTest
				sl.sampleMutator = func(l labels.Labels) labels.Labels {
					return mutateSampleLabels(l, &Target{labels: labels.FromStrings(tc.targetLabels...)}, false, nil)
				}
			})

			app := sl.appender()
			_, _, _, err := app.append([]byte(tc.exposedLabels), "text/plain", time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC))
			require.NoError(t, err)

			require.NoError(t, app.Commit())

			requireEqual(t, []sample{
				{
					L: labels.FromStrings(tc.expected...),
					T: timestamp.FromTime(time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC)),
					V: 0,
				},
			}, appTest.ResultSamples())
		})
	}
}

func TestScrapeLoopAppendCacheEntryButErrNotFound(t *testing.T) {
	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))

	fakeRef := storage.SeriesRef(1)
	expValue := float64(1)
	metric := []byte(`metric{n="1"} 1`)
	p, warning := textparse.New(metric, "text/plain", labels.NewSymbolTable(), textparse.ParserOptions{})
	require.NotNil(t, p)
	require.NoError(t, warning)

	var lset labels.Labels
	_, err := p.Next()
	require.NoError(t, err)
	p.Labels(&lset)
	hash := lset.Hash()

	// Create a fake entry in the cache
	sl.cache.addRef(metric, fakeRef, lset, hash)
	now := time.Now()

	app := sl.appender()
	_, _, _, err = app.append(metric, "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	expected := []sample{
		{
			L: lset,
			T: timestamp.FromTime(now),
			V: expValue,
		},
	}

	require.Equal(t, expected, appTest.ResultSamples())
}

type appendableFunc func(ctx context.Context) storage.Appender

func (a appendableFunc) Appender(ctx context.Context) storage.Appender { return a(ctx) }

func TestScrapeLoopAppendSampleLimit(t *testing.T) {
	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appendableFunc(func(ctx context.Context) storage.Appender {
			// Chain appTest to verify what samples passed through.
			return &limitAppender{Appender: appTest.Appender(ctx), limit: 1}
		})
		sl.sampleMutator = func(l labels.Labels) labels.Labels {
			if l.Has("deleteme") {
				return labels.EmptyLabels()
			}
			return l
		}
		sl.sampleLimit = 1 // Same as limitAppender.limit
	})

	// Get the value of the Counter before performing append.
	beforeMetric := dto.Metric{}
	err := sl.metrics.targetScrapeSampleLimit.Write(&beforeMetric)
	require.NoError(t, err)

	beforeMetricValue := beforeMetric.GetCounter().GetValue()

	now := time.Now()
	app := sl.appender()
	total, added, seriesAdded, err := app.append([]byte("metric_a 1\nmetric_b 1\nmetric_c 1\n"), "text/plain", now)
	require.ErrorIs(t, err, errSampleLimit)
	require.NoError(t, app.Rollback())
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 1, seriesAdded)

	// Check that the Counter has been incremented a single time for the scrape,
	// not multiple times for each sample.
	metric := dto.Metric{}
	err = sl.metrics.targetScrapeSampleLimit.Write(&metric)
	require.NoError(t, err)

	v := metric.GetCounter().GetValue()
	change := v - beforeMetricValue
	require.Equal(t, 1.0, change, "Unexpected change of sample limit metric: %f", change)

	// And verify that we got the samples that fit under the limit.
	want := []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now),
			V: 1,
		},
	}
	requireEqual(t, want, appTest.RolledbackSamples(), "Appended samples not as expected:\n%s", appTest)

	now = time.Now()
	app = sl.appender()
	total, added, seriesAdded, err = app.append([]byte("metric_a 1\nmetric_b 1\nmetric_c{deleteme=\"yes\"} 1\nmetric_d 1\nmetric_e 1\nmetric_f 1\nmetric_g 1\nmetric_h{deleteme=\"yes\"} 1\nmetric_i{deleteme=\"yes\"} 1\n"), "text/plain", now)
	require.ErrorIs(t, err, errSampleLimit)
	require.NoError(t, app.Rollback())
	require.Equal(t, 9, total)
	require.Equal(t, 6, added)
	require.Equal(t, 1, seriesAdded)
}

func TestScrapeLoop_HistogramBucketLimit(t *testing.T) {
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appendableFunc(func(ctx context.Context) storage.Appender {
			return &bucketLimitAppender{Appender: teststorage.NewAppendable().Appender(ctx), limit: 2}
		})
		sl.enableNativeHistogramScraping = true
		sl.sampleMutator = func(l labels.Labels) labels.Labels {
			if l.Has("deleteme") {
				return labels.EmptyLabels()
			}
			return l
		}
	})
	app := sl.appender()

	metric := dto.Metric{}
	err := sl.metrics.targetScrapeNativeHistogramBucketLimit.Write(&metric)
	require.NoError(t, err)
	beforeMetricValue := metric.GetCounter().GetValue()

	nativeHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:                      "testing",
			Name:                           "example_native_histogram",
			Help:                           "This is used for testing",
			ConstLabels:                    map[string]string{"some": "value"},
			NativeHistogramBucketFactor:    1.1, // 10% increase from bucket to bucket
			NativeHistogramMaxBucketNumber: 100, // intentionally higher than the limit we'll use in the scraper
		},
		[]string{"size"},
	)
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(nativeHistogram))
	nativeHistogram.WithLabelValues("S").Observe(1.0)
	nativeHistogram.WithLabelValues("M").Observe(1.0)
	nativeHistogram.WithLabelValues("L").Observe(1.0)
	nativeHistogram.WithLabelValues("M").Observe(10.0)
	nativeHistogram.WithLabelValues("L").Observe(10.0) // in different bucket since > 1*1.1

	gathered, err := registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, gathered)

	histogramMetricFamily := gathered[0]
	msg, err := MetricFamilyToProtobuf(histogramMetricFamily)
	require.NoError(t, err)

	now := time.Now()
	total, added, seriesAdded, err := app.append(msg, "application/vnd.google.protobuf", now)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 3, seriesAdded)

	err = sl.metrics.targetScrapeNativeHistogramBucketLimit.Write(&metric)
	require.NoError(t, err)
	metricValue := metric.GetCounter().GetValue()
	require.Equal(t, beforeMetricValue, metricValue)
	beforeMetricValue = metricValue

	nativeHistogram.WithLabelValues("L").Observe(100.0) // in different bucket since > 10*1.1

	gathered, err = registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, gathered)

	histogramMetricFamily = gathered[0]
	msg, err = MetricFamilyToProtobuf(histogramMetricFamily)
	require.NoError(t, err)

	now = time.Now()
	total, added, seriesAdded, err = app.append(msg, "application/vnd.google.protobuf", now)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 0, seriesAdded) // Series are cached.

	err = sl.metrics.targetScrapeNativeHistogramBucketLimit.Write(&metric)
	require.NoError(t, err)
	metricValue = metric.GetCounter().GetValue()
	require.Equal(t, beforeMetricValue, metricValue)
	beforeMetricValue = metricValue

	nativeHistogram.WithLabelValues("L").Observe(100000.0) // in different bucket since > 10*1.1

	gathered, err = registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, gathered)

	histogramMetricFamily = gathered[0]
	msg, err = MetricFamilyToProtobuf(histogramMetricFamily)
	require.NoError(t, err)

	now = time.Now()
	total, added, seriesAdded, err = app.append(msg, "application/vnd.google.protobuf", now)
	if !errors.Is(err, errBucketLimit) {
		t.Fatalf("Did not see expected histogram bucket limit error: %s", err)
	}
	require.NoError(t, app.Rollback())
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 0, seriesAdded) // Series are cached.

	err = sl.metrics.targetScrapeNativeHistogramBucketLimit.Write(&metric)
	require.NoError(t, err)
	metricValue = metric.GetCounter().GetValue()
	require.Equal(t, beforeMetricValue+1, metricValue)
}

func TestScrapeLoop_ChangingMetricString(t *testing.T) {
	// This is a regression test for the scrape loop cache not properly maintaining
	// IDs when the string representation of a metric changes across a scrape. Thus,
	// we use a real storage appender here.
	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))

	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte(`metric_a{a="1",b="1"} 1`), "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = sl.appender()
	_, _, _, err = app.append([]byte(`metric_a{b="1",a="1"} 2`), "text/plain", now.Add(time.Minute))
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	want := []sample{
		{
			L: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			T: timestamp.FromTime(now.Add(time.Minute)),
			V: 2,
		},
	}
	require.Equal(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", appTest)
}

func TestScrapeLoopAppendFailsWithNoContentType(t *testing.T) {
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		// Explicitly setting the lack of fallback protocol here to make it obvious.
		sl.fallbackScrapeProtocol = ""
	})

	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte("metric_a 1\n"), "", now)
	// We expected the appropriate error.
	require.ErrorContains(t, err, "non-compliant scrape target sending blank Content-Type and no fallback_scrape_protocol specified for target", "Expected \"non-compliant scrape\" error but got: %s", err)
}

// TestScrapeLoopAppendEmptyWithNoContentType ensures we there are no errors when we get a blank scrape or just want to append a stale marker.
func TestScrapeLoopAppendEmptyWithNoContentType(t *testing.T) {
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		// Explicitly setting the lack of fallback protocol here to make it obvious.
		sl.fallbackScrapeProtocol = ""
	})

	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte(""), "", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
}

func TestScrapeLoopAppendStaleness(t *testing.T) {
	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))

	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte("metric_a 1\n"), "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = sl.appender()
	_, _, _, err = app.append([]byte(""), "", now.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	want := []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now.Add(time.Second)),
			V: math.Float64frombits(value.StaleNaN),
		},
	}
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", appTest)
}

func TestScrapeLoopAppendNoStalenessIfTimestamp(t *testing.T) {
	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))
	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte("metric_a 1 1000\n"), "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = sl.appender()
	_, _, _, err = app.append([]byte(""), "", now.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	want := []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: 1000,
			V: 1,
		},
	}
	require.Equal(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", appTest)
}

func TestScrapeLoopAppendStalenessIfTrackTimestampStaleness(t *testing.T) {
	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appTest
		sl.trackTimestampsStaleness = true
	})

	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte("metric_a 1 1000\n"), "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = sl.appender()
	_, _, _, err = app.append([]byte(""), "", now.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	want := []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: 1000,
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now.Add(time.Second)),
			V: math.Float64frombits(value.StaleNaN),
		},
	}
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", appTest)
}

func TestScrapeLoopAppendExemplar(t *testing.T) {
	tests := []struct {
		title                           string
		alwaysScrapeClassicHist         bool
		enableNativeHistogramsIngestion bool
		scrapeText                      string
		contentType                     string
		discoveryLabels                 []string
		samples                         []sample
	}{
		{
			title:           "Metric without exemplars",
			scrapeText:      "metric_total{n=\"1\"} 0\n# EOF",
			contentType:     "application/openmetrics-text",
			discoveryLabels: []string{"n", "2"},
			samples: []sample{{
				L: labels.FromStrings("__name__", "metric_total", "exported_n", "1", "n", "2"),
				V: 0,
			}},
		},
		{
			title:           "Metric with exemplars",
			scrapeText:      "metric_total{n=\"1\"} 0 # {a=\"abc\"} 1.0\n# EOF",
			contentType:     "application/openmetrics-text",
			discoveryLabels: []string{"n", "2"},
			samples: []sample{{
				L: labels.FromStrings("__name__", "metric_total", "exported_n", "1", "n", "2"),
				V: 0,
				ES: []exemplar.Exemplar{
					{Labels: labels.FromStrings("a", "abc"), Value: 1},
				},
			}},
		},
		{
			title:           "Metric with exemplars and TS",
			scrapeText:      "metric_total{n=\"1\"} 0 # {a=\"abc\"} 1.0 10000\n# EOF",
			contentType:     "application/openmetrics-text",
			discoveryLabels: []string{"n", "2"},
			samples: []sample{{
				L:  labels.FromStrings("__name__", "metric_total", "exported_n", "1", "n", "2"),
				V:  0,
				ES: []exemplar.Exemplar{{Labels: labels.FromStrings("a", "abc"), Value: 1, Ts: 10000000, HasTs: true}},
			}},
		},
		{
			title: "Two metrics and exemplars",
			scrapeText: `metric_total{n="1"} 1 # {t="1"} 1.0 10000
metric_total{n="2"} 2 # {t="2"} 2.0 20000
# EOF`,
			contentType: "application/openmetrics-text",
			samples: []sample{{
				L:  labels.FromStrings("__name__", "metric_total", "n", "1"),
				V:  1,
				ES: []exemplar.Exemplar{{Labels: labels.FromStrings("t", "1"), Value: 1, Ts: 10000000, HasTs: true}},
			}, {
				L:  labels.FromStrings("__name__", "metric_total", "n", "2"),
				V:  2,
				ES: []exemplar.Exemplar{{Labels: labels.FromStrings("t", "2"), Value: 2, Ts: 20000000, HasTs: true}},
			}},
		},
		{
			title: "Native histogram with three exemplars from classic buckets",

			enableNativeHistogramsIngestion: true,
			scrapeText: `name: "test_histogram"
help: "Test histogram with many buckets removed to keep it manageable in size."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "59727"
        >
        value: -0.00039
        timestamp: <
          seconds: 1625851155
          nanos: 146848499
        >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "5617"
        >
        value: -0.00029
      >
    >
    bucket: <
      cumulative_count: 32
      upper_bound: -0.0001899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "58215"
        >
        value: -0.00019
        timestamp: <
          seconds: 1625851055
          nanos: 146848599
        >
      >
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
  >
  timestamp_ms: 1234568
>

`,
			contentType: "application/vnd.google.protobuf",
			samples: []sample{{
				T: 1234568,
				L: labels.FromStrings("__name__", "test_histogram"),
				H: &histogram.Histogram{
					Count:         175,
					ZeroCount:     2,
					Sum:           0.0008280461746287094,
					ZeroThreshold: 2.938735877055719e-39,
					Schema:        3,
					PositiveSpans: []histogram.Span{
						{Offset: -161, Length: 1},
						{Offset: 8, Length: 3},
					},
					NegativeSpans: []histogram.Span{
						{Offset: -162, Length: 1},
						{Offset: 23, Length: 4},
					},
					PositiveBuckets: []int64{1, 2, -1, -1},
					NegativeBuckets: []int64{1, 3, -2, -1, 1},
				},
				ES: []exemplar.Exemplar{
					// Native histogram exemplars are arranged by timestamp, and those with missing timestamps are dropped.
					{Labels: labels.FromStrings("dummyID", "58215"), Value: -0.00019, Ts: 1625851055146, HasTs: true},
					{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, Ts: 1625851155146, HasTs: true},
				},
			}},
		},
		{
			title: "Native histogram with three exemplars scraped as classic histogram",

			enableNativeHistogramsIngestion: true,
			scrapeText: `name: "test_histogram"
help: "Test histogram with many buckets removed to keep it manageable in size."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "59727"
        >
        value: -0.00039
        timestamp: <
          seconds: 1625851155
          nanos: 146848499
        >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "5617"
        >
        value: -0.00029
      >
    >
    bucket: <
      cumulative_count: 32
      upper_bound: -0.0001899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "58215"
        >
        value: -0.00019
        timestamp: <
          seconds: 1625851055
          nanos: 146848599
        >
      >
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
  >
  timestamp_ms: 1234568
>

`,
			alwaysScrapeClassicHist: true,
			contentType:             "application/vnd.google.protobuf",
			samples: []sample{
				{L: labels.FromStrings("__name__", "test_histogram_count"), T: 1234568, V: 175},
				{L: labels.FromStrings("__name__", "test_histogram_sum"), T: 1234568, V: 0.0008280461746287094},
				{L: labels.FromStrings("__name__", "test_histogram_bucket", "le", "-0.0004899999999999998"), T: 1234568, V: 2},
				{
					L: labels.FromStrings("__name__", "test_histogram_bucket", "le", "-0.0003899999999999998"), T: 1234568, V: 4,
					ES: []exemplar.Exemplar{{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, Ts: 1625851155146, HasTs: true}},
				},
				{
					L: labels.FromStrings("__name__", "test_histogram_bucket", "le", "-0.0002899999999999998"), T: 1234568, V: 16,
					ES: []exemplar.Exemplar{{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, Ts: 1234568, HasTs: false}},
				},
				{
					L: labels.FromStrings("__name__", "test_histogram_bucket", "le", "-0.0001899999999999998"), T: 1234568, V: 32,
					ES: []exemplar.Exemplar{{Labels: labels.FromStrings("dummyID", "58215"), Value: -0.00019, Ts: 1625851055146, HasTs: true}},
				},
				{L: labels.FromStrings("__name__", "test_histogram_bucket", "le", "+Inf"), T: 1234568, V: 175},
				{
					T: 1234568,
					L: labels.FromStrings("__name__", "test_histogram"),
					H: &histogram.Histogram{
						Count:         175,
						ZeroCount:     2,
						Sum:           0.0008280461746287094,
						ZeroThreshold: 2.938735877055719e-39,
						Schema:        3,
						PositiveSpans: []histogram.Span{
							{Offset: -161, Length: 1},
							{Offset: 8, Length: 3},
						},
						NegativeSpans: []histogram.Span{
							{Offset: -162, Length: 1},
							{Offset: 23, Length: 4},
						},
						PositiveBuckets: []int64{1, 2, -1, -1},
						NegativeBuckets: []int64{1, 3, -2, -1, 1},
					},
					ES: []exemplar.Exemplar{
						// Native histogram one is arranged by timestamp.
						// Exemplars with missing timestamps are dropped for native histograms.
						{Labels: labels.FromStrings("dummyID", "58215"), Value: -0.00019, Ts: 1625851055146, HasTs: true},
						{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, Ts: 1625851155146, HasTs: true},
					},
				},
			},
		},
		{
			title:                           "Native histogram with exemplars and no classic buckets",
			contentType:                     "application/vnd.google.protobuf",
			enableNativeHistogramsIngestion: true,
			scrapeText: `name: "test_histogram"
help: "Test histogram."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
	exemplars: <
	  label: <
        name: "dummyID"
        value: "59732"
      >
      value: -0.00039
      timestamp: <
        seconds: 1625851155
        nanos: 146848499
      >
	>
	exemplars: <
	  label: <
        name: "dummyID"
        value: "58242"
      >
      value: -0.00019
      timestamp: <
        seconds: 1625851055
        nanos: 146848599
      >
	>
	exemplars: <
      label: <
        name: "dummyID"
        value: "5617"
      >
      value: -0.00029
    >
  >
  timestamp_ms: 1234568
>

`,
			samples: []sample{{
				T: 1234568,
				L: labels.FromStrings("__name__", "test_histogram"),
				H: &histogram.Histogram{
					Count:         175,
					ZeroCount:     2,
					Sum:           0.0008280461746287094,
					ZeroThreshold: 2.938735877055719e-39,
					Schema:        3,
					PositiveSpans: []histogram.Span{
						{Offset: -161, Length: 1},
						{Offset: 8, Length: 3},
					},
					NegativeSpans: []histogram.Span{
						{Offset: -162, Length: 1},
						{Offset: 23, Length: 4},
					},
					PositiveBuckets: []int64{1, 2, -1, -1},
					NegativeBuckets: []int64{1, 3, -2, -1, 1},
				},
				ES: []exemplar.Exemplar{
					// Exemplars with missing timestamps are dropped for native histograms.
					{Labels: labels.FromStrings("dummyID", "58242"), Value: -0.00019, Ts: 1625851055146, HasTs: true},
					{Labels: labels.FromStrings("dummyID", "59732"), Value: -0.00039, Ts: 1625851155146, HasTs: true},
				},
			}},
		},
		{
			title:                           "Native histogram with exemplars but ingestion disabled",
			contentType:                     "application/vnd.google.protobuf",
			enableNativeHistogramsIngestion: false,
			scrapeText: `name: "test_histogram"
help: "Test histogram."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
	exemplars: <
	  label: <
        name: "dummyID"
        value: "59732"
      >
      value: -0.00039
      timestamp: <
        seconds: 1625851155
        nanos: 146848499
      >
	>
	exemplars: <
	  label: <
        name: "dummyID"
        value: "58242"
      >
      value: -0.00019
      timestamp: <
        seconds: 1625851055
        nanos: 146848599
      >
	>
	exemplars: <
      label: <
        name: "dummyID"
        value: "5617"
      >
      value: -0.00029
    >
  >
  timestamp_ms: 1234568
>

`,
			samples: []sample{
				{L: labels.FromStrings("__name__", "test_histogram_count"), T: 1234568, V: 175},
				{L: labels.FromStrings("__name__", "test_histogram_sum"), T: 1234568, V: 0.0008280461746287094},
				{L: labels.FromStrings("__name__", "test_histogram_bucket", "le", "+Inf"), T: 1234568, V: 175},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			discoveryLabels := &Target{
				labels: labels.FromStrings(test.discoveryLabels...),
			}

			appTest := teststorage.NewAppendable()
			sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
				sl.appendable = appTest
				sl.enableNativeHistogramScraping = test.enableNativeHistogramsIngestion
				sl.sampleMutator = func(l labels.Labels) labels.Labels {
					return mutateSampleLabels(l, discoveryLabels, false, nil)
				}
				sl.reportSampleMutator = func(l labels.Labels) labels.Labels {
					return mutateReportSampleLabels(l, discoveryLabels)
				}
				sl.alwaysScrapeClassicHist = test.alwaysScrapeClassicHist
				// This test does not care about metadata. Having this true would mean we need to add metadata to sample
				// expectations.
				sl.appendMetadataToWAL = false
			})
			app := sl.appender()

			now := time.Now()

			for i := range test.samples {
				if test.samples[i].T != 0 {
					continue
				}
				test.samples[i].T = timestamp.FromTime(now)

				// We need to set the timestamp for expected exemplars that does not have a timestamp.
				for j := range test.samples[i].ES {
					if test.samples[i].ES[j].Ts == 0 {
						test.samples[i].ES[j].Ts = timestamp.FromTime(now)
					}
				}
			}

			buf := &bytes.Buffer{}
			if test.contentType == "application/vnd.google.protobuf" {
				require.NoError(t, textToProto(test.scrapeText, buf))
			} else {
				buf.WriteString(test.scrapeText)
			}

			_, _, _, err := app.append(buf.Bytes(), test.contentType, now)
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			requireEqual(t, test.samples, appTest.ResultSamples())
		})
	}
}

func textToProto(text string, buf *bytes.Buffer) error {
	// In case of protobuf, we have to create the binary representation.
	pb := &dto.MetricFamily{}
	// From text to proto message.
	err := proto.UnmarshalText(text, pb)
	if err != nil {
		return err
	}
	// From proto message to binary protobuf.
	protoBuf, err := proto.Marshal(pb)
	if err != nil {
		return err
	}
	// Write first length, then binary protobuf.
	varintBuf := binary.AppendUvarint(nil, uint64(len(protoBuf)))
	buf.Write(varintBuf)
	buf.Write(protoBuf)
	return nil
}

func TestScrapeLoopAppendExemplarSeries(t *testing.T) {
	scrapeText := []string{`metric_total{n="1"} 1 # {t="1"} 1.0 10000
# EOF`, `metric_total{n="1"} 2 # {t="2"} 2.0 20000
# EOF`}
	samples := []sample{{
		L: labels.FromStrings("__name__", "metric_total", "n", "1"),
		V: 1,
		ES: []exemplar.Exemplar{
			{Labels: labels.FromStrings("t", "1"), Value: 1, Ts: 10000000, HasTs: true},
		},
	}, {
		L: labels.FromStrings("__name__", "metric_total", "n", "1"),
		V: 2,
		ES: []exemplar.Exemplar{
			{Labels: labels.FromStrings("t", "2"), Value: 2, Ts: 20000000, HasTs: true},
		},
	}}
	discoveryLabels := &Target{
		labels: labels.FromStrings(),
	}

	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appTest
		sl.sampleMutator = func(l labels.Labels) labels.Labels {
			return mutateSampleLabels(l, discoveryLabels, false, nil)
		}
		sl.reportSampleMutator = func(l labels.Labels) labels.Labels {
			return mutateReportSampleLabels(l, discoveryLabels)
		}
		// This test does not care about metadata. Having this true would mean we need to add metadata to sample
		// expectations.
		sl.appendMetadataToWAL = false
	})

	now := time.Now()
	for i := range samples {
		ts := now.Add(time.Second * time.Duration(i))
		samples[i].T = timestamp.FromTime(ts)
	}

	for i, st := range scrapeText {
		app := sl.appender()
		_, _, _, err := app.append([]byte(st), "application/openmetrics-text", timestamp.Time(samples[i].T))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	requireEqual(t, samples, appTest.ResultSamples())
}

func TestScrapeLoopRunReportsTargetDownOnScrapeError(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.appendable = appTest
	})
	scraper.scrapeFunc = func(context.Context, io.Writer) error {
		cancel()
		return errors.New("scrape failed")
	}

	sl.run(nil)
	require.Equal(t, 0.0, appTest.ResultSamples()[0].V, "bad 'up' value")
}

func TestScrapeLoopRunReportsTargetDownOnInvalidUTF8(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.appendable = appTest
	})
	scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
		cancel()
		_, _ = w.Write([]byte("a{l=\"\xff\"} 1\n"))
		return nil
	}

	sl.run(nil)
	require.Equal(t, 0.0, appTest.ResultSamples()[0].V, "bad 'up' value")
}

func TestScrapeLoopAppendGracefullyIfAmendOrOutOfOrderOrOutOfBounds(t *testing.T) {
	appTest := teststorage.NewAppendable().WithErrs(
		func(ls labels.Labels) error {
			switch ls.Get(model.MetricNameLabel) {
			case "out_of_order":
				return storage.ErrOutOfOrderSample
			case "amend":
				return storage.ErrDuplicateSampleForTimestamp
			case "out_of_bounds":
				return storage.ErrOutOfBounds
			default:
				return nil
			}
		}, nil, nil)
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))

	now := time.Unix(1, 0)
	app := sl.appender()
	total, added, seriesAdded, err := app.append([]byte("out_of_order 1\namend 1\nnormal 1\nout_of_bounds 1\n"), "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	want := []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "normal"),
			T: timestamp.FromTime(now),
			V: 1,
		},
	}
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", appTest)
	require.Equal(t, 4, total)
	require.Equal(t, 4, added)
	require.Equal(t, 1, seriesAdded)
}

func TestScrapeLoopOutOfBoundsTimeError(t *testing.T) {
	sl, _ := newTestScrapeLoop(t, withAppendable(
		appendableFunc(func(ctx context.Context) storage.Appender {
			return &timeLimitAppender{
				Appender: teststorage.NewAppendable().Appender(ctx),
				maxTime:  timestamp.FromTime(time.Now().Add(10 * time.Minute)),
			}
		}),
	))

	now := time.Now().Add(20 * time.Minute)
	app := sl.appender()
	total, added, seriesAdded, err := app.append([]byte("normal 1\n"), "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 0, seriesAdded)
}

func TestAcceptHeader(t *testing.T) {
	tests := []struct {
		name            string
		scrapeProtocols []config.ScrapeProtocol
		scheme          model.EscapingScheme
		expectedHeader  string
	}{
		{
			name:            "default scrape protocols with underscore escaping",
			scrapeProtocols: config.DefaultScrapeProtocols,
			scheme:          model.UnderscoreEscaping,
			expectedHeader:  "application/openmetrics-text;version=1.0.0;escaping=underscores;q=0.6,application/openmetrics-text;version=0.0.1;q=0.5,text/plain;version=1.0.0;escaping=underscores;q=0.4,text/plain;version=0.0.4;q=0.3,*/*;q=0.2",
		},
		{
			name:            "default proto first scrape protocols with underscore escaping",
			scrapeProtocols: config.DefaultProtoFirstScrapeProtocols,
			scheme:          model.DotsEscaping,
			expectedHeader:  "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.6,application/openmetrics-text;version=1.0.0;escaping=dots;q=0.5,application/openmetrics-text;version=0.0.1;q=0.4,text/plain;version=1.0.0;escaping=dots;q=0.3,text/plain;version=0.0.4;q=0.2,*/*;q=0.1",
		},
		{
			name:            "default scrape protocols with no escaping",
			scrapeProtocols: config.DefaultScrapeProtocols,
			scheme:          model.NoEscaping,
			expectedHeader:  "application/openmetrics-text;version=1.0.0;escaping=allow-utf-8;q=0.6,application/openmetrics-text;version=0.0.1;q=0.5,text/plain;version=1.0.0;escaping=allow-utf-8;q=0.4,text/plain;version=0.0.4;q=0.3,*/*;q=0.2",
		},
		{
			name:            "default proto first scrape protocols with no escaping",
			scrapeProtocols: config.DefaultProtoFirstScrapeProtocols,
			scheme:          model.NoEscaping,
			expectedHeader:  "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.6,application/openmetrics-text;version=1.0.0;escaping=allow-utf-8;q=0.5,application/openmetrics-text;version=0.0.1;q=0.4,text/plain;version=1.0.0;escaping=allow-utf-8;q=0.3,text/plain;version=0.0.4;q=0.2,*/*;q=0.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := acceptHeader(tc.scrapeProtocols, tc.scheme)
			require.Equal(t, tc.expectedHeader, header)
		})
	}
}

// setupTracing temporarily sets the global TracerProvider and Propagator
// and restores the original state after the test completes.
func setupTracing(t *testing.T) {
	t.Helper()

	origTracerProvider := otel.GetTracerProvider()
	origPropagator := otel.GetTextMapPropagator()

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	t.Cleanup(func() {
		otel.SetTracerProvider(origTracerProvider)
		otel.SetTextMapPropagator(origPropagator)
	})
}

// TestRequestTraceparentHeader verifies that the HTTP client used by the target scraper
// propagates the OpenTelemetry "traceparent" header correctly.
func TestRequestTraceparentHeader(t *testing.T) {
	setupTracing(t)

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// the traceparent header is sent.
		require.NotEmpty(t, r.Header.Get("traceparent"))
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	client, err := newScrapeClient(config_util.DefaultHTTPClientConfig, "test")
	require.NoError(t, err)

	ts := &targetScraper{
		Target: &Target{
			labels: labels.FromStrings(
				model.SchemeLabel, serverURL.Scheme,
				model.AddressLabel, serverURL.Host,
			),
			scrapeConfig: &config.ScrapeConfig{},
		},
		client: client,
	}

	resp, err := ts.scrape(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Cleanup(func() { _ = resp.Body.Close() })
}

func TestTargetScraperScrapeOK(t *testing.T) {
	const (
		configTimeout   = 1500 * time.Millisecond
		expectedTimeout = "1.5"
	)

	var (
		protobufParsing bool
		allowUTF8       bool
		qValuePattern   = regexp.MustCompile(`q=([0-9]+(\.\d+)?)`)
	)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accept := r.Header.Get("Accept")
			if allowUTF8 {
				require.Containsf(t, accept, "escaping=allow-utf-8", "Expected Accept header to allow utf8, got %q", accept)
			} else {
				require.NotContainsf(t, accept, "escaping=allow-utf-8", "Expected Accept header to not allow utf8, got %q", accept)
			}
			if protobufParsing {
				require.True(t, strings.HasPrefix(accept, "application/vnd.google.protobuf;"),
					"Expected Accept header to prefer application/vnd.google.protobuf.")
			}

			contentTypes := strings.SplitSeq(accept, ",")
			for st := range contentTypes {
				match := qValuePattern.FindStringSubmatch(st)
				require.Len(t, match, 3)
				qValue, err := strconv.ParseFloat(match[1], 64)
				require.NoError(t, err, "Error parsing q value")
				require.GreaterOrEqual(t, qValue, float64(0))
				require.LessOrEqual(t, qValue, float64(1))
				require.LessOrEqual(t, len(strings.Split(match[1], ".")[1]), 3, "q value should have at most 3 decimal places")
			}

			timeout := r.Header.Get("X-Prometheus-Scrape-Timeout-Seconds")
			require.Equal(t, expectedTimeout, timeout, "Expected scrape timeout header.")

			if allowUTF8 {
				w.Header().Set("Content-Type", `text/plain; version=1.0.0; escaping=allow-utf-8`)
			} else {
				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
			}
			_, _ = w.Write([]byte("metric_a 1\nmetric_b 2\n"))
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	runTest := func(t *testing.T, acceptHeader string) {
		ts := &targetScraper{
			Target: &Target{
				labels: labels.FromStrings(
					model.SchemeLabel, serverURL.Scheme,
					model.AddressLabel, serverURL.Host,
				),
				scrapeConfig: &config.ScrapeConfig{},
			},
			client:       http.DefaultClient,
			timeout:      configTimeout,
			acceptHeader: acceptHeader,
		}
		var buf bytes.Buffer

		resp, err := ts.scrape(context.Background())
		require.NoError(t, err)
		contentType, err := ts.readResponse(context.Background(), resp, &buf)
		require.NoError(t, err)
		if allowUTF8 {
			require.Equal(t, "text/plain; version=1.0.0; escaping=allow-utf-8", contentType)
		} else {
			require.Equal(t, "text/plain; version=0.0.4", contentType)
		}
		require.Equal(t, "metric_a 1\nmetric_b 2\n", buf.String())
	}

	for _, tc := range []struct {
		scrapeProtocols []config.ScrapeProtocol
		scheme          model.EscapingScheme
		protobufParsing bool
		allowUTF8       bool
	}{
		{
			scrapeProtocols: config.DefaultScrapeProtocols,
			scheme:          model.UnderscoreEscaping,
			protobufParsing: false,
			allowUTF8:       false,
		},
		{
			scrapeProtocols: config.DefaultProtoFirstScrapeProtocols,
			scheme:          model.UnderscoreEscaping,
			protobufParsing: true,
			allowUTF8:       false,
		},
		{
			scrapeProtocols: config.DefaultScrapeProtocols,
			scheme:          model.NoEscaping,
			protobufParsing: false,
			allowUTF8:       true,
		},
		{
			scrapeProtocols: config.DefaultProtoFirstScrapeProtocols,
			scheme:          model.NoEscaping,
			protobufParsing: true,
			allowUTF8:       true,
		},
	} {
		t.Run(fmt.Sprintf("%+v", tc), func(t *testing.T) {
			protobufParsing = tc.protobufParsing
			allowUTF8 = tc.allowUTF8
			runTest(t, acceptHeader(tc.scrapeProtocols, tc.scheme))
		})
	}
}

func TestTargetScrapeScrapeCancel(t *testing.T) {
	block := make(chan struct{})

	server := httptest.NewServer(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			<-block
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: &Target{
			labels: labels.FromStrings(
				model.SchemeLabel, serverURL.Scheme,
				model.AddressLabel, serverURL.Host,
			),
			scrapeConfig: &config.ScrapeConfig{},
		},
		client:       http.DefaultClient,
		acceptHeader: acceptHeader(config.DefaultGlobalConfig.ScrapeProtocols, model.UnderscoreEscaping),
	}
	ctx, cancel := context.WithCancel(context.Background())

	errc := make(chan error, 1)

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	go func() {
		_, err := ts.scrape(ctx)
		switch {
		case err == nil:
			errc <- errors.New("expected error but got nil")
		case !errors.Is(ctx.Err(), context.Canceled):
			errc <- fmt.Errorf("expected context cancellation error but got: %w", ctx.Err())
		default:
			close(errc)
		}
	}()

	select {
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape function did not return unexpectedly.")
	case err := <-errc:
		require.NoError(t, err)
	}
	// If this is closed in a defer above the function the test server
	// doesn't terminate and the test doesn't complete.
	close(block)
}

func TestTargetScrapeScrapeNotFound(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: &Target{
			labels: labels.FromStrings(
				model.SchemeLabel, serverURL.Scheme,
				model.AddressLabel, serverURL.Host,
			),
			scrapeConfig: &config.ScrapeConfig{},
		},
		client:       http.DefaultClient,
		acceptHeader: acceptHeader(config.DefaultGlobalConfig.ScrapeProtocols, model.UnderscoreEscaping),
	}

	resp, err := ts.scrape(context.Background())
	require.NoError(t, err)
	_, err = ts.readResponse(context.Background(), resp, io.Discard)
	require.ErrorContains(t, err, "404", "Expected \"404 NotFound\" error but got: %s", err)
}

func TestTargetScraperBodySizeLimit(t *testing.T) {
	const (
		bodySizeLimit = 15
		responseBody  = "metric_a 1\nmetric_b 2\n"
	)
	var gzipResponse bool
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
			if gzipResponse {
				w.Header().Set("Content-Encoding", "gzip")
				gw := gzip.NewWriter(w)
				defer func() { _ = gw.Close() }()
				_, _ = gw.Write([]byte(responseBody))
				return
			}
			_, _ = w.Write([]byte(responseBody))
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: &Target{
			labels: labels.FromStrings(
				model.SchemeLabel, serverURL.Scheme,
				model.AddressLabel, serverURL.Host,
			),
			scrapeConfig: &config.ScrapeConfig{},
		},
		client:        http.DefaultClient,
		bodySizeLimit: bodySizeLimit,
		acceptHeader:  acceptHeader(config.DefaultGlobalConfig.ScrapeProtocols, model.UnderscoreEscaping),
		metrics:       newTestScrapeMetrics(t),
	}
	var buf bytes.Buffer

	// Target response uncompressed body, scrape with body size limit.
	resp, err := ts.scrape(context.Background())
	require.NoError(t, err)
	_, err = ts.readResponse(context.Background(), resp, &buf)
	require.ErrorIs(t, err, errBodySizeLimit)
	require.Equal(t, bodySizeLimit, buf.Len())
	// Target response gzip compressed body, scrape with body size limit.
	gzipResponse = true
	buf.Reset()
	resp, err = ts.scrape(context.Background())
	require.NoError(t, err)
	_, err = ts.readResponse(context.Background(), resp, &buf)
	require.ErrorIs(t, err, errBodySizeLimit)
	require.Equal(t, bodySizeLimit, buf.Len())
	// Target response uncompressed body, scrape without body size limit.
	gzipResponse = false
	buf.Reset()
	ts.bodySizeLimit = 0
	resp, err = ts.scrape(context.Background())
	require.NoError(t, err)
	_, err = ts.readResponse(context.Background(), resp, &buf)
	require.NoError(t, err)
	require.Len(t, responseBody, buf.Len())
	// Target response gzip compressed body, scrape without body size limit.
	gzipResponse = true
	buf.Reset()
	resp, err = ts.scrape(context.Background())
	require.NoError(t, err)
	_, err = ts.readResponse(context.Background(), resp, &buf)
	require.NoError(t, err)
	require.Len(t, responseBody, buf.Len())
}

// testScraper implements the scraper interface and allows setting values
// returned by its methods. It also allows setting a custom scrape function.
type testScraper struct {
	offsetDur time.Duration

	lastStart    time.Time
	lastDuration time.Duration
	lastError    error

	scrapeErr  error
	scrapeFunc func(context.Context, io.Writer) error
}

func (ts *testScraper) offset(time.Duration, uint64) time.Duration {
	return ts.offsetDur
}

func (ts *testScraper) Report(start time.Time, duration time.Duration, err error) {
	ts.lastStart = start
	ts.lastDuration = duration
	ts.lastError = err
}

func (ts *testScraper) scrape(context.Context) (*http.Response, error) {
	return nil, ts.scrapeErr
}

func (ts *testScraper) readResponse(ctx context.Context, _ *http.Response, w io.Writer) (string, error) {
	if ts.scrapeFunc != nil {
		return "", ts.scrapeFunc(ctx, w)
	}
	return "", ts.scrapeErr
}

func TestScrapeLoop_RespectTimestamps(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	appTest := teststorage.NewAppendable().Then(s)
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))

	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte(`metric_a{a="1",b="1"} 1 0`), "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	want := []sample{
		{
			L: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			T: 0,
			V: 1,
		},
	}
	require.Equal(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", appTest)
}

func TestScrapeLoop_DiscardTimestamps(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	appTest := teststorage.NewAppendable().Then(s)
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appTest
		sl.honorTimestamps = false
	})

	now := time.Now()
	app := sl.appender()
	_, _, _, err := app.append([]byte(`metric_a{a="1",b="1"} 1 0`), "text/plain", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	want := []sample{
		{
			L: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			T: timestamp.FromTime(now),
			V: 1,
		},
	}
	require.Equal(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", appTest)
}

func TestScrapeLoopDiscardDuplicateLabels(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	appTest := teststorage.NewAppendable().Then(s)
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))

	// We add a good and a bad metric to check that both are discarded.
	app := sl.appender()
	_, _, _, err := app.append([]byte("test_metric{le=\"500\"} 1\ntest_metric{le=\"600\",le=\"700\"} 1\n"), "text/plain", time.Time{})
	require.Error(t, err)
	require.NoError(t, app.Rollback())
	// We need to cycle staleness cache maps after a manual rollback. Otherwise, they will have old entries in them,
	// which would cause ErrDuplicateSampleForTimestamp errors on the next append.
	sl.cache.iterDone(true)

	q, err := s.Querier(time.Time{}.UnixNano(), 0)
	require.NoError(t, err)
	series := q.Select(sl.ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
	require.False(t, series.Next(), "series found in tsdb")
	require.NoError(t, series.Err())

	// We add a good metric to check that it is recorded.
	app = sl.appender()
	_, _, _, err = app.append([]byte("test_metric{le=\"500\"} 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	q, err = s.Querier(time.Time{}.UnixNano(), 0)
	require.NoError(t, err)
	series = q.Select(sl.ctx, false, nil, labels.MustNewMatcher(labels.MatchEqual, "le", "500"))
	require.True(t, series.Next(), "series not found in tsdb")
	require.NoError(t, series.Err())
	require.False(t, series.Next(), "more than one series found in tsdb")
}

func TestScrapeLoopDiscardUnnamedMetrics(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	appTest := teststorage.NewAppendable().Then(s)
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appTest
		sl.sampleMutator = func(l labels.Labels) labels.Labels {
			if l.Has("drop") {
				return labels.FromStrings("no", "name") // This label set will trigger an error.
			}
			return l
		}
	})

	app := sl.appender()
	_, _, _, err := app.append([]byte("nok 1\nnok2{drop=\"drop\"} 1\n"), "text/plain", time.Time{})
	require.Error(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, errNameLabelMandatory, err)

	q, err := s.Querier(time.Time{}.UnixNano(), 0)
	require.NoError(t, err)
	series := q.Select(sl.ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
	require.False(t, series.Next(), "series found in tsdb")
	require.NoError(t, series.Err())
}

func TestReusableConfig(t *testing.T) {
	variants := []*config.ScrapeConfig{
		{
			JobName:       "prometheus",
			ScrapeTimeout: model.Duration(15 * time.Second),
		},
		{
			JobName:       "httpd",
			ScrapeTimeout: model.Duration(15 * time.Second),
		},
		{
			JobName:       "prometheus",
			ScrapeTimeout: model.Duration(5 * time.Second),
		},
		{
			JobName:     "prometheus",
			MetricsPath: "/metrics",
		},
		{
			JobName:     "prometheus",
			MetricsPath: "/metrics2",
		},
		{
			JobName:       "prometheus",
			ScrapeTimeout: model.Duration(5 * time.Second),
			MetricsPath:   "/metrics2",
		},
		{
			JobName:        "prometheus",
			ScrapeInterval: model.Duration(5 * time.Second),
			MetricsPath:    "/metrics2",
		},
		{
			JobName:        "prometheus",
			ScrapeInterval: model.Duration(5 * time.Second),
			SampleLimit:    1000,
			MetricsPath:    "/metrics2",
		},
	}

	match := [][]int{
		{0, 2},
		{4, 5},
		{4, 6},
		{4, 7},
		{5, 6},
		{5, 7},
		{6, 7},
	}
	noMatch := [][]int{
		{1, 2},
		{0, 4},
		{3, 4},
	}

	for i, m := range match {
		require.True(t, reusableCache(variants[m[0]], variants[m[1]]), "match test %d", i)
		require.True(t, reusableCache(variants[m[1]], variants[m[0]]), "match test %d", i)
		require.True(t, reusableCache(variants[m[1]], variants[m[1]]), "match test %d", i)
		require.True(t, reusableCache(variants[m[0]], variants[m[0]]), "match test %d", i)
	}
	for i, m := range noMatch {
		require.False(t, reusableCache(variants[m[0]], variants[m[1]]), "not match test %d", i)
		require.False(t, reusableCache(variants[m[1]], variants[m[0]]), "not match test %d", i)
	}
}

func TestReuseScrapeCache(t *testing.T) {
	var (
		app = teststorage.NewAppendable()
		cfg = &config.ScrapeConfig{
			JobName:                    "Prometheus",
			ScrapeTimeout:              model.Duration(5 * time.Second),
			ScrapeInterval:             model.Duration(5 * time.Second),
			MetricsPath:                "/metrics",
			MetricNameValidationScheme: model.UTF8Validation,
			MetricNameEscapingScheme:   model.AllowUTF8,
		}
		sp, _ = newScrapePool(cfg, app, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
		t1    = &Target{
			labels: labels.FromStrings("labelNew", "nameNew", "labelNew1", "nameNew1", "labelNew2", "nameNew2"),
			scrapeConfig: &config.ScrapeConfig{
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		}
		proxyURL, _ = url.Parse("http://localhost:2128")
	)
	defer sp.stop()
	sp.sync([]*Target{t1})

	steps := []struct {
		keep      bool
		newConfig *config.ScrapeConfig
	}{
		{
			keep: true,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(5 * time.Second),
				MetricsPath:                "/metrics",
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics2",
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: true,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				SampleLimit:                400,
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics2",
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				HonorTimestamps:            true,
				SampleLimit:                400,
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics2",
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: true,
			newConfig: &config.ScrapeConfig{
				JobName:         "Prometheus",
				HonorTimestamps: true,
				SampleLimit:     400,
				HTTPClientConfig: config_util.HTTPClientConfig{
					ProxyConfig: config_util.ProxyConfig{ProxyURL: config_util.URL{URL: proxyURL}},
				},
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics2",
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				HonorTimestamps:            true,
				HonorLabels:                true,
				SampleLimit:                400,
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics2",
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics",
				LabelLimit:                 1,
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics",
				LabelLimit:                 15,
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics",
				LabelLimit:                 15,
				LabelNameLengthLimit:       5,
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:                    "Prometheus",
				ScrapeInterval:             model.Duration(5 * time.Second),
				ScrapeTimeout:              model.Duration(15 * time.Second),
				MetricsPath:                "/metrics",
				LabelLimit:                 15,
				LabelNameLengthLimit:       5,
				LabelValueLengthLimit:      7,
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
		},
	}

	cacheAddr := func(sp *scrapePool) map[uint64]string {
		r := make(map[uint64]string)
		for fp, l := range sp.loops {
			r[fp] = fmt.Sprintf("%p", l.getCache())
		}
		return r
	}

	for i, s := range steps {
		initCacheAddr := cacheAddr(sp)
		require.NoError(t, sp.reload(s.newConfig))
		for fp, newCacheAddr := range cacheAddr(sp) {
			if s.keep {
				require.Equal(t, initCacheAddr[fp], newCacheAddr, "step %d: old cache and new cache are not the same", i)
			} else {
				require.NotEqual(t, initCacheAddr[fp], newCacheAddr, "step %d: old cache and new cache are the same", i)
			}
		}
		initCacheAddr = cacheAddr(sp)
		require.NoError(t, sp.reload(s.newConfig))
		for fp, newCacheAddr := range cacheAddr(sp) {
			require.Equal(t, initCacheAddr[fp], newCacheAddr, "step %d: reloading the exact config invalidates the cache", i)
		}
	}
}

func TestScrapeAddFast(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	sl, _ := newTestScrapeLoop(t, withAppendable(s))

	app := sl.appender()
	_, _, _, err := app.append([]byte("up 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Poison the cache. There is just one entry, and one series in the
	// storage. Changing the ref will create a 'not found' error.
	for _, v := range sl.getCache().series {
		v.ref++
	}

	app = sl.appender()
	_, _, _, err = app.append([]byte("up 1\n"), "text/plain", time.Time{}.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, app.Commit())
}

func TestReuseCacheRace(t *testing.T) {
	var (
		cfg = &config.ScrapeConfig{
			JobName:                    "Prometheus",
			ScrapeTimeout:              model.Duration(5 * time.Second),
			ScrapeInterval:             model.Duration(5 * time.Second),
			MetricsPath:                "/metrics",
			MetricNameValidationScheme: model.UTF8Validation,
			MetricNameEscapingScheme:   model.AllowUTF8,
		}
		buffers = pool.New(1e3, 100e6, 3, func(sz int) any { return make([]byte, 0, sz) })
		sp, _   = newScrapePool(cfg, teststorage.NewAppendable(), 0, nil, buffers, &Options{}, newTestScrapeMetrics(t))
		t1      = &Target{
			labels:       labels.FromStrings("labelNew", "nameNew"),
			scrapeConfig: &config.ScrapeConfig{},
		}
	)
	defer sp.stop()
	sp.sync([]*Target{t1})

	start := time.Now()
	for i := uint(1); i > 0; i++ {
		if time.Since(start) > 5*time.Second {
			break
		}
		require.NoError(t, sp.reload(&config.ScrapeConfig{
			JobName:                    "Prometheus",
			ScrapeTimeout:              model.Duration(1 * time.Millisecond),
			ScrapeInterval:             model.Duration(1 * time.Millisecond),
			MetricsPath:                "/metrics",
			SampleLimit:                i,
			MetricNameValidationScheme: model.UTF8Validation,
			MetricNameEscapingScheme:   model.AllowUTF8,
		}))
	}
}

func TestCheckAddError(t *testing.T) {
	var appErrs appendErrors
	sl, _ := newTestScrapeLoop(t)
	// TODO: Check err etc
	_, _ = sl.checkAddError(nil, storage.ErrOutOfOrderSample, nil, nil, &appErrs)
	require.Equal(t, 1, appErrs.numOutOfOrder)

	// TODO(bwplotka): Test partial error check and other cases
}

func TestScrapeReportSingleAppender(t *testing.T) {
	t.Parallel()
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	signal := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.appendable = s
		// Since we're writing samples directly below we need to provide a protocol fallback.
		sl.fallbackScrapeProtocol = "text/plain"
	})

	numScrapes := 0
	scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
		numScrapes++
		if numScrapes%4 == 0 {
			return errors.New("scrape failed")
		}
		_, _ = w.Write([]byte("metric_a 44\nmetric_b 44\nmetric_c 44\nmetric_d 44\n"))
		return nil
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	start := time.Now()
	for time.Since(start) < 3*time.Second {
		q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
		require.NoError(t, err)
		series := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".+"))

		c := 0
		for series.Next() {
			i := series.At().Iterator(nil)
			for i.Next() != chunkenc.ValNone {
				c++
			}
		}

		require.Equal(t, 0, c%9, "Appended samples not as expected: %d", c)
		require.NoError(t, q.Close())
	}
	cancel()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape wasn't stopped.")
	}
}

func TestScrapeReportLimit(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	cfg := &config.ScrapeConfig{
		JobName:                    "test",
		SampleLimit:                5,
		Scheme:                     "http",
		ScrapeInterval:             model.Duration(100 * time.Millisecond),
		ScrapeTimeout:              model.Duration(100 * time.Millisecond),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
	}

	ts, scrapedTwice := newScrapableServer("metric_a 44\nmetric_b 44\nmetric_c 44\nmetric_d 44\n")
	defer ts.Close()

	sp, err := newScrapePool(cfg, s, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
	require.NoError(t, err)
	defer sp.stop()

	testURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	sp.Sync([]*targetgroup.Group{
		{
			Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(testURL.Host)}},
		},
	})

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("target was not scraped twice")
	case <-scrapedTwice:
		// If the target has been scraped twice, report samples from the first
		// scrape have been inserted in the database.
	}

	ctx := t.Context()
	q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	series := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "up"))

	var found bool
	for series.Next() {
		i := series.At().Iterator(nil)
		for i.Next() == chunkenc.ValFloat {
			_, v := i.At()
			require.Equal(t, 1.0, v)
			found = true
		}
	}

	require.True(t, found)
}

func TestScrapeUTF8(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	cfg := &config.ScrapeConfig{
		JobName:                    "test",
		Scheme:                     "http",
		ScrapeInterval:             model.Duration(100 * time.Millisecond),
		ScrapeTimeout:              model.Duration(100 * time.Millisecond),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
	}
	ts, scrapedTwice := newScrapableServer("{\"with.dots\"} 42\n")
	defer ts.Close()

	sp, err := newScrapePool(cfg, s, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
	require.NoError(t, err)
	defer sp.stop()

	testURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	sp.Sync([]*targetgroup.Group{
		{
			Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(testURL.Host)}},
		},
	})

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("target was not scraped twice")
	case <-scrapedTwice:
		// If the target has been scraped twice, report samples from the first
		// scrape have been inserted in the database.
	}

	ctx := t.Context()
	q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })
	series := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "with.dots"))

	require.True(t, series.Next(), "series not found in tsdb")
}

func TestScrapeLoopLabelLimit(t *testing.T) {
	tests := []struct {
		title           string
		scrapeLabels    string
		discoveryLabels []string
		labelLimits     labelLimits
		expectErr       bool
	}{
		{
			title:           "Valid number of labels",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelLimit: 5},
			expectErr:       false,
		}, {
			title:           "Too many labels",
			scrapeLabels:    `metric{l1="1", l2="2", l3="3", l4="4", l5="5", l6="6"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelLimit: 5},
			expectErr:       true,
		}, {
			title:           "Too many labels including discovery labels",
			scrapeLabels:    `metric{l1="1", l2="2", l3="3", l4="4"} 0`,
			discoveryLabels: []string{"l5", "5", "l6", "6"},
			labelLimits:     labelLimits{labelLimit: 5},
			expectErr:       true,
		}, {
			title:           "Valid labels name length",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelNameLengthLimit: 10},
			expectErr:       false,
		}, {
			title:           "Label name too long",
			scrapeLabels:    `metric{label_name_too_long="0"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelNameLengthLimit: 10},
			expectErr:       true,
		}, {
			title:           "Discovery label name too long",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: []string{"label_name_too_long", "0"},
			labelLimits:     labelLimits{labelNameLengthLimit: 10},
			expectErr:       true,
		}, {
			title:           "Valid labels value length",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelValueLengthLimit: 10},
			expectErr:       false,
		}, {
			title:           "Label value too long",
			scrapeLabels:    `metric{l1="label_value_too_long"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelValueLengthLimit: 10},
			expectErr:       true,
		}, {
			title:           "Discovery label value too long",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: []string{"l1", "label_value_too_long"},
			labelLimits:     labelLimits{labelValueLengthLimit: 10},
			expectErr:       true,
		},
	}

	for _, test := range tests {
		discoveryLabels := &Target{
			labels: labels.FromStrings(test.discoveryLabels...),
		}

		sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
			sl.sampleMutator = func(l labels.Labels) labels.Labels {
				return mutateSampleLabels(l, discoveryLabels, false, nil)
			}
			sl.reportSampleMutator = func(l labels.Labels) labels.Labels {
				return mutateReportSampleLabels(l, discoveryLabels)
			}
			sl.labelLimits = &test.labelLimits
		})

		app := sl.appender()
		_, _, _, err := app.append([]byte(test.scrapeLabels), "text/plain", time.Now())

		t.Logf("Test:%s", test.title)
		if test.expectErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.NoError(t, app.Commit())
		}
	}
}

func TestTargetScrapeIntervalAndTimeoutRelabel(t *testing.T) {
	interval, _ := model.ParseDuration("2s")
	timeout, _ := model.ParseDuration("500ms")
	cfg := &config.ScrapeConfig{
		ScrapeInterval:             interval,
		ScrapeTimeout:              timeout,
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels:         model.LabelNames{model.ScrapeIntervalLabel},
				Regex:                relabel.MustNewRegexp("2s"),
				Replacement:          "3s",
				TargetLabel:          model.ScrapeIntervalLabel,
				Action:               relabel.Replace,
				NameValidationScheme: model.UTF8Validation,
			},
			{
				SourceLabels:         model.LabelNames{model.ScrapeTimeoutLabel},
				Regex:                relabel.MustNewRegexp("500ms"),
				Replacement:          "750ms",
				TargetLabel:          model.ScrapeTimeoutLabel,
				Action:               relabel.Replace,
				NameValidationScheme: model.UTF8Validation,
			},
		},
	}
	sp, _ := newScrapePool(cfg, teststorage.NewAppendable(), 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
	tgts := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{{model.AddressLabel: "127.0.0.1:9090"}},
		},
	}

	sp.Sync(tgts)
	defer sp.stop()

	require.Equal(t, "3s", sp.ActiveTargets()[0].labels.Get(model.ScrapeIntervalLabel))
	require.Equal(t, "750ms", sp.ActiveTargets()[0].labels.Get(model.ScrapeTimeoutLabel))
}

// Testing whether we can remove trailing .0 from histogram 'le' and summary 'quantile' labels.
func TestLeQuantileReLabel(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	cfg := &config.ScrapeConfig{
		JobName: "test",
		MetricRelabelConfigs: []*relabel.Config{
			{
				SourceLabels:         model.LabelNames{"le", "__name__"},
				Regex:                relabel.MustNewRegexp("(\\d+)\\.0+;.*_bucket"),
				Replacement:          relabel.DefaultRelabelConfig.Replacement,
				Separator:            relabel.DefaultRelabelConfig.Separator,
				TargetLabel:          "le",
				Action:               relabel.Replace,
				NameValidationScheme: model.UTF8Validation,
			},
			{
				SourceLabels:         model.LabelNames{"quantile"},
				Regex:                relabel.MustNewRegexp("(\\d+)\\.0+"),
				Replacement:          relabel.DefaultRelabelConfig.Replacement,
				Separator:            relabel.DefaultRelabelConfig.Separator,
				TargetLabel:          "quantile",
				Action:               relabel.Replace,
				NameValidationScheme: model.UTF8Validation,
			},
		},
		SampleLimit:                100,
		Scheme:                     "http",
		ScrapeInterval:             model.Duration(100 * time.Millisecond),
		ScrapeTimeout:              model.Duration(100 * time.Millisecond),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
	}

	metricsText := `
# HELP test_histogram This is a histogram with default buckets
# TYPE test_histogram histogram
test_histogram_bucket{address="0.0.0.0",port="5001",le="0.005"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="0.01"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="0.025"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="0.05"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="0.1"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="0.25"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="0.5"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="1.0"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="2.5"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="5.0"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="10.0"} 0
test_histogram_bucket{address="0.0.0.0",port="5001",le="+Inf"} 0
test_histogram_sum{address="0.0.0.0",port="5001"} 0
test_histogram_count{address="0.0.0.0",port="5001"} 0
# HELP test_summary Number of inflight requests sampled at a regular interval. Quantile buckets keep track of inflight requests over the last 60s.
# TYPE test_summary summary
test_summary{quantile="0.5"} 0
test_summary{quantile="0.9"} 0
test_summary{quantile="0.95"} 0
test_summary{quantile="0.99"} 0
test_summary{quantile="1.0"} 1
test_summary_sum 1
test_summary_count 199
`

	// The expected "le" values do not have the trailing ".0".
	expectedLeValues := []string{"0.005", "0.01", "0.025", "0.05", "0.1", "0.25", "0.5", "1", "2.5", "5", "10", "+Inf"}

	// The expected "quantile" values do not have the trailing ".0".
	expectedQuantileValues := []string{"0.5", "0.9", "0.95", "0.99", "1"}

	ts, scrapedTwice := newScrapableServer(metricsText)
	defer ts.Close()

	sp, err := newScrapePool(cfg, s, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
	require.NoError(t, err)
	defer sp.stop()

	testURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	sp.Sync([]*targetgroup.Group{
		{
			Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(testURL.Host)}},
		},
	})
	require.Len(t, sp.ActiveTargets(), 1)

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("target was not scraped")
	case <-scrapedTwice:
	}

	ctx := t.Context()
	q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	checkValues := func(labelName string, expectedValues []string, series storage.SeriesSet) {
		foundLeValues := map[string]bool{}

		for series.Next() {
			s := series.At()
			v := s.Labels().Get(labelName)
			require.NotContains(t, foundLeValues, v, "duplicate label value found")
			foundLeValues[v] = true
		}

		require.Len(t, foundLeValues, len(expectedValues), "number of label values not as expected")
		for _, v := range expectedValues {
			require.Contains(t, foundLeValues, v, "label value not found")
		}
	}

	series := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "test_histogram_bucket"))
	checkValues("le", expectedLeValues, series)

	series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "test_summary"))
	checkValues("quantile", expectedQuantileValues, series)
}

// Testing whether we can automatically convert scraped classic histograms into native histograms with custom buckets.
func TestConvertClassicHistogramsToNHCB(t *testing.T) {
	t.Parallel()

	genTestCounterText := func(name string) string {
		return fmt.Sprintf(`
# HELP %s some help text
# TYPE %s counter
%s{address="0.0.0.0",port="5001"} 1
`, name, name, name)
	}
	genTestHistText := func(name string) string {
		data := map[string]any{
			"name": name,
		}
		b := &bytes.Buffer{}
		require.NoError(t, template.Must(template.New("").Parse(`
# HELP {{.name}} This is a histogram with default buckets
# TYPE {{.name}} histogram
{{.name}}_bucket{address="0.0.0.0",port="5001",le="0.005"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="0.01"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="0.025"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="0.05"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="0.1"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="0.25"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="0.5"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="1"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="2.5"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="5"} 0
{{.name}}_bucket{address="0.0.0.0",port="5001",le="10"} 1
{{.name}}_bucket{address="0.0.0.0",port="5001",le="+Inf"} 1
{{.name}}_sum{address="0.0.0.0",port="5001"} 10
{{.name}}_count{address="0.0.0.0",port="5001"} 1
`)).Execute(b, data))
		return b.String()
	}
	genTestCounterProto := func(name string) string {
		return fmt.Sprintf(`
name: "%s"
help: "some help text"
type: COUNTER
metric: <
	label: <
		name: "address"
		value: "0.0.0.0"
	>
	label: <
		name: "port"
		value: "5001"
	>
	counter: <
		value: %d
	>
>
`, name, 1)
	}
	genTestHistProto := func(name string, hasClassic, hasExponential bool) string {
		var classic string
		if hasClassic {
			classic = `
bucket: <
	cumulative_count: 0
	upper_bound: 0.005
>
bucket: <
	cumulative_count: 0
	upper_bound: 0.01
>
bucket: <
	cumulative_count: 0
	upper_bound: 0.025
>
bucket: <
	cumulative_count: 0
	upper_bound: 0.05
>
bucket: <
	cumulative_count: 0
	upper_bound: 0.1
>
bucket: <
	cumulative_count: 0
	upper_bound: 0.25
>
bucket: <
	cumulative_count: 0
	upper_bound: 0.5
>
bucket: <
	cumulative_count: 0
	upper_bound: 1
>
bucket: <
	cumulative_count: 0
	upper_bound: 2.5
>
bucket: <
	cumulative_count: 0
	upper_bound: 5
>
bucket: <
	cumulative_count: 1
	upper_bound: 10
>`
		}
		var expo string
		if hasExponential {
			expo = `
schema: 3
zero_threshold: 2.938735877055719e-39
zero_count: 0
positive_span: <
	offset: 2
	length: 1
>
positive_delta: 1`
		}
		return fmt.Sprintf(`
name: "%s"
help: "This is a histogram with default buckets"
type: HISTOGRAM
metric: <
	label: <
		name: "address"
		value: "0.0.0.0"
	>
	label: <
		name: "port"
		value: "5001"
	>
	histogram: <
		sample_count: 1
		sample_sum: 10
		%s
		%s
	>
>
`, name, classic, expo)
	}

	metricsTexts := map[string]struct {
		text           []string
		contentType    string
		hasClassic     bool
		hasExponential bool
	}{
		"text": {
			text: []string{
				genTestCounterText("test_metric_1"),
				genTestCounterText("test_metric_1_count"),
				genTestCounterText("test_metric_1_sum"),
				genTestCounterText("test_metric_1_bucket"),
				genTestHistText("test_histogram_1"),
				genTestCounterText("test_metric_2"),
				genTestCounterText("test_metric_2_count"),
				genTestCounterText("test_metric_2_sum"),
				genTestCounterText("test_metric_2_bucket"),
				genTestHistText("test_histogram_2"),
				genTestCounterText("test_metric_3"),
				genTestCounterText("test_metric_3_count"),
				genTestCounterText("test_metric_3_sum"),
				genTestCounterText("test_metric_3_bucket"),
				genTestHistText("test_histogram_3"),
			},
			hasClassic: true,
		},
		"text, in different order": {
			text: []string{
				genTestCounterText("test_metric_1"),
				genTestCounterText("test_metric_1_count"),
				genTestCounterText("test_metric_1_sum"),
				genTestCounterText("test_metric_1_bucket"),
				genTestHistText("test_histogram_1"),
				genTestCounterText("test_metric_2"),
				genTestCounterText("test_metric_2_count"),
				genTestCounterText("test_metric_2_sum"),
				genTestCounterText("test_metric_2_bucket"),
				genTestHistText("test_histogram_2"),
				genTestHistText("test_histogram_3"),
				genTestCounterText("test_metric_3"),
				genTestCounterText("test_metric_3_count"),
				genTestCounterText("test_metric_3_sum"),
				genTestCounterText("test_metric_3_bucket"),
			},
			hasClassic: true,
		},
		"protobuf": {
			text: []string{
				genTestCounterProto("test_metric_1"),
				genTestCounterProto("test_metric_1_count"),
				genTestCounterProto("test_metric_1_sum"),
				genTestCounterProto("test_metric_1_bucket"),
				genTestHistProto("test_histogram_1", true, false),
				genTestCounterProto("test_metric_2"),
				genTestCounterProto("test_metric_2_count"),
				genTestCounterProto("test_metric_2_sum"),
				genTestCounterProto("test_metric_2_bucket"),
				genTestHistProto("test_histogram_2", true, false),
				genTestCounterProto("test_metric_3"),
				genTestCounterProto("test_metric_3_count"),
				genTestCounterProto("test_metric_3_sum"),
				genTestCounterProto("test_metric_3_bucket"),
				genTestHistProto("test_histogram_3", true, false),
			},
			contentType: "application/vnd.google.protobuf",
			hasClassic:  true,
		},
		"protobuf, in different order": {
			text: []string{
				genTestHistProto("test_histogram_1", true, false),
				genTestCounterProto("test_metric_1"),
				genTestCounterProto("test_metric_1_count"),
				genTestCounterProto("test_metric_1_sum"),
				genTestCounterProto("test_metric_1_bucket"),
				genTestHistProto("test_histogram_2", true, false),
				genTestCounterProto("test_metric_2"),
				genTestCounterProto("test_metric_2_count"),
				genTestCounterProto("test_metric_2_sum"),
				genTestCounterProto("test_metric_2_bucket"),
				genTestHistProto("test_histogram_3", true, false),
				genTestCounterProto("test_metric_3"),
				genTestCounterProto("test_metric_3_count"),
				genTestCounterProto("test_metric_3_sum"),
				genTestCounterProto("test_metric_3_bucket"),
			},
			contentType: "application/vnd.google.protobuf",
			hasClassic:  true,
		},
		"protobuf, with additional native exponential histogram": {
			text: []string{
				genTestCounterProto("test_metric_1"),
				genTestCounterProto("test_metric_1_count"),
				genTestCounterProto("test_metric_1_sum"),
				genTestCounterProto("test_metric_1_bucket"),
				genTestHistProto("test_histogram_1", true, true),
				genTestCounterProto("test_metric_2"),
				genTestCounterProto("test_metric_2_count"),
				genTestCounterProto("test_metric_2_sum"),
				genTestCounterProto("test_metric_2_bucket"),
				genTestHistProto("test_histogram_2", true, true),
				genTestCounterProto("test_metric_3"),
				genTestCounterProto("test_metric_3_count"),
				genTestCounterProto("test_metric_3_sum"),
				genTestCounterProto("test_metric_3_bucket"),
				genTestHistProto("test_histogram_3", true, true),
			},
			contentType:    "application/vnd.google.protobuf",
			hasClassic:     true,
			hasExponential: true,
		},
		"protobuf, with only native exponential histogram": {
			text: []string{
				genTestCounterProto("test_metric_1"),
				genTestCounterProto("test_metric_1_count"),
				genTestCounterProto("test_metric_1_sum"),
				genTestCounterProto("test_metric_1_bucket"),
				genTestHistProto("test_histogram_1", false, true),
				genTestCounterProto("test_metric_2"),
				genTestCounterProto("test_metric_2_count"),
				genTestCounterProto("test_metric_2_sum"),
				genTestCounterProto("test_metric_2_bucket"),
				genTestHistProto("test_histogram_2", false, true),
				genTestCounterProto("test_metric_3"),
				genTestCounterProto("test_metric_3_count"),
				genTestCounterProto("test_metric_3_sum"),
				genTestCounterProto("test_metric_3_bucket"),
				genTestHistProto("test_histogram_3", false, true),
			},
			contentType:    "application/vnd.google.protobuf",
			hasExponential: true,
		},
	}

	checkBucketValues := func(t testing.TB, expectedCount int, series storage.SeriesSet) {
		labelName := "le"
		var expectedValues []string
		if expectedCount > 0 {
			expectedValues = []string{"0.005", "0.01", "0.025", "0.05", "0.1", "0.25", "0.5", "1.0", "2.5", "5.0", "10.0", "+Inf"}
		}
		foundLeValues := map[string]bool{}

		for series.Next() {
			s := series.At()
			v := s.Labels().Get(labelName)
			require.NotContains(t, foundLeValues, v, "duplicate label value found")
			foundLeValues[v] = true
		}

		require.Len(t, foundLeValues, len(expectedValues), "unexpected number of label values, expected %v but found %v", expectedValues, foundLeValues)
		for _, v := range expectedValues {
			require.Contains(t, foundLeValues, v, "label value not found")
		}
	}

	// Checks that the expected series is present and runs a basic sanity check of the float values.
	checkFloatSeries := func(t testing.TB, series storage.SeriesSet, expectedCount int, expectedFloat float64) {
		count := 0
		for series.Next() {
			i := series.At().Iterator(nil)
		loop:
			for {
				switch i.Next() {
				case chunkenc.ValNone:
					break loop
				case chunkenc.ValFloat:
					_, f := i.At()
					require.Equal(t, expectedFloat, f)
				case chunkenc.ValHistogram:
					panic("unexpected value type: histogram")
				case chunkenc.ValFloatHistogram:
					panic("unexpected value type: float histogram")
				default:
					panic("unexpected value type")
				}
			}
			count++
		}
		require.Equal(t, expectedCount, count, "number of float series not as expected")
	}

	// Checks that the expected series is present and runs a basic sanity check of the histogram values.
	checkHistSeries := func(t testing.TB, series storage.SeriesSet, expectedCount int, expectedSchema int32) {
		count := 0
		for series.Next() {
			i := series.At().Iterator(nil)
		loop:
			for {
				switch i.Next() {
				case chunkenc.ValNone:
					break loop
				case chunkenc.ValFloat:
					panic("unexpected value type: float")
				case chunkenc.ValHistogram:
					_, h := i.AtHistogram(nil)
					require.Equal(t, expectedSchema, h.Schema)
					require.Equal(t, uint64(1), h.Count)
					require.Equal(t, 10.0, h.Sum)
				case chunkenc.ValFloatHistogram:
					_, h := i.AtFloatHistogram(nil)
					require.Equal(t, expectedSchema, h.Schema)
					require.Equal(t, uint64(1), h.Count)
					require.Equal(t, 10.0, h.Sum)
				default:
					panic("unexpected value type")
				}
			}
			count++
		}
		require.Equal(t, expectedCount, count, "number of histogram series not as expected")
	}

	for metricsTextName, metricsText := range metricsTexts {
		for name, tc := range map[string]struct {
			alwaysScrapeClassicHistograms bool
			convertClassicHistToNHCB      bool
		}{
			"convert with scrape": {
				alwaysScrapeClassicHistograms: true,
				convertClassicHistToNHCB:      true,
			},
			"convert without scrape": {
				alwaysScrapeClassicHistograms: false,
				convertClassicHistToNHCB:      true,
			},
			"scrape without convert": {
				alwaysScrapeClassicHistograms: true,
				convertClassicHistToNHCB:      false,
			},
			"scrape with nil convert": {
				alwaysScrapeClassicHistograms: true,
			},
			"neither scrape nor convert": {
				alwaysScrapeClassicHistograms: false,
				convertClassicHistToNHCB:      false,
			},
		} {
			var expectedClassicHistCount, expectedNativeHistCount int
			var expectCustomBuckets bool
			if metricsText.hasExponential {
				expectedNativeHistCount = 1
				expectCustomBuckets = false
				expectedClassicHistCount = 0
				if metricsText.hasClassic && tc.alwaysScrapeClassicHistograms {
					expectedClassicHistCount = 1
				}
			} else if metricsText.hasClassic {
				switch {
				case !tc.convertClassicHistToNHCB:
					expectedClassicHistCount = 1
					expectedNativeHistCount = 0
				case tc.alwaysScrapeClassicHistograms && tc.convertClassicHistToNHCB:
					expectedClassicHistCount = 1
					expectedNativeHistCount = 1
					expectCustomBuckets = true
				case !tc.alwaysScrapeClassicHistograms && tc.convertClassicHistToNHCB:
					expectedClassicHistCount = 0
					expectedNativeHistCount = 1
					expectCustomBuckets = true
				}
			}

			t.Run(fmt.Sprintf("%s with %s", name, metricsTextName), func(t *testing.T) {
				t.Parallel()
				s := teststorage.New(t)
				t.Cleanup(func() { _ = s.Close() })

				sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
					sl.appendable = s
					sl.alwaysScrapeClassicHist = tc.alwaysScrapeClassicHistograms
					sl.convertClassicHistToNHCB = tc.convertClassicHistToNHCB
					sl.enableNativeHistogramScraping = true
				})

				var content []byte
				contentType := metricsText.contentType
				switch contentType {
				case "application/vnd.google.protobuf":
					buf := &bytes.Buffer{}
					for _, text := range metricsText.text {
						// In case of protobuf, we have to create the binary representation.
						pb := &dto.MetricFamily{}
						// From text to proto message.
						require.NoError(t, proto.UnmarshalText(text, pb))
						// From proto message to binary protobuf.
						buf.Write(protoMarshalDelimited(t, pb))
					}
					content = buf.Bytes()
				case "text/plain", "":
					// The input text fragments already have a newline at the
					// end, so we just concatenate them without separator.
					content = []byte(strings.Join(metricsText.text, ""))
					contentType = "text/plain"
				default:
					t.Error("unexpected content type")
				}
				now := time.Now()
				app := sl.appender()
				_, _, _, err := app.append(content, contentType, now)
				require.NoError(t, err)
				require.NoError(t, app.Commit())

				var expectedSchema int32
				if expectCustomBuckets {
					expectedSchema = histogram.CustomBucketsSchema
				} else {
					expectedSchema = 3
				}

				// Validated what was appended can be queried.
				ctx := t.Context()
				q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
				require.NoError(t, err)
				t.Cleanup(func() { _ = q.Close() })

				var series storage.SeriesSet
				for i := 1; i <= 3; i++ {
					series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", fmt.Sprintf("test_metric_%d", i)))
					checkFloatSeries(t, series, 1, 1.)

					series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", fmt.Sprintf("test_metric_%d_count", i)))
					checkFloatSeries(t, series, 1, 1.)

					series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", fmt.Sprintf("test_metric_%d_sum", i)))
					checkFloatSeries(t, series, 1, 1.)

					series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", fmt.Sprintf("test_metric_%d_bucket", i)))
					checkFloatSeries(t, series, 1, 1.)

					series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", fmt.Sprintf("test_histogram_%d_count", i)))
					checkFloatSeries(t, series, expectedClassicHistCount, 1.)

					series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", fmt.Sprintf("test_histogram_%d_sum", i)))
					checkFloatSeries(t, series, expectedClassicHistCount, 10.)

					series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", fmt.Sprintf("test_histogram_%d_bucket", i)))
					checkBucketValues(t, expectedClassicHistCount, series)

					series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", fmt.Sprintf("test_histogram_%d", i)))
					checkHistSeries(t, series, expectedNativeHistCount, expectedSchema)
				}
			})
		}
	}
}

func TestTypeUnitReLabel(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	cfg := &config.ScrapeConfig{
		JobName: "test",
		MetricRelabelConfigs: []*relabel.Config{
			{
				SourceLabels:         model.LabelNames{"__name__"},
				Regex:                relabel.MustNewRegexp(".*_total$"),
				Replacement:          "counter",
				TargetLabel:          "__type__",
				Action:               relabel.Replace,
				NameValidationScheme: model.UTF8Validation,
			},
			{
				SourceLabels:         model.LabelNames{"__name__"},
				Regex:                relabel.MustNewRegexp(".*_bytes$"),
				Replacement:          "bytes",
				TargetLabel:          "__unit__",
				Action:               relabel.Replace,
				NameValidationScheme: model.UTF8Validation,
			},
		},
		SampleLimit:                100,
		Scheme:                     "http",
		ScrapeInterval:             model.Duration(100 * time.Millisecond),
		ScrapeTimeout:              model.Duration(100 * time.Millisecond),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
	}

	metricsText := `
# HELP test_metric_1_total This is a counter
# TYPE test_metric_1_total counter
test_metric_1_total 123

# HELP test_metric_2_total This is a counter
# TYPE test_metric_2_total counter
test_metric_2_total 234

# HELP disk_usage_bytes This is a gauge
# TYPE disk_usage_bytes gauge
disk_usage_bytes 456
`

	ts, scrapedTwice := newScrapableServer(metricsText)
	defer ts.Close()

	sp, err := newScrapePool(cfg, s, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
	require.NoError(t, err)
	defer sp.stop()

	testURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	sp.Sync([]*targetgroup.Group{
		{
			Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(testURL.Host)}},
		},
	})
	require.Len(t, sp.ActiveTargets(), 1)

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("target was not scraped")
	case <-scrapedTwice:
	}

	ctx := t.Context()
	q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Close() })

	series := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*_total$"))
	for series.Next() {
		s := series.At()
		require.Equal(t, "counter", s.Labels().Get("__type__"))
	}

	series = q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "disk_usage_bytes"))
	for series.Next() {
		s := series.At()
		require.Equal(t, "bytes", s.Labels().Get("__unit__"))
	}
}

func TestScrapeLoopRunCreatesStaleMarkersOnFailedScrapeForTimestampedMetrics(t *testing.T) {
	signal := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.ctx = ctx
		sl.appendable = appTest // Since we're writing samples directly below we need to provide a protocol fallback.
		sl.fallbackScrapeProtocol = "text/plain"
		sl.trackTimestampsStaleness = true
	})

	// Succeed once, several failures, then stop.
	numScrapes := 0
	scraper.scrapeFunc = func(_ context.Context, w io.Writer) error {
		numScrapes++

		switch numScrapes {
		case 1:
			_, _ = fmt.Fprintf(w, "metric_a 42 %d\n", time.Now().UnixNano()/int64(time.Millisecond))
			return nil
		case 5:
			cancel()
		}
		return errors.New("scrape failed")
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	got := appTest.ResultSamples()
	// 1 successfully scraped sample, 1 stale marker after first fail, 5 report samples for
	// each scrape successful or not.
	require.Len(t, got, 27, "Appended samples not as expected:\n%s", appTest)
	require.Equal(t, 42.0, got[0].V, "Appended first sample not as expected")
	require.True(t, value.IsStaleNaN(got[6].V),
		"Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(got[6].V))
}

func TestScrapeLoopCompression(t *testing.T) {
	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })

	metricsText := makeTestGauges(10)

	for _, tc := range []struct {
		enableCompression bool
		acceptEncoding    string
	}{
		{
			enableCompression: true,
			acceptEncoding:    "gzip",
		},
		{
			enableCompression: false,
			acceptEncoding:    "identity",
		},
	} {
		t.Run(fmt.Sprintf("compression=%v,acceptEncoding=%s", tc.enableCompression, tc.acceptEncoding), func(t *testing.T) {
			scraped := make(chan bool)

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, tc.acceptEncoding, r.Header.Get("Accept-Encoding"), "invalid value of the Accept-Encoding header")
				_, _ = fmt.Fprint(w, string(metricsText))
				close(scraped)
			}))
			defer ts.Close()

			cfg := &config.ScrapeConfig{
				JobName:                    "test",
				SampleLimit:                100,
				Scheme:                     "http",
				ScrapeInterval:             model.Duration(100 * time.Millisecond),
				ScrapeTimeout:              model.Duration(100 * time.Millisecond),
				EnableCompression:          tc.enableCompression,
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			}

			sp, err := newScrapePool(cfg, s, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
			require.NoError(t, err)
			defer sp.stop()

			testURL, err := url.Parse(ts.URL)
			require.NoError(t, err)
			sp.Sync([]*targetgroup.Group{
				{
					Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(testURL.Host)}},
				},
			})
			require.Len(t, sp.ActiveTargets(), 1)

			select {
			case <-time.After(5 * time.Second):
				t.Fatalf("target was not scraped")
			case <-scraped:
			}
		})
	}
}

func TestPickSchema(t *testing.T) {
	tcs := []struct {
		factor float64
		schema int32
	}{
		{
			factor: 65536,
			schema: -4,
		},
		{
			factor: 256,
			schema: -3,
		},
		{
			factor: 16,
			schema: -2,
		},
		{
			factor: 4,
			schema: -1,
		},
		{
			factor: 2,
			schema: 0,
		},
		{
			factor: 1.4,
			schema: 1,
		},
		{
			factor: 1.1,
			schema: 2,
		},
		{
			factor: 1.09,
			schema: 3,
		},
		{
			factor: 1.04,
			schema: 4,
		},
		{
			factor: 1.02,
			schema: 5,
		},
		{
			factor: 1.01,
			schema: 6,
		},
		{
			factor: 1.005,
			schema: 7,
		},
		{
			factor: 1.002,
			schema: 8,
		},
		// The default value of native_histogram_min_bucket_factor
		{
			factor: 0,
			schema: 8,
		},
	}

	for _, tc := range tcs {
		schema := pickSchema(tc.factor)
		require.Equal(t, tc.schema, schema)
	}
}

func BenchmarkTargetScraperGzip(b *testing.B) {
	scenarios := []struct {
		metricsCount int
		body         []byte
	}{
		{metricsCount: 1},
		{metricsCount: 100},
		{metricsCount: 1000},
		{metricsCount: 10000},
		{metricsCount: 100000},
	}

	for i := range scenarios {
		var buf bytes.Buffer
		var name string
		gw := gzip.NewWriter(&buf)
		for j := 0; j < scenarios[i].metricsCount; j++ {
			name = fmt.Sprintf("go_memstats_alloc_bytes_total_%d", j)
			_, _ = fmt.Fprintf(gw, "# HELP %s Total number of bytes allocated, even if freed.\n", name)
			_, _ = fmt.Fprintf(gw, "# TYPE %s counter\n", name)
			_, _ = fmt.Fprintf(gw, "%s %d\n", name, i*j)
		}
		require.NoError(b, gw.Close())
		scenarios[i].body = buf.Bytes()
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
		w.Header().Set("Content-Encoding", "gzip")
		for _, scenario := range scenarios {
			if strconv.Itoa(scenario.metricsCount) == r.URL.Query()["count"][0] {
				_, _ = w.Write(scenario.body)
				return
			}
		}
		w.WriteHeader(http.StatusBadRequest)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	client, err := config_util.NewClientFromConfig(config_util.DefaultHTTPClientConfig, "test_job")
	if err != nil {
		panic(err)
	}

	for _, scenario := range scenarios {
		b.Run(fmt.Sprintf("metrics=%d", scenario.metricsCount), func(b *testing.B) {
			ts := &targetScraper{
				Target: &Target{
					labels: labels.FromStrings(
						model.SchemeLabel, serverURL.Scheme,
						model.AddressLabel, serverURL.Host,
					),
					scrapeConfig: &config.ScrapeConfig{
						MetricNameValidationScheme: model.UTF8Validation,
						MetricNameEscapingScheme:   model.AllowUTF8,
						Params:                     url.Values{"count": []string{strconv.Itoa(scenario.metricsCount)}},
					},
				},
				client:  client,
				timeout: time.Second,
			}
			b.ResetTimer()
			for b.Loop() {
				_, err = ts.scrape(context.Background())
				require.NoError(b, err)
			}
		})
	}
}

// When a scrape contains multiple instances for the same time series we should increment
// prometheus_target_scrapes_sample_duplicate_timestamp_total metric.
func TestScrapeLoopSeriesAddedDuplicates(t *testing.T) {
	sl, _ := newTestScrapeLoop(t)

	app := sl.appender()
	total, added, seriesAdded, err := app.append([]byte("test_metric 1\ntest_metric 2\ntest_metric 3\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 1, seriesAdded)
	require.Equal(t, 2.0, prom_testutil.ToFloat64(sl.metrics.targetScrapeSampleDuplicate))

	app = sl.appender()
	total, added, seriesAdded, err = app.append([]byte("test_metric 1\ntest_metric 1\ntest_metric 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 0, seriesAdded)
	require.Equal(t, 4.0, prom_testutil.ToFloat64(sl.metrics.targetScrapeSampleDuplicate))

	// When different timestamps are supplied, multiple samples are accepted.
	app = sl.appender()
	total, added, seriesAdded, err = app.append([]byte("test_metric 1 1001\ntest_metric 1 1002\ntest_metric 1 1003\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 0, seriesAdded)
	// Metric is not higher than last time.
	require.Equal(t, 4.0, prom_testutil.ToFloat64(sl.metrics.targetScrapeSampleDuplicate))
}

// This tests running a full scrape loop and checking that the scrape option
// `native_histogram_min_bucket_factor` is used correctly.
func TestNativeHistogramMaxSchemaSet(t *testing.T) {
	testcases := map[string]struct {
		minBucketFactor string
		expectedSchema  int32
	}{
		"min factor not specified": {
			minBucketFactor: "",
			expectedSchema:  3, // Factor 1.09.
		},
		"min factor 1": {
			minBucketFactor: "native_histogram_min_bucket_factor: 1",
			expectedSchema:  3, // Factor 1.09.
		},
		"min factor 2": {
			minBucketFactor: "native_histogram_min_bucket_factor: 2",
			expectedSchema:  0, // Factor 2.00.
		},
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testNativeHistogramMaxSchemaSet(t, tc.minBucketFactor, tc.expectedSchema)
		})
	}
}

func testNativeHistogramMaxSchemaSet(t *testing.T, minBucketFactor string, expectedSchema int32) {
	// Create a ProtoBuf message to serve as a Prometheus metric.
	nativeHistogram := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace:                      "testing",
			Name:                           "example_native_histogram",
			Help:                           "This is used for testing",
			NativeHistogramBucketFactor:    1.1,
			NativeHistogramMaxBucketNumber: 100,
		},
	)
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(nativeHistogram))
	nativeHistogram.Observe(1.0)
	nativeHistogram.Observe(1.0)
	nativeHistogram.Observe(1.0)
	nativeHistogram.Observe(10.0) // in different bucket since > 1*1.1.
	nativeHistogram.Observe(10.0)

	gathered, err := registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, gathered)

	histogramMetricFamily := gathered[0]
	buffer := protoMarshalDelimited(t, histogramMetricFamily)

	// Create an HTTP server to serve /metrics via ProtoBuf
	metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", `application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited`)
		_, _ = w.Write(buffer)
	}))
	defer metricsServer.Close()

	// Create a scrape loop with the HTTP server as the target.
	configStr := fmt.Sprintf(`
global:
  metric_name_validation_scheme: legacy
  scrape_interval: 50ms
  scrape_timeout: 25ms
scrape_configs:
  - job_name: test
    scrape_native_histograms: true
    %s
    static_configs:
      - targets: [%s]
`, minBucketFactor, strings.ReplaceAll(metricsServer.URL, "http://", ""))

	s := teststorage.New(t)
	t.Cleanup(func() { _ = s.Close() })
	reg := prometheus.NewRegistry()

	mng, err := NewManager(&Options{DiscoveryReloadInterval: model.Duration(10 * time.Millisecond)}, nil, nil, s, reg)
	require.NoError(t, err)
	cfg, err := config.Load(configStr, promslog.NewNopLogger())
	require.NoError(t, err)
	require.NoError(t, mng.ApplyConfig(cfg))
	tsets := make(chan map[string][]*targetgroup.Group)
	go func() {
		require.NoError(t, mng.Run(tsets))
	}()
	defer mng.Stop()

	// Get the static targets and apply them to the scrape manager.
	require.Len(t, cfg.ScrapeConfigs, 1)
	scrapeCfg := cfg.ScrapeConfigs[0]
	require.Len(t, scrapeCfg.ServiceDiscoveryConfigs, 1)
	staticDiscovery, ok := scrapeCfg.ServiceDiscoveryConfigs[0].(discovery.StaticConfig)
	require.True(t, ok)
	require.Len(t, staticDiscovery, 1)
	tsets <- map[string][]*targetgroup.Group{"test": staticDiscovery}

	// Wait for the scrape loop to scrape the target.
	require.Eventually(t, func() bool {
		q, err := s.Querier(0, math.MaxInt64)
		require.NoError(t, err)
		seriesS := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "testing_example_native_histogram"))
		countSeries := 0
		for seriesS.Next() {
			countSeries++
		}
		return countSeries > 0
	}, 5*time.Second, 100*time.Millisecond)

	// Check that native histogram schema is as expected.
	q, err := s.Querier(0, math.MaxInt64)
	require.NoError(t, err)
	seriesS := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "testing_example_native_histogram"))
	var histogramSamples []*histogram.Histogram
	for seriesS.Next() {
		series := seriesS.At()
		it := series.Iterator(nil)
		for vt := it.Next(); vt != chunkenc.ValNone; vt = it.Next() {
			if vt != chunkenc.ValHistogram {
				// don't care about other samples
				continue
			}
			_, h := it.AtHistogram(nil)
			histogramSamples = append(histogramSamples, h)
		}
	}
	require.NoError(t, seriesS.Err())
	require.NotEmpty(t, histogramSamples)
	for _, h := range histogramSamples {
		require.Equal(t, expectedSchema, h.Schema)
	}
}

func TestTargetScrapeConfigWithLabels(t *testing.T) {
	t.Parallel()
	const (
		configTimeout        = 1500 * time.Millisecond
		expectedTimeout      = "1.5"
		expectedTimeoutLabel = "1s500ms"
		secondTimeout        = 500 * time.Millisecond
		secondTimeoutLabel   = "500ms"
		expectedParam        = "value1"
		secondParam          = "value2"
		expectedPath         = "/metric-ok"
		secondPath           = "/metric-nok"
		httpScheme           = "http"
		paramLabel           = "__param_param"
		jobName              = "test"
	)

	createTestServer := func(t *testing.T, done chan struct{}) *url.URL {
		server := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(done)
				require.Equal(t, expectedTimeout, r.Header.Get("X-Prometheus-Scrape-Timeout-Seconds"))
				require.Equal(t, expectedParam, r.URL.Query().Get("param"))
				require.Equal(t, expectedPath, r.URL.Path)

				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
				_, _ = w.Write([]byte("metric_a 1\nmetric_b 2\n"))
			}),
		)
		t.Cleanup(server.Close)
		serverURL, err := url.Parse(server.URL)
		require.NoError(t, err)
		return serverURL
	}

	run := func(t *testing.T, cfg *config.ScrapeConfig, targets []*targetgroup.Group) chan struct{} {
		done := make(chan struct{})
		srvURL := createTestServer(t, done)

		// Update target addresses to use the dynamically created server URL.
		for _, target := range targets {
			for i := range target.Targets {
				target.Targets[i][model.AddressLabel] = model.LabelValue(srvURL.Host)
			}
		}

		sp, err := newScrapePool(cfg, teststorage.NewAppendable(), 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
		require.NoError(t, err)
		t.Cleanup(sp.stop)

		sp.Sync(targets)
		return done
	}

	cases := []struct {
		name    string
		cfg     *config.ScrapeConfig
		targets []*targetgroup.Group
	}{
		{
			name: "Everything in scrape config",
			cfg: &config.ScrapeConfig{
				ScrapeInterval:             model.Duration(2 * time.Second),
				ScrapeTimeout:              model.Duration(configTimeout),
				Params:                     url.Values{"param": []string{expectedParam}},
				JobName:                    jobName,
				Scheme:                     httpScheme,
				MetricsPath:                expectedPath,
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
			},
			targets: []*targetgroup.Group{
				{
					Targets: []model.LabelSet{
						{model.AddressLabel: model.LabelValue("")},
					},
				},
			},
		},
		{
			name: "Overridden in target",
			cfg: &config.ScrapeConfig{
				ScrapeInterval:             model.Duration(2 * time.Second),
				ScrapeTimeout:              model.Duration(secondTimeout),
				JobName:                    jobName,
				Scheme:                     httpScheme,
				MetricsPath:                secondPath,
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
				Params:                     url.Values{"param": []string{secondParam}},
			},
			targets: []*targetgroup.Group{
				{
					Targets: []model.LabelSet{
						{
							model.AddressLabel:       model.LabelValue(""),
							model.ScrapeTimeoutLabel: expectedTimeoutLabel,
							model.MetricsPathLabel:   expectedPath,
							paramLabel:               expectedParam,
						},
					},
				},
			},
		},
		{
			name: "Overridden in relabel_config",
			cfg: &config.ScrapeConfig{
				ScrapeInterval:             model.Duration(2 * time.Second),
				ScrapeTimeout:              model.Duration(secondTimeout),
				JobName:                    jobName,
				Scheme:                     httpScheme,
				MetricsPath:                secondPath,
				MetricNameValidationScheme: model.UTF8Validation,
				MetricNameEscapingScheme:   model.AllowUTF8,
				Params:                     url.Values{"param": []string{secondParam}},
				RelabelConfigs: []*relabel.Config{
					{
						Action:               relabel.DefaultRelabelConfig.Action,
						Regex:                relabel.DefaultRelabelConfig.Regex,
						SourceLabels:         relabel.DefaultRelabelConfig.SourceLabels,
						TargetLabel:          model.ScrapeTimeoutLabel,
						Replacement:          expectedTimeoutLabel,
						NameValidationScheme: model.UTF8Validation,
					},
					{
						Action:               relabel.DefaultRelabelConfig.Action,
						Regex:                relabel.DefaultRelabelConfig.Regex,
						SourceLabels:         relabel.DefaultRelabelConfig.SourceLabels,
						TargetLabel:          paramLabel,
						Replacement:          expectedParam,
						NameValidationScheme: model.UTF8Validation,
					},
					{
						Action:               relabel.DefaultRelabelConfig.Action,
						Regex:                relabel.DefaultRelabelConfig.Regex,
						SourceLabels:         relabel.DefaultRelabelConfig.SourceLabels,
						TargetLabel:          model.MetricsPathLabel,
						Replacement:          expectedPath,
						NameValidationScheme: model.UTF8Validation,
					},
				},
			},
			targets: []*targetgroup.Group{
				{
					Targets: []model.LabelSet{
						{
							model.AddressLabel:       model.LabelValue(""),
							model.ScrapeTimeoutLabel: secondTimeoutLabel,
							model.MetricsPathLabel:   secondPath,
							paramLabel:               secondParam,
						},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			select {
			case <-run(t, c.cfg, c.targets):
			case <-time.After(10 * time.Second):
				t.Fatal("timeout after 10 seconds")
			}
		})
	}
}

func newScrapableServer(scrapeText string) (s *httptest.Server, scrapedTwice chan bool) {
	var scrapes int
	scrapedTwice = make(chan bool)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, scrapeText)
		scrapes++
		if scrapes == 2 {
			close(scrapedTwice)
		}
	})), scrapedTwice
}

// Regression test for the panic fixed in https://github.com/prometheus/prometheus/pull/15523.
func TestScrapePoolScrapeAfterReload(t *testing.T) {
	h := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte{0x42, 0x42})
		},
	))
	t.Cleanup(h.Close)

	cfg := &config.ScrapeConfig{
		BodySizeLimit:              1,
		JobName:                    "test",
		Scheme:                     "http",
		ScrapeInterval:             model.Duration(100 * time.Millisecond),
		ScrapeTimeout:              model.Duration(100 * time.Millisecond),
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
		EnableCompression:          false,
		ServiceDiscoveryConfigs: discovery.Configs{
			&discovery.StaticConfig{
				{
					Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(h.URL)}},
				},
			},
		},
	}

	p, err := newScrapePool(cfg, teststorage.NewAppendable(), 0, nil, nil, &Options{}, newTestScrapeMetrics(t))
	require.NoError(t, err)
	t.Cleanup(p.stop)

	p.Sync([]*targetgroup.Group{
		{
			Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(strings.TrimPrefix(h.URL, "http://"))}},
			Source:  "test",
		},
	})

	require.NoError(t, p.reload(cfg))

	<-time.After(1 * time.Second)
}

// Regression test against https://github.com/prometheus/prometheus/issues/16160.
// The first scrape fails with a parsing error, but the second should
// succeed and cause `metric_1=11` to appear in the appender.
func TestScrapeAppendWithParseError(t *testing.T) {
	const (
		scrape1 = `metric_a 1
`
		scrape2 = `metric_a 11
# EOF`
	)

	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, withAppendable(appTest))
	now := time.Now()

	app := sl.appender()
	_, _, _, err := app.append([]byte(scrape1), "application/openmetrics-text", now)
	require.Error(t, err)
	require.NoError(t, app.Rollback())

	app = sl.appender()
	_, _, _, err = app.append(nil, "application/openmetrics-text", now)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Empty(t, appTest.ResultSamples())

	app = sl.appender()
	_, _, _, err = app.append([]byte(scrape2), "application/openmetrics-text", now.Add(15*time.Second))
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	want := []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now.Add(15 * time.Second)),
			V: 11,
		},
	}
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", appTest)
}

// This test covers a case where there's a target with sample_limit set and some samples
// changes between scrapes.
func TestScrapeLoopAppendSampleLimitWithDisappearingSeries(t *testing.T) {
	const sampleLimit = 4

	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appTest
		sl.sampleLimit = sampleLimit
	})

	now := time.Now()
	app := sl.appender()
	samplesScraped, samplesAfterRelabel, createdSeries, err := app.append(
		// Start with 3 samples, all accepted.
		[]byte("metric_a 1\nmetric_b 1\nmetric_c 1\n"),
		"text/plain",
		now,
	)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 3, samplesScraped)      // All on scrape.
	require.Equal(t, 3, samplesAfterRelabel) // This is series after relabeling.
	require.Equal(t, 3, createdSeries)       // Newly added to TSDB.
	want := []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_b"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_c"),
			T: timestamp.FromTime(now),
			V: 1,
		},
	}
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", app)

	now = now.Add(time.Minute)
	app = sl.appender()
	samplesScraped, samplesAfterRelabel, createdSeries, err = app.append(
		// Start exporting 3 more samples, so we're over the limit now.
		[]byte("metric_a 1\nmetric_b 1\nmetric_c 1\nmetric_d 1\nmetric_e 1\nmetric_f 1\n"),
		"text/plain",
		now,
	)
	require.ErrorIs(t, err, errSampleLimit)
	require.NoError(t, app.Rollback())
	require.Equal(t, 6, samplesScraped)
	require.Equal(t, 6, samplesAfterRelabel)
	require.Equal(t, 1, createdSeries) // We've added one series before hitting the limit.
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", app)
	sl.cache.iterDone(false)

	now = now.Add(time.Minute)
	app = sl.appender()
	samplesScraped, samplesAfterRelabel, createdSeries, err = app.append(
		// Remove all samples except first 2.
		[]byte("metric_a 1\nmetric_b 1\n"),
		"text/plain",
		now,
	)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 2, samplesScraped)
	require.Equal(t, 2, samplesAfterRelabel)
	require.Equal(t, 0, createdSeries)
	// This is where important things happen. We should now see:
	// - Appends for samples from metric_a & metric_b.
	// - Append with stale markers for metric_c - this series was added during first scrape but disappeared during last scrape.
	// - Append with stale marker for metric_d - this series was added during second scrape before we hit the sample_limit.
	// We should NOT see:
	// - Appends with stale markers for metric_e & metric_f - both over the limit during second scrape, and so they never made it into TSDB.
	want = append(want, []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_b"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_c"),
			T: timestamp.FromTime(now),
			V: math.Float64frombits(value.StaleNaN),
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_d"),
			T: timestamp.FromTime(now),
			V: math.Float64frombits(value.StaleNaN),
		},
	}...)
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", app)
}

// This test covers a case where there's a target with sample_limit set and each scrape sees a completely
// different set of samples.
func TestScrapeLoopAppendSampleLimitReplaceAllSamples(t *testing.T) {
	const sampleLimit = 4

	appTest := teststorage.NewAppendable()
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.appendable = appTest
		sl.sampleLimit = sampleLimit
	})

	now := time.Now()
	app := sl.appender()
	samplesScraped, samplesAfterRelabel, createdSeries, err := app.append(
		// Start with 4 samples, all accepted.
		[]byte("metric_a 1\nmetric_b 1\nmetric_c 1\nmetric_d 1\n"),
		"text/plain",
		now,
	)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 4, samplesScraped)      // All on scrape.
	require.Equal(t, 4, samplesAfterRelabel) // This is series after relabeling.
	require.Equal(t, 4, createdSeries)       // Newly added to TSDB.
	want := []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_b"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_c"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_d"),
			T: timestamp.FromTime(now),
			V: 1,
		},
	}
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", app)

	now = now.Add(time.Minute)
	app = sl.appender()
	samplesScraped, samplesAfterRelabel, createdSeries, err = app.append(
		// Replace all samples with new time series.
		[]byte("metric_e 1\nmetric_f 1\nmetric_g 1\nmetric_h 1\n"),
		"text/plain",
		now,
	)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 4, samplesScraped)
	require.Equal(t, 4, samplesAfterRelabel)
	require.Equal(t, 4, createdSeries)
	// We replaced all samples from first scrape with new set of samples.
	// We expected to see:
	// - 4 appends for new samples.
	// - 4 appends with staleness markers for old samples.
	want = append(want, []sample{
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_e"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_f"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_g"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_h"),
			T: timestamp.FromTime(now),
			V: 1,
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			T: timestamp.FromTime(now),
			V: math.Float64frombits(value.StaleNaN),
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_b"),
			T: timestamp.FromTime(now),
			V: math.Float64frombits(value.StaleNaN),
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_c"),
			T: timestamp.FromTime(now),
			V: math.Float64frombits(value.StaleNaN),
		},
		{
			L: labels.FromStrings(model.MetricNameLabel, "metric_d"),
			T: timestamp.FromTime(now),
			V: math.Float64frombits(value.StaleNaN),
		},
	}...)
	requireEqual(t, want, appTest.ResultSamples(), "Appended samples not as expected:\n%s", app)
}

func TestScrapeLoopDisableStalenessMarkerInjection(t *testing.T) {
	loopDone := atomic.NewBool(false)

	appTest := teststorage.NewAppendable()
	sl, scraper := newTestScrapeLoop(t, withAppendable(appTest))
	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		if _, err := w.Write([]byte("metric_a 42\n")); err != nil {
			return err
		}
		return ctx.Err()
	}

	// Start the scrape loop.
	go func() {
		sl.run(nil)
		loopDone.Store(true)
	}()

	// Wait for some samples to be appended.
	require.Eventually(t, func() bool {
		return len(appTest.ResultSamples()) > 2
	}, 5*time.Second, 100*time.Millisecond, "Scrape loop didn't append any samples.")

	// Disable end of run staleness markers and stop the loop.
	sl.disableEndOfRunStalenessMarkers()
	sl.stop()
	require.Eventually(t, func() bool {
		return loopDone.Load()
	}, 5*time.Second, 100*time.Millisecond, "Scrape loop didn't stop.")

	// No stale markers should be appended, since they were disabled.
	for _, s := range appTest.ResultSamples() {
		if value.IsStaleNaN(s.V) {
			t.Fatalf("Got stale NaN samples while end of run staleness is disabled: %x", math.Float64bits(s.V))
		}
	}
}

// Recommended CLI invocation:
/*
	export bench=restartLoops && go test ./scrape/... \
		-run '^$' -bench '^BenchmarkScrapePoolRestartLoops' \
		-benchtime 5s -count 6 -cpu 2 -timeout 999m \
		| tee ${bench}.txt
*/
func BenchmarkScrapePoolRestartLoops(b *testing.B) {
	sp, err := newScrapePool(
		&config.ScrapeConfig{
			MetricNameValidationScheme: model.UTF8Validation,
			ScrapeInterval:             model.Duration(1 * time.Hour),
			ScrapeTimeout:              model.Duration(1 * time.Hour),
		},
		nil,
		0,
		nil,
		nil,
		&Options{},
		newTestScrapeMetrics(b),
	)
	require.NoError(b, err)
	b.Cleanup(sp.stop)

	for i := range 1000 {
		sp.activeTargets[uint64(i)] = &Target{scrapeConfig: &config.ScrapeConfig{}}
		sp.loops[uint64(i)] = noopLoop() // First restart will supplement those with proper scrapeLoops.
	}
	sp.restartLoops(true)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		sp.restartLoops(true)
	}
}

// TestNewScrapeLoopHonorLabelsWiring verifies that newScrapeLoop correctly wires
// HonorLabels (not HonorTimestamps) to the sampleMutator.
func TestNewScrapeLoopHonorLabelsWiring(t *testing.T) {
	// Scraped metric has label "lbl" with value "scraped".
	// Discovery target has label "lbl" with value "discovery".
	// With honor_labels=true, the scraped value should win.
	// With honor_labels=false, the discovery value should win and scraped moves to exported_lbl.
	testCases := []struct {
		name           string
		honorLabels    bool
		expectedLbl    string
		expectedExpLbl string // exported_lbl value, empty if not expected
	}{
		{
			name:        "honor_labels=true",
			honorLabels: true,
			expectedLbl: "scraped",
		},
		{
			name:           "honor_labels=false",
			honorLabels:    false,
			expectedLbl:    "discovery",
			expectedExpLbl: "scraped",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ts, scrapedTwice := newScrapableServer(`metric{lbl="scraped"} 1`)
			defer ts.Close()

			testURL, err := url.Parse(ts.URL)
			require.NoError(t, err)

			s := teststorage.New(t)
			defer s.Close()

			cfg := &config.ScrapeConfig{
				JobName:                    "test",
				Scheme:                     "http",
				HonorLabels:                tc.honorLabels,
				HonorTimestamps:            !tc.honorLabels, // Opposite of HonorLabels to catch wiring bugs
				ScrapeInterval:             model.Duration(1 * time.Second),
				ScrapeTimeout:              model.Duration(100 * time.Millisecond),
				MetricNameValidationScheme: model.UTF8Validation,
			}

			sp, err := newScrapePool(cfg, s, 0, nil, nil, &Options{skipOffsetting: true}, newTestScrapeMetrics(t))
			require.NoError(t, err)
			defer sp.stop()

			// Sync with a target that has a conflicting label.
			sp.Sync([]*targetgroup.Group{{
				Targets: []model.LabelSet{{
					model.AddressLabel: model.LabelValue(testURL.Host),
					"lbl":              "discovery",
				}},
			}})
			require.Len(t, sp.ActiveTargets(), 1)

			// Wait for scrape to complete.
			select {
			case <-time.After(5 * time.Second):
				t.Fatal("scrape did not complete in time")
			case <-scrapedTwice:
			}

			// Query the storage to verify label values.
			q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
			require.NoError(t, err)
			defer q.Close()

			series := q.Select(t.Context(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric"))
			require.True(t, series.Next(), "metric series not found")
			require.Equal(t, tc.expectedLbl, series.At().Labels().Get("lbl"))
			require.Equal(t, tc.expectedExpLbl, series.At().Labels().Get("exported_lbl"))
		})
	}
}

func TestDropsSeriesFromMetricRelabeling(t *testing.T) {
	target := &Target{}
	relabelConfig := []*relabel.Config{
		{
			SourceLabels:         model.LabelNames{"__name__"},
			Regex:                relabel.MustNewRegexp("test_metric.*$"),
			Action:               relabel.Keep,
			NameValidationScheme: model.UTF8Validation,
		},
		{
			SourceLabels:         model.LabelNames{"__name__"},
			Regex:                relabel.MustNewRegexp("test_metric_2$"),
			Action:               relabel.Drop,
			NameValidationScheme: model.UTF8Validation,
		},
	}
	sl, _ := newTestScrapeLoop(t, func(sl *scrapeLoop) {
		sl.sampleMutator = func(l labels.Labels) labels.Labels {
			return mutateSampleLabels(l, target, true, relabelConfig)
		}
	})

	app := sl.appender()
	total, added, seriesAdded, err := app.append([]byte("test_metric_1 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 1, seriesAdded)

	total, added, seriesAdded, err = app.append([]byte("test_metric_2 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 0, added)
	require.Equal(t, 0, seriesAdded)

	total, added, seriesAdded, err = app.append([]byte("unwanted_metric 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 0, added)
	require.Equal(t, 0, seriesAdded)

	require.NoError(t, app.Commit())
}
