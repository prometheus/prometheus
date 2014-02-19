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
	"text/template"
	"time"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

const (
	alertmanagerApiEventsPath = "/api/alerts"
	contentTypeJson           = "application/json"
)

var (
	deadline = flag.Duration("alertmanager.httpDeadline", 10*time.Second, "Alert manager HTTP API timeout.")
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
	pendingNotifications <-chan NotificationReqs
	// HTTP client with custom timeout settings.
	httpClient httpPoster
}

// Construct a new NotificationHandler.
func NewNotificationHandler(alertmanagerUrl string, notificationReqs <-chan NotificationReqs) *NotificationHandler {
	return &NotificationHandler{
		alertmanagerUrl:      alertmanagerUrl,
		pendingNotifications: notificationReqs,
		httpClient:           utility.NewDeadlineClient(*deadline),
	}
}

// Interpolate alert information into summary/description templates.
func interpolateMessage(msg string, labels clientmodel.LabelSet, value clientmodel.SampleValue) string {
	t := template.New("message")

	// Inject some convenience variables that are easier to remember for users
	// who are not used to Go's templating system.
	defs :=
		"{{$labels := .Labels}}" +
			"{{$value := .Value}}"

	if _, err := t.Parse(defs + msg); err != nil {
		glog.Warning("Error parsing template: ", err)
		return msg
	}

	l := map[string]string{}
	for k, v := range labels {
		l[string(k)] = string(v)
	}

	tmplData := struct {
		Labels map[string]string
		Value  clientmodel.SampleValue
	}{
		Labels: l,
		Value:  value,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, &tmplData); err != nil {
		glog.Warning("Error executing template: ", err)
		return msg
	}
	return buf.String()
}

// Send a list of notifications to the configured alert manager.
func (n *NotificationHandler) sendNotifications(reqs NotificationReqs) error {
	alerts := make([]map[string]interface{}, 0, len(reqs))
	for _, req := range reqs {
		alerts = append(alerts, map[string]interface{}{
			"Summary":     interpolateMessage(req.Summary, req.Labels, req.Value),
			"Description": interpolateMessage(req.Description, req.Labels, req.Value),
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
			glog.Warning("No alert manager configured, not dispatching notification")
			notificationsCount.Increment(map[string]string{result: dropped})
			continue
		}

		begin := time.Now()
		err := n.sendNotifications(reqs)
		recordOutcome(time.Since(begin), err)

		if err != nil {
			glog.Error("Error sending notification: ", err)
		}
	}
}
