// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
)

type targetRetrieverFunc func() []*retrieval.Target

func (f targetRetrieverFunc) Targets() []*retrieval.Target {
	return f()
}

type alertmanagerRetrieverFunc func() []string

func (f alertmanagerRetrieverFunc) Alertmanagers() []string {
	return f()
}

func TestEndpoints(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar"} 0+100x100
			test_metric1{foo="boo"} 1+0x100
			test_metric2{foo="boo"} 1+0x100
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer suite.Close()

	if err := suite.Run(); err != nil {
		t.Fatal(err)
	}

	now := model.Now()

	tr := targetRetrieverFunc(func() []*retrieval.Target {
		return []*retrieval.Target{
			retrieval.NewTarget(
				model.LabelSet{
					model.SchemeLabel:      "http",
					model.AddressLabel:     "example.com:8080",
					model.MetricsPathLabel: "/metrics",
				},
				model.LabelSet{},
				url.Values{},
			),
		}
	})

	ar := alertmanagerRetrieverFunc(func() []string {
		return []string{"http://alertmanager.example.com:8080/api/v1/alerts"}
	})

	api := &API{
		Storage:               suite.Storage(),
		QueryEngine:           suite.QueryEngine(),
		targetRetriever:       tr,
		alertmanagerRetriever: ar,
		now: func() model.Time { return now },
	}

	start := model.Time(0)
	var tests = []struct {
		endpoint apiFunc
		params   map[string]string
		query    url.Values
		response interface{}
		errType  errorType
	}{
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"2"},
				"time":  []string{"123.3"},
			},
			response: &queryData{
				ResultType: model.ValScalar,
				Result: &model.Scalar{
					Value:     2,
					Timestamp: start.Add(123*time.Second + 300*time.Millisecond),
				},
			},
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"0.333"},
				"time":  []string{"1970-01-01T00:02:03Z"},
			},
			response: &queryData{
				ResultType: model.ValScalar,
				Result: &model.Scalar{
					Value:     0.333,
					Timestamp: start.Add(123 * time.Second),
				},
			},
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"0.333"},
				"time":  []string{"1970-01-01T01:02:03+01:00"},
			},
			response: &queryData{
				ResultType: model.ValScalar,
				Result: &model.Scalar{
					Value:     0.333,
					Timestamp: start.Add(123 * time.Second),
				},
			},
		},
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"0.333"},
			},
			response: &queryData{
				ResultType: model.ValScalar,
				Result: &model.Scalar{
					Value:     0.333,
					Timestamp: now,
				},
			},
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"0"},
				"end":   []string{"2"},
				"step":  []string{"1"},
			},
			response: &queryData{
				ResultType: model.ValMatrix,
				Result: model.Matrix{
					&model.SampleStream{
						Values: []model.SamplePair{
							{Value: 0, Timestamp: start},
							{Value: 1, Timestamp: start.Add(1 * time.Second)},
							{Value: 2, Timestamp: start.Add(2 * time.Second)},
						},
						Metric: model.Metric{},
					},
				},
			},
		},
		// Missing query params in range queries.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"end":   []string{"2"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"0"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"0"},
				"end":   []string{"2"},
			},
			errType: errorBadData,
		},
		// Bad query expression.
		{
			endpoint: api.query,
			query: url.Values{
				"query": []string{"invalid][query"},
				"time":  []string{"1970-01-01T01:02:03+01:00"},
			},
			errType: errorBadData,
		},
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"invalid][query"},
				"start": []string{"0"},
				"end":   []string{"100"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		// Invalid step.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"1"},
				"end":   []string{"2"},
				"step":  []string{"0"},
			},
			errType: errorBadData,
		},
		// Start after end.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"2"},
				"end":   []string{"1"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		// Start overflows int64 internally.
		{
			endpoint: api.queryRange,
			query: url.Values{
				"query": []string{"time()"},
				"start": []string{"148966367200.372"},
				"end":   []string{"1489667272.372"},
				"step":  []string{"1"},
			},
			errType: errorBadData,
		},
		{
			endpoint: api.labelValues,
			params: map[string]string{
				"name": "__name__",
			},
			response: model.LabelValues{
				"test_metric1",
				"test_metric2",
			},
		},
		{
			endpoint: api.labelValues,
			params: map[string]string{
				"name": "foo",
			},
			response: model.LabelValues{
				"bar",
				"boo",
			},
		},
		// Bad name parameter.
		{
			endpoint: api.labelValues,
			params: map[string]string{
				"name": "not!!!allowed",
			},
			errType: errorBadData,
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric2",
					"foo":      "boo",
				},
			},
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric1{foo=~".+o"}`},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric1",
					"foo":      "boo",
				},
			},
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric1{foo=~"o$"}`, `test_metric1{foo=~".+o"}`},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric1",
					"foo":      "boo",
				},
			},
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric1{foo=~".+o"}`, `none`},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric1",
					"foo":      "boo",
				},
			},
		},
		// Start and end before series starts.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"-2"},
				"end":     []string{"-1"},
			},
			response: []model.Metric{},
		},
		// Start and end after series ends.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"100000"},
				"end":     []string{"100001"},
			},
			response: []model.Metric{},
		},
		// Start before series starts, end after series ends.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"-1"},
				"end":     []string{"100000"},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric2",
					"foo":      "boo",
				},
			},
		},
		// Start and end within series.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"1"},
				"end":     []string{"100"},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric2",
					"foo":      "boo",
				},
			},
		},
		// Start within series, end after.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"1"},
				"end":     []string{"100000"},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric2",
					"foo":      "boo",
				},
			},
		},
		// Start before series, end within series.
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric2`},
				"start":   []string{"-1"},
				"end":     []string{"1"},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric2",
					"foo":      "boo",
				},
			},
		},
		// Missing match[] query params in series requests.
		{
			endpoint: api.series,
			errType:  errorBadData,
		},
		{
			endpoint: api.dropSeries,
			errType:  errorBadData,
		},
		// The following tests delete time series from the test storage. They
		// must remain at the end and are fixed in their order.
		{
			endpoint: api.dropSeries,
			query: url.Values{
				"match[]": []string{`test_metric1{foo=~".+o"}`},
			},
			response: struct {
				NumDeleted int `json:"numDeleted"`
			}{1},
		},
		{
			endpoint: api.series,
			query: url.Values{
				"match[]": []string{`test_metric1`},
			},
			response: []model.Metric{
				{
					"__name__": "test_metric1",
					"foo":      "bar",
				},
			},
		}, {
			endpoint: api.dropSeries,
			query: url.Values{
				"match[]": []string{`{__name__=~".+"}`},
			},
			response: struct {
				NumDeleted int `json:"numDeleted"`
			}{2},
		}, {
			endpoint: api.targets,
			response: &TargetDiscovery{
				ActiveTargets: []*Target{
					{
						DiscoveredLabels: model.LabelSet{},
						Labels:           model.LabelSet{},
						ScrapeURL:        "http://example.com:8080/metrics",
						Health:           "unknown",
					},
				},
			},
		}, {
			endpoint: api.alertmanagers,
			response: &AlertmanagerDiscovery{
				ActiveAlertmanagers: []*AlertmanagerTarget{
					{
						URL: "http://alertmanager.example.com:8080/api/v1/alerts",
					},
				},
			},
		},
	}

	for _, test := range tests {
		// Build a context with the correct request params.
		ctx := context.Background()
		for p, v := range test.params {
			ctx = route.WithParam(ctx, p, v)
		}
		api.context = func(r *http.Request) context.Context {
			return ctx
		}

		req, err := http.NewRequest("ANY", fmt.Sprintf("http://example.com?%s", test.query.Encode()), nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, apiErr := test.endpoint(req)
		if apiErr != nil {
			if test.errType == errorNone {
				t.Fatalf("Unexpected error: %s", apiErr)
			}
			if test.errType != apiErr.typ {
				t.Fatalf("Expected error of type %q but got type %q", test.errType, apiErr.typ)
			}
			continue
		}
		if apiErr == nil && test.errType != errorNone {
			t.Fatalf("Expected error of type %q but got none", test.errType)
		}
		if !reflect.DeepEqual(resp, test.response) {
			t.Fatalf("Response does not match, expected:\n%+v\ngot:\n%+v", test.response, resp)
		}
		// Ensure that removed metrics are unindexed before the next request.
		suite.Storage().WaitForIndexing()
	}
}

