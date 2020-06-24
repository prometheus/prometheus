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

package remote

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
	"github.com/prometheus/prometheus/util/testutil"
)

var longErrMessage = strings.Repeat("error message", maxErrMsgLen)

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
			err:  errors.New("server returned HTTP status 300 Multiple Choices: " + longErrMessage[:maxErrMsgLen]),
		},
		{
			code: 404,
			err:  errors.New("server returned HTTP status 404 Not Found: " + longErrMessage[:maxErrMsgLen]),
		},
		{
			code: 500,
			err:  recoverableError{errors.New("server returned HTTP status 500 Internal Server Error: " + longErrMessage[:maxErrMsgLen])},
		},
	}

	for i, test := range tests {
		server := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, longErrMessage, test.code)
			}),
		)

		serverURL, err := url.Parse(server.URL)
		testutil.Ok(t, err)

		conf := &ClientConfig{
			URL:     &config_util.URL{URL: serverURL},
			Timeout: model.Duration(time.Second),
		}

		hash, err := toHash(conf)
		testutil.Ok(t, err)
		c, err := NewWriteClient(hash, conf)
		testutil.Ok(t, err)

		err = c.Store(context.Background(), []byte{})
		testutil.ErrorEqual(t, err, test.err, "unexpected error in test %d", i)

		server.Close()
	}
}
