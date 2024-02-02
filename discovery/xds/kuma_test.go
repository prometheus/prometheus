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

package xds

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	kumaConf = sdConf

	testKumaMadsV1Resources = []*MonitoringAssignment{
		{
			Mesh:    "metrics",
			Service: "prometheus",
			Targets: []*MonitoringAssignment_Target{
				{
					Name:        "prometheus-01",
					Scheme:      "http",
					Address:     "10.1.4.32:9090",
					MetricsPath: "/custom-metrics",
					Labels: map[string]string{
						"commit_hash": "620506a88",
					},
				},
				{
					Name:    "prometheus-02",
					Scheme:  "http",
					Address: "10.1.4.33:9090",
					Labels: map[string]string{
						"commit_hash": "3513bba00",
					},
				},
			},
			Labels: map[string]string{
				"kuma.io/zone": "us-east-1",
				"team":         "infra",
			},
		},
		{
			Mesh:    "metrics",
			Service: "grafana",
			Targets: []*MonitoringAssignment_Target{},
			Labels: map[string]string{
				"kuma.io/zone": "us-east-1",
				"team":         "infra",
			},
		},
		{
			Mesh:    "data",
			Service: "elasticsearch",
			Targets: []*MonitoringAssignment_Target{
				{
					Name:    "elasticsearch-01",
					Scheme:  "http",
					Address: "10.1.1.1",
					Labels: map[string]string{
						"role": "ml",
					},
				},
			},
		},
	}
)

func getKumaMadsV1DiscoveryResponse(resources ...*MonitoringAssignment) (*v3.DiscoveryResponse, error) {
	serialized := make([]*anypb.Any, len(resources))
	for i, res := range resources {
		data, err := proto.Marshal(res)
		if err != nil {
			return nil, err
		}

		serialized[i] = &anypb.Any{
			TypeUrl: KumaMadsV1ResourceTypeURL,
			Value:   data,
		}
	}
	return &v3.DiscoveryResponse{
		TypeUrl:   KumaMadsV1ResourceTypeURL,
		Resources: serialized,
	}, nil
}

func newKumaTestHTTPDiscovery(c KumaSDConfig) (*fetchDiscovery, error) {
	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	// TODO(ptodev): Add the ability to unregister refresh metrics.
	metrics := c.NewDiscovererMetrics(reg, refreshMetrics)
	err := metrics.Register()
	if err != nil {
		return nil, err
	}

	kd, err := NewKumaHTTPDiscovery(&c, nopLogger, metrics)
	if err != nil {
		return nil, err
	}

	pd, ok := kd.(*fetchDiscovery)
	if !ok {
		return nil, errors.New("not a fetchDiscovery")
	}
	return pd, nil
}

func TestKumaMadsV1ResourceParserInvalidTypeURL(t *testing.T) {
	resources := make([]*anypb.Any, 0)
	groups, err := kumaMadsV1ResourceParser(resources, "type.googleapis.com/some.api.v1.Monitoring")
	require.Nil(t, groups)
	require.Error(t, err)
}

func TestKumaMadsV1ResourceParserEmptySlice(t *testing.T) {
	resources := make([]*anypb.Any, 0)
	groups, err := kumaMadsV1ResourceParser(resources, KumaMadsV1ResourceTypeURL)
	require.Empty(t, groups)
	require.NoError(t, err)
}

func TestKumaMadsV1ResourceParserValidResources(t *testing.T) {
	res, err := getKumaMadsV1DiscoveryResponse(testKumaMadsV1Resources...)
	require.NoError(t, err)

	targets, err := kumaMadsV1ResourceParser(res.Resources, KumaMadsV1ResourceTypeURL)
	require.NoError(t, err)
	require.Len(t, targets, 3)

	expectedTargets := []model.LabelSet{
		{
			"__address__":                    "10.1.4.32:9090",
			"__metrics_path__":               "/custom-metrics",
			"__scheme__":                     "http",
			"instance":                       "prometheus-01",
			"__meta_kuma_mesh":               "metrics",
			"__meta_kuma_service":            "prometheus",
			"__meta_kuma_label_team":         "infra",
			"__meta_kuma_label_kuma_io_zone": "us-east-1",
			"__meta_kuma_label_commit_hash":  "620506a88",
			"__meta_kuma_dataplane":          "prometheus-01",
		},
		{
			"__address__":                    "10.1.4.33:9090",
			"__metrics_path__":               "",
			"__scheme__":                     "http",
			"instance":                       "prometheus-02",
			"__meta_kuma_mesh":               "metrics",
			"__meta_kuma_service":            "prometheus",
			"__meta_kuma_label_team":         "infra",
			"__meta_kuma_label_kuma_io_zone": "us-east-1",
			"__meta_kuma_label_commit_hash":  "3513bba00",
			"__meta_kuma_dataplane":          "prometheus-02",
		},
		{
			"__address__":            "10.1.1.1",
			"__metrics_path__":       "",
			"__scheme__":             "http",
			"instance":               "elasticsearch-01",
			"__meta_kuma_mesh":       "data",
			"__meta_kuma_service":    "elasticsearch",
			"__meta_kuma_label_role": "ml",
			"__meta_kuma_dataplane":  "elasticsearch-01",
		},
	}
	require.Equal(t, expectedTargets, targets)
}

