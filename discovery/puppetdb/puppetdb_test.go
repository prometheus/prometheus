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

package puppetdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/stretchr/testify/require"
)

func mockServer(t *testing.T) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query string `json:"query"`
		}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		http.ServeFile(w, r, "fixtures/"+request.Query+".json")
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestPuppetSlashInURL(t *testing.T) {
	tests := map[string]string{
		"https://puppetserver":      "https://puppetserver/pdb/query/v4",
		"https://puppetserver/":     "https://puppetserver/pdb/query/v4",
		"http://puppetserver:8080/": "http://puppetserver:8080/pdb/query/v4",
		"http://puppetserver:8080":  "http://puppetserver:8080/pdb/query/v4",
	}

	for serverURL, apiURL := range tests {
		cfg := SDConfig{
			HTTPClientConfig: config.DefaultHTTPClientConfig,
			URL:              serverURL,
			Query:            "vhosts", // This is not a valid PuppetDB query, but it is used by the mock.
			Port:             80,
			RefreshInterval:  model.Duration(30 * time.Second),
		}
		d, err := NewDiscovery(&cfg, log.NewNopLogger())
		require.NoError(t, err)
		require.Equal(t, apiURL, d.url)
	}
}

func TestPuppetDBRefresh(t *testing.T) {
	ts := mockServer(t)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		Query:            "vhosts", // This is not a valid PuppetDB query, but it is used by the mock.
		Port:             80,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	expectedTargets := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel:                             model.LabelValue("edinburgh.example.com:80"),
					model.LabelName("__meta_puppetdb_certname"):    model.LabelValue("edinburgh.example.com"),
					model.LabelName("__meta_puppetdb_environment"): model.LabelValue("prod"),
					model.LabelName("__meta_puppetdb_exported"):    model.LabelValue("false"),
					model.LabelName("__meta_puppetdb_file"):        model.LabelValue("/etc/puppetlabs/code/environments/prod/modules/upstream/apache/manifests/init.pp"),
					model.LabelName("__meta_puppetdb_resource"):    model.LabelValue("49af83866dc5a1518968b68e58a25319107afe11"),
					model.LabelName("__meta_puppetdb_tags"):        model.LabelValue(",roles::hypervisor,apache,apache::vhost,class,default-ssl,profile_hypervisor,vhost,profile_apache,hypervisor,__node_regexp__edinburgh,roles,node,"),
					model.LabelName("__meta_puppetdb_title"):       model.LabelValue("default-ssl"),
					model.LabelName("__meta_puppetdb_type"):        model.LabelValue("Apache::Vhost"),
				},
			},
			Source: ts.URL + "/pdb/query/v4?query=vhosts",
		},
	}
	require.Equal(t, tgs, expectedTargets)
}

func TestPuppetDBRefreshWithParameters(t *testing.T) {
	ts := mockServer(t)

	cfg := SDConfig{
		HTTPClientConfig:  config.DefaultHTTPClientConfig,
		URL:               ts.URL,
		Query:             "vhosts", // This is not a valid PuppetDB query, but it is used by the mock.
		Port:              80,
		IncludeParameters: true,
		RefreshInterval:   model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	expectedTargets := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{
					model.AddressLabel:                                           model.LabelValue("edinburgh.example.com:80"),
					model.LabelName("__meta_puppetdb_certname"):                  model.LabelValue("edinburgh.example.com"),
					model.LabelName("__meta_puppetdb_environment"):               model.LabelValue("prod"),
					model.LabelName("__meta_puppetdb_exported"):                  model.LabelValue("false"),
					model.LabelName("__meta_puppetdb_file"):                      model.LabelValue("/etc/puppetlabs/code/environments/prod/modules/upstream/apache/manifests/init.pp"),
					model.LabelName("__meta_puppetdb_parameter_access_log"):      model.LabelValue("true"),
					model.LabelName("__meta_puppetdb_parameter_access_log_file"): model.LabelValue("ssl_access_log"),
					model.LabelName("__meta_puppetdb_parameter_docroot"):         model.LabelValue("/var/www/html"),
					model.LabelName("__meta_puppetdb_parameter_ensure"):          model.LabelValue("absent"),
					model.LabelName("__meta_puppetdb_parameter_labels_alias"):    model.LabelValue("edinburgh"),
					model.LabelName("__meta_puppetdb_parameter_options"):         model.LabelValue("Indexes,FollowSymLinks,MultiViews"),
					model.LabelName("__meta_puppetdb_resource"):                  model.LabelValue("49af83866dc5a1518968b68e58a25319107afe11"),
					model.LabelName("__meta_puppetdb_tags"):                      model.LabelValue(",roles::hypervisor,apache,apache::vhost,class,default-ssl,profile_hypervisor,vhost,profile_apache,hypervisor,__node_regexp__edinburgh,roles,node,"),
					model.LabelName("__meta_puppetdb_title"):                     model.LabelValue("default-ssl"),
					model.LabelName("__meta_puppetdb_type"):                      model.LabelValue("Apache::Vhost"),
				},
			},
			Source: ts.URL + "/pdb/query/v4?query=vhosts",
		},
	}
	require.Equal(t, tgs, expectedTargets)
}

func TestPuppetDBInvalidCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.EqualError(t, err, "server returned HTTP status 400 Bad Request")
}

func TestPuppetDBInvalidFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "{}")
	}))

	t.Cleanup(ts.Close)

	cfg := SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		URL:              ts.URL,
		RefreshInterval:  model.Duration(30 * time.Second),
	}

	d, err := NewDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.EqualError(t, err, "unsupported content type text/plain; charset=utf-8")
}
