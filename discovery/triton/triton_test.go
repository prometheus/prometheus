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

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

var (
	conf = SDConfig{
		Account:         "testAccount",
		DNSSuffix:       "triton.example.com",
		Endpoint:        "127.0.0.1",
		Port:            443,
		Version:         1,
		RefreshInterval: 1,
		TLSConfig:       config.TLSConfig{InsecureSkipVerify: true},
	}
	badconf = SDConfig{
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
	groupsconf = SDConfig{
		Account:         "testAccount",
		DNSSuffix:       "triton.example.com",
		Endpoint:        "127.0.0.1",
		Groups:          []string{"foo", "bar"},
		Port:            443,
		Version:         1,
		RefreshInterval: 1,
		TLSConfig:       config.TLSConfig{InsecureSkipVerify: true},
	}
)

func TestTritonSDNew(t *testing.T) {
	td, err := New(nil, &conf)
	testutil.Ok(t, err)
	testutil.Assert(t, td != nil, "")
	testutil.Assert(t, td.client != nil, "")
	testutil.Assert(t, td.interval != 0, "")
	testutil.Assert(t, td.sdConfig != nil, "")
	testutil.Equals(t, conf.Account, td.sdConfig.Account)
	testutil.Equals(t, conf.DNSSuffix, td.sdConfig.DNSSuffix)
	testutil.Equals(t, conf.Endpoint, td.sdConfig.Endpoint)
	testutil.Equals(t, conf.Port, td.sdConfig.Port)
}

func TestTritonSDNewBadConfig(t *testing.T) {
	td, err := New(nil, &badconf)
	testutil.NotOk(t, err, "")
	testutil.Assert(t, td == nil, "")
}

func TestTritonSDNewGroupsConfig(t *testing.T) {
	td, err := New(nil, &groupsconf)
	testutil.Ok(t, err)
	testutil.Assert(t, td != nil, "")
	testutil.Assert(t, td.client != nil, "")
	testutil.Assert(t, td.interval != 0, "")
	testutil.Assert(t, td.sdConfig != nil, "")
	testutil.Equals(t, groupsconf.Account, td.sdConfig.Account)
	testutil.Equals(t, groupsconf.DNSSuffix, td.sdConfig.DNSSuffix)
	testutil.Equals(t, groupsconf.Endpoint, td.sdConfig.Endpoint)
	testutil.Equals(t, groupsconf.Groups, td.sdConfig.Groups)
	testutil.Equals(t, groupsconf.Port, td.sdConfig.Port)
}

func TestTritonSDRun(t *testing.T) {
	var (
		td, err     = New(nil, &conf)
		ch          = make(chan []*targetgroup.Group)
		ctx, cancel = context.WithCancel(context.Background())
	)

	testutil.Ok(t, err)
	testutil.Assert(t, td != nil, "")

	wait := make(chan struct{})
	go func() {
		td.Run(ctx, ch)
		close(wait)
	}()

	select {
	case <-time.After(60 * time.Millisecond):
		// Expected.
	case tgs := <-ch:
		t.Fatalf("Unexpected target groups in triton discovery: %s", tgs)
	}

	cancel()
	<-wait
}

func TestTritonSDRefreshNoTargets(t *testing.T) {
	tgts := testTritonSDRefresh(t, "{\"containers\":[]}")
	testutil.Assert(t, tgts == nil, "")
}

func TestTritonSDRefreshMultipleTargets(t *testing.T) {
	var (
		dstr = `{"containers":[
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
	)

	tgts := testTritonSDRefresh(t, dstr)
	testutil.Assert(t, tgts != nil, "")
	testutil.Equals(t, 2, len(tgts))
}

func TestTritonSDRefreshNoServer(t *testing.T) {
	var (
		td, err = New(nil, &conf)
	)
	testutil.Ok(t, err)
	testutil.Assert(t, td != nil, "")

	tg, rerr := td.refresh()
	testutil.NotOk(t, rerr, "")
	testutil.Equals(t, strings.Contains(rerr.Error(), "an error occurred when requesting targets from the discovery endpoint."), true)
	testutil.Assert(t, tg != nil, "")
	testutil.Assert(t, tg.Targets == nil, "")
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
	testutil.Assert(t, u != nil, "")

	host, strport, sherr := net.SplitHostPort(u.Host)
	testutil.Ok(t, sherr)
	testutil.Assert(t, host != "", "")
	testutil.Assert(t, strport != "", "")

	port, atoierr := strconv.Atoi(strport)
	testutil.Ok(t, atoierr)
	testutil.Assert(t, port != 0, "")

	td.sdConfig.Port = port

	testutil.Ok(t, err)
	testutil.Assert(t, td != nil, "")

	tg, err := td.refresh()
	testutil.Ok(t, err)
	testutil.Assert(t, tg != nil, "")

	return tg.Targets
}
