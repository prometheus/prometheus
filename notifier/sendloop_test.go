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
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
)

func TestCustomDo(t *testing.T) {
	const testURL = "http://testurl.com/"
	const testBody = "testbody"

	var received bool
	h := sendLoop{
		opts: &Options{
			Do: func(_ context.Context, _ *http.Client, req *http.Request) (*http.Response, error) {
				received = true
				body, err := io.ReadAll(req.Body)

				require.NoError(t, err)

				require.Equal(t, testBody, string(body))

				require.Equal(t, testURL, req.URL.String())

				return &http.Response{
					Body: io.NopCloser(bytes.NewBuffer(nil)),
				}, nil
			},
		},
	}

	h.sendOne(context.Background(), nil, testURL, []byte(testBody))

	require.True(t, received)
}

func TestHandlerNextBatch(t *testing.T) {
	sendLoop := newSendLoop("http://mock", nil, &config.DefaultAlertmanagerConfig, &Options{MaxBatchSize: DefaultMaxBatchSize}, slog.New(slog.DiscardHandler), newAlertMetrics(prometheus.NewRegistry(), nil))

	for i := range make([]struct{}, 2*DefaultMaxBatchSize+1) {
		sendLoop.queue = append(sendLoop.queue, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
	}
	expected := append([]*Alert{}, sendLoop.queue...)

	require.NoError(t, alertsEqual(expected[0:DefaultMaxBatchSize], sendLoop.nextBatch()))
	require.NoError(t, alertsEqual(expected[DefaultMaxBatchSize:2*DefaultMaxBatchSize], sendLoop.nextBatch()))
	require.NoError(t, alertsEqual(expected[2*DefaultMaxBatchSize:], sendLoop.nextBatch()))
	require.Empty(t, sendLoop.queue)
}

func TestAddAlertsToQueue(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "existing1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "existing2")}

	s := newSendLoop("http://foo.bar/", nil, nil, &Options{QueueCapacity: 5}, slog.New(slog.DiscardHandler), newAlertMetrics(prometheus.NewRegistry(), nil))
	s.add(alert1, alert2)
	require.Equal(t, []*Alert{alert1, alert2}, s.queue)
	require.Len(t, s.queue, 2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "new1")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "new2")}

	// Add new alerts to the queue, expect 0 dropped
	s.add(alert3, alert4)
	require.Zero(t, prom_testutil.ToFloat64(s.metrics.dropped.WithLabelValues(s.alertmanagerURL)))

	// Verify all new alerts were added to the queue
	require.Equal(t, []*Alert{alert1, alert2, alert3, alert4}, s.queue)
	require.Len(t, s.queue, 4)
}

func TestAddAlertsToQueueExceedingCapacity(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}

	s := newSendLoop("http://foo.bar/", nil, nil, &Options{QueueCapacity: 3}, slog.New(slog.DiscardHandler), newAlertMetrics(prometheus.NewRegistry(), nil))
	s.add(alert1, alert2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}

	// Add new alerts to queue, expect 1 dropped
	s.add(alert3, alert4)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(s.metrics.dropped.WithLabelValues(s.alertmanagerURL)))

	// Verify all new alerts were added to the queue
	require.Equal(t, []*Alert{alert2, alert3, alert4}, s.queue)
}

func TestAddAlertsToQueueExceedingTotalCapacity(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}

	s := newSendLoop("http://foo.bar/", nil, nil, &Options{QueueCapacity: 3}, slog.New(slog.DiscardHandler), newAlertMetrics(prometheus.NewRegistry(), nil))
	s.add(alert1, alert2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}
	alert5 := &Alert{Labels: labels.FromStrings("alertname", "alert5")}
	alert6 := &Alert{Labels: labels.FromStrings("alertname", "alert6")}

	// Add new alerts to queue, expect 3 dropped: 1 from new batch + 2 from existing queued items
	s.add(alert3, alert4, alert5, alert6)
	require.Equal(t, 3.0, prom_testutil.ToFloat64(s.metrics.dropped.WithLabelValues(s.alertmanagerURL)))

	// Verify all new alerts were added to the queue
	require.Equal(t, []*Alert{alert4, alert5, alert6}, s.queue)
}

func TestNextBatchAlertsFromQueue(t *testing.T) {
	s := newSendLoop("http://foo.bar/", nil, nil, &Options{QueueCapacity: 5, MaxBatchSize: 3}, slog.New(slog.DiscardHandler), newAlertMetrics(prometheus.NewRegistry(), nil))

	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}
	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	s.add(alert1, alert2, alert3)

	// Test batch-size alerts in the queue
	require.Equal(t, []*Alert{alert1, alert2, alert3}, s.nextBatch())
	require.Empty(t, s.nextBatch())

	// Test full queue
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}
	alert5 := &Alert{Labels: labels.FromStrings("alertname", "alert5")}
	s.add(alert1, alert2, alert3, alert4, alert5)
	require.Equal(t, []*Alert{alert1, alert2, alert3}, s.nextBatch())
	require.Equal(t, []*Alert{alert4, alert5}, s.nextBatch())
	require.Empty(t, s.nextBatch())
}

