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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	_ "github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

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

func newTestHTTPServerBuilder(expected *[]*Alert, errc chan<- error, u, p string, status *atomic.Int32) *httptest.Server {
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
			err = alertsEqual(*expected, alerts)
		}
		w.WriteHeader(int(status.Load()))
	}))
}

func TestHandlerSendAll(t *testing.T) {
	var (
		errc                      = make(chan error, 1)
		expected                  = make([]*Alert, 0)
		status1, status2, status3 atomic.Int32
	)
	status1.Store(int32(http.StatusOK))
	status2.Store(int32(http.StatusOK))
	status3.Store(int32(http.StatusOK))

	server1 := newTestHTTPServerBuilder(&expected, errc, "prometheus", "testing_password", &status1)
	server2 := newTestHTTPServerBuilder(&expected, errc, "", "", &status2)
	server3 := newTestHTTPServerBuilder(&expected, errc, "", "", &status3)
	defer server1.Close()
	defer server2.Close()
	defer server3.Close()

	h := NewManager(&Options{}, model.UTF8Validation, nil)

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

	am3Cfg := config.DefaultAlertmanagerConfig
	am3Cfg.Timeout = model.Duration(time.Second)

	opts := &Options{Do: do, QueueCapacity: 10_000, MaxBatchSize: DefaultMaxBatchSize}
	logger := slog.New(slog.DiscardHandler)

	h.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server1.URL },
			},
		},
		cfg:    &am1Cfg,
		client: authClient,
		sendLoops: map[string]*sendLoop{
			server1.URL: newSendLoop(server1.URL, authClient, &am1Cfg, opts, logger, h.metrics),
		},
		opts:    opts,
		metrics: h.metrics,
		logger:  logger,
	}

	h.alertmanagers["2"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server2.URL },
			},
			alertmanagerMock{
				urlf: func() string { return server3.URL },
			},
		},
		cfg: &am2Cfg,
		sendLoops: map[string]*sendLoop{
			server2.URL: newSendLoop(server2.URL, nil, &am2Cfg, opts, logger, h.metrics),
			server3.URL: newSendLoop(server3.URL, nil, &am3Cfg, opts, logger, h.metrics),
		},
		opts:    opts,
		metrics: h.metrics,
		logger:  logger,
	}

	h.alertmanagers["3"] = &alertmanagerSet{
		ams: []alertmanager{}, // empty set
		cfg: &am3Cfg,
		sendLoops: map[string]*sendLoop{
			server3.URL: newSendLoop(server3.URL, nil, &am3Cfg, opts, logger, h.metrics),
		},
		opts:    opts,
		metrics: h.metrics,
		logger:  logger,
	}

	var alerts []*Alert
	for i := range DefaultMaxBatchSize {
		alerts = append(alerts, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
		expected = append(expected, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
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

	// start send loops
	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}
	defer func() {
		for _, ams := range h.alertmanagers {
			ams.cleanSendLoops(ams.ams...)
		}
	}()

	h.Send(alerts...)
	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.sent.WithLabelValues(server1.URL)) == DefaultMaxBatchSize
	}, time.Second*2, time.Millisecond*10)
	checkNoErr()

	// the only am in set 1 is down
	status1.Store(int32(http.StatusNotFound))
	h.Send(alerts...)
	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.errors.WithLabelValues(server1.URL)) == DefaultMaxBatchSize
	}, time.Second*2, time.Millisecond*10)
	checkNoErr()

	// fix the am
	status1.Store(int32(http.StatusOK))

	// only one of the ams in set 2 is down
	status2.Store(int32(http.StatusInternalServerError))
	h.Send(alerts...)
	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.errors.WithLabelValues(server2.URL)) == DefaultMaxBatchSize
	}, time.Second*2, time.Millisecond*10)
	checkNoErr()

	// both ams in set 2 are down
	status3.Store(int32(http.StatusInternalServerError))
	h.Send(alerts...)
	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.errors.WithLabelValues(server2.URL)) == DefaultMaxBatchSize*2 &&
			prom_testutil.ToFloat64(h.metrics.errors.WithLabelValues(server3.URL)) == DefaultMaxBatchSize
	}, time.Second*3, time.Millisecond*10)
	checkNoErr()
}

