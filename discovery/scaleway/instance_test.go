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

package scaleway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var (
	testProjectID     = "8feda53f-15f0-447f-badf-ebe32dad2fc0"
	testSecretKeyFile = "testdata/secret_key"
	testSecretKey     = "6d6579e5-a5b9-49fc-a35f-b4feb9b87301"
	testAccessKey     = "SCW0W8NG6024YHRJ7723"
)

func TestScalewayInstanceRefresh(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(mockScalewayInstance))
	defer mock.Close()

	cfgString := fmt.Sprintf(`
---
role: instance
project_id: %s
secret_key: %s
access_key: %s
api_url: %s
`, testProjectID, testSecretKey, testAccessKey, mock.URL)
	var cfg SDConfig
	require.NoError(t, yaml.UnmarshalStrict([]byte(cfgString), &cfg))

	d, err := newRefresher(&cfg)
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
			"__address__":                                     "10.70.60.57:80",
			"__meta_scaleway_instance_boot_type":              "local",
			"__meta_scaleway_instance_hostname":               "scw-nervous-shirley",
			"__meta_scaleway_instance_id":                     "93c18a61-b681-49d0-a1cc-62b43883ae89",
			"__meta_scaleway_instance_image_arch":             "x86_64",
			"__meta_scaleway_instance_image_id":               "45a86b35-eca6-4055-9b34-ca69845da146",
			"__meta_scaleway_instance_image_name":             "Ubuntu 18.04 Bionic Beaver",
			"__meta_scaleway_instance_location_cluster_id":    "40",
			"__meta_scaleway_instance_location_hypervisor_id": "1601",
			"__meta_scaleway_instance_location_node_id":       "29",
			"__meta_scaleway_instance_name":                   "scw-nervous-shirley",
			"__meta_scaleway_instance_organization_id":        "cb334986-b054-4725-9d3a-40850fdc6015",
			"__meta_scaleway_instance_private_ipv4":           "10.70.60.57",
			"__meta_scaleway_instance_project_id":             "cb334986-b054-4725-9d5a-30850fdc6015",
			"__meta_scaleway_instance_public_ipv6":            "2001:bc8:630:1e1c::1",
			"__meta_scaleway_instance_region":                 "fr-par",
			"__meta_scaleway_instance_security_group_id":      "a6a794b7-c05b-4b20-b9fe-e17a9bb85cf0",
			"__meta_scaleway_instance_security_group_name":    "Default security group",
			"__meta_scaleway_instance_status":                 "running",
			"__meta_scaleway_instance_type":                   "DEV1-S",
			"__meta_scaleway_instance_zone":                   "fr-par-1",
		},
		{
			"__address__":                                     "10.193.162.9:80",
			"__meta_scaleway_instance_boot_type":              "local",
			"__meta_scaleway_instance_hostname":               "scw-quizzical-feistel",
			"__meta_scaleway_instance_id":                     "5b6198b4-c677-41b5-9c05-04557264ae1f",
			"__meta_scaleway_instance_image_arch":             "x86_64",
			"__meta_scaleway_instance_image_id":               "71733b74-260f-4d0d-8f18-f37dcfa56d6f",
			"__meta_scaleway_instance_image_name":             "Debian Buster",
			"__meta_scaleway_instance_location_cluster_id":    "4",
			"__meta_scaleway_instance_location_hypervisor_id": "201",
			"__meta_scaleway_instance_location_node_id":       "5",
			"__meta_scaleway_instance_name":                   "scw-quizzical-feistel",
			"__meta_scaleway_instance_organization_id":        "cb334986-b054-4725-9d3a-40850fdc6015",
			"__meta_scaleway_instance_private_ipv4":           "10.193.162.9",
			"__meta_scaleway_instance_project_id":             "cb334986-b054-4725-9d5a-30850fdc6015",
			"__meta_scaleway_instance_public_ipv4":            "151.115.45.127",
			"__meta_scaleway_instance_public_ipv6":            "2001:bc8:1e00:6204::1",
			"__meta_scaleway_instance_region":                 "fr-par",
			"__meta_scaleway_instance_security_group_id":      "d35dd210-8392-44dc-8854-121e419a0f56",
			"__meta_scaleway_instance_security_group_name":    "Default security group",
			"__meta_scaleway_instance_status":                 "running",
			"__meta_scaleway_instance_type":                   "DEV1-S",
			"__meta_scaleway_instance_zone":                   "fr-par-1",
		},
		{
			"__address__":                                     "51.158.183.115:80",
			"__meta_scaleway_instance_boot_type":              "local",
			"__meta_scaleway_instance_hostname":               "routed-dualstack",
			"__meta_scaleway_instance_id":                     "4904366a-7e26-4b65-b97b-6392c761247a",
			"__meta_scaleway_instance_image_arch":             "x86_64",
			"__meta_scaleway_instance_image_id":               "3e0a5b84-1d69-4993-8fa4-0d7df52d5160",
			"__meta_scaleway_instance_image_name":             "Ubuntu 22.04 Jammy Jellyfish",
			"__meta_scaleway_instance_location_cluster_id":    "19",
			"__meta_scaleway_instance_location_hypervisor_id": "1201",
			"__meta_scaleway_instance_location_node_id":       "24",
			"__meta_scaleway_instance_name":                   "routed-dualstack",
			"__meta_scaleway_instance_organization_id":        "20b3d507-96ac-454c-a795-bc731b46b12f",
			"__meta_scaleway_instance_project_id":             "20b3d507-96ac-454c-a795-bc731b46b12f",
			"__meta_scaleway_instance_public_ipv4":            "51.158.183.115",
			"__meta_scaleway_instance_region":                 "nl-ams",
			"__meta_scaleway_instance_security_group_id":      "984414da-9fc2-49c0-a925-fed6266fe092",
			"__meta_scaleway_instance_security_group_name":    "Default security group",
			"__meta_scaleway_instance_status":                 "running",
			"__meta_scaleway_instance_type":                   "DEV1-S",
			"__meta_scaleway_instance_zone":                   "nl-ams-1",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func mockScalewayInstance(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Auth-Token") != testSecretKey {
		http.Error(w, "bad token id", http.StatusUnauthorized)
		return
	}
	if r.URL.Path != "/instance/v1/zones/fr-par-1/servers" {
		http.Error(w, "bad url", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	instance, err := os.ReadFile("testdata/instance.json")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(instance)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func TestScalewayInstanceAuthToken(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(mockScalewayInstance))
	defer mock.Close()

	cfgString := fmt.Sprintf(`
---
role: instance
project_id: %s
secret_key_file: %s
access_key: %s
api_url: %s
`, testProjectID, testSecretKeyFile, testAccessKey, mock.URL)
	var cfg SDConfig
	require.NoError(t, yaml.UnmarshalStrict([]byte(cfgString), &cfg))

	d, err := newRefresher(&cfg)
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)
}
