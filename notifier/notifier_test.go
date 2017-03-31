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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
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
	h := New(&Options{})

	for i := range make([]struct{}, 2*maxBatchSize+1) {
		h.queue = append(h.queue, &model.Alert{
			Labels: model.LabelSet{
				"alertname": model.LabelValue(fmt.Sprintf("%d", i)),
			},
		})
	}

	expected := append(model.Alerts{}, h.queue...)

	b := h.nextBatch()

	if len(b) != maxBatchSize {
		t.Fatalf("Expected first batch of length %d, but got %d", maxBatchSize, len(b))
	}
	if reflect.DeepEqual(expected[0:maxBatchSize], b) {
		t.Fatalf("First batch did not match")
	}

	b = h.nextBatch()

	if len(b) != maxBatchSize {
		t.Fatalf("Expected second batch of length %d, but got %d", maxBatchSize, len(b))
	}
	if reflect.DeepEqual(expected[maxBatchSize:2*maxBatchSize], b) {
		t.Fatalf("Second batch did not match")
	}

	b = h.nextBatch()

	if len(b) != 1 {
		t.Fatalf("Expected third batch of length %d, but got %d", 1, len(b))
	}
	if reflect.DeepEqual(expected[2*maxBatchSize:], b) {
		t.Fatalf("Third batch did not match")
	}

	if len(h.queue) != 0 {
		t.Fatalf("Expected queue to be empty but got %d alerts", len(h.queue))
	}
}

func alertsEqual(a, b model.Alerts) bool {
	if len(a) != len(b) {
		return false
	}
	for i, alert := range a {
		if !alert.Labels.Equal(b[i].Labels) {
			return false
		}
	}
	return true
}

func TestHandlerSendAll(t *testing.T) {
	var (
		expected         model.Alerts
		status1, status2 int
	)

	f := func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var alerts model.Alerts
		if err := json.NewDecoder(r.Body).Decode(&alerts); err != nil {
			t.Fatalf("Unexpected error on input decoding: %s", err)
		}

		if !alertsEqual(alerts, expected) {
			t.Errorf("%#v %#v", *alerts[0], *expected[0])
			t.Fatalf("Unexpected alerts received %v exp %v", alerts, expected)
		}
	}
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f(w, r)
		w.WriteHeader(status1)
	}))
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f(w, r)
		w.WriteHeader(status2)
	}))

	defer server1.Close()
	defer server2.Close()

	h := New(&Options{})
	h.alertmanagers = append(h.alertmanagers, &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server1.URL },
			},
			alertmanagerMock{
				urlf: func() string { return server2.URL },
			},
		},
		cfg: &config.AlertmanagerConfig{
			Timeout: time.Second,
		},
	})

	for i := range make([]struct{}, maxBatchSize) {
		h.queue = append(h.queue, &model.Alert{
			Labels: model.LabelSet{
				"alertname": model.LabelValue(fmt.Sprintf("%d", i)),
			},
		})
		expected = append(expected, &model.Alert{
			Labels: model.LabelSet{
				"alertname": model.LabelValue(fmt.Sprintf("%d", i)),
			},
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
	h := New(&Options{
		Do: func(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
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
	})

	h.sendOne(context.Background(), nil, testURL, []byte(testBody))

	if !received {
		t.Fatal("Expected to receive an alert, but didn't")
	}
}

func TestExternalLabels(t *testing.T) {
	h := New(&Options{
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
	})

	// This alert should get the external label attached.
	h.Send(&model.Alert{
		Labels: model.LabelSet{
			"alertname": "test",
		},
	})

	// This alert should get the external label attached, but then set to "c"
	// through relabelling.
	h.Send(&model.Alert{
		Labels: model.LabelSet{
			"alertname": "externalrelabelthis",
		},
	})

	expected := []*model.Alert{
		{
			Labels: model.LabelSet{
				"alertname": "test",
				"a":         "b",
			},
		},
		{
			Labels: model.LabelSet{
				"alertname": "externalrelabelthis",
				"a":         "c",
			},
		},
	}

	if !alertsEqual(expected, h.queue) {
		t.Errorf("Expected alerts %v, got %v", expected, h.queue)
	}
}

func TestHandlerRelabel(t *testing.T) {
	h := New(&Options{
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
	})

	// This alert should be dropped due to the configuration
	h.Send(&model.Alert{
		Labels: model.LabelSet{
			"alertname": "drop",
		},
	})

	// This alert should be replaced due to the configuration
	h.Send(&model.Alert{
		Labels: model.LabelSet{
			"alertname": "rename",
		},
	})

	expected := []*model.Alert{
		{
			Labels: model.LabelSet{
				"alertname": "renamed",
			},
		},
	}

	if !alertsEqual(expected, h.queue) {
		t.Errorf("Expected alerts %v, got %v", expected, h.queue)
	}
}

func TestHandlerQueueing(t *testing.T) {
	var (
		unblock  = make(chan struct{})
		called   = make(chan struct{})
		expected model.Alerts
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- struct{}{}
		<-unblock

		defer r.Body.Close()

		var alerts model.Alerts
		if err := json.NewDecoder(r.Body).Decode(&alerts); err != nil {
			t.Fatalf("Unexpected error on input decoding: %s", err)
		}

		if !alertsEqual(expected, alerts) {
			t.Errorf("Expected alerts %v, got %v", expected, alerts)
		}
	}))

	h := New(&Options{
		QueueCapacity: 3 * maxBatchSize,
	})
	h.alertmanagers = append(h.alertmanagers, &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server.URL },
			},
		},
		cfg: &config.AlertmanagerConfig{
			Timeout: time.Second,
		},
	})

	var alerts model.Alerts
	for i := range make([]struct{}, 20*maxBatchSize) {
		alerts = append(alerts, &model.Alert{
			Labels: model.LabelSet{
				"alertname": model.LabelValue(fmt.Sprintf("%d", i)),
			},
		})
	}

	go h.Run()
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

func (a alertmanagerMock) url() string {
	return a.urlf()
}
