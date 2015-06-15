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
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
)

type testHTTPPoster struct {
	message      string
	receivedPost chan<- bool
}

func (p *testHTTPPoster) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
	var buf bytes.Buffer
	buf.ReadFrom(body)
	p.message = buf.String()
	p.receivedPost <- true
	return &http.Response{
		Body: ioutil.NopCloser(&bytes.Buffer{}),
	}, nil
}

type testNotificationScenario struct {
	description string
	summary     string
	message     string
}

func (s *testNotificationScenario) test(i int, t *testing.T) {
	h := NewNotificationHandler(&NotificationHandlerOptions{
		AlertmanagerURL: "alertmanager_url",
		QueueCapacity:   0,
		Deadline:        10 * time.Second,
	})
	defer h.Stop()

	receivedPost := make(chan bool, 1)
	poster := testHTTPPoster{receivedPost: receivedPost}
	h.httpClient = &poster

	go h.Run()

	h.SubmitReqs(NotificationReqs{
		{
			Summary:     s.summary,
			Description: s.description,
			Labels: clientmodel.LabelSet{
				clientmodel.LabelName("instance"): clientmodel.LabelValue("testinstance"),
			},
			Value:        clientmodel.SampleValue(1.0 / 3.0),
			ActiveSince:  time.Time{},
			RuleString:   "Test rule string",
			GeneratorURL: "prometheus_url",
		},
	})

	<-receivedPost
	if poster.message != s.message {
		t.Fatalf("%d. Expected '%s', received '%s'", i, s.message, poster.message)
	}
}

func TestNotificationHandler(t *testing.T) {
	scenarios := []testNotificationScenario{
		{
			// Correct message.
			summary:     "Summary",
			description: "Description",
			message:     `[{"description":"Description","labels":{"instance":"testinstance"},"payload":{"activeSince":"0001-01-01T00:00:00Z","alertingRule":"Test rule string","generatorURL":"prometheus_url","value":"0.3333333333333333"},"summary":"Summary"}]`,
		},
	}

	for i, s := range scenarios {
		s.test(i, t)
	}
}