func TestHandlerSendAllRemapPerAm(t *testing.T) {
	var (
		errc      = make(chan error, 1)
		expected1 = make([]*Alert, 0)
		expected2 = make([]*Alert, 0)
		expected3 = make([]*Alert, 0)

		status1, status2, status3 atomic.Int32
	)
	status1.Store(int32(http.StatusOK))
	status2.Store(int32(http.StatusOK))
	status3.Store(int32(http.StatusOK))

	server1 := newTestHTTPServerBuilder(&expected1, errc, "", "", &status1)
	server2 := newTestHTTPServerBuilder(&expected2, errc, "", "", &status2)
	server3 := newTestHTTPServerBuilder(&expected3, errc, "", "", &status3)

	defer server1.Close()
	defer server2.Close()
	defer server3.Close()

	h := NewManager(&Options{}, model.UTF8Validation, nil)
	h.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	am2Cfg := config.DefaultAlertmanagerConfig
	am2Cfg.Timeout = model.Duration(time.Second)
	am2Cfg.AlertRelabelConfigs = []*relabel.Config{
		{
			SourceLabels:         model.LabelNames{"alertnamedrop"},
			Action:               "drop",
			Regex:                relabel.MustNewRegexp(".+"),
			NameValidationScheme: model.UTF8Validation,
		},
	}

	am3Cfg := config.DefaultAlertmanagerConfig
	am3Cfg.Timeout = model.Duration(time.Second)
	am3Cfg.AlertRelabelConfigs = []*relabel.Config{
		{
			SourceLabels:         model.LabelNames{"alertname"},
			Action:               "drop",
			Regex:                relabel.MustNewRegexp(".+"),
			NameValidationScheme: model.UTF8Validation,
		},
	}

	opts := &Options{Do: do, QueueCapacity: 10_000, MaxBatchSize: DefaultMaxBatchSize}
	logger := slog.New(slog.DiscardHandler)

	h.alertmanagers = map[string]*alertmanagerSet{
		// Drop no alerts.
		"1": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server1.URL },
				},
			},
			cfg: &am1Cfg,
			sendLoops: map[string]*sendLoop{
				server1.URL: newSendLoop(server1.URL, nil, &am1Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			metrics: h.metrics,
			logger:  logger,
		},
		// Drop only alerts with the "alertnamedrop" label.
		"2": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server2.URL },
				},
			},
			cfg: &am2Cfg,
			sendLoops: map[string]*sendLoop{
				server2.URL: newSendLoop(server2.URL, nil, &am2Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			metrics: h.metrics,
			logger:  logger,
		},
		// Drop all alerts.
		"3": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server3.URL },
				},
			},
			cfg: &am3Cfg,
			sendLoops: map[string]*sendLoop{
				server3.URL: newSendLoop(server3.URL, nil, &am3Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			metrics: h.metrics,
			logger:  logger,
		},
		// Empty list of Alertmanager endpoints.
		"4": {
			ams:       []alertmanager{},
			cfg:       &config.DefaultAlertmanagerConfig,
			sendLoops: make(map[string]*sendLoop),
			opts:      opts,
			metrics:   h.metrics,
			logger:    logger,
		},
	}

	var alerts []*Alert
	for i := range make([]struct{}, DefaultMaxBatchSize/2) {
		alerts = append(alerts,
			&Alert{
				Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
			},
			&Alert{
				Labels: labels.FromStrings("alertname", "test", "alertnamedrop", strconv.Itoa(i)),
			},
		)

		expected1 = append(expected1,
			&Alert{
				Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
			}, &Alert{
				Labels: labels.FromStrings("alertname", "test", "alertnamedrop", strconv.Itoa(i)),
			},
		)

		expected2 = append(expected2, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
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

	// start send loops
	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}
	defer func() {
		// stop send loops.
		for _, ams := range h.alertmanagers {
			ams.cleanSendLoops(ams.ams...)
		}
	}()

	// all ams are up
	h.Send(alerts...)
	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.sent.WithLabelValues(server1.URL)) == DefaultMaxBatchSize
	}, time.Second*2, time.Millisecond*10)
	checkNoErr()

	// the only am in set 1 goes down
	status1.Store(int32(http.StatusInternalServerError))
	h.Send(alerts...)
	// wait for metrics to update
	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.errors.WithLabelValues(server1.URL)) == DefaultMaxBatchSize
	}, time.Second*2, time.Millisecond*10)
	checkNoErr()

	// reset set 1
	status1.Store(int32(http.StatusOK))

	// set 3 loses its only am, but all alerts were dropped
	// so there was nothing to send, keeping sendAll true
	status3.Store(int32(http.StatusInternalServerError))
	h.Send(alerts...)
	checkNoErr()
}

func TestExternalLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	h := NewManager(&Options{
		QueueCapacity:  3 * DefaultMaxBatchSize,
		MaxBatchSize:   DefaultMaxBatchSize,
		ExternalLabels: labels.FromStrings("a", "b"),
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels:         model.LabelNames{"alertname"},
				TargetLabel:          "a",
				Action:               "replace",
				Regex:                relabel.MustNewRegexp("externalrelabelthis"),
				Replacement:          "c",
				NameValidationScheme: model.UTF8Validation,
			},
		},
		Registerer: reg,
	}, model.UTF8Validation, nil)

	cfg := config.DefaultAlertmanagerConfig
	h.alertmanagers = map[string]*alertmanagerSet{
		"test": {
			cfg: &cfg,
			sendLoops: map[string]*sendLoop{
				"test": newSendLoop("test", nil, &cfg, h.opts, slog.New(slog.DiscardHandler), h.metrics),
			},
		},
	}

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

	require.NoError(t, alertsEqual(expected, h.alertmanagers["test"].sendLoops["test"].queue))
}

func TestHandlerRelabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	h := NewManager(&Options{
		QueueCapacity: 3 * DefaultMaxBatchSize,
		MaxBatchSize:  DefaultMaxBatchSize,
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels:         model.LabelNames{"alertname"},
				Action:               "drop",
				Regex:                relabel.MustNewRegexp("drop"),
				NameValidationScheme: model.UTF8Validation,
			},
			{
				SourceLabels:         model.LabelNames{"alertname"},
				TargetLabel:          "alertname",
				Action:               "replace",
				Regex:                relabel.MustNewRegexp("rename"),
				Replacement:          "renamed",
				NameValidationScheme: model.UTF8Validation,
			},
		},
		Registerer: reg,
	}, model.UTF8Validation, nil)

	cfg := config.DefaultAlertmanagerConfig
	h.alertmanagers = map[string]*alertmanagerSet{
		"test": {
			cfg: &cfg,
			sendLoops: map[string]*sendLoop{
				"test": newSendLoop("test", nil, &cfg, h.opts, slog.New(slog.DiscardHandler), h.metrics),
			},
		},
	}

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

	require.NoError(t, alertsEqual(expected, h.alertmanagers["test"].sendLoops["test"].queue))
}

