// Copyright 2013 Prometheus Team
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
	"flag"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

const (
	alertmanagerApiEventsPath = "/api/alerts"
	contentTypeJson           = "application/json"
)

// String constants for instrumentation.
const (
	namespace = "prometheus"
	subsystem = "notifications"

	result  = "result"
	success = "success"
	failure = "failure"
	dropped = "dropped"
)

var (
	deadline = flag.Duration("alertmanager.http-deadline", 10*time.Second, "Alert manager HTTP API timeout.")
)

// A request for sending a notification to the alert manager for a single alert
// vector element.
type NotificationReq struct {
	// Short-form alert summary. May contain text/template-style interpolations.
	Summary string
	// Longer alert description. May contain text/template-style interpolations.
	Description string
	// Labels associated with this alert notification, including alert name.
	Labels clientmodel.LabelSet
	// Current value of alert
	Value clientmodel.SampleValue
	// Since when this alert has been active (pending or firing).
	ActiveSince time.Time
	// A textual representation of the rule that triggered the alert.
	RuleString string
	// Prometheus console link to alert expression.
	GeneratorUrl string
}

type NotificationReqs []*NotificationReq

type httpPoster interface {
	Post(url string, bodyType string, body io.Reader) (*http.Response, error)
}

// NotificationHandler is responsible for dispatching alert notifications to an
// alert manager service.
type NotificationHandler struct {
	// The URL of the alert manager to send notifications to.
	alertmanagerUrl string
	// Buffer of notifications that have not yet been sent.
	pendingNotifications chan NotificationReqs
	// HTTP client with custom timeout settings.
	httpClient httpPoster

	notificationLatency        *prometheus.SummaryVec
	notificationsQueueLength   prometheus.Gauge
	notificationsQueueCapacity prometheus.Metric

	stopped chan struct{}
}

// Construct a new NotificationHandler.
func NewNotificationHandler(alertmanagerUrl string, notificationQueueCapacity int) *NotificationHandler {
	return &NotificationHandler{
		alertmanagerUrl:      alertmanagerUrl,
		pendingNotifications: make(chan NotificationReqs, notificationQueueCapacity),

		httpClient: utility.NewDeadlineClient(*deadline),

		notificationLatency: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "latency_milliseconds",
				Help:      "Latency quantiles for sending alert notifications.",
			},
			[]string{result},
		),
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
			float64(notificationQueueCapacity),
		),
		stopped: make(chan struct{}),
	}
}

// Send a list of notifications to the configured alert manager.
func (n *NotificationHandler) sendNotifications(reqs NotificationReqs) error {
	alerts := make([]map[string]interface{}, 0, len(reqs))
	for _, req := range reqs {
		alerts = append(alerts, map[string]interface{}{
			"Summary":     req.Summary,
			"Description": req.Description,
			"Labels":      req.Labels,
			"Payload": map[string]interface{}{
				"Value":        req.Value,
				"ActiveSince":  req.ActiveSince,
				"GeneratorUrl": req.GeneratorUrl,
				"AlertingRule": req.RuleString,
			},
		})
	}
	buf, err := json.Marshal(alerts)
	if err != nil {
		return err
	}
	glog.V(1).Infoln("Sending notifications to alertmanager:", string(buf))
	resp, err := n.httpClient.Post(
		n.alertmanagerUrl+alertmanagerApiEventsPath,
		contentTypeJson,
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
		if n.alertmanagerUrl == "" {
			glog.Warning("No alert manager configured, not dispatching notification")
			n.notificationLatency.WithLabelValues(dropped).Observe(0)
			continue
		}

		begin := time.Now()
		err := n.sendNotifications(reqs)
		labelValue := success

		if err != nil {
			glog.Error("Error sending notification: ", err)
			labelValue = failure
		}

		n.notificationLatency.WithLabelValues(labelValue).Observe(
			float64(time.Since(begin) / time.Millisecond),
		)
	}
	close(n.stopped)
}

// SubmitReqs queues the given notification requests for processing.
func (n *NotificationHandler) SubmitReqs(reqs NotificationReqs) {
	n.pendingNotifications <- reqs
}

// Stop shuts down the notification handler.
func (n *NotificationHandler) Stop() {
	glog.Info("Stopping notification handler...")
	close(n.pendingNotifications)
	<-n.stopped
	glog.Info("Notification handler stopped.")
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
