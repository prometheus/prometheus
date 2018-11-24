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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/util/testutil"
)

type OpenstackSDHypervisorTestSuite struct {
	Mock *SDMock
}

func (s *OpenstackSDHypervisorTestSuite) TearDownSuite() {
	s.Mock.ShutdownServer()
}

func (s *OpenstackSDHypervisorTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleHypervisorListSuccessfully()

	s.Mock.HandleVersionsSuccessfully()
	s.Mock.HandleAuthSuccessfully()
}

func (s *OpenstackSDHypervisorTestSuite) openstackAuthSuccess() (Discovery, error) {
	conf := SDConfig{
		IdentityEndpoint: s.Mock.Endpoint(),
		Password:         "test",
		Username:         "test",
		DomainName:       "12345",
		Region:           "RegionOne",
		Role:             "hypervisor",
	}
	return NewDiscovery(&conf, nil)
}

func TestOpenstackSDHypervisorRefresh(t *testing.T) {

	mock := &OpenstackSDHypervisorTestSuite{}
	mock.SetupTest(t)

	hypervisor, _ := mock.openstackAuthSuccess()
	tg, err := hypervisor.refresh()
	testutil.Ok(t, err)
	testutil.Assert(t, tg != nil, "")
	testutil.Assert(t, tg.Targets != nil, "")
	testutil.Assert(t, len(tg.Targets) == 2, "")

	testutil.Equals(t, tg.Targets[0]["__address__"], model.LabelValue("172.16.70.14:0"))
	testutil.Equals(t, tg.Targets[0]["__meta_openstack_hypervisor_hostname"], model.LabelValue("nc14.cloud.com"))
	testutil.Equals(t, tg.Targets[0]["__meta_openstack_hypervisor_type"], model.LabelValue("QEMU"))
	testutil.Equals(t, tg.Targets[0]["__meta_openstack_hypervisor_host_ip"], model.LabelValue("172.16.70.14"))
	testutil.Equals(t, tg.Targets[0]["__meta_openstack_hypervisor_state"], model.LabelValue("up"))
	testutil.Equals(t, tg.Targets[0]["__meta_openstack_hypervisor_status"], model.LabelValue("enabled"))

	testutil.Equals(t, tg.Targets[1]["__address__"], model.LabelValue("172.16.70.13:0"))
	testutil.Equals(t, tg.Targets[1]["__meta_openstack_hypervisor_hostname"], model.LabelValue("cc13.cloud.com"))
	testutil.Equals(t, tg.Targets[1]["__meta_openstack_hypervisor_type"], model.LabelValue("QEMU"))
	testutil.Equals(t, tg.Targets[1]["__meta_openstack_hypervisor_host_ip"], model.LabelValue("172.16.70.13"))
	testutil.Equals(t, tg.Targets[1]["__meta_openstack_hypervisor_state"], model.LabelValue("up"))
	testutil.Equals(t, tg.Targets[1]["__meta_openstack_hypervisor_status"], model.LabelValue("enabled"))

	mock.TearDownSuite()
}
