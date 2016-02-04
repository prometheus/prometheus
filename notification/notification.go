// Copyright 2013 The Prometheus Authors
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

package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
)

const (
	alertPushEndpoint = "/api/v1/alerts"
	contentTypeJSON   = "application/json"
)

// String constants for instrumentation.
const (
	namespace = "prometheus"
	subsystem = "notifications"
)

// Handler is responsible for dispatching alert notifications to an
// alert manager service.
type Handler struct {
	queue model.Alerts
	opts  *HandlerOptions

	more   chan struct{}
	mtx    sync.RWMutex
	ctx    context.Context
	cancel func()

	latency       prometheus.Summary
	errors        prometheus.Counter
	dropped       prometheus.Counter
	sent          prometheus.Counter
	queueLength   prometheus.Gauge
	queueCapacity prometheus.Metric
}

// HandlerOptions are the configurable parameters of a Handler.
type HandlerOptions struct {
	AlertmanagerURL string
	QueueCapacity   int
	Timeout         time.Duration
	ExternalLabels  model.LabelSet
}

// NewHandler constructs a new Handler.
func New(o *HandlerOptions) *Handler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Handler{
		queue:  make(model.Alerts, 0, o.QueueCapacity),
		ctx:    ctx,
		cancel: cancel,
		more:   make(chan struct{}, 1),
		opts:   o,

		latency: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "latency_seconds",
			Help:      "Latency quantiles for sending alert notifications (not including dropped notifications).",
		}),
		errors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "Total number of errors sending alert notifications.",
		}),
		sent: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sent_total",
			Help:      "Total number of alerts successfully sent.",
		}),
		dropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "dropped_total",
			Help:      "Total number of alerts dropped due to alert manager missing in configuration.",
		}),
		queueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_length",
			Help:      "The number of alert notifications in the queue.",
		}),
		queueCapacity: prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "queue_capacity"),
				"The capacity of the alert notifications queue.",
				nil, nil,
			),
			prometheus.GaugeValue,
			float64(o.QueueCapacity),
		),
	}
}

// ApplyConfig updates the status state as the new config requires.
// Returns true on success.
func (n *Handler) ApplyConfig(conf *config.Config) bool {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.opts.ExternalLabels = conf.GlobalConfig.ExternalLabels
	return true
}

const maxBatchSize = 64

func (n *Handler) queueLen() int {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	return len(n.queue)
}

func (n *Handler) nextBatch() []*model.Alert {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	var alerts model.Alerts

	if len(n.queue) > maxBatchSize {
		alerts = append(make(model.Alerts, 0, maxBatchSize), n.queue[:maxBatchSize]...)
		n.queue = n.queue[maxBatchSize:]
	} else {
		alerts = append(make(model.Alerts, 0, len(n.queue)), n.queue...)
		n.queue = n.queue[:0]
	}

	return alerts
}

// Run dispatches notifications continuously.
func (n *Handler) Run() {
	// Just warn once in the beginning to prevent noisy logs.
	if n.opts.AlertmanagerURL == "" {
		log.Warnf("No AlertManager configured, not dispatching any alerts")
	}

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-n.more:
		}

		alerts := n.nextBatch()

		if len(alerts) == 0 {
			continue
		}
		if n.opts.AlertmanagerURL == "" {
			n.dropped.Add(float64(len(alerts)))
			continue
		}

		begin := time.Now()

		if err := n.send(alerts...); err != nil {
			log.Errorf("Error sending %d alerts: %s", len(alerts), err)
			n.errors.Inc()
			n.dropped.Add(float64(len(alerts)))
		}

		n.latency.Observe(float64(time.Since(begin)) / float64(time.Second))
		n.sent.Add(float64(len(alerts)))

		// If the queue still has items left, kick off the next iteration.
		if n.queueLen() > 0 {
			n.setMore()
		}
	}
}

// SubmitReqs queues the given notification requests for processing.
// Panics if called on a handler that is not running.
func (n *Handler) Send(alerts ...*model.Alert) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	// Queue capacity should be significantly larger than a single alert
	// batch could be.
	if d := len(alerts) - n.opts.QueueCapacity; d > 0 {
		alerts = alerts[d:]

		log.Warnf("Alert batch larger than queue capacity, dropping %d alerts", d)
		n.dropped.Add(float64(d))
	}

	// If the queue is full, remove the oldest alerts in favor
	// of newer ones.
	if d := (len(n.queue) + len(alerts)) - n.opts.QueueCapacity; d > 0 {
		n.queue = n.queue[d:]

		log.Warnf("Alert notification queue full, dropping %d alerts", d)
		n.dropped.Add(float64(d))
	}
	n.queue = append(n.queue, alerts...)

	// Notify sending goroutine that there are alerts to be processed.
	n.setMore()
}

// setMore signals that the alert queue has items.
func (n *Handler) setMore() {
	// If we cannot send on the channel, it means the signal already exists
	// and has not been consumed yet.
	select {
	case n.more <- struct{}{}:
	default:
	}
}

func (n *Handler) postURL() string {
	return strings.TrimRight(n.opts.AlertmanagerURL, "/") + alertPushEndpoint
}

func (n *Handler) send(alerts ...*model.Alert) error {
	// Attach external labels before sending alerts.
	for _, a := range alerts {
		for ln, lv := range n.opts.ExternalLabels {
			if _, ok := a.Labels[ln]; !ok {
				a.Labels[ln] = lv
			}
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(alerts); err != nil {
		return err
	}
	ctx, _ := context.WithTimeout(context.Background(), n.opts.Timeout)

	resp, err := ctxhttp.Post(ctx, http.DefaultClient, n.postURL(), contentTypeJSON, &buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("bad response status %v", resp.Status)
	}
	return nil
}

// Stop shuts down the notification handler.
func (n *Handler) Stop() {
	log.Info("Stopping notification handler...")

	n.cancel()
}

// Describe implements prometheus.Collector.
func (n *Handler) Describe(ch chan<- *prometheus.Desc) {
	ch <- n.latency.Desc()
	ch <- n.errors.Desc()
	ch <- n.sent.Desc()
	ch <- n.dropped.Desc()
	ch <- n.queueLength.Desc()
	ch <- n.queueCapacity.Desc()
}

// Collect implements prometheus.Collector.
func (n *Handler) Collect(ch chan<- prometheus.Metric) {
	n.queueLength.Set(float64(n.queueLen()))

	ch <- n.latency
	ch <- n.errors
	ch <- n.sent
	ch <- n.dropped
	ch <- n.queueLength
	ch <- n.queueCapacity
}