func TestMetrics(t *testing.T) {
	const alertmanagerURL = "http://alertmanager:9093"

	// Use a single registry throughout the test - this is critical to catch registry conflicts
	reg := prometheus.NewRegistry()
	alertmanagersDiscoveredFunc := func() float64 { return 0 }
	metrics := newAlertMetrics(reg, alertmanagersDiscoveredFunc)

	logger := slog.New(slog.DiscardHandler)
	opts := &Options{QueueCapacity: 10, MaxBatchSize: DefaultMaxBatchSize}

	// Create first sendLoop - this initializes metrics with the alertmanager URL label
	sendLoop1 := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, logger, metrics)

	// Verify metrics are initialized
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.dropped.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))

	// Stop the sendLoop - this should clean up all metrics
	sendLoop1.stop()

	// Create second sendLoop with the same URL - this should NOT panic or conflict
	// because metrics were properly cleaned up
	sendLoop2 := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, logger, metrics)
	defer sendLoop2.stop()

	// Verify metrics are re-initialized correctly
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.dropped.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))
}

func TestSendAllInvalidAPIVersion(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, nil)
	cfg := &config.AlertmanagerConfig{
		APIVersion: "v3-invalid",
	}
	s := newSendLoop("http://mock", nil, cfg, &Options{QueueCapacity: 10}, slog.New(slog.DiscardHandler), metrics)
	defer s.stop()

	alerts := []*Alert{
		{Labels: labels.FromStrings("alertname", "test")},
	}
	require.False(t, s.sendAll(alerts))
}

func TestSendAllHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, nil)
	cfg := config.DefaultAlertmanagerConfig
	cfg.APIVersion = config.AlertmanagerAPIVersionV2

	s := newSendLoop(ts.URL, ts.Client(), &cfg, &Options{
		QueueCapacity: 10,
		Do:            do,
	}, slog.New(slog.DiscardHandler), metrics)
	defer s.stop()

	alerts := []*Alert{
		{Labels: labels.FromStrings("alertname", "test")},
	}
	require.False(t, s.sendAll(alerts))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(metrics.errors.WithLabelValues(ts.URL)))
}

func TestDrainQueue(t *testing.T) {
	var count int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, nil)
	cfg := config.DefaultAlertmanagerConfig
	cfg.APIVersion = config.AlertmanagerAPIVersionV2

	s := newSendLoop(ts.URL, ts.Client(), &cfg, &Options{
		QueueCapacity: 10,
		MaxBatchSize:  2,
		Do:            do,
	}, slog.New(slog.DiscardHandler), metrics)

	alerts := []*Alert{
		{Labels: labels.FromStrings("alertname", "a1")},
		{Labels: labels.FromStrings("alertname", "a2")},
		{Labels: labels.FromStrings("alertname", "a3")},
	}
	s.add(alerts...)

	require.Equal(t, 3, s.queueLen())

	s.drainQueue()

	require.Equal(t, 2, count)
	require.Equal(t, 0, s.queueLen())
	require.Equal(t, 3.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(ts.URL)))
}

type logRecord struct {
	Msg  string
	Attr map[string]any
}

type recordHandler struct {
	logs *[]logRecord
}

func (recordHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h recordHandler) Handle(_ context.Context, r slog.Record) error {
	attr := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attr[a.Key] = a.Value.Any()
		return true
	})
	*h.logs = append(*h.logs, logRecord{Msg: r.Message, Attr: attr})
	return nil
}
func (h recordHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h recordHandler) WithGroup(_ string) slog.Handler      { return h }

func TestStopWithoutDrain(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, nil)
	cfg := config.DefaultAlertmanagerConfig

	var logs []logRecord
	logger := slog.New(recordHandler{logs: &logs})

	s := newSendLoop("http://mock", nil, &cfg, &Options{
		QueueCapacity:   10,
		DrainOnShutdown: false,
		Do:              do,
	}, logger, metrics)

	alerts := []*Alert{
		{Labels: labels.FromStrings("alertname", "a1")},
		{Labels: labels.FromStrings("alertname", "a2")},
	}
	s.add(alerts...)

	s.stop()

	// Verify that the correct warning log was recorded
	var found bool
	for _, l := range logs {
		if l.Msg == "Alert notification queue not drained on shutdown, dropping alerts" {
			found = true
			require.Equal(t, int64(2), l.Attr["count"])
		}
	}
	require.True(t, found, "warning log not found")
}

func TestSendLoopShutdown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, nil)
	cfg := config.DefaultAlertmanagerConfig
	cfg.APIVersion = config.AlertmanagerAPIVersionV2

	s := newSendLoop(ts.URL, ts.Client(), &cfg, &Options{
		QueueCapacity:   10,
		DrainOnShutdown: true,
		MaxBatchSize:    10,
		Do:              do,
	}, slog.New(slog.DiscardHandler), metrics)

	done := make(chan struct{})
	go func() {
		s.loop()
		close(done)
	}()

	s.add(&Alert{Labels: labels.FromStrings("alertname", "a1")})

	time.Sleep(100 * time.Millisecond)

	s.stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not shut down on stop")
	}
}
