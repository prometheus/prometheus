// Copyright 2017 The Prometheus Authors
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

package remote_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	. "github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/util/testutil"
	apiv1 "github.com/prometheus/prometheus/web/api/v1"
)

var longErrMessage = strings.Repeat("error message", MaxErrMsgLen)

func TestStoreHTTPErrorHandling(t *testing.T) {
	tests := []struct {
		code int
		err  error
	}{
		{
			code: 200,
			err:  nil,
		},
		{
			code: 300,
			err:  errors.New("server returned HTTP status 300 Multiple Choices: " + longErrMessage[:MaxErrMsgLen]),
		},
		{
			code: 404,
			err:  errors.New("server returned HTTP status 404 Not Found: " + longErrMessage[:MaxErrMsgLen]),
		},
		{
			code: 500,
			err:  RecoverableError{errors.New("server returned HTTP status 500 Internal Server Error: " + longErrMessage[:MaxErrMsgLen])},
		},
	}

	for i, test := range tests {
		server := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, longErrMessage, test.code)
			}),
		)

		serverURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}

		c, err := NewClient(0, &ClientConfig{
			URL:     &config_util.URL{URL: serverURL},
			Timeout: model.Duration(time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}

		err = c.Store(context.Background(), []byte{})
		if !testutil.ErrorEqual(err, test.err) {
			t.Errorf("%d. Unexpected error; want %v, got %v", i, test.err, err)
		}

		server.Close()
	}
}

func TestReadClient(t *testing.T) {
	suite, err := promql.NewTest(t, `
		load 1m
			test_metric1{foo="bar",baz="qux"} 1
	`)
	testutil.Ok(t, err)

	defer suite.Close()

	testutil.Ok(t, suite.Run())

	// Construct a remote read server.
	api := apiv1.NewAPI(suite.QueryEngine(), suite.Storage(), nil, nil,
		func() config.Config {
			return config.Config{
				GlobalConfig: config.GlobalConfig{
					ExternalLabels: labels.Labels{
						// We expect external labels to be added, with the source labels honored.
						{Name: "baz", Value: "a"},
						{Name: "b", Value: "c"},
						{Name: "d", Value: "e"},
					},
				},
			}
		}, nil,
		func(f http.HandlerFunc) http.HandlerFunc {
			return f
		},
		nil, false, nil, nil, 1e6, 1, 0, nil,
	)

	router := route.New()
	api.Register(router)

	server := httptest.NewServer(router)

	readURL, err := url.Parse(server.URL + "/read")
	if err != nil {
		t.Fatal(err)
	}

	// Construct a remote read client.
	c, err := NewClient(0, &ClientConfig{
		URL:     &config_util.URL{URL: readURL},
		Timeout: model.Duration(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Encode the request.
	matcher1, err := labels.NewMatcher(labels.MatchEqual, "__name__", "test_metric1")
	testutil.Ok(t, err)

	matcher2, err := labels.NewMatcher(labels.MatchEqual, "d", "e")
	testutil.Ok(t, err)

	query, err := ToQuery(0, 1, []*labels.Matcher{matcher1, matcher2}, &storage.SelectParams{Step: 0, Func: "avg"})
	testutil.Ok(t, err)

	result, err := c.Read(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}

	testutil.Equals(t, &prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "test_metric1"},
					{Name: "b", Value: "c"},
					{Name: "baz", Value: "qux"},
					{Name: "d", Value: "e"},
					{Name: "foo", Value: "bar"},
				},
				Samples: []prompb.Sample{{Value: 1, Timestamp: 0}},
			},
		},
	}, result)

	server.Close()
}
