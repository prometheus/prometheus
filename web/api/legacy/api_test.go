// Copyright 2015 The Prometheus Authors
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

package legacy

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
)

// This is a bit annoying. On one hand, we have to choose a current timestamp
// because the storage doesn't have a mocked-out time yet and would otherwise
// immediately throw away "old" samples. On the other hand, we have to make
// sure that the float value survives the parsing and re-formatting in the
// query layer precisely without any change. Thus we round to seconds and then
// add known-good digits after the decimal point which behave well in
// parsing/re-formatting.
var testTimestamp = model.TimeFromUnix(time.Now().Round(time.Second).Unix()).Add(124 * time.Millisecond)

func testNow() model.Time {
	return testTimestamp
}

func TestQuery(t *testing.T) {
	scenarios := []struct {
		// URL query string.
		queryStr string
		// Expected HTTP response status code.
		status int
		// Regex to match against response body.
		bodyRe string
	}{
		{
			queryStr: "",
			status:   http.StatusOK,
			bodyRe:   `{"type":"error","value":"parse error at char 1: no expression found in input","version":1}`,
		},
		{
			queryStr: "expr=1.4",
			status:   http.StatusOK,
			bodyRe:   `{"type":"scalar","value":"1.4","version":1}`,
		},
		{
			queryStr: "expr=testmetric",
			status:   http.StatusOK,
			bodyRe:   `{"type":"vector","value":\[{"metric":{"__name__":"testmetric"},"value":"0","timestamp":\d+\.\d+}\],"version":1}`,
		},
		{
			queryStr: "expr=testmetric&timestamp=" + testTimestamp.String(),
			status:   http.StatusOK,
			bodyRe:   `{"type":"vector","value":\[{"metric":{"__name__":"testmetric"},"value":"0","timestamp":` + testTimestamp.String() + `}\],"version":1}`,
		},
		{
			queryStr: "expr=testmetric&timestamp=" + testTimestamp.Add(-time.Hour).String(),
			status:   http.StatusOK,
			bodyRe:   `{"type":"vector","value":\[\],"version":1}`,
		},
		{
			queryStr: "timestamp=invalid",
			status:   http.StatusBadRequest,
			bodyRe:   "invalid query timestamp",
		},
		{
			queryStr: "expr=(badexpression",
			status:   http.StatusOK,
			bodyRe:   `{"type":"error","value":"parse error at char 15: unclosed left parenthesis","version":1}`,
		},
	}

	storage, closer := local.NewTestStorage(t, 1)
	defer closer.Close()
	storage.Append(&model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: "testmetric",
		},
		Timestamp: testTimestamp,
		Value:     0,
	})
	storage.WaitForIndexing()

	api := &API{
		Now:         testNow,
		Storage:     storage,
		QueryEngine: promql.NewEngine(storage, nil),
	}
	rtr := route.New()
	api.Register(rtr.WithPrefix("/api"))

	server := httptest.NewServer(rtr)
	defer server.Close()

	for i, s := range scenarios {
		// Do query.
		resp, err := http.Get(server.URL + "/api/query?" + s.queryStr)
		if err != nil {
			t.Fatalf("%d. Error querying API: %s", i, err)
		}

		// Check status code.
		if resp.StatusCode != s.status {
			t.Fatalf("%d. Unexpected status code; got %d, want %d", i, resp.StatusCode, s.status)
		}

		// Check response headers.
		ct := resp.Header["Content-Type"]
		if len(ct) != 1 {
			t.Fatalf("%d. Unexpected number of 'Content-Type' headers; got %d, want 1", i, len(ct))
		}
		if ct[0] != "application/json" {
			t.Fatalf("%d. Unexpected 'Content-Type' header; got %s; want %s", i, ct[0], "application/json")
		}

		// Check body.
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("%d. Error reading response body: %s", i, err)
		}
		re := regexp.MustCompile(s.bodyRe)
		if !re.Match(b) {
			t.Fatalf("%d. Body didn't match '%s'. Body: %s", i, s.bodyRe, string(b))
		}
	}
}