func TestHandlerQueuing(t *testing.T) {
	var (
		expectedc = make(chan []*Alert)
		called    = make(chan struct{})
		done      = make(chan struct{})
		errc      = make(chan error, 1)
	)

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
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

	reg := prometheus.NewRegistry()
	h := NewManager(
		&Options{
			QueueCapacity: 3 * DefaultMaxBatchSize,
			MaxBatchSize:  DefaultMaxBatchSize,
			Registerer:    reg,
		},
		model.UTF8Validation,
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
		sendLoops: map[string]*sendLoop{
			server.URL: newSendLoop(server.URL, nil, &am1Cfg, h.opts, slog.New(slog.DiscardHandler), h.metrics),
		},
	}

	go h.Run(nil)
	defer h.Stop()

	// start send loops
	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	var alerts []*Alert
	for i := range make([]struct{}, 20*DefaultMaxBatchSize) {
		alerts = append(alerts, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
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
	h.Send(alerts[:4*DefaultMaxBatchSize]...)
	for i := 1; i < 4; i++ {
		assertAlerts(alerts[i*DefaultMaxBatchSize : (i+1)*DefaultMaxBatchSize])
	}

	// Send one batch, wait for it to arrive and block the server so the queue fills up.
	h.Send(alerts[:DefaultMaxBatchSize]...)
	<-called

	// Send several batches while the server is still blocked so the queue
	// fills up to its maximum capacity (3*DefaultMaxBatchSize). Then check that the
	// queue is truncated in the front.
	h.Send(alerts[1*DefaultMaxBatchSize : 2*DefaultMaxBatchSize]...) // this batch should be dropped.
	h.Send(alerts[2*DefaultMaxBatchSize : 3*DefaultMaxBatchSize]...)
	h.Send(alerts[3*DefaultMaxBatchSize : 4*DefaultMaxBatchSize]...)

	// Send the batch that drops the first one.
	h.Send(alerts[4*DefaultMaxBatchSize : 5*DefaultMaxBatchSize]...)

	// Unblock the server.
	expectedc <- alerts[:DefaultMaxBatchSize]
	select {
	case err := <-errc:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Alerts were not pushed.")
	}

	// Verify that we receive the last 3 batches.
	for i := 2; i < 5; i++ {
		assertAlerts(alerts[i*DefaultMaxBatchSize : (i+1)*DefaultMaxBatchSize])
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

	n := NewManager(&Options{}, model.UTF8Validation, nil)

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

	n := NewManager(&Options{}, model.UTF8Validation, nil)

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

// TestHangingNotifier ensures that the notifier takes into account SD changes even when there are
// queued alerts. This test reproduces the issue described in https://github.com/prometheus/prometheus/issues/13676.
// and https://github.com/prometheus/prometheus/issues/8768.
func TestHangingNotifier(t *testing.T) {
	const (
		batches     = 100
		alertsCount = DefaultMaxBatchSize * batches
	)

	var (
		sendTimeout = 100 * time.Millisecond
		sdUpdatert  = sendTimeout / 2

		done = make(chan struct{})
	)

	// Set up a faulty Alertmanager.
	var faultyCalled atomic.Bool
	faultyServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		faultyCalled.Store(true)
		select {
		case <-done:
		case <-time.After(time.Hour):
		}
	}))
	defer func() {
		close(done)
	}()

	faultyURL, err := url.Parse(faultyServer.URL)
	require.NoError(t, err)
	faultyURL.Path = "/api/v2/alerts"

	// Set up a functional Alertmanager.
	var functionalCalled atomic.Bool
	functionalServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		functionalCalled.Store(true)
	}))
	defer functionalServer.Close()
	functionalURL, err := url.Parse(functionalServer.URL)
	require.NoError(t, err)
	functionalURL.Path = "/api/v2/alerts"

	// Initialize the discovery manager
	// This is relevant as the updates aren't sent continually in real life, but only each updatert.
	// The old implementation of TestHangingNotifier didn't take that into account.
	ctx, cancelSdManager := context.WithCancel(t.Context())
	defer cancelSdManager()
	reg := prometheus.NewRegistry()
	sdMetrics, err := discovery.RegisterSDMetrics(reg, discovery.NewRefreshMetrics(reg))
	require.NoError(t, err)
	sdManager := discovery.NewManager(
		ctx,
		promslog.NewNopLogger(),
		reg,
		sdMetrics,
		discovery.Name("sd-manager"),
		discovery.Updatert(sdUpdatert),
	)
	go sdManager.Run()

	// Set up the notifier with both faulty and functional Alertmanagers.
	notifier := NewManager(
		&Options{
			QueueCapacity: alertsCount,
			Registerer:    reg,
		},
		model.UTF8Validation,
		nil,
	)
	notifier.alertmanagers = make(map[string]*alertmanagerSet)
	amCfg := config.DefaultAlertmanagerConfig
	amCfg.Timeout = model.Duration(sendTimeout)
	notifier.alertmanagers["config-0"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return faultyURL.String() },
			},
			alertmanagerMock{
				urlf: func() string { return functionalURL.String() },
			},
		},
		cfg:     &amCfg,
		metrics: notifier.metrics,
		sendLoops: map[string]*sendLoop{
			faultyURL.String():     newSendLoop(faultyURL.String(), nil, &amCfg, notifier.opts, slog.New(slog.DiscardHandler), notifier.metrics),
			functionalURL.String(): newSendLoop(functionalURL.String(), nil, &amCfg, notifier.opts, slog.New(slog.DiscardHandler), notifier.metrics),
		},
		opts:   &Options{Do: do, MaxBatchSize: DefaultMaxBatchSize},
		logger: slog.New(slog.DiscardHandler),
	}

	for _, ams := range notifier.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	go notifier.Run(sdManager.SyncCh())
	defer notifier.Stop()

	require.Len(t, notifier.Alertmanagers(), 2)

	// Enqueue the alerts.
	var alerts []*Alert
	for i := range make([]struct{}, alertsCount) {
		alerts = append(alerts, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
	}
	notifier.Send(alerts...)

	// Wait for the Alertmanagers to start receiving alerts.
	// 10*sdUpdatert is used as an arbitrary timeout here.
	timeout := time.After(10 * sdUpdatert)
loop1:
	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for the alertmanagers to be reached for the first time.")
		default:
			if faultyCalled.Load() && functionalCalled.Load() {
				break loop1
			}
		}
	}

	// Request to remove the faulty Alertmanager.
	c := map[string]discovery.Configs{
		"config-0": {
			discovery.StaticConfig{
				&targetgroup.Group{
					Targets: []model.LabelSet{
						{
							model.AddressLabel: model.LabelValue(functionalURL.Host),
						},
					},
				},
			},
		},
	}
	require.NoError(t, sdManager.ApplyConfig(c))

	timeout = time.After(batches * sendTimeout)
