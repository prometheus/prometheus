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
	"io"
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
	t.Parallel()
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
	require.Len(t, tg.Targets, 3)

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
		{
			// A server whose optional pointer fields (id, and everything under
			// properties/metadata used for labels) are null in the API response
			// must not panic and must simply omit the corresponding labels.
			"__address__":                        "10.0.0.99:80",
			"__meta_ionos_server_ip":             ",10.0.0.99,",
			"__meta_ionos_server_nic_ip_unnamed": ",10.0.0.99,",
			"__meta_ionos_server_servers_id":     "8feda53f-15f0-447f-badf-ebe32dad2fc0/servers",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

// ionosTestDiscovery returns a discovery pointed at a mock serving body, so
// refresh() runs the response through the real SDK decoder and the nil pointers
// below are the ones a live API would produce.
func ionosTestDiscovery(t *testing.T, body string) *serverDiscovery {
	t.Helper()

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, body)
		require.NoError(t, err)
	}))
	t.Cleanup(mock.Close)

	cfg := DefaultSDConfig
	cfg.DatacenterID = ionosTestDatacenterID
	cfg.HTTPClientConfig.BearerToken = ionosTestBearerToken
	cfg.ionosEndpoint = mock.URL

	d, err := newServerDiscovery(&cfg, nil)
	require.NoError(t, err)

	return d
}

// ionosServersBody wraps server objects in the collection the API returns.
func ionosServersBody(items string) string {
	return `{
		"id": "` + ionosTestDatacenterID + `/servers",
		"type": "collection",
		"items": [` + items + `]
	}`
}

// TestIONOSServerRefreshNilContainers covers responses where the API omitted a
// container rather than a leaf field. Items, properties and ips are all
// pointers carrying omitempty in the SDK, but refresh() walked into them with no
// nil check, so a datacenter with no servers, a server with no properties or a
// NIC with no IPs panicked the whole Prometheus process instead of degrading the
// target.
func TestIONOSServerRefreshNilContainers(t *testing.T) {
	t.Parallel()
	const serversID = "8feda53f-15f0-447f-badf-ebe32dad2fc0/servers"

	for _, tc := range []struct {
		name     string
		body     string
		expected []model.LabelSet
	}{
		{
			name: "NoServerItems",
			body: `{"id": "` + ionosTestDatacenterID + `/servers", "type": "collection"}`,
		},
		{
			name: "NilServerProperties",
			body: ionosServersBody(`{
				"id": "srv-1",
				"type": "server",
				"properties": null,
				"entities": {"nics": {"items": [
					{"id": "nic-1", "type": "nic", "properties": {"ips": ["10.0.0.1"]}}
				]}}
			}`),
			expected: []model.LabelSet{{
				"__address__":                        "10.0.0.1:80",
				"__meta_ionos_server_id":             "srv-1",
				"__meta_ionos_server_ip":             ",10.0.0.1,",
				"__meta_ionos_server_nic_ip_unnamed": ",10.0.0.1,",
				"__meta_ionos_server_servers_id":     serversID,
			}},
		},
		{
			name: "NoNICItems",
			body: ionosServersBody(`{
				"id": "srv-2",
				"type": "server",
				"properties": {"name": "no-nics"},
				"entities": {"nics": {"id": "srv-2/nics", "type": "collection"}}
			}`),
		},
		{
			name: "NilNICProperties",
			body: ionosServersBody(`{
				"id": "srv-3",
				"type": "server",
				"properties": {"name": "nic-without-properties"},
				"entities": {"nics": {"items": [
					{"id": "nic-3", "type": "nic", "properties": null}
				]}}
			}`),
		},
		{
			name: "NilNICIps",
			body: ionosServersBody(`{
				"id": "srv-4",
				"type": "server",
				"properties": {"name": "nic-without-ips"},
				"entities": {"nics": {"items": [
					{"id": "nic-4", "type": "nic", "properties": {"name": "eth0"}}
				]}}
			}`),
		},
		{
			name: "NilBootCdromAndBootVolumeID",
			body: ionosServersBody(`{
				"id": "srv-5",
				"type": "server",
				"properties": {"bootCdrom": {}, "bootVolume": {"id": null}},
				"entities": {"nics": {"items": [
					{"id": "nic-5", "type": "nic", "properties": {"ips": ["10.0.0.5"]}}
				]}}
			}`),
			expected: []model.LabelSet{{
				"__address__":                        "10.0.0.5:80",
				"__meta_ionos_server_id":             "srv-5",
				"__meta_ionos_server_ip":             ",10.0.0.5,",
				"__meta_ionos_server_nic_ip_unnamed": ",10.0.0.5,",
				"__meta_ionos_server_servers_id":     serversID,
			}},
		},
		{
			name: "NoVolumeItems",
			body: ionosServersBody(`{
				"id": "srv-6",
				"type": "server",
				"properties": {"name": "no-volume-items"},
				"entities": {
					"nics": {"items": [
						{"id": "nic-6", "type": "nic", "properties": {"ips": ["10.0.0.6"]}}
					]},
					"volumes": {"id": "srv-6/volumes", "type": "collection"}
				}
			}`),
			expected: []model.LabelSet{{
				"__address__":                        "10.0.0.6:80",
				"__meta_ionos_server_id":             "srv-6",
				"__meta_ionos_server_ip":             ",10.0.0.6,",
				"__meta_ionos_server_name":           "no-volume-items",
				"__meta_ionos_server_nic_ip_unnamed": ",10.0.0.6,",
				"__meta_ionos_server_servers_id":     serversID,
			}},
		},
		{
			name: "NilVolumeProperties",
			body: ionosServersBody(`{
				"id": "srv-7",
				"type": "server",
				"properties": {"name": "volume-without-properties"},
				"entities": {
					"nics": {"items": [
						{"id": "nic-7", "type": "nic", "properties": {"ips": ["10.0.0.7"]}}
					]},
					"volumes": {"items": [{"id": "vol-7", "type": "volume", "properties": null}]}
				}
			}`),
			expected: []model.LabelSet{{
				"__address__":                        "10.0.0.7:80",
				"__meta_ionos_server_id":             "srv-7",
				"__meta_ionos_server_ip":             ",10.0.0.7,",
				"__meta_ionos_server_name":           "volume-without-properties",
				"__meta_ionos_server_nic_ip_unnamed": ",10.0.0.7,",
				"__meta_ionos_server_servers_id":     serversID,
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tgs, err := ionosTestDiscovery(t, tc.body).refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, tgs, 1)
			require.Equal(t, tc.expected, tgs[0].Targets)
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
