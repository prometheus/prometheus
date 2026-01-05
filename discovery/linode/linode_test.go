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

package linode

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery"
)

func TestLinodeSDRefresh(t *testing.T) {
	sdmock := NewSDMock(t)
	sdmock.Setup()

	tests := map[string]struct {
		region      string
		targetCount int
		want        []model.LabelSet
	}{
		"no_region": {region: "", targetCount: 4, want: []model.LabelSet{
			{
				"__address__":                        model.LabelValue("45.33.82.151:80"),
				"__meta_linode_instance_id":          model.LabelValue("26838044"),
				"__meta_linode_instance_label":       model.LabelValue("prometheus-linode-sd-exporter-1"),
				"__meta_linode_image":                model.LabelValue("linode/arch"),
				"__meta_linode_private_ipv4":         model.LabelValue("192.168.170.51"),
				"__meta_linode_public_ipv4":          model.LabelValue("45.33.82.151"),
				"__meta_linode_public_ipv6":          model.LabelValue("2600:3c03::f03c:92ff:fe1a:1382"),
				"__meta_linode_private_ipv4_rdns":    model.LabelValue(""),
				"__meta_linode_public_ipv4_rdns":     model.LabelValue("li1028-151.members.linode.com"),
				"__meta_linode_public_ipv6_rdns":     model.LabelValue(""),
				"__meta_linode_region":               model.LabelValue("us-east"),
				"__meta_linode_type":                 model.LabelValue("g6-standard-2"),
				"__meta_linode_status":               model.LabelValue("running"),
				"__meta_linode_tags":                 model.LabelValue(",monitoring,"),
				"__meta_linode_group":                model.LabelValue(""),
				"__meta_linode_gpus":                 model.LabelValue("0"),
				"__meta_linode_hypervisor":           model.LabelValue("kvm"),
				"__meta_linode_backups":              model.LabelValue("disabled"),
				"__meta_linode_specs_disk_bytes":     model.LabelValue("85899345920"),
				"__meta_linode_specs_memory_bytes":   model.LabelValue("4294967296"),
				"__meta_linode_specs_vcpus":          model.LabelValue("2"),
				"__meta_linode_specs_transfer_bytes": model.LabelValue("4194304000"),
				"__meta_linode_extra_ips":            model.LabelValue(",96.126.108.16,192.168.201.25,"),
			},
			{
				"__address__":                        model.LabelValue("139.162.196.43:80"),
				"__meta_linode_instance_id":          model.LabelValue("26848419"),
				"__meta_linode_instance_label":       model.LabelValue("prometheus-linode-sd-exporter-2"),
				"__meta_linode_image":                model.LabelValue("linode/debian10"),
				"__meta_linode_private_ipv4":         model.LabelValue(""),
				"__meta_linode_public_ipv4":          model.LabelValue("139.162.196.43"),
				"__meta_linode_public_ipv6":          model.LabelValue("2a01:7e00::f03c:92ff:fe1a:9976"),
				"__meta_linode_private_ipv4_rdns":    model.LabelValue(""),
				"__meta_linode_public_ipv4_rdns":     model.LabelValue("li1359-43.members.linode.com"),
				"__meta_linode_public_ipv6_rdns":     model.LabelValue(""),
				"__meta_linode_region":               model.LabelValue("eu-west"),
				"__meta_linode_type":                 model.LabelValue("g6-standard-2"),
				"__meta_linode_status":               model.LabelValue("running"),
				"__meta_linode_tags":                 model.LabelValue(",monitoring,"),
				"__meta_linode_group":                model.LabelValue(""),
				"__meta_linode_gpus":                 model.LabelValue("0"),
				"__meta_linode_hypervisor":           model.LabelValue("kvm"),
				"__meta_linode_backups":              model.LabelValue("disabled"),
				"__meta_linode_specs_disk_bytes":     model.LabelValue("85899345920"),
				"__meta_linode_specs_memory_bytes":   model.LabelValue("4294967296"),
				"__meta_linode_specs_vcpus":          model.LabelValue("2"),
				"__meta_linode_specs_transfer_bytes": model.LabelValue("4194304000"),
			},
			{
				"__address__":                        model.LabelValue("192.53.120.25:80"),
				"__meta_linode_instance_id":          model.LabelValue("26837938"),
				"__meta_linode_instance_label":       model.LabelValue("prometheus-linode-sd-exporter-3"),
				"__meta_linode_image":                model.LabelValue("linode/ubuntu20.04"),
				"__meta_linode_private_ipv4":         model.LabelValue(""),
				"__meta_linode_public_ipv4":          model.LabelValue("192.53.120.25"),
				"__meta_linode_public_ipv6":          model.LabelValue("2600:3c04::f03c:92ff:fe1a:fb68"),
				"__meta_linode_private_ipv4_rdns":    model.LabelValue(""),
				"__meta_linode_public_ipv4_rdns":     model.LabelValue("li2216-25.members.linode.com"),
				"__meta_linode_public_ipv6_rdns":     model.LabelValue(""),
				"__meta_linode_region":               model.LabelValue("ca-central"),
				"__meta_linode_type":                 model.LabelValue("g6-standard-1"),
				"__meta_linode_status":               model.LabelValue("running"),
				"__meta_linode_tags":                 model.LabelValue(",monitoring,"),
				"__meta_linode_group":                model.LabelValue(""),
				"__meta_linode_gpus":                 model.LabelValue("0"),
				"__meta_linode_hypervisor":           model.LabelValue("kvm"),
				"__meta_linode_backups":              model.LabelValue("disabled"),
				"__meta_linode_specs_disk_bytes":     model.LabelValue("53687091200"),
				"__meta_linode_specs_memory_bytes":   model.LabelValue("2147483648"),
				"__meta_linode_specs_vcpus":          model.LabelValue("1"),
				"__meta_linode_specs_transfer_bytes": model.LabelValue("2097152000"),
				"__meta_linode_ipv6_ranges":          model.LabelValue(",2600:3c04:e001:456::/64,"),
			},
			{
				"__address__":                        model.LabelValue("66.228.47.103:80"),
				"__meta_linode_instance_id":          model.LabelValue("26837992"),
				"__meta_linode_instance_label":       model.LabelValue("prometheus-linode-sd-exporter-4"),
				"__meta_linode_image":                model.LabelValue("linode/ubuntu20.04"),
				"__meta_linode_private_ipv4":         model.LabelValue("192.168.148.94"),
				"__meta_linode_public_ipv4":          model.LabelValue("66.228.47.103"),
				"__meta_linode_public_ipv6":          model.LabelValue("2600:3c03::f03c:92ff:fe1a:fb4c"),
				"__meta_linode_private_ipv4_rdns":    model.LabelValue(""),
				"__meta_linode_public_ipv4_rdns":     model.LabelValue("li328-103.members.linode.com"),
				"__meta_linode_public_ipv6_rdns":     model.LabelValue(""),
				"__meta_linode_region":               model.LabelValue("us-east"),
				"__meta_linode_type":                 model.LabelValue("g6-nanode-1"),
				"__meta_linode_status":               model.LabelValue("running"),
				"__meta_linode_tags":                 model.LabelValue(",monitoring,"),
				"__meta_linode_group":                model.LabelValue(""),
				"__meta_linode_gpus":                 model.LabelValue("0"),
				"__meta_linode_hypervisor":           model.LabelValue("kvm"),
				"__meta_linode_backups":              model.LabelValue("disabled"),
				"__meta_linode_specs_disk_bytes":     model.LabelValue("26843545600"),
				"__meta_linode_specs_memory_bytes":   model.LabelValue("1073741824"),
				"__meta_linode_specs_vcpus":          model.LabelValue("1"),
				"__meta_linode_specs_transfer_bytes": model.LabelValue("1048576000"),
				"__meta_linode_extra_ips":            model.LabelValue(",172.104.18.104,"),
				"__meta_linode_ipv6_ranges":          model.LabelValue(",2600:3c03:e000:123::/64,"),
			},
		}},
		"us-east": {region: "us-east", targetCount: 2, want: []model.LabelSet{
			{
				"__address__":                        model.LabelValue("45.33.82.151:80"),
				"__meta_linode_instance_id":          model.LabelValue("26838044"),
				"__meta_linode_instance_label":       model.LabelValue("prometheus-linode-sd-exporter-1"),
				"__meta_linode_image":                model.LabelValue("linode/arch"),
				"__meta_linode_private_ipv4":         model.LabelValue("192.168.170.51"),
				"__meta_linode_public_ipv4":          model.LabelValue("45.33.82.151"),
				"__meta_linode_public_ipv6":          model.LabelValue("2600:3c03::f03c:92ff:fe1a:1382"),
				"__meta_linode_private_ipv4_rdns":    model.LabelValue(""),
				"__meta_linode_public_ipv4_rdns":     model.LabelValue("li1028-151.members.linode.com"),
				"__meta_linode_public_ipv6_rdns":     model.LabelValue(""),
				"__meta_linode_region":               model.LabelValue("us-east"),
				"__meta_linode_type":                 model.LabelValue("g6-standard-2"),
				"__meta_linode_status":               model.LabelValue("running"),
				"__meta_linode_tags":                 model.LabelValue(",monitoring,"),
				"__meta_linode_group":                model.LabelValue(""),
				"__meta_linode_gpus":                 model.LabelValue("0"),
				"__meta_linode_hypervisor":           model.LabelValue("kvm"),
				"__meta_linode_backups":              model.LabelValue("disabled"),
				"__meta_linode_specs_disk_bytes":     model.LabelValue("85899345920"),
				"__meta_linode_specs_memory_bytes":   model.LabelValue("4294967296"),
				"__meta_linode_specs_vcpus":          model.LabelValue("2"),
				"__meta_linode_specs_transfer_bytes": model.LabelValue("4194304000"),
				"__meta_linode_extra_ips":            model.LabelValue(",96.126.108.16,192.168.201.25,"),
			},
			{
				"__address__":                        model.LabelValue("66.228.47.103:80"),
				"__meta_linode_instance_id":          model.LabelValue("26837992"),
				"__meta_linode_instance_label":       model.LabelValue("prometheus-linode-sd-exporter-4"),
				"__meta_linode_image":                model.LabelValue("linode/ubuntu20.04"),
				"__meta_linode_private_ipv4":         model.LabelValue("192.168.148.94"),
				"__meta_linode_public_ipv4":          model.LabelValue("66.228.47.103"),
				"__meta_linode_public_ipv6":          model.LabelValue("2600:3c03::f03c:92ff:fe1a:fb4c"),
				"__meta_linode_private_ipv4_rdns":    model.LabelValue(""),
				"__meta_linode_public_ipv4_rdns":     model.LabelValue("li328-103.members.linode.com"),
				"__meta_linode_public_ipv6_rdns":     model.LabelValue(""),
				"__meta_linode_region":               model.LabelValue("us-east"),
				"__meta_linode_type":                 model.LabelValue("g6-nanode-1"),
				"__meta_linode_status":               model.LabelValue("running"),
				"__meta_linode_tags":                 model.LabelValue(",monitoring,"),
				"__meta_linode_group":                model.LabelValue(""),
				"__meta_linode_gpus":                 model.LabelValue("0"),
				"__meta_linode_hypervisor":           model.LabelValue("kvm"),
				"__meta_linode_backups":              model.LabelValue("disabled"),
				"__meta_linode_specs_disk_bytes":     model.LabelValue("26843545600"),
				"__meta_linode_specs_memory_bytes":   model.LabelValue("1073741824"),
				"__meta_linode_specs_vcpus":          model.LabelValue("1"),
				"__meta_linode_specs_transfer_bytes": model.LabelValue("1048576000"),
				"__meta_linode_extra_ips":            model.LabelValue(",172.104.18.104,"),
				"__meta_linode_ipv6_ranges":          model.LabelValue(",2600:3c03:e000:123::/64,"),
			},
		}},
		"us-central": {region: "ca-central", targetCount: 1, want: []model.LabelSet{
			{
				"__address__":                        model.LabelValue("192.53.120.25:80"),
				"__meta_linode_instance_id":          model.LabelValue("26837938"),
				"__meta_linode_instance_label":       model.LabelValue("prometheus-linode-sd-exporter-3"),
				"__meta_linode_image":                model.LabelValue("linode/ubuntu20.04"),
				"__meta_linode_private_ipv4":         model.LabelValue(""),
				"__meta_linode_public_ipv4":          model.LabelValue("192.53.120.25"),
				"__meta_linode_public_ipv6":          model.LabelValue("2600:3c04::f03c:92ff:fe1a:fb68"),
				"__meta_linode_private_ipv4_rdns":    model.LabelValue(""),
				"__meta_linode_public_ipv4_rdns":     model.LabelValue("li2216-25.members.linode.com"),
				"__meta_linode_public_ipv6_rdns":     model.LabelValue(""),
				"__meta_linode_region":               model.LabelValue("ca-central"),
				"__meta_linode_type":                 model.LabelValue("g6-standard-1"),
				"__meta_linode_status":               model.LabelValue("running"),
				"__meta_linode_tags":                 model.LabelValue(",monitoring,"),
				"__meta_linode_group":                model.LabelValue(""),
				"__meta_linode_gpus":                 model.LabelValue("0"),
				"__meta_linode_hypervisor":           model.LabelValue("kvm"),
				"__meta_linode_backups":              model.LabelValue("disabled"),
				"__meta_linode_specs_disk_bytes":     model.LabelValue("53687091200"),
				"__meta_linode_specs_memory_bytes":   model.LabelValue("2147483648"),
				"__meta_linode_specs_vcpus":          model.LabelValue("1"),
				"__meta_linode_specs_transfer_bytes": model.LabelValue("2097152000"),
				"__meta_linode_ipv6_ranges":          model.LabelValue(",2600:3c04:e001:456::/64,"),
			},
		}},
	}

	for _, tc := range tests {
		cfg := DefaultSDConfig
		if tc.region != "" {
			cfg.Region = tc.region
		}
		cfg.HTTPClientConfig.Authorization = &config.Authorization{
			Credentials: tokenID,
			Type:        "Bearer",
		}

		reg := prometheus.NewRegistry()
		refreshMetrics := discovery.NewRefreshMetrics(reg)
		metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
		require.NoError(t, metrics.Register())
		defer metrics.Unregister()
		defer refreshMetrics.Unregister()

		d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
			Logger:  promslog.NewNopLogger(),
			Metrics: metrics,
			SetName: "linode",
		})
		require.NoError(t, err)
		endpoint, err := url.Parse(sdmock.Endpoint())
		require.NoError(t, err)
		d.client.SetBaseURL(endpoint.String())

		tgs, err := d.refresh(context.Background())
		require.NoError(t, err)

		require.Len(t, tgs, 1)

		tg := tgs[0]
		require.NotNil(t, tg)
		require.NotNil(t, tg.Targets)
		require.Len(t, tg.Targets, tc.targetCount)

		for i, lbls := range tc.want {
			t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
				require.Equal(t, lbls, tg.Targets[i])
			})
		}
	}
}