loop2:
	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout, the faulty alertmanager not removed on time.")
		default:
			// The faulty alertmanager was dropped.
			if len(notifier.Alertmanagers()) == 1 {
				// The notifier should not wait until the alerts queue of the functional am is empty to apply the discovery changes.
				require.NotEmpty(t, notifier.alertmanagers["config-0"].sendLoops[functionalURL.String()].queue)
				break loop2
			}
		}
	}
}

func TestStop_DrainingDisabled(t *testing.T) {
	releaseReceiver := make(chan struct{})
	receiverReceivedRequest := make(chan struct{}, 2)
	alertsReceived := atomic.NewInt64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let the test know we've received a request.
		receiverReceivedRequest <- struct{}{}

		var alerts []*Alert

		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		err = json.Unmarshal(b, &alerts)
		require.NoError(t, err)

		alertsReceived.Add(int64(len(alerts)))

		// Wait for the test to release us.
		<-releaseReceiver

		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		server.Close()
	}()

	reg := prometheus.NewRegistry()
	m := NewManager(
		&Options{
			QueueCapacity:   10,
			DrainOnShutdown: false,
			Registerer:      reg,
		},
		model.UTF8Validation,
		nil,
	)

	m.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	m.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server.URL },
			},
		},
		cfg: &am1Cfg,
		sendLoops: map[string]*sendLoop{
			server.URL: newSendLoop(server.URL, nil, &am1Cfg, m.opts, slog.New(slog.DiscardHandler), m.metrics),
		},
		opts:   &Options{Do: do, MaxBatchSize: DefaultMaxBatchSize},
		logger: slog.New(slog.DiscardHandler),
	}

	for _, ams := range m.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	notificationManagerStopped := make(chan struct{})

	go func() {
		defer close(notificationManagerStopped)
		m.Run(nil)
	}()

	// Queue two alerts. The first should be immediately sent to the receiver, which should block until we release it later.
	m.Send(&Alert{Labels: labels.FromStrings(labels.AlertName, "alert-1")})

	select {
	case <-receiverReceivedRequest:
		// Nothing more to do.
	case <-time.After(time.Second):
		require.FailNow(t, "gave up waiting for receiver to receive notification of first alert")
	}

	m.Send(&Alert{Labels: labels.FromStrings(labels.AlertName, "alert-2")})

	// Stop the notification manager, pause to allow the shutdown to be observed, and then allow the receiver to proceed.
	m.Stop()
	time.Sleep(time.Second)
	close(releaseReceiver)

	// Wait for the notification manager to stop and confirm only the first notification was sent.
	// The second notification should be dropped.
	select {
	case <-notificationManagerStopped:
		// Nothing more to do.
	case <-time.After(time.Second):
		require.FailNow(t, "gave up waiting for notification manager to stop")
	}

	require.Equal(t, int64(1), alertsReceived.Load())
}

