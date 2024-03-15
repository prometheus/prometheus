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

package linode

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// SDMock is the interface for the Linode mock.
type SDMock struct {
	t      *testing.T
	Server *httptest.Server
	Mux    *http.ServeMux
}

// NewSDMock returns a new SDMock.
func NewSDMock(t *testing.T) *SDMock {
	return &SDMock{
		t: t,
	}
}

// Endpoint returns the URI to the mock server.
func (m *SDMock) Endpoint() string {
	return m.Server.URL + "/"
}

// Setup creates the mock server.
func (m *SDMock) Setup() {
	m.Mux = http.NewServeMux()
	m.Server = httptest.NewServer(m.Mux)
}

// ShutdownServer creates the mock server.
func (m *SDMock) ShutdownServer() {
	m.Server.Close()
}

// HandleLinodeInstancesList mocks linode instances list.
func (m *SDMock) HandleLinodeInstancesList() {
	m.Mux.HandleFunc("/v4/linode/instances", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", tokenID) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		switch fltr := r.Header.Get("X-Filter"); fltr {
		case "{\"region\": \"us-east\"}":
			fmt.Fprint(w, instancesUsEast)
		case "{\"region\": \"ca-central\"}":
			fmt.Fprint(w, instancesCaCentral)
		default:
			fmt.Fprint(w, instancesAll)
		}
	})
}

// HandleLinodeNetworking/ipv6/ranges mocks linode networking ipv6 ranges endpoint.
func (m *SDMock) HandleLinodeNetworkingIPv6Ranges() {
	m.Mux.HandleFunc("/v4/networking/ipv6/ranges", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", tokenID) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		switch fltr := r.Header.Get("X-Filter"); fltr {
		case "{\"region\": \"us-east\"}":
			fmt.Fprint(w, networkingIpv6RangesUsEast)
		case "{\"region\": \"ca-central\"}":
			fmt.Fprint(w, networkingIpv6RangesCaCentral)
		default:
			fmt.Fprint(w, networkingIpv6RangesAll)
		}
	})
}

// HandleLinodeNetworkingIPs mocks linode networking ips endpoint.
func (m *SDMock) HandleLinodeNetworkingIPs() {
	m.Mux.HandleFunc("/v4/networking/ips", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", tokenID) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		switch fltr := r.Header.Get("X-Filter"); fltr {
		case "{\"region\": \"us-east\"}":
			fmt.Fprint(w, networkingIpsUsEast)
		case "{\"region\": \"ca-central\"}":
			fmt.Fprint(w, networkingIpsCaCentral)
		default:
			fmt.Fprint(w, networkingIpsAll)
		}
	})
}

// HandleLinodeAccountEvents mocks linode the account/events endpoint.
func (m *SDMock) HandleLinodeAccountEvents() {
	m.Mux.HandleFunc("/v4/account/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", tokenID) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Header.Get("X-Filter") == "" {
			// This should never happen; if the client sends an events request without
			// a filter, cause it to fail. The error below is not a real response from
			// the API, but should aid in debugging failed tests.
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `
{
	"errors": [
		{
			"reason": "Request missing expected X-Filter headers"
		}
	]
}`,
			)
			return
		}

		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, `
{
	"data": [],
	"results": 0,
	"pages": 1,
	"page": 1
}`,
		)
	})
}
