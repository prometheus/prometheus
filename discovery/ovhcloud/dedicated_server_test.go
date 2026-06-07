// Copyright The Prometheus Authors
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

package ovhcloud

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

func TestOvhcloudDedicatedServerRefresh(t *testing.T) {
	var cfg SDConfig

	mock := httptest.NewServer(http.HandlerFunc(MockDedicatedAPI))
	defer mock.Close()
	cfgString := fmt.Sprintf(`
---
service: dedicated_server
endpoint: %s
application_key: %s
application_secret: %s
consumer_key: %s`, mock.URL, ovhcloudApplicationKeyTest, ovhcloudApplicationSecretTest, ovhcloudConsumerKeyTest)

	require.NoError(t, yaml.UnmarshalStrict([]byte(cfgString), &cfg))
	d, err := newRefresher(&cfg, promslog.NewNopLogger())
	require.NoError(t, err)
	ctx := context.Background()
	targetGroups, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, targetGroups, 1)
	targetGroup := targetGroups[0]
	require.NotNil(t, targetGroup)
	require.NotNil(t, targetGroup.Targets)
	require.Len(t, targetGroup.Targets, 1)

	for i, lbls := range []model.LabelSet{
		{
			"__address__": "1.2.3.4",
			"__meta_ovhcloud_dedicated_server_commercial_range": "Advance-1 Gen 2",
			"__meta_ovhcloud_dedicated_server_datacenter":       "gra3",
			"__meta_ovhcloud_dedicated_server_ipv4":             "1.2.3.4",
			"__meta_ovhcloud_dedicated_server_ipv6":             "",
			"__meta_ovhcloud_dedicated_server_link_speed":       "123",
			"__meta_ovhcloud_dedicated_server_name":             "abcde",
			"__meta_ovhcloud_dedicated_server_no_intervention":  "false",
			"__meta_ovhcloud_dedicated_server_os":               "debian11_64",
			"__meta_ovhcloud_dedicated_server_rack":             "TESTRACK",
			"__meta_ovhcloud_dedicated_server_reverse":          "abcde-rev",
			"__meta_ovhcloud_dedicated_server_server_id":        "1234",
			"__meta_ovhcloud_dedicated_server_state":            "test",
			"__meta_ovhcloud_dedicated_server_support_level":    "pro",
			"instance": "abcde",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, targetGroup.Targets[i])
		})
	}
}

func MockDedicatedAPI(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Ovh-Application") != ovhcloudApplicationKeyTest {
		http.Error(w, "bad application key", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/dedicated/server" {
		dedicatedServersList, err := os.ReadFile("testdata/dedicated_server/dedicated_servers.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = w.Write(dedicatedServersList)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if r.URL.Path == "/dedicated/server/abcde" {
		dedicatedServer, err := os.ReadFile("testdata/dedicated_server/dedicated_servers_details.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = w.Write(dedicatedServer)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if r.URL.Path == "/dedicated/server/abcde/ips" {
		dedicatedServerIPs, err := os.ReadFile("testdata/dedicated_server/dedicated_servers_abcde_ips.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = w.Write(dedicatedServerIPs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
