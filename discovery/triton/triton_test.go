// Copyright 2016 The Prometheus Authors
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

package triton

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

var (
	conf = SDConfig{
		Account:         "testAccount",
		Role:            "container",
		DNSSuffix:       "triton.example.com",
		Endpoint:        "127.0.0.1",
		Port:            443,
		Version:         1,
		RefreshInterval: 1,
		TLSConfig:       config.TLSConfig{InsecureSkipVerify: true},
	}
	badconf = SDConfig{
		Account:         "badTestAccount",
		Role:            "container",
		DNSSuffix:       "bad.triton.example.com",
		Endpoint:        "127.0.0.1",
		Port:            443,
		Version:         1,
		RefreshInterval: 1,
		TLSConfig: config.TLSConfig{
			InsecureSkipVerify: false,
			KeyFile:            "shouldnotexist.key",
			CAFile:             "shouldnotexist.ca",
			CertFile:           "shouldnotexist.cert",
		},
	}
	groupsconf = SDConfig{
		Account:         "testAccount",
		Role:            "container",
		DNSSuffix:       "triton.example.com",
		Endpoint:        "127.0.0.1",
		Groups:          []string{"foo", "bar"},
		Port:            443,
		Version:         1,
		RefreshInterval: 1,
		TLSConfig:       config.TLSConfig{InsecureSkipVerify: true},
	}
	cnconf = SDConfig{
		Account:         "testAccount",
		Role:            "cn",
		DNSSuffix:       "triton.example.com",
		Endpoint:        "127.0.0.1",
		Port:            443,
		Version:         1,
		RefreshInterval: 1,
		TLSConfig:       config.TLSConfig{InsecureSkipVerify: true},
	}
)

func newTritonDiscovery(c SDConfig) (*Discovery, error) {
	return New(nil, &c, prometheus.NewRegistry())
}

func TestTritonSDNew(t *testing.T) {
	td, err := newTritonDiscovery(conf)
	require.NoError(t, err)
	require.NotNil(t, td)
	require.NotNil(t, td.client)
	require.NotZero(t, td.interval)
	require.NotNil(t, td.sdConfig)
	require.Equal(t, conf.Account, td.sdConfig.Account)
	require.Equal(t, conf.DNSSuffix, td.sdConfig.DNSSuffix)
	require.Equal(t, conf.Endpoint, td.sdConfig.Endpoint)
	require.Equal(t, conf.Port, td.sdConfig.Port)
}

func TestTritonSDNewBadConfig(t *testing.T) {
	td, err := newTritonDiscovery(badconf)
	require.Error(t, err)
	require.Nil(t, td)
}

func TestTritonSDNewGroupsConfig(t *testing.T) {
	td, err := newTritonDiscovery(groupsconf)
	require.NoError(t, err)
	require.NotNil(t, td)
	require.NotNil(t, td.client)
	require.NotZero(t, td.interval)
	require.NotNil(t, td.sdConfig)
	require.Equal(t, groupsconf.Account, td.sdConfig.Account)
	require.Equal(t, groupsconf.DNSSuffix, td.sdConfig.DNSSuffix)
	require.Equal(t, groupsconf.Endpoint, td.sdConfig.Endpoint)
	require.Equal(t, groupsconf.Groups, td.sdConfig.Groups)
	require.Equal(t, groupsconf.Port, td.sdConfig.Port)
}

func TestTritonSDNewCNConfig(t *testing.T) {
	td, err := newTritonDiscovery(cnconf)
	require.NoError(t, err)
	require.NotNil(t, td)
	require.NotNil(t, td.client)
	require.NotZero(t, td.interval)
	require.NotZero(t, td.sdConfig)
	require.Equal(t, cnconf.Role, td.sdConfig.Role)
	require.Equal(t, cnconf.Account, td.sdConfig.Account)
	require.Equal(t, cnconf.DNSSuffix, td.sdConfig.DNSSuffix)
	require.Equal(t, cnconf.Endpoint, td.sdConfig.Endpoint)
	require.Equal(t, cnconf.Port, td.sdConfig.Port)
}

func TestTritonSDRefreshNoTargets(t *testing.T) {
	tgts := testTritonSDRefresh(t, conf, "{\"containers\":[]}")
	require.Nil(t, tgts)
}

func TestTritonSDRefreshMultipleTargets(t *testing.T) {
	dstr := `{"containers":[
		 	{
                                "groups":["foo","bar","baz"],
				"server_uuid":"44454c4c-5000-104d-8037-b7c04f5a5131",
				"vm_alias":"server01",
				"vm_brand":"lx",
				"vm_image_uuid":"7b27a514-89d7-11e6-bee6-3f96f367bee7",
				"vm_uuid":"ad466fbf-46a2-4027-9b64-8d3cdb7e9072"
			},
			{
				"server_uuid":"a5894692-bd32-4ca1-908a-e2dda3c3a5e6",
				"vm_alias":"server02",
				"vm_brand":"kvm",
				"vm_image_uuid":"a5894692-bd32-4ca1-908a-e2dda3c3a5e6",
				"vm_uuid":"7b27a514-89d7-11e6-bee6-3f96f367bee7"
			}]
		}`

	tgts := testTritonSDRefresh(t, conf, dstr)
	require.NotNil(t, tgts)
	require.Len(t, tgts, 2)
}

func TestTritonSDRefreshNoServer(t *testing.T) {
	td, _ := newTritonDiscovery(conf)

	_, err := td.refresh(context.Background())
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "an error occurred when requesting targets from the discovery endpoint"))
}

func TestTritonSDRefreshCancelled(t *testing.T) {
	td, _ := newTritonDiscovery(conf)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := td.refresh(ctx)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), context.Canceled.Error()))
}

func TestTritonSDRefreshCNsUUIDOnly(t *testing.T) {
	dstr := `{"cns":[
		 	{
				"server_uuid":"44454c4c-5000-104d-8037-b7c04f5a5131"
			},
			{
				"server_uuid":"a5894692-bd32-4ca1-908a-e2dda3c3a5e6"
			}]
		}`

	tgts := testTritonSDRefresh(t, cnconf, dstr)
	require.NotNil(t, tgts)
	require.Len(t, tgts, 2)
}

func TestTritonSDRefreshCNsWithHostname(t *testing.T) {
	dstr := `{"cns":[
		 	{
				"server_uuid":"44454c4c-5000-104d-8037-b7c04f5a5131",
				"server_hostname": "server01"
			},
			{
				"server_uuid":"a5894692-bd32-4ca1-908a-e2dda3c3a5e6",
				"server_hostname": "server02"
			}]
		}`

	tgts := testTritonSDRefresh(t, cnconf, dstr)
	require.NotNil(t, tgts)
	require.Len(t, tgts, 2)
}

func testTritonSDRefresh(t *testing.T, c SDConfig, dstr string) []model.LabelSet {
	var (
		td, _ = newTritonDiscovery(c)
		s     = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, dstr)
		}))
	)

	defer s.Close()

	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	require.NotNil(t, u)

	host, strport, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	require.NotEmpty(t, host)
	require.NotEmpty(t, strport)

	port, err := strconv.Atoi(strport)
	require.NoError(t, err)
	require.NotZero(t, port)

	td.sdConfig.Port = port

	tgs, err := td.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	tg := tgs[0]
	require.NotNil(t, tg)

	return tg.Targets
}
