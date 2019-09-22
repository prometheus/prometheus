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
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/relabel"
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
		testutil.Equals(t, c.out, postPath(c.in, config.AlertmanagerAPIVersionV1))
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

	testutil.Ok(t, alertsEqual(expected[0:maxBatchSize], h.nextBatch()))
	testutil.Ok(t, alertsEqual(expected[maxBatchSize:2*maxBatchSize], h.nextBatch()))
	testutil.Ok(t, alertsEqual(expected[2*maxBatchSize:], h.nextBatch()))
	testutil.Assert(t, len(h.queue) == 0, "Expected queue to be empty but got %d alerts", len(h.queue))
}

func alertsEqual(a, b []*Alert) error {
	if len(a) != len(b) {
		return errors.Errorf("length mismatch: %v != %v", a, b)
	}
	for i, alert := range a {
		if !labels.Equal(alert.Labels, b[i].Labels) {
			return errors.Errorf("label mismatch at index %d: %s != %s", i, alert.Labels, b[i].Labels)
		}
	}
	return nil
}

func TestHandlerSendAll(t *testing.T) {
	var (
		errc             = make(chan error, 1)
		expected         = make([]*Alert, 0, maxBatchSize)
		status1, status2 = int32(http.StatusOK), int32(http.StatusOK)
	)

	newHTTPServer := func(u, p string, status *int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			defer func() {
				if err == nil {
					return
				}
				select {
				case errc <- err:
				default:
				}
			}()
			user, pass, _ := r.BasicAuth()
			if user != u || pass != p {
				err = errors.Errorf("unexpected user/password: %s/%s != %s/%s", user, pass, u, p)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var alerts []*Alert
			err = json.NewDecoder(r.Body).Decode(&alerts)
			if err == nil {
				err = alertsEqual(expected, alerts)
			}
			w.WriteHeader(int(atomic.LoadInt32(status)))
		}))
	}
	server1 := newHTTPServer("prometheus", "testing_password", &status1)
	server2 := newHTTPServer("", "", &status2)
	defer server1.Close()
	defer server2.Close()

	h := NewManager(&Options{}, nil)

	authClient, _ := config_util.NewClientFromConfig(
		config_util.HTTPClientConfig{
			BasicAuth: &config_util.BasicAuth{
				Username: "prometheus",
				Password: "testing_password",
			},
		}, "auth_alertmanager", false)

	h.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	am2Cfg := config.DefaultAlertmanagerConfig
	am2Cfg.Timeout = model.Duration(time.Second)

	h.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server1.URL },
			},
		},
		cfg:    &am1Cfg,
		client: authClient,
	}

	h.alertmanagers["2"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server2.URL },
			},
		},
		cfg: &am2Cfg,
	}

	for i := range make([]struct{}, maxBatchSize) {
		h.queue = append(h.queue, &Alert{
			Labels: labels.FromStrings("alertname", fmt.Sprintf("%d", i)),
		})
		expected = append(expected, &Alert{
			Labels: labels.FromStrings("alertname", fmt.Sprintf("%d", i)),
		})
	}

	checkNoErr := func() {
		t.Helper()
		select {
		case err := <-errc:
			testutil.Ok(t, err)
		default:
		}
	}

	testutil.Assert(t, h.sendAll(h.queue...), "all sends failed unexpectedly")
	checkNoErr()

	atomic.StoreInt32(&status1, int32(http.StatusNotFound))
	testutil.Assert(t, h.sendAll(h.queue...), "all sends failed unexpectedly")
	checkNoErr()

	atomic.StoreInt32(&status2, int32(http.StatusInternalServerError))
	testutil.Assert(t, !h.sendAll(h.queue...), "all sends succeeded unexpectedly")
	checkNoErr()
}

func TestCustomDo(t *testing.T) {
	const testURL = "http://testurl.com/"
	const testBody = "testbody"

	var received bool
	h := NewManager(&Options{
		Do: func(_ context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
			received = true
			body, err := ioutil.ReadAll(req.Body)

			testutil.Ok(t, err)

			testutil.Equals(t, testBody, string(body))

			testutil.Equals(t, testURL, req.URL.String())

			return &http.Response{
				Body: ioutil.NopCloser(bytes.NewBuffer(nil)),
			}, nil
		},
	}, nil)

	h.sendOne(context.Background(), nil, testURL, []byte(testBody))

	testutil.Assert(t, received, "Expected to receive an alert, but didn't")
}

