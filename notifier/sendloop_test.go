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
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
)

func TestCustomDo(t *testing.T) {
	const testURL = "http://testurl.com/"
	const testBody = "testbody"

	var received bool
	h := sendLoop{
		cfg: &config.DefaultAlertmanagerConfig,
		opts: &Options{
			Do: func(_ context.Context, _ *http.Client, req *http.Request) (*http.Response, error) {
				received = true
				body, err := io.ReadAll(req.Body)

				require.NoError(t, err)

				require.Equal(t, testBody, string(body))

				require.Equal(t, testURL, req.URL.String())

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(nil)),
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

func TestTracingInstrumentation(t *testing.T) {
	// Save original tracer provider and restore after test
	origTracerProvider := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTracerProvider)
	})

	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(spanRecorder),
	)
	otel.SetTracerProvider(tp)

	const (
		testURL        = "http://alertmanager:9093/api/v2/alerts"
		testAlertCount = 3
		testBody       = `[{"labels":{"alertname":"test"}}]`
	)

	tests := []struct {
		name           string
		statusCode     int
		shouldHaveErr  bool
		expectedStatus codes.Code
	}{
		{
			name:           "successful send",
			statusCode:     http.StatusOK,
			shouldHaveErr:  false,
			expectedStatus: codes.Unset,
		},
		{
			name:           "failed send - bad status",
			statusCode:     http.StatusInternalServerError,
			shouldHaveErr:  true,
			expectedStatus: codes.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spanRecorder.Reset()

			h := sendLoop{
				cfg: &config.DefaultAlertmanagerConfig,
				opts: &Options{
					Do: func(_ context.Context, _ *http.Client, _ *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: tt.statusCode,
							Body:       io.NopCloser(bytes.NewBuffer(nil)),
						}, nil
					},
				},
			}

			ctx, span := otel.Tracer("").Start(context.Background(), "Alert Send Batch", trace.WithSpanKind(trace.SpanKindClient))
			span.SetAttributes(
				attribute.String("alertmanager_url", testURL),
				attribute.Int("alert_count", testAlertCount),
				attribute.Int("batch_size_bytes", len(testBody)),
				attribute.String("api_version", string(config.AlertmanagerAPIVersionV2)),
			)

			err := h.sendOne(ctx, nil, testURL, []byte(testBody))
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()

			if tt.shouldHaveErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.NoError(t, tp.ForceFlush(context.Background()))

			spans := spanRecorder.Ended()
			require.Len(t, spans, 1, "expected exactly one span to be created")

			recordedSpan := spans[0]

			require.Equal(t, "Alert Send Batch", recordedSpan.Name())

			require.Equal(t, trace.SpanKindClient, recordedSpan.SpanKind())

			attrs := recordedSpan.Attributes()
			expectedAttrs := map[attribute.Key]attribute.Value{
				"alertmanager_url": attribute.StringValue(testURL),
				"alert_count":      attribute.Int64Value(int64(testAlertCount)),
				"batch_size_bytes": attribute.Int64Value(int64(len(testBody))),
				"api_version":      attribute.StringValue(string(config.AlertmanagerAPIVersionV2)),
				"http.status_code": attribute.Int64Value(int64(tt.statusCode)),
			}

			for key, expectedVal := range expectedAttrs {
				found := false
				for _, attr := range attrs {
					if attr.Key == key {
						require.Equal(t, expectedVal, attr.Value, "attribute %s has wrong value", key)
						found = true
						break
					}
				}
				require.True(t, found, "expected attribute %s not found", key)
			}

			require.Equal(t, tt.expectedStatus, recordedSpan.Status().Code)

			if tt.shouldHaveErr {
				events := recordedSpan.Events()
				require.NotEmpty(t, events, "expected error to be recorded as event")
				hasExceptionEvent := false
				for _, event := range events {
					if event.Name == "exception" {
						hasExceptionEvent = true
						break
					}
				}
				require.True(t, hasExceptionEvent, "expected exception event in span")
			}
		})
	}
}
