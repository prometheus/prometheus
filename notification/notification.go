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
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/httputil"
)

const (
	alertmanagerAPIEventsPath = "/api/alerts"
	contentTypeJSON           = "application/json"
)

// String constants for instrumentation.
const (
	namespace = "prometheus"
	subsystem = "notifications"
)

// NotificationReq is a request for sending a notification to the alert manager
// for a single alert vector element.
type NotificationReq struct {
	// Short-form alert summary. May contain text/template-style interpolations.
	Summary string
	// Longer alert description. May contain text/template-style interpolations.
	Description string
	// A reference to the runbook for the alert.
	Runbook string
	// Labels associated with this alert notification, including alert name.
	Labels clientmodel.LabelSet
	// Current value of alert
	Value clientmodel.SampleValue
	// Since when this alert has been active (pending or firing).
	ActiveSince time.Time
	// A textual representation of the rule that triggered the alert.
	RuleString string
	// Prometheus console link to alert expression.
	GeneratorURL string
}

// NotificationReqs is just a short-hand for []*NotificationReq. No methods
// attached. Arguably, it's more confusing than helpful. Perhaps we should
// remove it...
type NotificationReqs []*NotificationReq

type httpPoster interface {
	Post(url string, bodyType string, body io.Reader) (*http.Response, error)
}

// NotificationHandler is responsible for dispatching alert notifications to an
// alert manager service.
type NotificationHandler struct {
	// The URLs of the alert managers to send notifications to.
	alertmanagerURLs []string
	// Buffer of notifications that have not yet been sent.
	pendingNotifications chan NotificationReqs
	// HTTP client with custom timeout settings.
	httpClient httpPoster

	notificationLatency        prometheus.Summary
	notificationErrors         prometheus.Counter
	notificationDropped        prometheus.Counter
	notificationsQueueLength   prometheus.Gauge
	notificationsQueueCapacity prometheus.Metric

	stopped chan struct{}
	mtx     sync.RWMutex
}

// NotificationHandlerOptions are the configurable parameters of a NotificationHandler.
type NotificationHandlerOptions struct {
	QueueCapacity int
	Deadline      time.Duration
}

// NewNotificationHandler constructs a new NotificationHandler.
func NewNotificationHandler(o *NotificationHandlerOptions) *NotificationHandler {
	return &NotificationHandler{
		pendingNotifications: make(chan NotificationReqs, o.QueueCapacity),

		httpClient: httputil.NewDeadlineClient(o.Deadline, nil),

		notificationLatency: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "latency_milliseconds",
			Help:      "Latency quantiles for sending alert notifications (not including dropped notifications).",
		}),
		notificationErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "Total number of errors sending alert notifications.",
		}),
		notificationDropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "dropped_total",
			Help:      "Total number of alert notifications dropped due to alert manager missing in configuration.",
		}),
		notificationsQueueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_length",
			Help:      "The number of alert notifications in the queue.",
		}),
		notificationsQueueCapacity: prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "queue_capacity"),
				"The capacity of the alert notifications queue.",
				nil, nil,
			),
			prometheus.GaugeValue,
			float64(o.QueueCapacity),
		),
		stopped: make(chan struct{}),
	}
}

// ApplyConfig updates the notification handler's state as the config requires.
func (n *NotificationHandler) ApplyConfig(conf *config.Config) bool {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	var amURLs []string
	for _, au := range conf.AlertmanagerURLs {
		amURLs = append(amURLs, strings.TrimRight(au, "/"))
	}
	n.alertmanagerURLs = amURLs

	return true
}

// Send a list of notifications to the configured alert manager.
func (n *NotificationHandler) sendNotifications(reqs NotificationReqs) error {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	if len(n.alertmanagerURLs) == 0 {
		return nil
	}

	alerts := make([]map[string]interface{}, 0, len(reqs))
	for _, req := range reqs {
		alerts = append(alerts, map[string]interface{}{
			"summary":     req.Summary,
			"description": req.Description,
			"runbook":     req.Runbook,
			"labels":      req.Labels,
			"payload": map[string]interface{}{
				"value":        req.Value,
				"activeSince":  req.ActiveSince,
				"generatorURL": req.GeneratorURL,
				"alertingRule": req.RuleString,
			},
		})
	}
	buf, err := json.Marshal(alerts)
	if err != nil {
		return err
	}
	log.Debugf("Sending notifications to alertmanager: %s", buf)

	resp, err := n.httpClient.Post(
		n.alertmanagerURLs[0]+alertmanagerAPIEventsPath,
		contentTypeJSON,
		bytes.NewBuffer(buf),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// BUG: Do we need to check the response code?
	return nil
}

// Run dispatches notifications continuously.
func (n *NotificationHandler) Run() {
	for reqs := range n.pendingNotifications {
		if len(n.alertmanagerURLs) == 0 {
			log.Warn("No alert manager configured, not dispatching notification")
			n.notificationDropped.Inc()
			continue
		}

		begin := time.Now()
		err := n.sendNotifications(reqs)

		if err != nil {
			log.Error("Error sending notification: ", err)
			n.notificationErrors.Inc()
		}

		n.notificationLatency.Observe(float64(time.Since(begin) / time.Millisecond))
	}
	close(n.stopped)
}

// SubmitReqs queues the given notification requests for processing.
func (n *NotificationHandler) SubmitReqs(reqs NotificationReqs) {
	n.pendingNotifications <- reqs
}

// Stop shuts down the notification handler.
func (n *NotificationHandler) Stop() {
	log.Info("Stopping notification handler...")
	close(n.pendingNotifications)
	<-n.stopped
	log.Info("Notification handler stopped.")
}

// Describe implements prometheus.Collector.
func (n *NotificationHandler) Describe(ch chan<- *prometheus.Desc) {
	n.notificationLatency.Describe(ch)
	ch <- n.notificationsQueueLength.Desc()
	ch <- n.notificationsQueueCapacity.Desc()
}

// Collect implements prometheus.Collector.
func (n *NotificationHandler) Collect(ch chan<- prometheus.Metric) {
	n.notificationLatency.Collect(ch)
	n.notificationsQueueLength.Set(float64(len(n.pendingNotifications)))
	ch <- n.notificationsQueueLength
	ch <- n.notificationsQueueCapacity
}
