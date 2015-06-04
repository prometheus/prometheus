package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/promql"
)

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

	api := &API{
		Storage:     suite.Storage(),
		QueryEngine: suite.QueryEngine(),
	}

	start := clientmodel.Timestamp(0)
	var tests = []struct {
		endpoint apiFunc
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
				ResultType: promql.ExprScalar,
				Result: &promql.Scalar{
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
				ResultType: promql.ExprScalar,
				Result: &promql.Scalar{
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
				ResultType: promql.ExprScalar,
				Result: &promql.Scalar{
					Value:     0.333,
					Timestamp: start.Add(123 * time.Second),
				},
			},
		},
		{
			endpoint: api.metricNames,
			response: clientmodel.LabelValues{
				"test_metric1",
				"test_metric2",
			},
		},
	}

	for _, test := range tests {
		req, err := http.NewRequest("ANY", fmt.Sprintf("http://example.com?%s", test.query.Encode()), nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, apierr := test.endpoint(req)
		if apierr != nil {
			if test.errType == "" {
				t.Fatalf("Unexpected error: %s", apierr)
			}
			if test.errType != apierr.typ {
				t.Fatalf("Expected error of type %q but got type %q", test.errType, apierr.typ)
			}
			continue
		}
		if apierr == nil && test.errType != "" {
			t.Fatalf("Expected error of type %q but got none", test.errType)
		}
		if !reflect.DeepEqual(resp, test.response) {
			t.Fatalf("Response does not match, expected:\n%v\ngot:\n%v", test.response, resp)
		}
	}
}

func TestRespondSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	respond(w, "test")

	if w.Code != 200 {
		t.Fatalf("Return code %d expected in success response but got %d", 200, w.Code)
	}
	var res response
	err := json.Unmarshal([]byte(w.Body.String()), &res)
	if err != nil {
		t.Fatal(err)
	}

	if h := w.Header().Get("Content-Type"); h != "application/json" {
		t.Fatalf("expected Content-Type %q but got %q", "application/json", h)
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
	w := httptest.NewRecorder()
	respondError(w, &apiError{errorTimeout, errors.New("message")}, "test")

	if w.Code != 422 {
		t.Fatalf("Return code %d expected in success response but got %d", 422, w.Code)
	}
	var res response
	err := json.Unmarshal([]byte(w.Body.String()), &res)
	if err != nil {
		t.Fatal(err)
	}

	if h := w.Header().Get("Content-Type"); h != "application/json" {
		t.Fatalf("expected Content-Type %q but got %q", "application/json", h)
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
			input:  "123",
			result: time.Unix(123, 0),
		}, {
			input:  "123.123",
			result: time.Unix(123, 123000000),
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
		res := clientmodel.TimestampFromTime(test.result)
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
