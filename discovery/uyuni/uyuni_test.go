// Copyright 2020 The Prometheus Authors
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

package uyuni

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func testUpdateServices(respHandler http.HandlerFunc) ([]*targetgroup.Group, error) {
	// Create a test server with mock HTTP handler.
	ts := httptest.NewServer(respHandler)
	defer ts.Close()

	conf := SDConfig{
		Server: ts.URL,
	}

	md, err := NewDiscovery(&conf, nil)
	if err != nil {
		return nil, err
	}

	return md.refresh(context.Background())
}

func TestUyuniSDHandleError(t *testing.T) {
	var (
		errTesting  = "unable to login to Uyuni API: request error: bad status code - 500"
		respHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, ``)
		}
	)
	tgs, err := testUpdateServices(respHandler)

	require.EqualError(t, err, errTesting)
	require.Equal(t, len(tgs), 0)
}
