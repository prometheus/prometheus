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
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
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
			err:  RecoverableError{errors.New("server returned HTTP status 500 Internal Server Error: " + longErrMessage[:maxErrMsgLen]), defaultBackoff},
		},
	}

	for _, test := range tests {
		server := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, longErrMessage, test.code)
			}),
		)

		serverURL, err := url.Parse(server.URL)
		require.NoError(t, err)

		conf := &ClientConfig{
			URL:     &config_util.URL{URL: serverURL},
			Timeout: model.Duration(time.Second),
		}

		hash, err := toHash(conf)
		require.NoError(t, err)
		c, err := NewWriteClient(hash, conf)
		require.NoError(t, err)

		err = c.Store(context.Background(), []byte{}, 0)
		if test.err != nil {
			require.EqualError(t, err, test.err.Error())
		} else {
			require.NoError(t, err)
		}

		server.Close()
	}
}

func TestClientRetryAfter(t *testing.T) {
	setupServer := func(statusCode int) *httptest.Server {
		return httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "5")
				http.Error(w, longErrMessage, statusCode)
			}),
		)
	}

	getClientConfig := func(serverURL *url.URL, retryOnRateLimit bool) *ClientConfig {
		return &ClientConfig{
			URL:              &config_util.URL{URL: serverURL},
			Timeout:          model.Duration(time.Second),
			RetryOnRateLimit: retryOnRateLimit,
		}
	}

	getClient := func(conf *ClientConfig) WriteClient {
		hash, err := toHash(conf)
		require.NoError(t, err)
		c, err := NewWriteClient(hash, conf)
		require.NoError(t, err)
		return c
	}

	testCases := []struct {
		name                string
		statusCode          int
		retryOnRateLimit    bool
		expectedRecoverable bool
		expectedRetryAfter  model.Duration
	}{
		{"TooManyRequests - No Retry", http.StatusTooManyRequests, false, false, 0},
		{"TooManyRequests - With Retry", http.StatusTooManyRequests, true, true, 5 * model.Duration(time.Second)},
		{"InternalServerError", http.StatusInternalServerError, false, true, 5 * model.Duration(time.Second)}, // HTTP 5xx errors do not depend on retryOnRateLimit.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := setupServer(tc.statusCode)
			defer server.Close()

			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			c := getClient(getClientConfig(serverURL, tc.retryOnRateLimit))

			var recErr RecoverableError
			err = c.Store(context.Background(), []byte{}, 0)
			require.Equal(t, tc.expectedRecoverable, errors.As(err, &recErr), "Mismatch in expected recoverable error status.")
			if tc.expectedRecoverable {
				require.Equal(t, tc.expectedRetryAfter, recErr.retryAfter)
			}
		})
	}
}

func TestRetryAfterDuration(t *testing.T) {
	tc := []struct {
		name     string
		tInput   string
		expected model.Duration
	}{
		{
			name:     "seconds",
			tInput:   "120",
			expected: model.Duration(time.Second * 120),
		},
		{
			name:     "date-time default",
			tInput:   time.RFC1123, // Expected layout is http.TimeFormat, hence an error.
			expected: defaultBackoff,
		},
		{
			name:     "retry-after not provided",
			tInput:   "", // Expected layout is http.TimeFormat, hence an error.
			expected: defaultBackoff,
		},
	}
	for _, c := range tc {
		require.Equal(t, c.expected, retryAfterDuration(c.tInput), c.name)
	}
}