func TestStop_DrainingEnabled(t *testing.T) {
	releaseReceiver := make(chan struct{})
	receiverReceivedRequest := make(chan struct{}, 2)
	alertsReceived := atomic.NewInt64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var alerts []*Alert

		// Let the test know we've received a request.
		receiverReceivedRequest <- struct{}{}

		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		err = json.Unmarshal(b, &alerts)
		require.NoError(t, err)

		alertsReceived.Add(int64(len(alerts)))

		// Wait for the test to release us.
		<-releaseReceiver

		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		server.Close()
	}()

	reg := prometheus.NewRegistry()
	m := NewManager(
		&Options{
			QueueCapacity:   10,
			DrainOnShutdown: true,
			Registerer:      reg,
		},
		model.UTF8Validation,
		nil,
	)

	m.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	m.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server.URL },
			},
		},
		cfg: &am1Cfg,
		sendLoops: map[string]*sendLoop{
			server.URL: newSendLoop(server.URL, nil, &am1Cfg, m.opts, slog.New(slog.DiscardHandler), m.metrics),
		},
		opts:    &Options{Do: do, MaxBatchSize: DefaultMaxBatchSize},
		metrics: m.metrics,
		logger:  slog.New(slog.DiscardHandler),
	}

	for _, ams := range m.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	notificationManagerStopped := make(chan struct{})

	go func() {
		defer close(notificationManagerStopped)
		m.Run(nil)
	}()

	// Queue two alerts. The first should be immediately sent to the receiver, which should block until we release it later.
	m.Send(&Alert{Labels: labels.FromStrings(labels.AlertName, "alert-1")})

	select {
	case <-receiverReceivedRequest:
		// Nothing more to do.
	case <-time.After(time.Second):
		require.FailNow(t, "gave up waiting for receiver to receive notification of first alert")
	}

	m.Send(&Alert{Labels: labels.FromStrings(labels.AlertName, "alert-2")})

	// Stop the notification manager and allow the receiver to proceed.
	m.Stop()
	close(releaseReceiver)

	// Wait for the notification manager to stop and confirm both notifications were sent.
	select {
	case <-notificationManagerStopped:
		// Nothing more to do.
	case <-time.After(200 * time.Millisecond):
		require.FailNow(t, "gave up waiting for notification manager to stop")
	}

	require.Equal(t, int64(2), alertsReceived.Load())
}

// TestQueuesDrainingOnApplyConfig ensures that when an alertmanagerSet disappears after an ApplyConfig(), its
// sendLoops queues are drained only when DrainOnShutdown is set.
func TestQueuesDrainingOnApplyConfig(t *testing.T) {
	for _, drainOnShutDown := range []bool{false, true} {
		t.Run(strconv.FormatBool(drainOnShutDown), func(t *testing.T) {
			t.Parallel()
			alertSent := make(chan struct{})

			server := newImmediateAlertManager(alertSent)
			defer server.Close()

			h := NewManager(&Options{}, model.UTF8Validation, nil)
			h.alertmanagers = make(map[string]*alertmanagerSet)

			amCfg := config.DefaultAlertmanagerConfig
			amCfg.Timeout = model.Duration(time.Second)

			opts := &Options{Do: do, QueueCapacity: 10, MaxBatchSize: DefaultMaxBatchSize, DrainOnShutdown: drainOnShutDown}
			logger := slog.New(slog.DiscardHandler)

			h.alertmanagers["1"] = &alertmanagerSet{
				ams: []alertmanager{
					alertmanagerMock{
						urlf: func() string { return server.URL },
					},
				},
				cfg: &amCfg,
				sendLoops: map[string]*sendLoop{
					server.URL: newSendLoop(server.URL, nil, &amCfg, opts, logger, h.metrics),
				},
				opts:    opts,
				metrics: h.metrics,
				logger:  logger,
			}

			// The send loops were not started, nothing will be sent.
			h.Send([]*Alert{{Labels: labels.FromStrings("alertname", "foo")}}...)

			// Remove the alertmanagerSet.
			h.ApplyConfig(&config.Config{})

			select {
			case <-alertSent:
				if !drainOnShutDown {
					require.FailNow(t, "no alert should be sent")
				}
			case <-time.After(100 * time.Millisecond):
				if drainOnShutDown {
					require.FailNow(t, "alert wasn't received")
				}
			}
		})
	}
}

