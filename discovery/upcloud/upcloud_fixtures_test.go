// Copyright 2025 The Prometheus Authors
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

package upcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestUpCloudRefreshWithFixtures(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(mockUpCloudAPI))
	defer mock.Close()

	cfg := &SDConfig{
		HTTPClientConfig: config.HTTPClientConfig{
			BearerToken: "test-token",
		},
		RefreshInterval: model.Duration(30 * time.Second),
		Port:            9100,
	}

	logger := slog.Default()
	metrics := &upcloudMetrics{
		refreshMetrics: &mockRefreshMetricsInstantiator{},
	}

	// Create discovery with mock server
	t.Setenv("UPCLOUD_TOKEN", "test-token")
	d, err := NewDiscovery(cfg, logger, metrics)
	require.NoError(t, err)

	// Replace the client with one that uses our HTTP fixtures
	d.client = &httpFixtureClient{
		baseURL: mock.URL,
	}

	ctx := context.Background()
	targetGroups, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, targetGroups, 1)
	targetGroup := targetGroups[0]
	require.NotNil(t, targetGroup)
	require.Equal(t, "UpCloud", targetGroup.Source)
	require.Len(t, targetGroup.Targets, 3)

	// Verify first target (web-server-1)
	target1 := targetGroup.Targets[0]
	require.Equal(t, model.LabelValue("192.0.2.1:9100"), target1[model.AddressLabel])
	require.Equal(t, model.LabelValue("00798b85-efdc-41ca-8021-f6ef457b8531"), target1[upcloudServerLabelUUID])
	require.Equal(t, model.LabelValue("web-server-1"), target1[upcloudServerLabelTitle])
	require.Equal(t, model.LabelValue("web1.example.com"), target1[upcloudServerLabelHostname])
	require.Equal(t, model.LabelValue("fi-hel1"), target1[upcloudServerLabelZone])
	require.Equal(t, model.LabelValue("1xCPU-1GB"), target1[upcloudServerLabelPlan])
	require.Equal(t, model.LabelValue("started"), target1[upcloudServerLabelState])
	require.Equal(t, model.LabelValue("1"), target1[upcloudServerLabelCoreNumber])
	require.Equal(t, model.LabelValue("1024"), target1[upcloudServerLabelMemoryAmount])
	require.Equal(t, model.LabelValue("192.0.2.1"), target1[upcloudServerLabelPublicIPv4])
	require.Equal(t, model.LabelValue("10.0.0.1"), target1[upcloudServerLabelPrivateIPv4])
	require.Equal(t, model.LabelValue("2001:db8::1"), target1[upcloudServerLabelPublicIPv6])
	require.Equal(t, model.LabelValue("true"), target1[upcloudServerLabelTag+"production"])
	require.Equal(t, model.LabelValue("true"), target1[upcloudServerLabelTag+"web"])
	require.Equal(t, model.LabelValue("production"), target1[upcloudServerLabelLabel+"environment"])
	require.Equal(t, model.LabelValue("platform"), target1[upcloudServerLabelLabel+"team"])

	// Verify second target (db-server-1)
	target2 := targetGroup.Targets[1]
	require.Equal(t, model.LabelValue("192.0.2.2:9100"), target2[model.AddressLabel])
	require.Equal(t, model.LabelValue("00898b85-efdc-41ca-8021-f6ef457b8532"), target2[upcloudServerLabelUUID])
	require.Equal(t, model.LabelValue("db-server-1"), target2[upcloudServerLabelTitle])
	require.Equal(t, model.LabelValue("2"), target2[upcloudServerLabelCoreNumber])
	require.Equal(t, model.LabelValue("4096"), target2[upcloudServerLabelMemoryAmount])
	require.Equal(t, model.LabelValue("true"), target2[upcloudServerLabelTag+"production"])
	require.Equal(t, model.LabelValue("true"), target2[upcloudServerLabelTag+"database"])
	require.Equal(t, model.LabelValue("production"), target2[upcloudServerLabelLabel+"environment"])
	require.Equal(t, model.LabelValue("database"), target2[upcloudServerLabelLabel+"role"])
	require.Equal(t, model.LabelValue("enabled"), target2[upcloudServerLabelLabel+"backup"])

	// Verify third target (test-server-1) - uses private IP since no public IP
	target3 := targetGroup.Targets[2]
	require.Equal(t, model.LabelValue("10.0.0.3:9100"), target3[model.AddressLabel])
	require.Equal(t, model.LabelValue("00998b85-efdc-41ca-8021-f6ef457b8533"), target3[upcloudServerLabelUUID])
	require.Equal(t, model.LabelValue("test-server-1"), target3[upcloudServerLabelTitle])
	require.Equal(t, model.LabelValue("stopped"), target3[upcloudServerLabelState])
	require.Equal(t, model.LabelValue("true"), target3[upcloudServerLabelTag+"development"])
	require.Equal(t, model.LabelValue("development"), target3[upcloudServerLabelLabel+"environment"])
	require.Equal(t, model.LabelValue("true"), target3[upcloudServerLabelLabel+"temporary"])
}

// httpFixtureClient implements the Client interface using HTTP requests to fixture data.
type httpFixtureClient struct {
	baseURL string
}

func (c *httpFixtureClient) GetServers(_ context.Context) (*upcloud.Servers, error) {
	resp, err := http.Get(c.baseURL + "/1.3/server")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var servers upcloud.Servers
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return nil, err
	}

	return &servers, nil
}

func (c *httpFixtureClient) GetServerDetails(_ context.Context, uuid string) (*upcloud.ServerDetails, error) {
	resp, err := http.Get(c.baseURL + "/1.3/server/" + uuid)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var serverDetails upcloud.ServerDetails
	if err := json.NewDecoder(resp.Body).Decode(&serverDetails); err != nil {
		return nil, err
	}

	return &serverDetails, nil
}

// mockUpCloudAPI simulates the UpCloud API responses using test fixtures.
func mockUpCloudAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/1.3/server":
		serversData, err := os.ReadFile("testdata/servers.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(serversData)

	case "/1.3/server/00798b85-efdc-41ca-8021-f6ef457b8531":
		serverData, err := os.ReadFile("testdata/server_00798b85-efdc-41ca-8021-f6ef457b8531.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(serverData)

	case "/1.3/server/00898b85-efdc-41ca-8021-f6ef457b8532":
		serverData, err := os.ReadFile("testdata/server_00898b85-efdc-41ca-8021-f6ef457b8532.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(serverData)

	case "/1.3/server/00998b85-efdc-41ca-8021-f6ef457b8533":
		serverData, err := os.ReadFile("testdata/server_00998b85-efdc-41ca-8021-f6ef457b8533.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(serverData)

	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
