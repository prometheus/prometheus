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
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

func TestPostURL(t *testing.T) {
	var cases = []struct {
		in, out string
	}{
		{
			in:  "http://localhost:9093",
			out: "http://localhost:9093/api/v1/alerts",
		},
		{
			in:  "http://localhost:9093/",
			out: "http://localhost:9093/api/v1/alerts",
		},
		{
			in:  "http://localhost:9093/prefix",
			out: "http://localhost:9093/prefix/api/v1/alerts",
		},
		{
			in:  "http://localhost:9093/prefix//",
			out: "http://localhost:9093/prefix/api/v1/alerts",
		},
		{
			in:  "http://localhost:9093/prefix//",
			out: "http://localhost:9093/prefix/api/v1/alerts",
		},
	}
	for _, c := range cases {
		if res := postURL(c.in); res != c.out {
			t.Errorf("Expected post URL %q for %q but got %q", c.out, c.in, res)
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
		if r.URL.Path != alertPushEndpoint {
			t.Fatalf("Bad endpoint %q used, expected %q", r.URL.Path, alertPushEndpoint)
		}
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

	h := New(&Options{
		AlertmanagerURLs: []string{server1.URL, server2.URL},
		Timeout:          time.Minute,
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
	if ne := h.sendAll(h.queue...); ne != 0 {
		t.Fatalf("Unexpected number of failed sends: %d", ne)
	}

	status1 = http.StatusNotFound
	if ne := h.sendAll(h.queue...); ne != 1 {
		t.Fatalf("Unexpected number of failed sends: %d", ne)
	}

	status2 = http.StatusInternalServerError
	if ne := h.sendAll(h.queue...); ne != 2 {
		t.Fatalf("Unexpected number of failed sends: %d", ne)
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
		AlertmanagerURLs: []string{server.URL},
		Timeout:          time.Second,
		QueueCapacity:    3 * maxBatchSize,
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
