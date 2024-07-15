// Copyright 2020 The Prometheus Authors
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

package digitalocean

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery"
)

type DigitalOceanSDTestSuite struct {
	Mock *SDMock
}

func (s *DigitalOceanSDTestSuite) TearDownSuite() {
	s.Mock.ShutdownServer()
}

func (s *DigitalOceanSDTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleDropletsList()
}

func TestDigitalOceanSDRefresh(t *testing.T) {
	sdmock := &DigitalOceanSDTestSuite{}
	sdmock.SetupTest(t)
	t.Cleanup(sdmock.TearDownSuite)

	cfg := DefaultSDConfig
	cfg.HTTPClientConfig.BearerToken = tokenID

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	d, err := NewDiscovery(&cfg, log.NewNopLogger(), metrics)
	require.NoError(t, err)
	endpoint, err := url.Parse(sdmock.Mock.Endpoint())
	require.NoError(t, err)
	d.client.BaseURL = endpoint

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 4)

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                      model.LabelValue("104.236.32.182:80"),
			"__meta_digitalocean_droplet_id":   model.LabelValue("3164444"),
			"__meta_digitalocean_droplet_name": model.LabelValue("example.com"),
			"__meta_digitalocean_image":        model.LabelValue("ubuntu-16-04-x64"),
			"__meta_digitalocean_image_name":   model.LabelValue("14.04 x64"),
			"__meta_digitalocean_private_ipv4": model.LabelValue(""),
			"__meta_digitalocean_public_ipv4":  model.LabelValue("104.236.32.182"),
			"__meta_digitalocean_public_ipv6":  model.LabelValue("2604:A880:0800:0010:0000:0000:02DD:4001"),
			"__meta_digitalocean_region":       model.LabelValue("nyc3"),
			"__meta_digitalocean_size":         model.LabelValue("s-1vcpu-1gb"),
			"__meta_digitalocean_status":       model.LabelValue("active"),
			"__meta_digitalocean_vpc":          model.LabelValue("f9b0769c-e118-42fb-a0c4-fed15ef69662"),
			"__meta_digitalocean_features":     model.LabelValue(",backups,ipv6,virtio,"),
		},
		{
			"__address__":                      model.LabelValue("104.131.186.241:80"),
			"__meta_digitalocean_droplet_id":   model.LabelValue("3164494"),
			"__meta_digitalocean_droplet_name": model.LabelValue("prometheus"),
			"__meta_digitalocean_image":        model.LabelValue("ubuntu-16-04-x64"),
			"__meta_digitalocean_image_name":   model.LabelValue("14.04 x64"),
			"__meta_digitalocean_private_ipv4": model.LabelValue(""),
			"__meta_digitalocean_public_ipv4":  model.LabelValue("104.131.186.241"),
			"__meta_digitalocean_public_ipv6":  model.LabelValue(""),
			"__meta_digitalocean_region":       model.LabelValue("nyc3"),
			"__meta_digitalocean_size":         model.LabelValue("s-1vcpu-1gb"),
			"__meta_digitalocean_status":       model.LabelValue("active"),
			"__meta_digitalocean_vpc":          model.LabelValue("f9b0769c-e118-42fb-a0c4-fed15ef69662"),
			"__meta_digitalocean_tags":         model.LabelValue(",monitor,"),
			"__meta_digitalocean_features":     model.LabelValue(",virtio,"),
		},
		{
			"__address__":                      model.LabelValue("167.172.111.118:80"),
			"__meta_digitalocean_droplet_id":   model.LabelValue("175072239"),
			"__meta_digitalocean_droplet_name": model.LabelValue("prometheus-demo-old"),
			"__meta_digitalocean_image":        model.LabelValue("ubuntu-18-04-x64"),
			"__meta_digitalocean_image_name":   model.LabelValue("18.04.3 (LTS) x64"),
			"__meta_digitalocean_private_ipv4": model.LabelValue("10.135.64.211"),
			"__meta_digitalocean_public_ipv4":  model.LabelValue("167.172.111.118"),
			"__meta_digitalocean_public_ipv6":  model.LabelValue(""),
			"__meta_digitalocean_region":       model.LabelValue("fra1"),
			"__meta_digitalocean_size":         model.LabelValue("s-1vcpu-1gb"),
			"__meta_digitalocean_status":       model.LabelValue("off"),
			"__meta_digitalocean_vpc":          model.LabelValue("953d698c-dc84-11e8-80bc-3cfdfea9fba1"),
			"__meta_digitalocean_features":     model.LabelValue(",ipv6,private_networking,"),
		},
		{
			"__address__":                      model.LabelValue("138.65.56.69:80"),
			"__meta_digitalocean_droplet_id":   model.LabelValue("176011507"),
			"__meta_digitalocean_droplet_name": model.LabelValue("prometheus-demo"),
			"__meta_digitalocean_image":        model.LabelValue("ubuntu-18-04-x64"),
			"__meta_digitalocean_image_name":   model.LabelValue("18.04.3 (LTS) x64"),
			"__meta_digitalocean_private_ipv4": model.LabelValue("10.135.64.212"),
			"__meta_digitalocean_public_ipv4":  model.LabelValue("138.65.56.69"),
			"__meta_digitalocean_public_ipv6":  model.LabelValue("2a03:b0c0:3:f0::cf2:4"),
			"__meta_digitalocean_region":       model.LabelValue("fra1"),
			"__meta_digitalocean_size":         model.LabelValue("s-1vcpu-1gb"),
			"__meta_digitalocean_status":       model.LabelValue("active"),
			"__meta_digitalocean_vpc":          model.LabelValue("953d698c-dc84-11e8-80bc-3cfdfea9fba1"),
			"__meta_digitalocean_features":     model.LabelValue(",ipv6,private_networking,"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}
