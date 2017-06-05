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
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

var (
	conf = config.TritonSDConfig{
		Account:         "testAccount",
		DNSSuffix:       "triton.example.com",
		Endpoint:        "127.0.0.1",
		Port:            443,
		Version:         1,
		RefreshInterval: 1,
		TLSConfig:       config.TLSConfig{InsecureSkipVerify: true},
	}
	badconf = config.TritonSDConfig{
		Account:         "badTestAccount",
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
	logger = log.Base()
)

func TestTritonSDNew(t *testing.T) {
	td, err := New(logger, &conf)
	assert.Nil(t, err)
	assert.NotNil(t, td)
	assert.NotNil(t, td.client)
	assert.NotNil(t, td.interval)
	assert.NotNil(t, td.logger)
	assert.Equal(t, logger, td.logger, "td.logger equals logger")
	assert.NotNil(t, td.sdConfig)
	assert.Equal(t, conf.Account, td.sdConfig.Account)
	assert.Equal(t, conf.DNSSuffix, td.sdConfig.DNSSuffix)
	assert.Equal(t, conf.Endpoint, td.sdConfig.Endpoint)
	assert.Equal(t, conf.Port, td.sdConfig.Port)
}

func TestTritonSDNewBadConfig(t *testing.T) {
	td, err := New(logger, &badconf)
	assert.NotNil(t, err)
	assert.Nil(t, td)
}

func TestTritonSDRun(t *testing.T) {
	var (
		td, err     = New(logger, &conf)
		ch          = make(chan []*config.TargetGroup)
		ctx, cancel = context.WithCancel(context.Background())
	)

	assert.Nil(t, err)
	assert.NotNil(t, td)

	go td.Run(ctx, ch)

	select {
	case <-time.After(60 * time.Millisecond):
		// Expected.
	case tgs := <-ch:
		t.Fatalf("Unexpected target groups in triton discovery: %s", tgs)
	}

	cancel()
}

func TestTritonSDRefreshNoTargets(t *testing.T) {
	tgts := testTritonSDRefresh(t, "{\"containers\":[]}")
	assert.Nil(t, tgts)
}

func TestTritonSDRefreshMultipleTargets(t *testing.T) {
	var (
		dstr = `{"containers":[
		 	{
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
	)

	tgts := testTritonSDRefresh(t, dstr)
	assert.NotNil(t, tgts)
	assert.Equal(t, 2, len(tgts))
}

func TestTritonSDRefreshNoServer(t *testing.T) {
	var (
		td, err = New(logger, &conf)
	)
	assert.Nil(t, err)
	assert.NotNil(t, td)

	tg, rerr := td.refresh()
	assert.NotNil(t, rerr)
	assert.Contains(t, rerr.Error(), "an error occurred when requesting targets from the discovery endpoint.")
	assert.NotNil(t, tg)
	assert.Nil(t, tg.Targets)
}

func testTritonSDRefresh(t *testing.T, dstr string) []model.LabelSet {
	var (
		td, err = New(logger, &conf)
		s       = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, dstr)
		}))
	)

	defer s.Close()

	u, uperr := url.Parse(s.URL)
	assert.Nil(t, uperr)
	assert.NotNil(t, u)

	host, strport, sherr := net.SplitHostPort(u.Host)
	assert.Nil(t, sherr)
	assert.NotNil(t, host)
	assert.NotNil(t, strport)

	port, atoierr := strconv.Atoi(strport)
	assert.Nil(t, atoierr)
	assert.NotNil(t, port)

	td.sdConfig.Port = port

	assert.Nil(t, err)
	assert.NotNil(t, td)

	tg, err := td.refresh()
	assert.Nil(t, err)
	assert.NotNil(t, tg)

	return tg.Targets
}
