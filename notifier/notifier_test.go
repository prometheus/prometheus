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
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	old_ctx "golang.org/x/net/context"
	yaml "gopkg.in/yaml.v2"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/util/httputil"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestPostPath(t *testing.T) {
	var cases = []struct {
		in, out string
	}{
		{
			in:  "",
			out: "/api/v1/alerts",
		},
		{
			in:  "/",
			out: "/api/v1/alerts",
		},
		{
			in:  "/prefix",
			out: "/prefix/api/v1/alerts",
		},
		{
			in:  "/prefix//",
			out: "/prefix/api/v1/alerts",
		},
		{
			in:  "prefix//",
			out: "/prefix/api/v1/alerts",
		},
	}
	for _, c := range cases {
		if res := postPath(c.in); res != c.out {
			t.Errorf("Expected post path %q for %q but got %q", c.out, c.in, res)
		}
	}
}

func TestHandlerNextBatch(t *testing.T) {
	h := NewManager(&Options{}, nil)

	for i := range make([]struct{}, 2*maxBatchSize+1) {
		h.queue = append(h.queue, &Alert{
			Labels: labels.FromStrings("alertname", fmt.Sprintf("%d", i)),
		})
	}

	expected := append([]*Alert{}, h.queue...)

	b := h.nextBatch()

	if len(b) != maxBatchSize {
		t.Fatalf("Expected first batch of length %d, but got %d", maxBatchSize, len(b))
	}
	if !alertsEqual(expected[0:maxBatchSize], b) {
		t.Fatalf("First batch did not match")
	}

	b = h.nextBatch()

	if len(b) != maxBatchSize {
		t.Fatalf("Expected second batch of length %d, but got %d", maxBatchSize, len(b))
	}
	if !alertsEqual(expected[maxBatchSize:2*maxBatchSize], b) {
		t.Fatalf("Second batch did not match")
	}

	b = h.nextBatch()

	if len(b) != 1 {
		t.Fatalf("Expected third batch of length %d, but got %d", 1, len(b))
	}
	if !alertsEqual(expected[2*maxBatchSize:], b) {
		t.Fatalf("Third batch did not match")
	}

	if len(h.queue) != 0 {
		t.Fatalf("Expected queue to be empty but got %d alerts", len(h.queue))
	}
}

func alertsEqual(a, b []*Alert) bool {
	if len(a) != len(b) {
		fmt.Println("len mismatch")
		return false
	}
	for i, alert := range a {
		if !labels.Equal(alert.Labels, b[i].Labels) {
			fmt.Println("mismatch", alert.Labels, b[i].Labels)
			return false
		}
	}
	return true
}

func TestHandlerSendAll(t *testing.T) {
	var (
		expected         []*Alert
		status1, status2 int
	)

	f := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var alerts []*Alert
		if err := json.NewDecoder(r.Body).Decode(&alerts); err != nil {
			t.Fatalf("Unexpected error on input decoding: %s", err)
		}

		if !alertsEqual(alerts, expected) {
			t.Errorf("%#v %#v", *alerts[0], *expected[0])
			t.Fatalf("Unexpected alerts received %v exp %v", alerts, expected)
		}
	}
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		if user != "prometheus" || pass != "testing_password" {
			t.Fatalf("Incorrect auth details for an alertmanager")
		}

		f(w, r)
		w.WriteHeader(status1)
	}))
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		if user != "" || pass != "" {
			t.Fatalf("Incorrectly received auth details for an alertmanager")
		}

		f(w, r)
		w.WriteHeader(status2)
	}))

	defer server1.Close()
	defer server2.Close()

	h := NewManager(&Options{}, nil)

	authClient, _ := httputil.NewClientFromConfig(config_util.HTTPClientConfig{
		BasicAuth: &config_util.BasicAuth{
			Username: "prometheus",
			Password: "testing_password",
		},
	}, "auth_alertmanager")

	h.alertmanagers = make(map[string]*alertmanagerSet)

	h.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server1.URL },
			},
		},
		cfg: &config.AlertmanagerConfig{
			Timeout: time.Second,
		},
		client: authClient,
	}

	h.alertmanagers["2"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server2.URL },
			},
		},
		cfg: &config.AlertmanagerConfig{
			Timeout: time.Second,
		},
	}

	for i := range make([]struct{}, maxBatchSize) {
		h.queue = append(h.queue, &Alert{
			Labels: labels.FromStrings("alertname", fmt.Sprintf("%d", i)),
		})
		expected = append(expected, &Alert{
			Labels: labels.FromStrings("alertname", fmt.Sprintf("%d", i)),
		})
	}

	status1 = http.StatusOK
	status2 = http.StatusOK
	if !h.sendAll(h.queue...) {
		t.Fatalf("all sends failed unexpectedly")
	}

	status1 = http.StatusNotFound
	if !h.sendAll(h.queue...) {
		t.Fatalf("all sends failed unexpectedly")
	}

	status2 = http.StatusInternalServerError
	if h.sendAll(h.queue...) {
		t.Fatalf("all sends succeeded unexpectedly")
	}
}

