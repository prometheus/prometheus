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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func testUpdateServices(respHandler http.HandlerFunc) ([]*targetgroup.Group, error) {
	// Create a test server with mock HTTP handler.
	ts := httptest.NewServer(respHandler)
	defer ts.Close()

	conf := SDConfig{
		Server: ts.URL,
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := conf.NewDiscovererMetrics(reg, refreshMetrics)
	err := metrics.Register()
	if err != nil {
		return nil, err
	}
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	md, err := NewDiscovery(&conf, nil, metrics)
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
	require.Empty(t, tgs)
}

func TestUyuniSDLogin(t *testing.T) {
	var (
		errTesting  = "unable to get the managed system groups information of monitored clients: request error: bad status code - 500"
		call        = 0
		respHandler = func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/xml")
			switch call {
			case 0:
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, `<?xml version="1.0"?>
<methodResponse>
	<params>
		<param>
			<value>
				a token
			</value>
		</param>
	</params>
</methodResponse>`)
			case 1:
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, ``)
			}
			call++
		}
	)
	tgs, err := testUpdateServices(respHandler)

	require.EqualError(t, err, errTesting)
	require.Empty(t, tgs)
}

func TestUyuniSDSkipLogin(t *testing.T) {
	var (
		errTesting  = "unable to get the managed system groups information of monitored clients: request error: bad status code - 500"
		respHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, ``)
		}
	)

	// Create a test server with mock HTTP handler.
	ts := httptest.NewServer(http.HandlerFunc(respHandler))
	defer ts.Close()

	conf := SDConfig{
		Server: ts.URL,
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := conf.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	md, err := NewDiscovery(&conf, nil, metrics)
	if err != nil {
		t.Error(err)
	}
	// simulate a cached token
	md.token = `a token`
	md.tokenExpiration = time.Now().Add(time.Minute)

	tgs, err := md.refresh(context.Background())

	require.EqualError(t, err, errTesting)
	require.Empty(t, tgs)
}
