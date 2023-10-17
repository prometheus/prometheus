// Copyright 2022 The Prometheus Authors
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

package nomad

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type NomadSDTestSuite struct {
	Mock *SDMock
}

// SDMock is the interface for the nomad mock
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

func (s *NomadSDTestSuite) TearDownSuite() {
	s.Mock.ShutdownServer()
}

func (s *NomadSDTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleServicesList()
	s.Mock.HandleServiceHashiCupsGet()
}

func (m *SDMock) HandleServicesList() {
	m.Mux.HandleFunc("/v1/services", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, `
		[
			{
				"Namespace": "default",
				"Services": [
				{
					"ServiceName": "hashicups",
					"Tags": [
					"metrics"
					]
				}
				]
			}
		]`,
		)
	})
}

func (m *SDMock) HandleServiceHashiCupsGet() {
	m.Mux.HandleFunc("/v1/service/hashicups", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprint(w, `
		[
			{
				"ID": "_nomad-task-6a1d5f0a-7362-3f5d-9baf-5ed438918e50-group-hashicups-hashicups-hashicups_ui",
				"ServiceName": "hashicups",
				"Namespace": "default",
				"NodeID": "d92fdc3c-9c2b-298a-e8f4-c33f3a449f09",
				"Datacenter": "dc1",
				"JobID": "dashboard",
				"AllocID": "6a1d5f0a-7362-3f5d-9baf-5ed438918e50",
				"Tags": [
				"metrics"
				],
				"Address": "127.0.0.1",
				"Port": 30456,
				"CreateIndex": 226,
				"ModifyIndex": 226
			}
		]`,
		)
	})
}

func TestConfiguredService(t *testing.T) {
	conf := &SDConfig{
		Server: "http://localhost:4646",
	}
	_, err := NewDiscovery(conf, nil)
	require.NoError(t, err)
}

func TestNomadSDRefresh(t *testing.T) {
	sdmock := &NomadSDTestSuite{}
	sdmock.SetupTest(t)
	t.Cleanup(sdmock.TearDownSuite)

	endpoint, err := url.Parse(sdmock.Mock.Endpoint())
	require.NoError(t, err)

	cfg := DefaultSDConfig
	cfg.Server = endpoint.String()
	d, err := NewDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Equal(t, 1, len(tg.Targets))

	lbls := model.LabelSet{
		"__address__":                  model.LabelValue("127.0.0.1:30456"),
		"__meta_nomad_address":         model.LabelValue("127.0.0.1"),
		"__meta_nomad_dc":              model.LabelValue("dc1"),
		"__meta_nomad_namespace":       model.LabelValue("default"),
		"__meta_nomad_node_id":         model.LabelValue("d92fdc3c-9c2b-298a-e8f4-c33f3a449f09"),
		"__meta_nomad_service":         model.LabelValue("hashicups"),
		"__meta_nomad_service_address": model.LabelValue("127.0.0.1"),
		"__meta_nomad_service_id":      model.LabelValue("_nomad-task-6a1d5f0a-7362-3f5d-9baf-5ed438918e50-group-hashicups-hashicups-hashicups_ui"),
		"__meta_nomad_service_port":    model.LabelValue("30456"),
		"__meta_nomad_tags":            model.LabelValue(",metrics,"),
	}
	require.Equal(t, lbls, tg.Targets[0])
}
