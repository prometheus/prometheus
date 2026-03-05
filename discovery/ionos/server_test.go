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

package ionos

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	ionoscloud "github.com/ionos-cloud/sdk-go/v6"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

var (
	ionosTestBearerToken  = config.Secret("jwt")
	ionosTestDatacenterID = "8feda53f-15f0-447f-badf-ebe32dad2fc0"
)

func TestIONOSServerRefresh(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(mockIONOSServers))
	defer mock.Close()

	cfg := DefaultSDConfig
	cfg.DatacenterID = ionosTestDatacenterID
	cfg.HTTPClientConfig.BearerToken = ionosTestBearerToken
	cfg.ionosEndpoint = mock.URL

	d, err := newServerDiscovery(&cfg, nil)
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 2)

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                           "85.215.243.177:80",
			"__meta_ionos_server_availability_zone": "ZONE_2",
			"__meta_ionos_server_boot_cdrom_id":     "0e4d57f9-cd78-11e9-b88c-525400f64d8d",
			"__meta_ionos_server_cpu_family":        "INTEL_SKYLAKE",
			"__meta_ionos_server_id":                "b501942c-4e08-43e6-8ec1-00e59c64e0e4",
			"__meta_ionos_server_ip":                ",85.215.243.177,185.56.150.9,85.215.238.118,",
			"__meta_ionos_server_nic_ip_metrics":    ",85.215.243.177,",
			"__meta_ionos_server_nic_ip_unnamed":    ",185.56.150.9,85.215.238.118,",
			"__meta_ionos_server_lifecycle":         "AVAILABLE",
			"__meta_ionos_server_name":              "prometheus-2",
			"__meta_ionos_server_servers_id":        "8feda53f-15f0-447f-badf-ebe32dad2fc0/servers",
			"__meta_ionos_server_state":             "RUNNING",
			"__meta_ionos_server_type":              "ENTERPRISE",
		},
		{
			"__address__":                           "85.215.248.84:80",
			"__meta_ionos_server_availability_zone": "ZONE_1",
			"__meta_ionos_server_boot_cdrom_id":     "0e4d57f9-cd78-11e9-b88c-525400f64d8d",
			"__meta_ionos_server_cpu_family":        "INTEL_SKYLAKE",
			"__meta_ionos_server_id":                "523415e6-ff8c-4dc0-86d3-09c256039b30",
			"__meta_ionos_server_ip":                ",85.215.248.84,",
			"__meta_ionos_server_nic_ip_unnamed":    ",85.215.248.84,",
			"__meta_ionos_server_lifecycle":         "AVAILABLE",
			"__meta_ionos_server_name":              "prometheus-1",
			"__meta_ionos_server_servers_id":        "8feda53f-15f0-447f-badf-ebe32dad2fc0/servers",
			"__meta_ionos_server_state":             "RUNNING",
			"__meta_ionos_server_type":              "ENTERPRISE",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func mockIONOSServers(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != fmt.Sprintf("Bearer %s", ionosTestBearerToken) {
		http.Error(w, "bad token", http.StatusUnauthorized)
		return
	}
	if r.URL.Path != fmt.Sprintf("%s/datacenters/%s/servers", ionoscloud.DefaultIonosBasePath, ionosTestDatacenterID) {
		http.Error(w, fmt.Sprintf("bad url: %s", r.URL.Path), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	server, err := os.ReadFile("testdata/servers.json")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(server)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