func TestRespondSuccess(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, "test")
	}))
	defer s.Close()

	resp, err := http.Get(s.URL)
	if err != nil {
		t.Fatalf("Error on test request: %s", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		t.Fatalf("Error reading response body: %s", err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Return code %d expected in success response but got %d", 200, resp.StatusCode)
	}
	if h := resp.Header.Get("Content-Type"); h != "application/json" {
		t.Fatalf("Expected Content-Type %q but got %q", "application/json", h)
	}

	var res response
	if err = json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("Error unmarshaling JSON body: %s", err)
	}

	exp := &response{
		Status: statusSuccess,
		Data:   "test",
	}
	if !reflect.DeepEqual(&res, exp) {
		t.Fatalf("Expected response \n%v\n but got \n%v\n", res, exp)
	}
}

func TestRespondError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondError(w, &apiError{errorTimeout, errors.New("message")}, "test")
	}))
	defer s.Close()

	resp, err := http.Get(s.URL)
	if err != nil {
		t.Fatalf("Error on test request: %s", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		t.Fatalf("Error reading response body: %s", err)
	}

	if want, have := http.StatusServiceUnavailable, resp.StatusCode; want != have {
		t.Fatalf("Return code %d expected in error response but got %d", want, have)
	}
	if h := resp.Header.Get("Content-Type"); h != "application/json" {
		t.Fatalf("Expected Content-Type %q but got %q", "application/json", h)
	}

	var res response
	if err = json.Unmarshal([]byte(body), &res); err != nil {
		t.Fatalf("Error unmarshaling JSON body: %s", err)
	}

	exp := &response{
		Status:    statusError,
		Data:      "test",
		ErrorType: errorTimeout,
		Error:     "message",
	}
	if !reflect.DeepEqual(&res, exp) {
		t.Fatalf("Expected response \n%v\n but got \n%v\n", res, exp)
	}
}

