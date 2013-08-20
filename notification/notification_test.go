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
	"io"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"
)

type testHttpPoster struct {
	message      string
	receivedPost chan<- bool
}

func (p *testHttpPoster) Post(url string, bodyType string, body io.Reader) (*http.Response, error) {
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
	notifications := make(chan NotificationReqs)
	defer close(notifications)
	h := NewNotificationHandler("alertmanager_url", notifications)

	receivedPost := make(chan bool, 1)
	poster := testHttpPoster{receivedPost: receivedPost}
	h.httpClient = &poster

	go h.Run()

	notifications <- NotificationReqs{
		{
			Summary:     s.summary,
			Description: s.description,
			Labels: clientmodel.LabelSet{
				clientmodel.LabelName("instance"): clientmodel.LabelValue("testinstance"),
			},
			Value:        clientmodel.SampleValue(1.0 / 3.0),
			ActiveSince:  time.Time{},
			RuleString:   "Test rule string",
			GeneratorUrl: "prometheus_url",
		},
	}

	<-receivedPost
	if poster.message != s.message {
		t.Fatalf("%d. Expected '%s', received '%s'", i, s.message, poster.message)
	}
}

func TestNotificationHandler(t *testing.T) {
	scenarios := []testNotificationScenario{
		{
			// Correct message.
			summary:     "{{$labels.instance}} = {{$value}}",
			description: "The alert value for {{$labels.instance}} is {{$value}}",
			message:     `[{"Description":"The alert value for testinstance is 0.3333333333333333","Labels":{"instance":"testinstance"},"Payload":{"ActiveSince":"0001-01-01T00:00:00Z","AlertingRule":"Test rule string","GeneratorUrl":"prometheus_url","Value":"0.333333"},"Summary":"testinstance = 0.3333333333333333"}]`,
		},
		{
			// Bad message referring to unknown label.
			summary:     "{{$labels.badlabel}} = {{$value}}",
			description: "The alert value for {{$labels.badlabel}} is {{$value}}",
			message:     `[{"Description":"The alert value for \u003cno value\u003e is 0.3333333333333333","Labels":{"instance":"testinstance"},"Payload":{"ActiveSince":"0001-01-01T00:00:00Z","AlertingRule":"Test rule string","GeneratorUrl":"prometheus_url","Value":"0.333333"},"Summary":"\u003cno value\u003e = 0.3333333333333333"}]`,
		},
		{
			// Bad message referring to unknown variable.
			summary:     "{{$labels.instance}} = {{$badvar}}",
			description: "The alert value for {{$labels.instance}} is {{$badvar}}",
			message:     `[{"Description":"The alert value for {{$labels.instance}} is {{$badvar}}","Labels":{"instance":"testinstance"},"Payload":{"ActiveSince":"0001-01-01T00:00:00Z","AlertingRule":"Test rule string","GeneratorUrl":"prometheus_url","Value":"0.333333"},"Summary":"{{$labels.instance}} = {{$badvar}}"}]`,
		},
		{
			// Bad message referring to unknown struct field.
			summary:     "{{$labels.instance}} = {{.Val}}",
			description: "The alert value for {{$labels.instance}} is {{.Val}}",
			message:     `[{"Description":"The alert value for {{$labels.instance}} is {{.Val}}","Labels":{"instance":"testinstance"},"Payload":{"ActiveSince":"0001-01-01T00:00:00Z","AlertingRule":"Test rule string","GeneratorUrl":"prometheus_url","Value":"0.333333"},"Summary":"{{$labels.instance}} = {{.Val}}"}]`,
		},
	}

	for i, s := range scenarios {
		s.test(i, t)
	}
}
