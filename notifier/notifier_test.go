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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/alertmanager/api/v2/models"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

func TestPostPath(t *testing.T) {
	cases := []struct {
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
		require.Equal(t, c.out, postPath(c.in, config.AlertmanagerAPIVersionV1))
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

	require.NoError(t, alertsEqual(expected[0:maxBatchSize], h.nextBatch()))
	require.NoError(t, alertsEqual(expected[maxBatchSize:2*maxBatchSize], h.nextBatch()))
	require.NoError(t, alertsEqual(expected[2*maxBatchSize:], h.nextBatch()))
	require.Empty(t, h.queue, "Expected queue to be empty but got %d alerts", len(h.queue))
}

func alertsEqual(a, b []*Alert) error {
	if len(a) != len(b) {
		return fmt.Errorf("length mismatch: %v != %v", a, b)
	}
	for i, alert := range a {
		if !labels.Equal(alert.Labels, b[i].Labels) {
			return fmt.Errorf("label mismatch at index %d: %s != %s", i, alert.Labels, b[i].Labels)
		}
	}
	return nil
}

func TestHandlerSendAll(t *testing.T) {
	var (
		errc             = make(chan error, 1)
		expected         = make([]*Alert, 0, maxBatchSize)
		status1, status2 atomic.Int32
	)
	status1.Store(int32(http.StatusOK))
	status2.Store(int32(http.StatusOK))

	newHTTPServer := func(u, p string, status *atomic.Int32) *httptest.Server {
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
				err = fmt.Errorf("unexpected user/password: %s/%s != %s/%s", user, pass, u, p)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			b, err := io.ReadAll(r.Body)
			if err != nil {
				err = fmt.Errorf("error reading body: %w", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			var alerts []*Alert
			err = json.Unmarshal(b, &alerts)
			if err == nil {
				err = alertsEqual(expected, alerts)
			}
			w.WriteHeader(int(status.Load()))
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
		}, "auth_alertmanager")

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
			require.NoError(t, err)
		default:
		}
	}

	require.True(t, h.sendAll(h.queue...), "all sends failed unexpectedly")
	checkNoErr()

	status1.Store(int32(http.StatusNotFound))
	require.True(t, h.sendAll(h.queue...), "all sends failed unexpectedly")
	checkNoErr()

	status2.Store(int32(http.StatusInternalServerError))
	require.False(t, h.sendAll(h.queue...), "all sends succeeded unexpectedly")
	checkNoErr()
}

func TestCustomDo(t *testing.T) {
	const testURL = "http://testurl.com/"
	const testBody = "testbody"

	var received bool
	h := NewManager(&Options{
		Do: func(_ context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
			received = true
			body, err := io.ReadAll(req.Body)

			require.NoError(t, err)

			require.Equal(t, testBody, string(body))

			require.Equal(t, testURL, req.URL.String())

			return &http.Response{
				Body: io.NopCloser(bytes.NewBuffer(nil)),
			}, nil
		},
	}, nil)

	h.sendOne(context.Background(), nil, testURL, []byte(testBody))

	require.True(t, received, "Expected to receive an alert, but didn't")
}

func TestExternalLabels(t *testing.T) {
	h := NewManager(&Options{
		QueueCapacity:  3 * maxBatchSize,
		ExternalLabels: labels.FromStrings("a", "b"),
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

	require.NoError(t, alertsEqual(expected, h.queue))
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

	require.NoError(t, alertsEqual(expected, h.queue))
}

func TestHandlerQueuing(t *testing.T) {
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

			b, err := io.ReadAll(r.Body)
			if err != nil {
				panic(err)
			}

			err = json.Unmarshal(b, &alerts)
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
				require.NoError(t, err)
				return
			case <-time.After(5 * time.Second):
				require.FailNow(t, "Alerts were not pushed.")
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
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Alerts were not pushed.")
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
	_, _, err := AlertmanagerFromGroup(tg, &config.AlertmanagerConfig{})

	require.NoError(t, err)

	// Target modified during alertmanager extraction
	require.Equal(t, tg, makeInputTargetGroup())
}

func TestReload(t *testing.T) {
	tests := []struct {
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
			out: "http://alertmanager:9093/api/v2/alerts",
		},
	}

	n := NewManager(&Options{}, nil)

	cfg := &config.Config{}
	s := `
alerting:
  alertmanagers:
  - static_configs:
`
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	require.NoError(t, err, "Unable to load YAML config.")
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 1)

	err = n.ApplyConfig(cfg)
	require.NoError(t, err, "Error applying the config.")

	tgs := make(map[string][]*targetgroup.Group)
	for _, tt := range tests {
		for k := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
			tgs[k] = []*targetgroup.Group{
				tt.in,
			}
			break
		}
		n.reload(tgs)
		res := n.Alertmanagers()[0].String()

		require.Equal(t, tt.out, res)
	}
}

