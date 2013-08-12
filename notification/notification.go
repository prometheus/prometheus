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
	"io/ioutil"
	"log"
	"net/http"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/utility"
)

const (
	alertmanagerApiEventsPath = "/api/events"
	contentTypeJson           = "application/json"
)

var (
	deadline = flag.Duration("alertmanager.httpDeadline", 10*time.Second, "Alert manager HTTP API timeout.")
)

// NotificationHandler is responsible for dispatching alert notifications to an
// alert manager service.
type NotificationHandler struct {
	// The URL of the alert manager to send notifications to.
	alertmanagerUrl string
	// The URL of this Prometheus instance to include in notifications.
	prometheusUrl string
	// Buffer of notifications that have not yet been sent.
	pendingNotifications <-chan rules.NotificationReqs
	// HTTP client with custom timeout settings.
	httpClient http.Client
}

// Construct a new NotificationHandler.
func NewNotificationHandler(alertmanagerUrl string, prometheusUrl string, notificationReqs <-chan rules.NotificationReqs) *NotificationHandler {
	return &NotificationHandler{
		alertmanagerUrl:      alertmanagerUrl,
		pendingNotifications: notificationReqs,
		httpClient:           utility.NewDeadlineClient(*deadline),
		prometheusUrl:        prometheusUrl,
	}
}

// Send a list of notifications to the configured alert manager.
func (n *NotificationHandler) sendNotifications(reqs rules.NotificationReqs) error {
	alerts := make([]map[string]interface{}, 0, len(reqs))
	for _, req := range reqs {
		alerts = append(alerts, map[string]interface{}{
			"Summary":     req.Rule.Summary,
			"Description": req.Rule.Description,
			"Labels": req.ActiveAlert.Labels.Merge(clientmodel.LabelSet{
				rules.AlertNameLabel: clientmodel.LabelValue(req.Rule.Name()),
			}),
			"Payload": map[string]interface{}{
				"Value":        req.ActiveAlert.Value,
				"ActiveSince":  req.ActiveAlert.ActiveSince,
				"GeneratorUrl": n.prometheusUrl,
				"AlertingRule": req.Rule.String(),
			},
		})
	}
	buf, err := json.Marshal(alerts)
	if err != nil {
		return err
	}
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

// Report notification queue occupancy and capacity.
func (n *NotificationHandler) reportQueues() {
	notificationsQueueSize.Set(map[string]string{facet: occupancy}, float64(len(n.pendingNotifications)))
	notificationsQueueSize.Set(map[string]string{facet: capacity}, float64(cap(n.pendingNotifications)))
}

// Continuously dispatch notifications.
func (n *NotificationHandler) Run() {
	queueReportTicker := time.NewTicker(time.Second)
	go func() {
		for _ = range queueReportTicker.C {
			n.reportQueues()
		}
	}()
	defer queueReportTicker.Stop()

	for reqs := range n.pendingNotifications {
		if n.alertmanagerUrl == "" {
			log.Println("No alert manager configured, not dispatching notification")
			notificationsCount.Increment(map[string]string{result: dropped})
			continue
		}

		begin := time.Now()
		err := n.sendNotifications(reqs)
		recordOutcome(time.Since(begin), err)

		if err != nil {
			log.Println("Error sending notification:", err)
		}
	}
}