func TestCustomDo(t *testing.T) {
	const testURL = "http://testurl.com/"
	const testBody = "testbody"

	var received bool
	h := NewManager(&Options{
		Do: func(ctx old_ctx.Context, client *http.Client, req *http.Request) (*http.Response, error) {
			received = true
			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("Unable to read request body: %v", err)
			}
			if string(body) != testBody {
				t.Fatalf("Unexpected body; want %v, got %v", testBody, string(body))
			}
			if req.URL.String() != testURL {
				t.Fatalf("Unexpected URL; want %v, got %v", testURL, req.URL.String())
			}
			return &http.Response{
				Body: ioutil.NopCloser(nil),
			}, nil
		},
	}, nil)

	h.sendOne(context.Background(), nil, testURL, []byte(testBody))

	if !received {
		t.Fatal("Expected to receive an alert, but didn't")
	}
}

func TestExternalLabels(t *testing.T) {
	h := NewManager(&Options{
		QueueCapacity:  3 * maxBatchSize,
		ExternalLabels: model.LabelSet{"a": "b"},
		RelabelConfigs: []*config.RelabelConfig{
			{
				SourceLabels: model.LabelNames{"alertname"},
				TargetLabel:  "a",
				Action:       "replace",
				Regex:        config.MustNewRegexp("externalrelabelthis"),
				Replacement:  "c",
			},
		},
	}, nil)

	// This alert should get the external label attached.
	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "test"),
	})

	// This alert should get the external label attached, but then set to "c"
	// through relabelling.
	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "externalrelabelthis"),
	})

	expected := []*Alert{
		{Labels: labels.FromStrings("alertname", "test", "a", "b")},
		{Labels: labels.FromStrings("alertname", "externalrelabelthis", "a", "c")},
	}

	if !alertsEqual(expected, h.queue) {
		t.Errorf("Expected alerts %v, got %v", expected, h.queue)
	}
}

func TestHandlerRelabel(t *testing.T) {
	h := NewManager(&Options{
		QueueCapacity: 3 * maxBatchSize,
		RelabelConfigs: []*config.RelabelConfig{
			{
				SourceLabels: model.LabelNames{"alertname"},
				Action:       "drop",
				Regex:        config.MustNewRegexp("drop"),
			},
			{
				SourceLabels: model.LabelNames{"alertname"},
				TargetLabel:  "alertname",
				Action:       "replace",
				Regex:        config.MustNewRegexp("rename"),
				Replacement:  "renamed",
			},
		},
	}, nil)

	// This alert should be dropped due to the configuration
	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "drop"),
	})

	// This alert should be replaced due to the configuration
	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "rename"),
	})

	expected := []*Alert{
		{Labels: labels.FromStrings("alertname", "renamed")},
	}

	if !alertsEqual(expected, h.queue) {
		t.Errorf("Expected alerts %v, got %v", expected, h.queue)
	}
}

func TestHandlerQueueing(t *testing.T) {
	var (
		unblock  = make(chan struct{})
		called   = make(chan struct{})
		expected []*Alert
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- struct{}{}
		<-unblock

		defer r.Body.Close()

		var alerts []*Alert
		if err := json.NewDecoder(r.Body).Decode(&alerts); err != nil {
			t.Fatalf("Unexpected error on input decoding: %s", err)
		}

		if !alertsEqual(expected, alerts) {
			t.Errorf("Expected alerts %v, got %v", expected, alerts)
		}
	}))

	h := NewManager(&Options{
		QueueCapacity: 3 * maxBatchSize,
	},
		nil,
	)

	h.alertmanagers = make(map[string]*alertmanagerSet)

	h.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server.URL },
			},
		},
		cfg: &config.AlertmanagerConfig{
			Timeout: time.Second,
		},
	}

	var alerts []*Alert

	for i := range make([]struct{}, 20*maxBatchSize) {
		alerts = append(alerts, &Alert{
			Labels: labels.FromStrings("alertname", fmt.Sprintf("%d", i)),
		})
	}

	c := make(chan map[string][]*targetgroup.Group)
	go h.Run(c)
	defer h.Stop()

	h.Send(alerts[:4*maxBatchSize]...)

	// If the batch is larger than the queue size, the front should be truncated
	// from the front. Thus, we start at i=1.
	for i := 1; i < 4; i++ {
		select {
		case <-called:
			expected = alerts[i*maxBatchSize : (i+1)*maxBatchSize]
			unblock <- struct{}{}
		case <-time.After(5 * time.Second):
			t.Fatalf("Alerts were not pushed")
		}
	}

	// Send one batch, wait for it to arrive and block so the queue fills up.
	// Then check whether the queue is truncated in the front once its full.
	h.Send(alerts[:maxBatchSize]...)
	<-called

	// Fill the 3*maxBatchSize queue.
	h.Send(alerts[1*maxBatchSize : 2*maxBatchSize]...)
	h.Send(alerts[2*maxBatchSize : 3*maxBatchSize]...)
	h.Send(alerts[3*maxBatchSize : 4*maxBatchSize]...)

	// Send the batch that drops the first one.
	h.Send(alerts[4*maxBatchSize : 5*maxBatchSize]...)

	expected = alerts[:maxBatchSize]
	unblock <- struct{}{}

	for i := 2; i < 4; i++ {
		select {
		case <-called:
			expected = alerts[i*maxBatchSize : (i+1)*maxBatchSize]
			unblock <- struct{}{}
		case <-time.After(5 * time.Second):
			t.Fatalf("Alerts were not pushed")
		}
	}
}

