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

package dockerswarm

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
	"gopkg.in/yaml.v2"
)

// SDMock is the interface for the DigitalOcean mock
type SDMock struct {
	t         *testing.T
	Server    *httptest.Server
	Mux       *http.ServeMux
	directory string
}

// NewSDMock returns a new SDMock.
func NewSDMock(t *testing.T, directory string) *SDMock {
	return &SDMock{
		t:         t,
		directory: directory,
	}
}

// Endpoint returns the URI to the mock server
func (m *SDMock) Endpoint() string {
	return m.Server.URL + "/"
}

// Setup creates the mock server
func (m *SDMock) Setup() {
	m.Mux = http.NewServeMux()
	m.Server = httptest.NewServer(m.Mux)
	m.t.Cleanup(m.Server.Close)
	m.SetupHandlers()
}

// HandleNodesList mocks nodes list.
func (m *SDMock) SetupHandlers() {
	headers := make(map[string]string)
	rawHeaders, err := ioutil.ReadFile(filepath.Join("testdata", m.directory, "headers.yml"))
	testutil.Ok(m.t, err)
	yaml.Unmarshal(rawHeaders, &headers)

	prefix := "/"
	if v, ok := headers["Api-Version"]; ok {
		prefix += "v" + v + "/"
	}

	for _, path := range []string{"_ping", "nodes", "services", "tasks"} {
		p := path
		handler := prefix + p
		if p == "_ping" {
			handler = "/" + p
		}
		m.Mux.HandleFunc(handler, func(w http.ResponseWriter, r *http.Request) {
			for k, v := range headers {
				w.Header().Add(k, v)
			}
			if response, err := ioutil.ReadFile(filepath.Join("testdata", m.directory, p+".json")); err == nil {
				w.Header().Add("content-type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(response)
				return
			}
			if response, err := ioutil.ReadFile(filepath.Join("testdata", m.directory, p)); err == nil {
				w.WriteHeader(http.StatusOK)
				w.Write(response)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		})
	}
}
