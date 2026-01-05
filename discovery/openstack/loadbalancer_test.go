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

package openstack

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type OpenstackSDLoadBalancerTestSuite struct {
	Mock *SDMock
}

func (s *OpenstackSDLoadBalancerTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleLoadBalancerListSuccessfully()
	s.Mock.HandleListenersListSuccessfully()
	s.Mock.HandleFloatingIPListSuccessfully()

	s.Mock.HandleVersionsSuccessfully()
	s.Mock.HandleAuthSuccessfully()
}

func (s *OpenstackSDLoadBalancerTestSuite) openstackAuthSuccess() (refresher, error) {
	conf := SDConfig{
		IdentityEndpoint: s.Mock.Endpoint(),
		Password:         "test",
		Username:         "test",
		DomainName:       "12345",
		Region:           "RegionOne",
		Role:             "loadbalancer",
	}
	return newRefresher(&conf, nil)
}

func TestOpenstackSDLoadBalancerRefresh(t *testing.T) {
	mock := &OpenstackSDLoadBalancerTestSuite{}
	mock.SetupTest(t)

	instance, err := mock.openstackAuthSuccess()
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := instance.refresh(ctx)

	require.NoError(t, err)
	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 4)

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                                       model.LabelValue("10.0.0.32:9273"),
			"__meta_openstack_loadbalancer_id":                  model.LabelValue("ef079b0c-e610-4dfb-b1aa-b49f07ac48e5"),
			"__meta_openstack_loadbalancer_name":                model.LabelValue("lb1"),
			"__meta_openstack_loadbalancer_operating_status":    model.LabelValue("ONLINE"),
			"__meta_openstack_loadbalancer_provisioning_status": model.LabelValue("ACTIVE"),
			"__meta_openstack_loadbalancer_availability_zone":   model.LabelValue("az1"),
			"__meta_openstack_loadbalancer_floating_ip":         model.LabelValue("192.168.1.2"),
			"__meta_openstack_loadbalancer_vip":                 model.LabelValue("10.0.0.32"),
			"__meta_openstack_loadbalancer_provider":            model.LabelValue("amphora"),
			"__meta_openstack_loadbalancer_tags":                model.LabelValue("tag1,tag2"),
			"__meta_openstack_project_id":                       model.LabelValue("fcad67a6189847c4aecfa3c81a05783b"),
		},
		{
			"__address__":                                       model.LabelValue("10.0.2.78:8080"),
			"__meta_openstack_loadbalancer_id":                  model.LabelValue("d92c471e-8d3e-4b9f-b2b5-9c72a9e3ef54"),
			"__meta_openstack_loadbalancer_name":                model.LabelValue("lb3"),
			"__meta_openstack_loadbalancer_operating_status":    model.LabelValue("ONLINE"),
			"__meta_openstack_loadbalancer_provisioning_status": model.LabelValue("ACTIVE"),
			"__meta_openstack_loadbalancer_availability_zone":   model.LabelValue("az3"),
			"__meta_openstack_loadbalancer_floating_ip":         model.LabelValue("192.168.3.4"),
			"__meta_openstack_loadbalancer_vip":                 model.LabelValue("10.0.2.78"),
			"__meta_openstack_loadbalancer_provider":            model.LabelValue("amphora"),
			"__meta_openstack_loadbalancer_tags":                model.LabelValue("tag5,tag6"),
			"__meta_openstack_project_id":                       model.LabelValue("ac57f03dba1a4fdebff3e67201bc7a85"),
		},
		{
			"__address__":                                       model.LabelValue("10.0.3.99:9090"),
			"__meta_openstack_loadbalancer_id":                  model.LabelValue("f5c7e918-df38-4a5a-a7d4-d9c27ab2cf67"),
			"__meta_openstack_loadbalancer_name":                model.LabelValue("lb4"),
			"__meta_openstack_loadbalancer_operating_status":    model.LabelValue("ONLINE"),
			"__meta_openstack_loadbalancer_provisioning_status": model.LabelValue("ACTIVE"),
			"__meta_openstack_loadbalancer_availability_zone":   model.LabelValue("az1"),
			"__meta_openstack_loadbalancer_floating_ip":         model.LabelValue("192.168.4.5"),
			"__meta_openstack_loadbalancer_vip":                 model.LabelValue("10.0.3.99"),
			"__meta_openstack_loadbalancer_provider":            model.LabelValue("amphora"),
			"__meta_openstack_project_id":                       model.LabelValue("fa8c372dfe4d4c92b0c4e3a2d9b3c9fa"),
		},
		{
			"__address__":                                       model.LabelValue("10.0.4.88:9876"),
			"__meta_openstack_loadbalancer_id":                  model.LabelValue("e83a6d92-7a3e-4567-94b3-20c83b32a75e"),
			"__meta_openstack_loadbalancer_name":                model.LabelValue("lb5"),
			"__meta_openstack_loadbalancer_operating_status":    model.LabelValue("ONLINE"),
			"__meta_openstack_loadbalancer_provisioning_status": model.LabelValue("ACTIVE"),
			"__meta_openstack_loadbalancer_availability_zone":   model.LabelValue("az4"),
			"__meta_openstack_loadbalancer_vip":                 model.LabelValue("10.0.4.88"),
			"__meta_openstack_loadbalancer_provider":            model.LabelValue("amphora"),
			"__meta_openstack_project_id":                       model.LabelValue("a5d3b2e1e6f34cd9a5f7c2f01a6b8e29"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func TestOpenstackSDLoadBalancerRefreshWithDoneContext(t *testing.T) {
	mock := &OpenstackSDLoadBalancerTestSuite{}
	mock.SetupTest(t)

	loadbalancer, _ := mock.openstackAuthSuccess()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := loadbalancer.refresh(ctx)
	require.ErrorContains(t, err, context.Canceled.Error(), "%q doesn't contain %q", err, context.Canceled)
}