type alertmanagerMock struct {
	urlf func() string
}

func (a alertmanagerMock) url() *url.URL {
	u, err := url.Parse(a.urlf())
	if err != nil {
		panic(err)
	}
	return u
}

func TestLabelSetNotReused(t *testing.T) {
	tg := makeInputTargetGroup()
	_, _, err := alertmanagerFromGroup(tg, &config.AlertmanagerConfig{})
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(tg, makeInputTargetGroup()) {
		t.Fatal("Target modified during alertmanager extraction")
	}
}

func TestReload(t *testing.T) {
	var tests = []struct {
		in  *targetgroup.Group
		out string
	}{
		{
			in: &targetgroup.Group{
				Targets: []model.LabelSet{
					{
						"__address__": "alertmanager:9093",
					},
				},
			},
			out: "http://alertmanager:9093/api/v1/alerts",
		},
	}

	n := NewManager(&Options{}, nil)

	cfg := &config.Config{}
	s := `
alerting:
  alertmanagers:
  - static_configs:
`
	if err := yaml.Unmarshal([]byte(s), cfg); err != nil {
		t.Fatalf("Unable to load YAML config: %s", err)
	}

	if err := n.ApplyConfig(cfg); err != nil {
		t.Fatalf("Error Applying the config:%v", err)
	}

	tgs := make(map[string][]*targetgroup.Group)
	for _, tt := range tests {

		b, err := json.Marshal(cfg.AlertingConfig.AlertmanagerConfigs[0])
		if err != nil {
			t.Fatalf("Error creating config hash:%v", err)
		}
		tgs[fmt.Sprintf("%x", md5.Sum(b))] = []*targetgroup.Group{
			tt.in,
		}
		n.reload(tgs)
		res := n.Alertmanagers()[0].String()

		testutil.Equals(t, res, tt.out)
	}

}

func TestDroppedAlertmanagers(t *testing.T) {
	var tests = []struct {
		in  *targetgroup.Group
		out string
	}{
		{
			in: &targetgroup.Group{
				Targets: []model.LabelSet{
					{
						"__address__": "alertmanager:9093",
					},
				},
			},
			out: "http://alertmanager:9093/api/v1/alerts",
		},
	}

	n := NewManager(&Options{}, nil)

	cfg := &config.Config{}
	s := `
alerting:
  alertmanagers:
  - static_configs:
    relabel_configs:
      - source_labels: ['__address__']
        regex: 'alertmanager:9093'
        action: drop
`
	if err := yaml.Unmarshal([]byte(s), cfg); err != nil {
		t.Fatalf("Unable to load YAML config: %s", err)
	}

	if err := n.ApplyConfig(cfg); err != nil {
		t.Fatalf("Error Applying the config:%v", err)
	}

	tgs := make(map[string][]*targetgroup.Group)
	for _, tt := range tests {

		b, err := json.Marshal(cfg.AlertingConfig.AlertmanagerConfigs[0])
		if err != nil {
			t.Fatalf("Error creating config hash:%v", err)
		}
		tgs[fmt.Sprintf("%x", md5.Sum(b))] = []*targetgroup.Group{
			tt.in,
		}
		n.reload(tgs)
		res := n.DroppedAlertmanagers()[0].String()

		testutil.Equals(t, res, tt.out)
	}

}

func makeInputTargetGroup() *targetgroup.Group {
	return &targetgroup.Group{
		Targets: []model.LabelSet{
			{
				model.AddressLabel:            model.LabelValue("1.1.1.1:9090"),
				model.LabelName("notcommon1"): model.LabelValue("label"),
			},
		},
		Labels: model.LabelSet{
			model.LabelName("common"): model.LabelValue("label"),
		},
		Source: "testsource",
	}
}
