// Copyright 2021 The Prometheus Authors
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
	"net/http"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/config"
	"github.com/stretchr/testify/require"
)

func TestOAuth2RoundTripper(t *testing.T) {
	token := "12345"
	var gotReq *http.Request

	rt := newOAuth2RoundTripper(&config.OAuth2Config{AccessToken: token}, promhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		gotReq = req
		return &http.Response{StatusCode: http.StatusOK}, nil
	}))

	client := &http.Client{Transport: rt}

	req, err := http.NewRequest(http.MethodPost, "google.com", strings.NewReader("Hello, world!"))
	require.NoError(t, err)

	_, err = client.Do(req)
	require.NoError(t, err)
	require.NotNil(t, gotReq)

	require.Equal(t, req.Header.Get("Authorization"), "Bearer "+token)
}