func TestKumaMadsV1ResourceParserInvalidResources(t *testing.T) {
	data, err := protoJSONMarshalOptions.Marshal(&MonitoringAssignment_Target{})
	require.NoError(t, err)

	resources := []*anypb.Any{{
		TypeUrl: KumaMadsV1ResourceTypeURL,
		Value:   data,
	}}
	groups, err := kumaMadsV1ResourceParser(resources, KumaMadsV1ResourceTypeURL)
	require.Nil(t, groups)
	require.Error(t, err)

	require.Contains(t, err.Error(), "cannot parse")
}

func TestNewKumaHTTPDiscovery(t *testing.T) {
	kd, err := newKumaTestHTTPDiscovery(kumaConf)
	require.NoError(t, err)
	require.NotNil(t, kd)

	resClient, ok := kd.client.(*HTTPResourceClient)
	require.True(t, ok)
	require.Equal(t, kumaConf.Server, resClient.Server())
	require.Equal(t, KumaMadsV1ResourceTypeURL, resClient.ResourceTypeURL())
	require.Equal(t, kumaConf.ClientID, resClient.ID())
	require.Equal(t, KumaMadsV1ResourceType, resClient.config.ResourceType)

	kd.metrics.Unregister()
}

func TestKumaHTTPDiscoveryRefresh(t *testing.T) {
	s := createTestHTTPServer(t, func(request *v3.DiscoveryRequest) (*v3.DiscoveryResponse, error) {
		if request.VersionInfo == "1" {
			return nil, nil
		}

		res, err := getKumaMadsV1DiscoveryResponse(testKumaMadsV1Resources...)
		require.NoError(t, err)

		res.VersionInfo = "1"
		res.Nonce = "abc"

		return res, nil
	})
	defer s.Close()

	cfgString := fmt.Sprintf(`
---
server: %s
refresh_interval: 10s
tls_config:
  insecure_skip_verify: true
`, s.URL)

	var cfg KumaSDConfig
	require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))

	kd, err := newKumaTestHTTPDiscovery(cfg)
	require.NoError(t, err)
	require.NotNil(t, kd)

	ch := make(chan []*targetgroup.Group, 1)
	kd.poll(context.Background(), ch)

	groups := <-ch
	require.Len(t, groups, 1)

	targets := groups[0].Targets
	require.Len(t, targets, 3)

	expectedTargets := []model.LabelSet{
		{
			"__address__":                    "10.1.4.32:9090",
			"__metrics_path__":               "/custom-metrics",
			"__scheme__":                     "http",
			"instance":                       "prometheus-01",
			"__meta_kuma_mesh":               "metrics",
			"__meta_kuma_service":            "prometheus",
			"__meta_kuma_label_team":         "infra",
			"__meta_kuma_label_kuma_io_zone": "us-east-1",
			"__meta_kuma_label_commit_hash":  "620506a88",
			"__meta_kuma_dataplane":          "prometheus-01",
		},
		{
			"__address__":                    "10.1.4.33:9090",
			"__metrics_path__":               "",
			"__scheme__":                     "http",
			"instance":                       "prometheus-02",
			"__meta_kuma_mesh":               "metrics",
			"__meta_kuma_service":            "prometheus",
			"__meta_kuma_label_team":         "infra",
			"__meta_kuma_label_kuma_io_zone": "us-east-1",
			"__meta_kuma_label_commit_hash":  "3513bba00",
			"__meta_kuma_dataplane":          "prometheus-02",
		},
		{
			"__address__":            "10.1.1.1",
			"__metrics_path__":       "",
			"__scheme__":             "http",
			"instance":               "elasticsearch-01",
			"__meta_kuma_mesh":       "data",
			"__meta_kuma_service":    "elasticsearch",
			"__meta_kuma_label_role": "ml",
			"__meta_kuma_dataplane":  "elasticsearch-01",
		},
	}
	require.Equal(t, expectedTargets, targets)

	// Should skip the next update.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	kd.poll(ctx, ch)
	select {
	case <-ctx.Done():
		return
	case <-ch:
		require.Fail(t, "no update expected")
	}

	kd.metrics.Unregister()
}
