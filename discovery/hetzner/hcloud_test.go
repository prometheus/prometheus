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

package hetzner

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

type hcloudSDTestSuite struct {
	Mock *SDMock
}

func (s *hcloudSDTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleHcloudServers()
	s.Mock.HandleHcloudNetworks()
}

func TestHCloudSDRefresh(t *testing.T) {
	suite := &hcloudSDTestSuite{}
	suite.SetupTest(t)

	cfg := DefaultSDConfig
	cfg.HTTPClientConfig.BearerToken = hcloudTestToken
	cfg.hcloudEndpoint = suite.Mock.Endpoint()

	d, err := newHcloudDiscovery(&cfg, promslog.NewNopLogger())
	require.NoError(t, err)

	targetGroups, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, targetGroups, 1)

	targetGroup := targetGroups[0]
	require.NotNil(t, targetGroup, "targetGroup should not be nil")
	require.NotNil(t, targetGroup.Targets, "targetGroup.targets should not be nil")
	require.Len(t, targetGroup.Targets, 3)

	for i, labelSet := range []model.LabelSet{
		{
			"__address__":                                            model.LabelValue("1.2.3.4:80"),
			"__meta_hetzner_role":                                    model.LabelValue("hcloud"),
			"__meta_hetzner_server_id":                               model.LabelValue("42"),
			"__meta_hetzner_server_name":                             model.LabelValue("my-server"),
			"__meta_hetzner_server_status":                           model.LabelValue("running"),
			"__meta_hetzner_public_ipv4":                             model.LabelValue("1.2.3.4"),
			"__meta_hetzner_public_ipv6_network":                     model.LabelValue("2001:db8::/64"),
			"__meta_hetzner_datacenter":                              model.LabelValue("fsn1-dc8"),
			"__meta_hetzner_hcloud_image_name":                       model.LabelValue("ubuntu-20.04"),
			"__meta_hetzner_hcloud_image_description":                model.LabelValue("Ubuntu 20.04 Standard 64 bit"),
			"__meta_hetzner_hcloud_image_os_flavor":                  model.LabelValue("ubuntu"),
			"__meta_hetzner_hcloud_image_os_version":                 model.LabelValue("20.04"),
			"__meta_hetzner_hcloud_datacenter_location":              model.LabelValue("fsn1"),
			"__meta_hetzner_hcloud_datacenter_location_network_zone": model.LabelValue("eu-central"),
			"__meta_hetzner_hcloud_cpu_cores":                        model.LabelValue("1"),
			"__meta_hetzner_hcloud_cpu_type":                         model.LabelValue("shared"),
			"__meta_hetzner_hcloud_memory_size_gb":                   model.LabelValue("1"),
			"__meta_hetzner_hcloud_disk_size_gb":                     model.LabelValue("25"),
			"__meta_hetzner_hcloud_server_type":                      model.LabelValue("cx11"),
			"__meta_hetzner_hcloud_private_ipv4_mynet":               model.LabelValue("10.0.0.2"),
			"__meta_hetzner_hcloud_labelpresent_my_key":              model.LabelValue("true"),
			"__meta_hetzner_hcloud_label_my_key":                     model.LabelValue("my-value"),
		},
		{
			"__address__":                                            model.LabelValue("1.2.3.5:80"),
			"__meta_hetzner_role":                                    model.LabelValue("hcloud"),
			"__meta_hetzner_server_id":                               model.LabelValue("44"),
			"__meta_hetzner_server_name":                             model.LabelValue("another-server"),
			"__meta_hetzner_server_status":                           model.LabelValue("stopped"),
			"__meta_hetzner_datacenter":                              model.LabelValue("fsn1-dc14"),
			"__meta_hetzner_public_ipv4":                             model.LabelValue("1.2.3.5"),
			"__meta_hetzner_public_ipv6_network":                     model.LabelValue("2001:db9::/64"),
			"__meta_hetzner_hcloud_image_name":                       model.LabelValue("ubuntu-20.04"),
			"__meta_hetzner_hcloud_image_description":                model.LabelValue("Ubuntu 20.04 Standard 64 bit"),
			"__meta_hetzner_hcloud_image_os_flavor":                  model.LabelValue("ubuntu"),
			"__meta_hetzner_hcloud_image_os_version":                 model.LabelValue("20.04"),
			"__meta_hetzner_hcloud_datacenter_location":              model.LabelValue("fsn1"),
			"__meta_hetzner_hcloud_datacenter_location_network_zone": model.LabelValue("eu-central"),
			"__meta_hetzner_hcloud_cpu_cores":                        model.LabelValue("2"),
			"__meta_hetzner_hcloud_cpu_type":                         model.LabelValue("shared"),
			"__meta_hetzner_hcloud_memory_size_gb":                   model.LabelValue("1"),
			"__meta_hetzner_hcloud_disk_size_gb":                     model.LabelValue("50"),
			"__meta_hetzner_hcloud_server_type":                      model.LabelValue("cpx11"),
			"__meta_hetzner_hcloud_labelpresent_key":                 model.LabelValue("true"),
			"__meta_hetzner_hcloud_labelpresent_other_key":           model.LabelValue("true"),
			"__meta_hetzner_hcloud_label_key":                        model.LabelValue(""),
			"__meta_hetzner_hcloud_label_other_key":                  model.LabelValue("value"),
		},
		{
			"__address__":                                            model.LabelValue("1.2.3.6:80"),
			"__meta_hetzner_role":                                    model.LabelValue("hcloud"),
			"__meta_hetzner_server_id":                               model.LabelValue("36"),
			"__meta_hetzner_server_name":                             model.LabelValue("deleted-image-server"),
			"__meta_hetzner_server_status":                           model.LabelValue("stopped"),
			"__meta_hetzner_datacenter":                              model.LabelValue("fsn1-dc14"),
			"__meta_hetzner_public_ipv4":                             model.LabelValue("1.2.3.6"),
			"__meta_hetzner_public_ipv6_network":                     model.LabelValue("2001:db7::/64"),
			"__meta_hetzner_hcloud_datacenter_location":              model.LabelValue("fsn1"),
			"__meta_hetzner_hcloud_datacenter_location_network_zone": model.LabelValue("eu-central"),
			"__meta_hetzner_hcloud_cpu_cores":                        model.LabelValue("2"),
			"__meta_hetzner_hcloud_cpu_type":                         model.LabelValue("shared"),
			"__meta_hetzner_hcloud_memory_size_gb":                   model.LabelValue("1"),
			"__meta_hetzner_hcloud_disk_size_gb":                     model.LabelValue("50"),
			"__meta_hetzner_hcloud_server_type":                      model.LabelValue("cpx11"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, labelSet, targetGroup.Targets[i])
		})
	}
}
