// Copyright 2022 The Prometheus Authors
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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
	
	file, err := os.Open("testdata/targets.json")
	require.NoError(t, err)
	defer file.Close()

	var jsonTargets []model.LabelSet

	err = json.NewDecoder(file).Decode(&jsonTargets)
	require.NoError(t, err)

	require.Len(t, tg.Targets, len(jsonTargets))

	for i, lbls := range jsonTargets {
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

    limitStr := r.URL.Query().Get("limit")
    offsetStr := r.URL.Query().Get("offset")
    
    limit := 1000 
    offset := 0   

    if limitStr != "" {
        if l, err := strconv.Atoi(limitStr); err == nil {
            limit = l
        }
    }

    if offsetStr != "" {
        if o, err := strconv.Atoi(offsetStr); err == nil {
            offset = o
        }
    }

    w.Header().Set("Content-Type", "application/json")
    
    serverData, err := os.ReadFile("testdata/servers.json")
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    var allServers map[string]interface{}
    if err := json.Unmarshal(serverData, &allServers); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    servers, ok := allServers["items"].([]interface{})
    if !ok {
        http.Error(w, "Invalid server data", http.StatusInternalServerError)
        return
    }

    totalServers := len(servers)
    if offset >= totalServers {
        w.Write([]byte(`{"items":[]}`))
        return
    }

    end := offset + limit
    if end > totalServers {
        end = totalServers
    }

    paginatedServers := servers[offset:end]

    allServers["items"] = paginatedServers

    paginatedData, err := json.Marshal(allServers)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    _, err = w.Write(paginatedData)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}
