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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

func TestHandlerPostURL(t *testing.T) {
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
	h := &Handler{
		opts: &HandlerOptions{},
	}

	for _, c := range cases {
		h.opts.AlertmanagerURL = c.in
		if res := h.postURL(); res != c.out {
			t.Errorf("Expected post URL %q for %q but got %q", c.out, c.in, res)
		}
	}
}

func TestHandlerNextBatch(t *testing.T) {
	h := New(&HandlerOptions{})

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

func TestHandlerSend(t *testing.T) {
	var (
		expected model.Alerts
		status   int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		w.WriteHeader(status)
	}))

	defer server.Close()

	h := New(&HandlerOptions{
		AlertmanagerURL: server.URL,
		Timeout:         time.Minute,
		ExternalLabels:  model.LabelSet{"a": "b"},
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
				"a":         "b",
			},
		})
	}

	status = http.StatusOK

	if err := h.send(h.queue...); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	status = 500

	if err := h.send(h.queue...); err == nil {
		t.Fatalf("Expected error but got none")
	}
}

func TestHandlerFull(t *testing.T) {
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

	h := New(&HandlerOptions{
		AlertmanagerURL: server.URL,
		Timeout:         time.Second,
		QueueCapacity:   3 * maxBatchSize,
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
