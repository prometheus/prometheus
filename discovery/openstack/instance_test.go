// Copyright 2017 The Prometheus Authors
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
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/util/testutil"
)

type OpenstackSDInstanceTestSuite struct {
	Mock *SDMock
}

func (s *OpenstackSDInstanceTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleServerListSuccessfully()
	s.Mock.HandleFloatingIPListSuccessfully()

	s.Mock.HandleVersionsSuccessfully()
	s.Mock.HandleAuthSuccessfully()
}

func (s *OpenstackSDInstanceTestSuite) openstackAuthSuccess() (refresher, error) {
	conf := SDConfig{
		IdentityEndpoint: s.Mock.Endpoint(),
		Password:         "test",
		Username:         "test",
		DomainName:       "12345",
		Region:           "RegionOne",
		Role:             "instance",
		AllTenants:       true,
	}
	return newRefresher(&conf, nil)
}

func TestOpenstackSDInstanceRefresh(t *testing.T) {

	mock := &OpenstackSDInstanceTestSuite{}
	mock.SetupTest(t)

	instance, err := mock.openstackAuthSuccess()
	testutil.Ok(t, err)

	ctx := context.Background()
	tgs, err := instance.refresh(ctx)

	testutil.Ok(t, err)
	testutil.Equals(t, 1, len(tgs))

	tg := tgs[0]
	testutil.Assert(t, tg != nil, "")
	testutil.Assert(t, tg.Targets != nil, "")
	testutil.Equals(t, 4, len(tg.Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                      model.LabelValue("10.0.0.32:0"),
			"__meta_openstack_instance_flavor": model.LabelValue("1"),
			"__meta_openstack_instance_id":     model.LabelValue("ef079b0c-e610-4dfb-b1aa-b49f07ac48e5"),
			"__meta_openstack_instance_status": model.LabelValue("ACTIVE"),
			"__meta_openstack_instance_name":   model.LabelValue("herp"),
			"__meta_openstack_private_ip":      model.LabelValue("10.0.0.32"),
			"__meta_openstack_public_ip":       model.LabelValue("10.10.10.2"),
			"__meta_openstack_address_pool":    model.LabelValue("private"),
			"__meta_openstack_project_id":      model.LabelValue("fcad67a6189847c4aecfa3c81a05783b"),
			"__meta_openstack_user_id":         model.LabelValue("9349aff8be7545ac9d2f1d00999a23cd"),
		},
		{
			"__address__":                      model.LabelValue("10.0.0.31:0"),
			"__meta_openstack_instance_flavor": model.LabelValue("1"),
			"__meta_openstack_instance_id":     model.LabelValue("9e5476bd-a4ec-4653-93d6-72c93aa682ba"),
			"__meta_openstack_instance_status": model.LabelValue("ACTIVE"),
			"__meta_openstack_instance_name":   model.LabelValue("derp"),
			"__meta_openstack_private_ip":      model.LabelValue("10.0.0.31"),
			"__meta_openstack_address_pool":    model.LabelValue("private"),
			"__meta_openstack_project_id":      model.LabelValue("fcad67a6189847c4aecfa3c81a05783b"),
			"__meta_openstack_user_id":         model.LabelValue("9349aff8be7545ac9d2f1d00999a23cd"),
		},
		{
			"__address__":                      model.LabelValue("10.0.0.33:0"),
			"__meta_openstack_instance_flavor": model.LabelValue("4"),
			"__meta_openstack_instance_id":     model.LabelValue("9e5476bd-a4ec-4653-93d6-72c93aa682bb"),
			"__meta_openstack_instance_status": model.LabelValue("ACTIVE"),
			"__meta_openstack_instance_name":   model.LabelValue("merp"),
			"__meta_openstack_private_ip":      model.LabelValue("10.0.0.33"),
			"__meta_openstack_address_pool":    model.LabelValue("private"),
			"__meta_openstack_tag_env":         model.LabelValue("prod"),
			"__meta_openstack_project_id":      model.LabelValue("fcad67a6189847c4aecfa3c81a05783b"),
			"__meta_openstack_user_id":         model.LabelValue("9349aff8be7545ac9d2f1d00999a23cd"),
		},
		{
			"__address__":                      model.LabelValue("10.0.0.34:0"),
			"__meta_openstack_instance_flavor": model.LabelValue("4"),
			"__meta_openstack_instance_id":     model.LabelValue("9e5476bd-a4ec-4653-93d6-72c93aa682bb"),
			"__meta_openstack_instance_status": model.LabelValue("ACTIVE"),
			"__meta_openstack_instance_name":   model.LabelValue("merp"),
			"__meta_openstack_private_ip":      model.LabelValue("10.0.0.34"),
			"__meta_openstack_address_pool":    model.LabelValue("private"),
			"__meta_openstack_tag_env":         model.LabelValue("prod"),
			"__meta_openstack_public_ip":       model.LabelValue("10.10.10.4"),
			"__meta_openstack_project_id":      model.LabelValue("fcad67a6189847c4aecfa3c81a05783b"),
			"__meta_openstack_user_id":         model.LabelValue("9349aff8be7545ac9d2f1d00999a23cd"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			testutil.Equals(t, lbls, tg.Targets[i])
		})
	}
}

func TestOpenstackSDInstanceRefreshWithDoneContext(t *testing.T) {
	mock := &OpenstackSDHypervisorTestSuite{}
	mock.SetupTest(t)

	hypervisor, _ := mock.openstackAuthSuccess()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := hypervisor.refresh(ctx)
	testutil.NotOk(t, err)
	testutil.Assert(t, strings.Contains(err.Error(), context.Canceled.Error()), "%q doesn't contain %q", err, context.Canceled)
}
