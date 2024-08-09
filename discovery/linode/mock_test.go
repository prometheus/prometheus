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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const tokenID = "7b2c56dd51edd90952c1b94c472b94b176f20c5c777e376849edd8ad1c6c03bb"

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
	m.t.Cleanup(m.Server.Close)
	m.SetupHandlers()
}

// SetupHandlers for endpoints of interest.
func (m *SDMock) SetupHandlers() {
	for _, handler := range []string{"/v4/account/events", "/v4/linode/instances", "/v4/networking/ips", "/v4/networking/ipv6/ranges"} {
		m.Mux.HandleFunc(handler, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", tokenID) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			xFilter := struct {
				Region string `json:"region"`
			}{}
			json.Unmarshal([]byte(r.Header.Get("X-Filter")), &xFilter)

			directory := "testdata/no_region_filter"
			if xFilter.Region != "" { // Validate region filter matches test criteria.
				directory = "testdata/" + xFilter.Region
			}
			if response, err := os.ReadFile(filepath.Join(directory, r.URL.Path+".json")); err == nil {
				w.Header().Add("content-type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				w.Write(response)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		})
	}
}
