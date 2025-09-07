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
	yaml "go.yaml.in/yaml/v2"
)

func TestOvhCloudVpsRefresh(t *testing.T) {
	var cfg SDConfig

	mock := httptest.NewServer(http.HandlerFunc(MockVpsAPI))
	defer mock.Close()
	cfgString := fmt.Sprintf(`
---
service: vps
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
			"__address__":                               "192.0.2.1",
			"__meta_ovhcloud_vps_ipv4":                  "192.0.2.1",
			"__meta_ovhcloud_vps_ipv6":                  "",
			"__meta_ovhcloud_vps_cluster":               "cluster_test",
			"__meta_ovhcloud_vps_datacenter":            "[]",
			"__meta_ovhcloud_vps_disk":                  "40",
			"__meta_ovhcloud_vps_display_name":          "abc",
			"__meta_ovhcloud_vps_maximum_additional_ip": "16",
			"__meta_ovhcloud_vps_memory":                "2048",
			"__meta_ovhcloud_vps_memory_limit":          "2048",
			"__meta_ovhcloud_vps_model_name":            "vps-value-1-2-40",
			"__meta_ovhcloud_vps_name":                  "abc",
			"__meta_ovhcloud_vps_netboot_mode":          "local",
			"__meta_ovhcloud_vps_offer":                 "VPS abc",
			"__meta_ovhcloud_vps_offer_type":            "ssd",
			"__meta_ovhcloud_vps_state":                 "running",
			"__meta_ovhcloud_vps_vcore":                 "1",
			"__meta_ovhcloud_vps_model_vcore":           "1",
			"__meta_ovhcloud_vps_version":               "2019v1",
			"__meta_ovhcloud_vps_zone":                  "zone",
			"instance":                                  "abc",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, targetGroup.Targets[i])
		})
	}
}

func MockVpsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Ovh-Application") != ovhcloudApplicationKeyTest {
		http.Error(w, "bad application key", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/vps" {
		dedicatedServersList, err := os.ReadFile("testdata/vps/vps.json")
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
	if r.URL.Path == "/vps/abc" {
		dedicatedServer, err := os.ReadFile("testdata/vps/vps_details.json")
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
	if r.URL.Path == "/vps/abc/ips" {
		dedicatedServerIPs, err := os.ReadFile("testdata/vps/vps_abc_ips.json")
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