func TestExternalLabels(t *testing.T) {
	h := NewManager(&Options{
		QueueCapacity:  3 * maxBatchSize,
		ExternalLabels: labels.Labels{{Name: "a", Value: "b"}},
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels: model.LabelNames{"alertname"},
				TargetLabel:  "a",
				Action:       "replace",
				Regex:        relabel.MustNewRegexp("externalrelabelthis"),
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

	testutil.Ok(t, alertsEqual(expected, h.queue))
}

func TestHandlerRelabel(t *testing.T) {
	h := NewManager(&Options{
		QueueCapacity: 3 * maxBatchSize,
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels: model.LabelNames{"alertname"},
				Action:       "drop",
				Regex:        relabel.MustNewRegexp("drop"),
			},
			{
				SourceLabels: model.LabelNames{"alertname"},
				TargetLabel:  "alertname",
				Action:       "replace",
				Regex:        relabel.MustNewRegexp("rename"),
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

	testutil.Ok(t, alertsEqual(expected, h.queue))
}

func TestHandlerQueueing(t *testing.T) {
	var (
		expectedc = make(chan []*Alert)
		called    = make(chan struct{})
		done      = make(chan struct{})
		errc      = make(chan error, 1)
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Notify the test function that we have received something.
		select {
		case called <- struct{}{}:
		case <-done:
			return
		}

		// Wait for the test function to unblock us.
		select {
		case expected := <-expectedc:
			var alerts []*Alert
			err := json.NewDecoder(r.Body).Decode(&alerts)
			if err == nil {
				err = alertsEqual(expected, alerts)
			}
			select {
			case errc <- err:
			default:
			}
		case <-done:
		}
	}))
	defer func() {
		close(done)
		server.Close()
	}()

	h := NewManager(
		&Options{
			QueueCapacity: 3 * maxBatchSize,
		},
		nil,
	)

	h.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	h.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server.URL },
			},
		},
		cfg: &am1Cfg,
	}
	go h.Run(nil)
	defer h.Stop()

	var alerts []*Alert
	for i := range make([]struct{}, 20*maxBatchSize) {
		alerts = append(alerts, &Alert{
			Labels: labels.FromStrings("alertname", fmt.Sprintf("%d", i)),
		})
	}

	assertAlerts := func(expected []*Alert) {
		t.Helper()
		for {
			select {
			case <-called:
				expectedc <- expected
			case err := <-errc:
				testutil.Ok(t, err)
				return
			case <-time.After(5 * time.Second):
				t.Fatalf("Alerts were not pushed")
			}
		}
	}

	// If the batch is larger than the queue capacity, it should be truncated
	// from the front.
	h.Send(alerts[:4*maxBatchSize]...)
	for i := 1; i < 4; i++ {
		assertAlerts(alerts[i*maxBatchSize : (i+1)*maxBatchSize])
	}

	// Send one batch, wait for it to arrive and block the server so the queue fills up.
	h.Send(alerts[:maxBatchSize]...)
	<-called

	// Send several batches while the server is still blocked so the queue
	// fills up to its maximum capacity (3*maxBatchSize). Then check that the
	// queue is truncated in the front.
	h.Send(alerts[1*maxBatchSize : 2*maxBatchSize]...) // this batch should be dropped.
	h.Send(alerts[2*maxBatchSize : 3*maxBatchSize]...)
	h.Send(alerts[3*maxBatchSize : 4*maxBatchSize]...)

	// Send the batch that drops the first one.
	h.Send(alerts[4*maxBatchSize : 5*maxBatchSize]...)

	// Unblock the server.
	expectedc <- alerts[:maxBatchSize]
	select {
	case err := <-errc:
		testutil.Ok(t, err)
	case <-time.After(5 * time.Second):
		t.Fatalf("Alerts were not pushed")
	}

	// Verify that we receive the last 3 batches.
	for i := 2; i < 5; i++ {
		assertAlerts(alerts[i*maxBatchSize : (i+1)*maxBatchSize])
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

	testutil.Ok(t, err)

	// Target modified during alertmanager extraction
	testutil.Equals(t, tg, makeInputTargetGroup())
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
	if err := yaml.UnmarshalStrict([]byte(s), cfg); err != nil {
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
	if err := yaml.UnmarshalStrict([]byte(s), cfg); err != nil {
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
