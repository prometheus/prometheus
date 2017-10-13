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
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/testutil"
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
)

func TestTritonSDNew(t *testing.T) {
	td, err := New(nil, &conf)

	testutil.Ok(t, err)

	testutil.Assert(t, td != nil, testutil.NotNilErrorMessage)
	testutil.Assert(t, td.client != nil, testutil.NotNilErrorMessage)
	testutil.Assert(t, td.sdConfig != nil, testutil.NotNilErrorMessage)

	testutil.Equals(t, time.Duration(conf.RefreshInterval), td.interval)
	testutil.Equals(t, conf.Account, td.sdConfig.Account)
	testutil.Equals(t, conf.DNSSuffix, td.sdConfig.DNSSuffix)
	testutil.Equals(t, conf.Endpoint, td.sdConfig.Endpoint)
	testutil.Equals(t, conf.Port, td.sdConfig.Port)
}

func TestTritonSDNewBadConfig(t *testing.T) {
	td, err := New(nil, &badconf)

	testutil.NotOk(t, err)
	testutil.Assert(t, td == nil, testutil.NotNilErrorMessage)
}

func TestTritonSDRun(t *testing.T) {
	var (
		td, err     = New(nil, &conf)
		ch          = make(chan []*config.TargetGroup)
		ctx, cancel = context.WithCancel(context.Background())
	)

	testutil.Ok(t, err)
	testutil.Assert(t, td != nil, testutil.NotNilErrorMessage)

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
	testutil.Assert(t, tgts == nil, testutil.NilErrorMessage)
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

	testutil.Assert(t, tgts != nil, testutil.NotNilErrorMessage)
	testutil.Equals(t, 2, len(tgts))
}

func TestTritonSDRefreshNoServer(t *testing.T) {
	var (
		td, err = New(nil, &conf)
	)
	testutil.Ok(t, err)
	testutil.Assert(t, td != nil, testutil.NotNilErrorMessage)

	tg, rerr := td.refresh()

	testutil.NotOk(t, rerr)
	testutil.Assert(
		t,
		strings.Contains(rerr.Error(), "an error occurred when requesting targets from the discovery endpoint."),
		"Invalid error message",
	)

	testutil.Assert(t, tg != nil, testutil.NotNilErrorMessage)
	testutil.Assert(t, tg.Targets == nil, testutil.NilErrorMessage)
}

func testTritonSDRefresh(t *testing.T, dstr string) []model.LabelSet {
	var (
		td, err = New(nil, &conf)
		s       = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, dstr)
		}))
	)

	defer s.Close()

	u, uperr := url.Parse(s.URL)

	testutil.Ok(t, uperr)
	testutil.Assert(t, u != nil, testutil.NotNilErrorMessage)

	host, strport, sherr := net.SplitHostPort(u.Host)

	testutil.Ok(t, sherr)
	testutil.Assert(t, host != "", "Expected non empty string for host")
	testutil.Assert(t, strport != "", "Expected non empty string for strport")

	port, atoierr := strconv.Atoi(strport)

	testutil.Ok(t, atoierr)
	testutil.Assert(t, port != 0, "Expected non-zero value for port")

	td.sdConfig.Port = port

	testutil.Ok(t, err)
	testutil.Assert(t, td != nil, testutil.NotNilErrorMessage)

	tg, err := td.refresh()
	testutil.Ok(t, err)
	testutil.Assert(t, tg != nil, testutil.NotNilErrorMessage)

	return tg.Targets
}