func TestParseTime(t *testing.T) {
	ts, err := time.Parse(time.RFC3339Nano, "2015-06-03T13:21:58.555Z")
	if err != nil {
		panic(err)
	}

	var tests = []struct {
		input  string
		fail   bool
		result time.Time
	}{
		{
			input: "",
			fail:  true,
		}, {
			input: "abc",
			fail:  true,
		}, {
			input: "30s",
			fail:  true,
		}, {
			// Internal int64 overflow.
			input: "-148966367200.372",
			fail:  true,
		}, {
			// Internal int64 overflow.
			input: "148966367200.372",
			fail:  true,
		}, {
			input:  "123",
			result: time.Unix(123, 0),
		}, {
			input:  "123.123",
			result: time.Unix(123, 123000000),
		}, {
			input:  "2015-06-03T13:21:58.555Z",
			result: ts,
		}, {
			input:  "2015-06-03T14:21:58.555+01:00",
			result: ts,
		},
	}

	for _, test := range tests {
		ts, err := parseTime(test.input)
		if err != nil && !test.fail {
			t.Errorf("Unexpected error for %q: %s", test.input, err)
			continue
		}
		if err == nil && test.fail {
			t.Errorf("Expected error for %q but got none", test.input)
			continue
		}
		res := model.TimeFromUnixNano(test.result.UnixNano())
		if !test.fail && ts != res {
			t.Errorf("Expected time %v for input %q but got %v", res, test.input, ts)
		}
	}
}

func TestParseDuration(t *testing.T) {
	var tests = []struct {
		input  string
		fail   bool
		result time.Duration
	}{
		{
			input: "",
			fail:  true,
		}, {
			input: "abc",
			fail:  true,
		}, {
			input: "2015-06-03T13:21:58.555Z",
			fail:  true,
		}, {
			// Internal int64 overflow.
			input: "-148966367200.372",
			fail:  true,
		}, {
			// Internal int64 overflow.
			input: "148966367200.372",
			fail:  true,
		}, {
			input:  "123",
			result: 123 * time.Second,
		}, {
			input:  "123.333",
			result: 123*time.Second + 333*time.Millisecond,
		}, {
			input:  "15s",
			result: 15 * time.Second,
		}, {
			input:  "5m",
			result: 5 * time.Minute,
		},
	}

	for _, test := range tests {
		d, err := parseDuration(test.input)
		if err != nil && !test.fail {
			t.Errorf("Unexpected error for %q: %s", test.input, err)
			continue
		}
		if err == nil && test.fail {
			t.Errorf("Expected error for %q but got none", test.input)
			continue
		}
		if !test.fail && d != test.result {
			t.Errorf("Expected duration %v for input %q but got %v", test.result, test.input, d)
		}
	}
}

func TestOptionsMethod(t *testing.T) {
	r := route.New(nil)
	api := &API{}
	api.Register(r)

	s := httptest.NewServer(r)
	defer s.Close()

	req, err := http.NewRequest("OPTIONS", s.URL+"/any_path", nil)
	if err != nil {
		t.Fatalf("Error creating OPTIONS request: %s", err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Error executing OPTIONS request: %s", err)
	}

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Expected status %d, got %d", http.StatusNoContent, resp.StatusCode)
	}

	for h, v := range corsHeaders {
		if resp.Header.Get(h) != v {
			t.Fatalf("Expected %q for header %q, got %q", v, h, resp.Header.Get(h))
		}
	}
}
