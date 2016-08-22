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

package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/relabel"
)

const (
	alertPushEndpoint = "/api/v1/alerts"
	contentTypeJSON   = "application/json"
)

// String constants for instrumentation.
const (
	namespace         = "prometheus"
	subsystem         = "notifications"
	alertmanagerLabel = "alertmanager"
)

// Notifier is responsible for dispatching alert notifications to an
// alert manager service.
type Notifier struct {
	queue model.Alerts
	opts  *Options

	more   chan struct{}
	mtx    sync.RWMutex
	ctx    context.Context
	cancel func()

	latency       *prometheus.SummaryVec
	errors        *prometheus.CounterVec
	sent          *prometheus.CounterVec
	dropped       prometheus.Counter
	queueLength   prometheus.Gauge
	queueCapacity prometheus.Metric
}

// Options are the configurable parameters of a Handler.
type Options struct {
	AlertmanagerURLs []string
	QueueCapacity    int
	Timeout          time.Duration
	ExternalLabels   model.LabelSet
	RelabelConfigs   []*config.RelabelConfig
}

// New constructs a new Notifier.
func New(o *Options) *Notifier {
	ctx, cancel := context.WithCancel(context.Background())

	return &Notifier{
		queue:  make(model.Alerts, 0, o.QueueCapacity),
		ctx:    ctx,
		cancel: cancel,
		more:   make(chan struct{}, 1),
		opts:   o,

		latency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "latency_seconds",
			Help:      "Latency quantiles for sending alert notifications (not including dropped notifications).",
		},
			[]string{alertmanagerLabel},
		),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "Total number of errors sending alert notifications.",
		},
			[]string{alertmanagerLabel},
		),
		sent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sent_total",
			Help:      "Total number of alerts successfully sent.",
		},
			[]string{alertmanagerLabel},
		),
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
func (n *Notifier) ApplyConfig(conf *config.Config) error {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.opts.ExternalLabels = conf.GlobalConfig.ExternalLabels
	n.opts.RelabelConfigs = conf.AlertingConfig.AlertRelabelConfigs
	return nil
}

const maxBatchSize = 64

func (n *Notifier) queueLen() int {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	return len(n.queue)
}

func (n *Notifier) nextBatch() []*model.Alert {
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
func (n *Notifier) Run() {
	numAMs := len(n.opts.AlertmanagerURLs)
	// Just warn once in the beginning to prevent noisy logs.
	if numAMs == 0 {
		log.Warnf("No AlertManagers configured, not dispatching any alerts")
		return
	}

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-n.more:
		}
		alerts := n.nextBatch()

		if numAMs > 0 {

			if len(alerts) > 0 {
				numErrors := n.sendAll(alerts...)
				// Increment the dropped counter if we could not send
				// successfully to a single AlertManager.
				if numErrors == numAMs {
					n.dropped.Add(float64(len(alerts)))
				}
			}
		} else {
			n.dropped.Add(float64(len(alerts)))
		}
		// If the queue still has items left, kick off the next iteration.
		if n.queueLen() > 0 {
			n.setMore()
		}
	}
}

// Send queues the given notification requests for processing.
// Panics if called on a handler that is not running.
func (n *Notifier) Send(alerts ...*model.Alert) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	alerts = n.relabelAlerts(alerts)

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

func (n *Notifier) relabelAlerts(alerts []*model.Alert) []*model.Alert {
	var relabeledAlerts []*model.Alert
	for _, alert := range alerts {
		labels := relabel.Process(alert.Labels, n.opts.RelabelConfigs...)
		if labels != nil {
			alert.Labels = labels
			relabeledAlerts = append(relabeledAlerts, alert)
		}
	}
	return relabeledAlerts
}

// setMore signals that the alert queue has items.
func (n *Notifier) setMore() {
	// If we cannot send on the channel, it means the signal already exists
	// and has not been consumed yet.
	select {
	case n.more <- struct{}{}:
	default:
	}
}

func postURL(u string) string {
	return strings.TrimRight(u, "/") + alertPushEndpoint
}

// sendAll sends the alerts to all configured Alertmanagers at concurrently.
// It returns the number of sends that have failed.
func (n *Notifier) sendAll(alerts ...*model.Alert) int {
	begin := time.Now()

	// Attach external labels before sending alerts.
	for _, a := range alerts {
		for ln, lv := range n.opts.ExternalLabels {
			if _, ok := a.Labels[ln]; !ok {
				a.Labels[ln] = lv
			}
		}
	}

	b, err := json.Marshal(alerts)
	if err != nil {
		log.Errorf("Encoding alerts failed: %s", err)
		return len(n.opts.AlertmanagerURLs)
	}
	ctx, _ := context.WithTimeout(context.Background(), n.opts.Timeout)

	send := func(u string) error {
		resp, err := ctxhttp.Post(ctx, http.DefaultClient, postURL(u), contentTypeJSON, bytes.NewReader(b))
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("bad response status %v", resp.Status)
		}
		return err
	}

	var (
		wg        sync.WaitGroup
		numErrors uint64
	)
	for _, u := range n.opts.AlertmanagerURLs {
		wg.Add(1)

		go func(u string) {
			if err := send(u); err != nil {
				log.With("alertmanager", u).With("count", fmt.Sprintf("%d", len(alerts))).Errorf("Error sending alerts: %s", err)
				n.errors.WithLabelValues(u).Inc()
				atomic.AddUint64(&numErrors, 1)
			}
			n.latency.WithLabelValues(u).Observe(time.Since(begin).Seconds())
			n.sent.WithLabelValues(u).Add(float64(len(alerts)))

			wg.Done()
		}(u)
	}
	wg.Wait()

	return int(numErrors)
}

// Stop shuts down the notification handler.
func (n *Notifier) Stop() {
	log.Info("Stopping notification handler...")
	n.cancel()
}

// Describe implements prometheus.Collector.
func (n *Notifier) Describe(ch chan<- *prometheus.Desc) {
	n.latency.Describe(ch)
	n.errors.Describe(ch)
	n.sent.Describe(ch)

	ch <- n.dropped.Desc()
	ch <- n.queueLength.Desc()
	ch <- n.queueCapacity.Desc()
}

// Collect implements prometheus.Collector.
func (n *Notifier) Collect(ch chan<- prometheus.Metric) {
	n.queueLength.Set(float64(n.queueLen()))

	n.latency.Collect(ch)
	n.errors.Collect(ch)
	n.sent.Collect(ch)

	ch <- n.dropped
	ch <- n.queueLength
	ch <- n.queueCapacity
}