func TestApplyConfig(t *testing.T) {
	targetURL := "alertmanager:9093"
	targetGroup := &targetgroup.Group{
		Targets: []model.LabelSet{
			{
				"__address__": model.LabelValue(targetURL),
			},
		},
	}
	alertmanagerURL := fmt.Sprintf("http://%s/api/v2/alerts", targetURL)

	n := NewManager(&Options{}, model.UTF8Validation, nil)
	cfg := &config.Config{}
	s := `
alerting:
  alertmanagers:
  - file_sd_configs:
    - files:
      - foo.json
`
	// 1. Ensure known alertmanagers are not dropped during ApplyConfig.
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 1)

	// First, apply the config and reload.
	require.NoError(t, n.ApplyConfig(cfg))
	tgs := map[string][]*targetgroup.Group{"config-0": {targetGroup}}
	n.reload(tgs)
	require.Len(t, n.Alertmanagers(), 1)
	require.Equal(t, alertmanagerURL, n.Alertmanagers()[0].String())

	// Reapply the config.
	require.NoError(t, n.ApplyConfig(cfg))
	// Ensure the known alertmanagers are not dropped.
	require.Len(t, n.Alertmanagers(), 1)
	require.Equal(t, alertmanagerURL, n.Alertmanagers()[0].String())

	// 2. Ensure known alertmanagers are not dropped during ApplyConfig even when
	// the config order changes.
	s = `
alerting:
  alertmanagers:
  - static_configs:
  - file_sd_configs:
    - files:
      - foo.json
`
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 2)

	require.NoError(t, n.ApplyConfig(cfg))
	require.Len(t, n.Alertmanagers(), 1)
	// Ensure no unnecessary alertmanagers are injected.
	require.Empty(t, n.alertmanagers["config-0"].ams)
	// Ensure the config order is taken into account.
	ams := n.alertmanagers["config-1"].ams
	require.Len(t, ams, 1)
	require.Equal(t, alertmanagerURL, ams[0].url().String())

	// 3. Ensure known alertmanagers are reused for new config with identical AlertmanagerConfig.
	s = `
alerting:
  alertmanagers:
  - file_sd_configs:
    - files:
      - foo.json
  - file_sd_configs:
    - files:
      - foo.json
`
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 2)

	require.NoError(t, n.ApplyConfig(cfg))
	require.Len(t, n.Alertmanagers(), 2)
	for cfgIdx := range 2 {
		ams := n.alertmanagers[fmt.Sprintf("config-%d", cfgIdx)].ams
		require.Len(t, ams, 1)
		require.Equal(t, alertmanagerURL, ams[0].url().String())
	}

	// 4. Ensure known alertmanagers are reused only for identical AlertmanagerConfig.
	s = `
alerting:
  alertmanagers:
  - file_sd_configs:
    - files:
      - foo.json
    path_prefix: /bar
  - file_sd_configs:
    - files:
      - foo.json
    relabel_configs:
    - source_labels: ['__address__']
      regex: 'doesntmatter:1234'
      action: drop
`
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 2)

	require.NoError(t, n.ApplyConfig(cfg))
	require.Empty(t, n.Alertmanagers())
}

// TestAlerstRelabelingIsIsolated ensures that a mutation alerts relabeling in an
// alertmanagerSet doesn't affect others.
// See https://github.com/prometheus/prometheus/pull/17063.
func TestAlerstRelabelingIsIsolated(t *testing.T) {
	var (
		errc      = make(chan error, 1)
		expected1 = make([]*Alert, 0)
		expected2 = make([]*Alert, 0)

		status1, status2 atomic.Int32
	)
	status1.Store(int32(http.StatusOK))
	status2.Store(int32(http.StatusOK))

	server1 := newTestHTTPServerBuilder(&expected1, errc, "", "", &status1)
	server2 := newTestHTTPServerBuilder(&expected2, errc, "", "", &status2)

	defer server1.Close()
	defer server2.Close()

	h := NewManager(&Options{}, model.UTF8Validation, nil)
	h.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)
	am1Cfg.AlertRelabelConfigs = []*relabel.Config{
		{
			SourceLabels:         model.LabelNames{"alertname"},
			Regex:                relabel.MustNewRegexp("(.*)"),
			TargetLabel:          "parasite",
			Action:               relabel.Replace,
			Replacement:          "yes",
			NameValidationScheme: model.UTF8Validation,
		},
	}

	am2Cfg := config.DefaultAlertmanagerConfig
	am2Cfg.Timeout = model.Duration(time.Second)

	h.alertmanagers = map[string]*alertmanagerSet{
		"am1": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server1.URL },
				},
			},
			cfg: &am1Cfg,
			sendLoops: map[string]*sendLoop{
				server1.URL: newSendLoop(server1.URL, nil, &am1Cfg, &Options{}, h.logger, h.metrics),
			},
			opts:    &Options{},
			metrics: h.metrics,
			logger:  h.logger,
		},
		"am2": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server2.URL },
				},
			},
			cfg: &am2Cfg,
			sendLoops: map[string]*sendLoop{
				server2.URL: newSendLoop(server2.URL, nil, &am2Cfg, &Options{}, h.logger, h.metrics),
			},
			opts:    &Options{},
			metrics: h.metrics,
			logger:  h.logger,
		},
	}

	// start send loops
	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}
	defer func() {
		for _, ams := range h.alertmanagers {
			ams.cleanSendLoops(ams.ams...)
		}
	}()

	testAlert := &Alert{
		Labels: labels.FromStrings("alertname", "test"),
	}

	expected1 = append(expected1, &Alert{
		Labels: labels.FromStrings("alertname", "test", "parasite", "yes"),
	})

	// am2 shouldn't get the parasite label.
	expected2 = append(expected2, &Alert{
		Labels: labels.FromStrings("alertname", "test"),
	})

	checkNoErr := func() {
		t.Helper()
		select {
		case err := <-errc:
			require.NoError(t, err)
		default:
		}
	}

	h.Send(testAlert)
	checkNoErr()
}