func TestDroppedAlertmanagers(t *testing.T) {
	tests := []struct {
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
			out: "http://alertmanager:9093/api/v2/alerts",
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
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	require.NoError(t, err, "Unable to load YAML config.")
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 1)

	err = n.ApplyConfig(cfg)
	require.NoError(t, err, "Error applying the config.")

	tgs := make(map[string][]*targetgroup.Group)
	for _, tt := range tests {
		for k := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
			tgs[k] = []*targetgroup.Group{
				tt.in,
			}
			break
		}

		n.reload(tgs)
		res := n.DroppedAlertmanagers()[0].String()

		require.Equal(t, res, tt.out)
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

func TestLabelsToOpenAPILabelSet(t *testing.T) {
	require.Equal(t, models.LabelSet{"aaa": "111", "bbb": "222"}, labelsToOpenAPILabelSet(labels.FromStrings("aaa", "111", "bbb", "222")))
}

// TestHangingNotifier validates that targets updates happen even when there are
// queued alerts.
func TestHangingNotifier(t *testing.T) {
	// Note: When targets are not updated in time, this test is flaky because go
	// selects are not deterministic. Therefore we run 10 subtests to run into the issue.
	for i := 0; i < 10; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var (
				done    = make(chan struct{})
				changed = make(chan struct{})
				syncCh  = make(chan map[string][]*targetgroup.Group)
			)

			defer func() {
				close(done)
			}()

			var calledOnce bool
			// Setting up a bad server. This server hangs for 2 seconds.
			badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calledOnce {
					t.Fatal("hanging server called multiple times")
				}
				calledOnce = true
				select {
				case <-done:
				case <-time.After(2 * time.Second):
				}
			}))
			badURL, err := url.Parse(badServer.URL)
			require.NoError(t, err)
			badAddress := badURL.Host // Used for __name__ label in targets.

			// Setting up a bad server. This server returns fast, signaling requests on
			// by closing the changed channel.
			goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(changed)
			}))
			goodURL, err := url.Parse(goodServer.URL)
			require.NoError(t, err)
			goodAddress := goodURL.Host // Used for __name__ label in targets.

			h := NewManager(
				&Options{
					QueueCapacity: 20 * maxBatchSize,
				},
				nil,
			)

			h.alertmanagers = make(map[string]*alertmanagerSet)

			am1Cfg := config.DefaultAlertmanagerConfig
			am1Cfg.Timeout = model.Duration(200 * time.Millisecond)

			h.alertmanagers["config-0"] = &alertmanagerSet{
				ams:     []alertmanager{},
				cfg:     &am1Cfg,
				metrics: h.metrics,
			}
			go h.Run(syncCh)
			defer h.Stop()

			var alerts []*Alert
			for i := range make([]struct{}, 20*maxBatchSize) {
				alerts = append(alerts, &Alert{
					Labels: labels.FromStrings("alertname", fmt.Sprintf("%d", i)),
				})
			}

			// Injecting the hanging server URL.
			syncCh <- map[string][]*targetgroup.Group{
				"config-0": {
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: model.LabelValue(badAddress),
							},
						},
					},
				},
			}

			// Queing alerts.
			h.Send(alerts...)

			// Updating with a working alertmanager target.
			go func() {
				select {
				case syncCh <- map[string][]*targetgroup.Group{
					"config-0": {
						{
							Targets: []model.LabelSet{
								{
									model.AddressLabel: model.LabelValue(goodAddress),
								},
							},
						},
					},
				}:
				case <-done:
				}
			}()

			select {
			case <-time.After(1 * time.Second):
				t.Fatalf("Timeout after 1 second, targets not synced in time.")
			case <-changed:
				// The good server has been hit in less than 3 seconds, therefore
				// targets have been updated before a second call could be made to the
				// bad server.
			}
		})
	}
}
