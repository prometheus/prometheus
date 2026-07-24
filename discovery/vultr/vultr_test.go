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

package vultr

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery"
)

type VultrSDTestSuite struct {
	Mock *SDMock
}

func (s *VultrSDTestSuite) TearDownSuite() {
	s.Mock.ShutdownServer()
}

func (s *VultrSDTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleInstanceList()
	s.Mock.HandleBaremetalList()
}

func TestVultrSDRefresh(t *testing.T) {
	sdMock := &VultrSDTestSuite{}
	sdMock.SetupTest(t)
	t.Cleanup(sdMock.TearDownSuite)

	cfg := DefaultSDConfig
	cfg.HTTPClientConfig.BearerToken = APIKey

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "vultr",
	})
	require.NoError(t, err)
	endpoint, err := url.Parse(sdMock.Mock.Endpoint())
	require.NoError(t, err)
	d.client.BaseURL = endpoint

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 6)

	for i, k := range []model.LabelSet{
		{
			"__address__":                                model.LabelValue("149.28.234.27:80"),
			"__meta_vultr_instance_id":                   model.LabelValue("dbdbd38c-9884-4c92-95fe-899e50dee717"),
			"__meta_vultr_instance_label":                model.LabelValue("np-2-eae38a19b0f3"),
			"__meta_vultr_instance_os":                   model.LabelValue("Marketplace"),
			"__meta_vultr_instance_os_id":                model.LabelValue("426"),
			"__meta_vultr_instance_region":               model.LabelValue("ewr"),
			"__meta_vultr_instance_plan":                 model.LabelValue("vhf-2c-4gb"),
			"__meta_vultr_instance_main_ip":              model.LabelValue("149.28.234.27"),
			"__meta_vultr_instance_internal_ip":          model.LabelValue("10.1.96.5"),
			"__meta_vultr_instance_main_ipv6":            model.LabelValue(""),
			"__meta_vultr_instance_features":             model.LabelValue(",backups,"),
			"__meta_vultr_instance_tags":                 model.LabelValue(",tag1,tag2,tag3,"),
			"__meta_vultr_instance_hostname":             model.LabelValue("np-2-eae38a19b0f3"),
			"__meta_vultr_instance_server_status":        model.LabelValue("ok"),
			"__meta_vultr_instance_vcpu_count":           model.LabelValue("2"),
			"__meta_vultr_instance_ram_mb":               model.LabelValue("4096"),
			"__meta_vultr_instance_disk_gb":              model.LabelValue("128"),
			"__meta_vultr_instance_allowed_bandwidth_gb": model.LabelValue("3000"),
		},
		{
			"__address__":                                model.LabelValue("45.63.1.222:80"),
			"__meta_vultr_instance_id":                   model.LabelValue("fccb117c-62f7-4b17-995d-a8e56dd30b33"),
			"__meta_vultr_instance_label":                model.LabelValue("np-2-fd0714b5fe42"),
			"__meta_vultr_instance_os":                   model.LabelValue("Marketplace"),
			"__meta_vultr_instance_os_id":                model.LabelValue("426"),
			"__meta_vultr_instance_region":               model.LabelValue("ewr"),
			"__meta_vultr_instance_plan":                 model.LabelValue("vhf-2c-4gb"),
			"__meta_vultr_instance_main_ip":              model.LabelValue("45.63.1.222"),
			"__meta_vultr_instance_internal_ip":          model.LabelValue("10.1.96.6"),
			"__meta_vultr_instance_main_ipv6":            model.LabelValue(""),
			"__meta_vultr_instance_hostname":             model.LabelValue("np-2-fd0714b5fe42"),
			"__meta_vultr_instance_server_status":        model.LabelValue("ok"),
			"__meta_vultr_instance_vcpu_count":           model.LabelValue("2"),
			"__meta_vultr_instance_ram_mb":               model.LabelValue("4096"),
			"__meta_vultr_instance_disk_gb":              model.LabelValue("128"),
			"__meta_vultr_instance_allowed_bandwidth_gb": model.LabelValue("3000"),
		},
		{
			"__address__":                                model.LabelValue("149.28.237.151:80"),
			"__meta_vultr_instance_id":                   model.LabelValue("c4e58767-f61b-4c5e-9bce-43761cc04868"),
			"__meta_vultr_instance_label":                model.LabelValue("np-2-d04e7829fa43"),
			"__meta_vultr_instance_os":                   model.LabelValue("Marketplace"),
			"__meta_vultr_instance_os_id":                model.LabelValue("426"),
			"__meta_vultr_instance_region":               model.LabelValue("ewr"),
			"__meta_vultr_instance_plan":                 model.LabelValue("vhf-2c-4gb"),
			"__meta_vultr_instance_main_ip":              model.LabelValue("149.28.237.151"),
			"__meta_vultr_instance_internal_ip":          model.LabelValue("10.1.96.7"),
			"__meta_vultr_instance_main_ipv6":            model.LabelValue(""),
			"__meta_vultr_instance_hostname":             model.LabelValue("np-2-d04e7829fa43"),
			"__meta_vultr_instance_server_status":        model.LabelValue("ok"),
			"__meta_vultr_instance_vcpu_count":           model.LabelValue("2"),
			"__meta_vultr_instance_ram_mb":               model.LabelValue("4096"),
			"__meta_vultr_instance_disk_gb":              model.LabelValue("128"),
			"__meta_vultr_instance_allowed_bandwidth_gb": model.LabelValue("3000"),
		},
		{
			"__address__":                          model.LabelValue("192.0.2.123:80"),
			"__meta_vultr_baremetal_id":            model.LabelValue("cb676a46-66fd-4dfb-b839-443f2e6c0b60"),
			"__meta_vultr_baremetal_label":         model.LabelValue("Example Bare Metal"),
			"__meta_vultr_baremetal_os":            model.LabelValue("Application"),
			"__meta_vultr_baremetal_ram_mb":        model.LabelValue("32768 MB"),
			"__meta_vultr_baremetal_disk_gb":       model.LabelValue("2x 240GB SSD"),
			"__meta_vultr_baremetal_main_ip":       model.LabelValue("192.0.2.123"),
			"__meta_vultr_baremetal_cpu_count":     model.LabelValue("4"),
			"__meta_vultr_baremetal_region":        model.LabelValue("ams"),
			"__meta_vultr_baremetal_plan":          model.LabelValue("vbm-4c-32gb"),
			"__meta_vultr_baremetal_server_status": model.LabelValue("active"),
			"__meta_vultr_baremetal_netmask_v4":    model.LabelValue("255.255.254.0"),
			"__meta_vultr_baremetal_gateway_v4":    model.LabelValue("192.0.2.1"),
			"__meta_vultr_baremetal_main_ipv6":     model.LabelValue("2001:0db8:5001:3990:0ec4:7aff:fe8e:f97a"),
			"__meta_vultr_baremetal_os_id":         model.LabelValue("186"),
			"__meta_vultr_baremetal_app_id":        model.LabelValue("3"),
			"__meta_vultr_baremetal_image_id":      model.LabelValue(""),
			"__meta_vultr_baremetal_features":      model.LabelValue(",backups,"),
			"__meta_vultr_baremetal_tags":          model.LabelValue(",tag1,tag2,tag3,"),
		},
		{
			"__address__":                          model.LabelValue("192.0.2.124:80"),
			"__meta_vultr_baremetal_id":            model.LabelValue("sdfgsdfgsdfg-66fd-4dfb-b839-443f2e6c0b60"),
			"__meta_vultr_baremetal_label":         model.LabelValue("Example Bare Metal 2"),
			"__meta_vultr_baremetal_os":            model.LabelValue("Application"),
			"__meta_vultr_baremetal_ram_mb":        model.LabelValue("32768 MB"),
			"__meta_vultr_baremetal_disk_gb":       model.LabelValue("2x 240GB SSD"),
			"__meta_vultr_baremetal_main_ip":       model.LabelValue("192.0.2.124"),
			"__meta_vultr_baremetal_cpu_count":     model.LabelValue("4"),
			"__meta_vultr_baremetal_region":        model.LabelValue("ams"),
			"__meta_vultr_baremetal_plan":          model.LabelValue("vbm-4c-32gb"),
			"__meta_vultr_baremetal_server_status": model.LabelValue("active"),
			"__meta_vultr_baremetal_netmask_v4":    model.LabelValue("255.255.254.0"),
			"__meta_vultr_baremetal_gateway_v4":    model.LabelValue("192.0.2.1"),
			"__meta_vultr_baremetal_main_ipv6":     model.LabelValue("2001:0db8:5001:3990:0ec4:7aff:fe8e:f97a"),
			"__meta_vultr_baremetal_os_id":         model.LabelValue("186"),
			"__meta_vultr_baremetal_app_id":        model.LabelValue("3"),
			"__meta_vultr_baremetal_image_id":      model.LabelValue(""),
			"__meta_vultr_baremetal_features":      model.LabelValue(",backups,"),
			"__meta_vultr_baremetal_tags":          model.LabelValue(",tag1,tag2,tag3,"),
		},
		{
			"__address__":                          model.LabelValue("192.0.2.125:80"),
			"__meta_vultr_baremetal_id":            model.LabelValue("wertwery6-66fd-4dfb-b839-443f2e6c0b60"),
			"__meta_vultr_baremetal_label":         model.LabelValue("Example Bare Metal 3"),
			"__meta_vultr_baremetal_os":            model.LabelValue("Application"),
			"__meta_vultr_baremetal_ram_mb":        model.LabelValue("32768 MB"),
			"__meta_vultr_baremetal_disk_gb":       model.LabelValue("2x 240GB SSD"),
			"__meta_vultr_baremetal_main_ip":       model.LabelValue("192.0.2.125"),
			"__meta_vultr_baremetal_cpu_count":     model.LabelValue("4"),
			"__meta_vultr_baremetal_region":        model.LabelValue("ams"),
			"__meta_vultr_baremetal_plan":          model.LabelValue("vbm-4c-32gb"),
			"__meta_vultr_baremetal_server_status": model.LabelValue("active"),
			"__meta_vultr_baremetal_netmask_v4":    model.LabelValue("255.255.254.0"),
			"__meta_vultr_baremetal_gateway_v4":    model.LabelValue("192.0.2.1"),
			"__meta_vultr_baremetal_main_ipv6":     model.LabelValue("2001:0db8:5001:3990:0ec4:7aff:fe8e:f97a"),
			"__meta_vultr_baremetal_os_id":         model.LabelValue("186"),
			"__meta_vultr_baremetal_app_id":        model.LabelValue("3"),
			"__meta_vultr_baremetal_image_id":      model.LabelValue(""),
			"__meta_vultr_baremetal_features":      model.LabelValue(",backups,"),
			"__meta_vultr_baremetal_tags":          model.LabelValue(",tag1,tag2,tag3,"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, k, tg.Targets[i])
		})
	}
}