// Regression test for https://github.com/prometheus/prometheus/issues/7676
// The test creates a black hole alertmanager that never responds to any requests.
// The alertmanager_config.timeout is set to infinite (1 year).
// We check that the notifier does not hang and throughput is not affected.
func TestNotifierQueueIndependentOfFailedAlertmanager(t *testing.T) {
	stopBlackHole := make(chan struct{})
	blackHoleAM := newBlackHoleAlertmanager(stopBlackHole)
	defer func() {
		close(stopBlackHole)
		blackHoleAM.Close()
	}()

	doneAlertReceive := make(chan struct{})
	immediateAM := newImmediateAlertManager(doneAlertReceive)
	defer immediateAM.Close()

	do := func(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
		return client.Do(req.WithContext(ctx))
	}

	reg := prometheus.NewRegistry()
	h := NewManager(&Options{
		Do:            do,
		QueueCapacity: 10,
		MaxBatchSize:  DefaultMaxBatchSize,
		Registerer:    reg,
	}, model.UTF8Validation, nil)

	h.alertmanagers = make(map[string]*alertmanagerSet)

	amCfg := config.DefaultAlertmanagerConfig
	amCfg.Timeout = model.Duration(time.Hour * 24 * 365)

	h.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return blackHoleAM.URL },
			},
		},
		cfg:  &amCfg,
		opts: h.opts,
		sendLoops: map[string]*sendLoop{
			blackHoleAM.URL: newSendLoop(blackHoleAM.URL, http.DefaultClient, &amCfg, h.opts, slog.New(slog.DiscardHandler), h.metrics),
		},
		metrics: h.metrics,
	}

	h.alertmanagers["2"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return immediateAM.URL },
			},
		},
		cfg:  &amCfg,
		opts: h.opts,
		sendLoops: map[string]*sendLoop{
			immediateAM.URL: newSendLoop(immediateAM.URL, http.DefaultClient, &amCfg, h.opts, slog.New(slog.DiscardHandler), h.metrics),
		},
		metrics: h.metrics,
	}

	doneSendAll := make(chan struct{})
	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}
	defer func() {
		for _, ams := range h.alertmanagers {
			ams.cleanSendLoops(ams.ams...)
		}
	}()

	go func() {
		h.Send(&Alert{
			Labels: labels.FromStrings("alertname", "test"),
		})
		close(doneSendAll)
	}()

	select {
	case <-doneAlertReceive:
		// This is the happy case, the alert was received by the immediate alertmanager.
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for alert to be received by immediate alertmanager")
	}

	select {
	case <-doneSendAll:
		// This is the happy case, the sendAll function returned.
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for sendAll to return")
	}
}

func newBlackHoleAlertmanager(stop <-chan struct{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Do nothing, wait to be canceled.
		<-stop
		w.WriteHeader(http.StatusOK)
	}))
}

func newImmediateAlertManager(done chan<- struct{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
}
