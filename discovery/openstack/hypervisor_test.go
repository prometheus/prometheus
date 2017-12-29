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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/common/model"
)

type OpenstackSDHypervisorTestSuite struct {
	suite.Suite
	Mock *SDMock
}

func (s *OpenstackSDHypervisorTestSuite) TearDownSuite() {
	s.Mock.ShutdownServer()
}

func (s *OpenstackSDHypervisorTestSuite) SetupTest() {
	s.Mock = NewSDMock(s.T())
	s.Mock.Setup()

	s.Mock.HandleHypervisorListSuccessfully()

	s.Mock.HandleVersionsSuccessfully()
	s.Mock.HandleAuthSuccessfully()
}

func TestOpenstackSDHypervisorSuite(t *testing.T) {
	suite.Run(t, new(OpenstackSDHypervisorTestSuite))
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

func (s *OpenstackSDHypervisorTestSuite) TestOpenstackSDHypervisorRefresh() {
	hypervisor, _ := s.openstackAuthSuccess()
	tg, err := hypervisor.refresh()
	assert.Nil(s.T(), err)
	require.NotNil(s.T(), tg)
	require.NotNil(s.T(), tg.Targets)
	require.Len(s.T(), tg.Targets, 2)

	assert.Equal(s.T(), tg.Targets[0]["__address__"], model.LabelValue("172.16.70.14:0"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_hypervisor_hostname"], model.LabelValue("nc14.cloud.com"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_hypervisor_type"], model.LabelValue("QEMU"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_hypervisor_host_ip"], model.LabelValue("172.16.70.14"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_hypervisor_state"], model.LabelValue("up"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_hypervisor_status"], model.LabelValue("enabled"))

	assert.Equal(s.T(), tg.Targets[1]["__address__"], model.LabelValue("172.16.70.13:0"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_hypervisor_hostname"], model.LabelValue("cc13.cloud.com"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_hypervisor_type"], model.LabelValue("QEMU"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_hypervisor_host_ip"], model.LabelValue("172.16.70.13"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_hypervisor_state"], model.LabelValue("up"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_hypervisor_status"], model.LabelValue("enabled"))
}
