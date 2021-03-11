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
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var (
	testProjectID = "8feda53f-15f0-447f-badf-ebe32dad2fc0"
	testSecretKey = "6d6579e5-a5b9-49fc-a35f-b4feb9b87301"
	testAccessKey = "SCW0W8NG6024YHRJ7723"
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

	require.Equal(t, 1, len(tgs))

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Equal(t, 2, len(tg.Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                           "10.70.60.57:80",
			"__meta_scaleway_instance_id":           "93c18a61-b681-49d0-a1cc-62b43883ae89",
			"__meta_scaleway_instance_image_name":   "Ubuntu 18.04 Bionic Beaver",
			"__meta_scaleway_instance_name":         "scw-nervous-shirley",
			"__meta_scaleway_instance_private_ipv4": "10.70.60.57",
			"__meta_scaleway_instance_project_id":   "cb334986-b054-4725-9d5a-30850fdc6015",
			"__meta_scaleway_instance_public_ipv6":  "2001:bc8:630:1e1c::1",
			"__meta_scaleway_instance_status":       "running",
			"__meta_scaleway_instance_type":         "DEV1-S",
			"__meta_scaleway_instance_zone":         "fr-par-1",
		},
		{
			"__address__":                           "10.193.162.9:80",
			"__meta_scaleway_instance_id":           "5b6198b4-c677-41b5-9c05-04557264ae1f",
			"__meta_scaleway_instance_image_name":   "Debian Buster",
			"__meta_scaleway_instance_name":         "scw-quizzical-feistel",
			"__meta_scaleway_instance_private_ipv4": "10.193.162.9",
			"__meta_scaleway_instance_project_id":   "cb334986-b054-4725-9d5a-30850fdc6015",
			"__meta_scaleway_instance_public_ipv4":  "151.115.45.127",
			"__meta_scaleway_instance_public_ipv6":  "2001:bc8:1e00:6204::1",
			"__meta_scaleway_instance_status":       "running",
			"__meta_scaleway_instance_type":         "DEV1-S",
			"__meta_scaleway_instance_zone":         "fr-par-1",
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
	if r.RequestURI != "/instance/v1/zones/fr-par-1/servers?page=1" {
		http.Error(w, "bad url", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	instance, err := ioutil.ReadFile("testdata/instance.json")
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
