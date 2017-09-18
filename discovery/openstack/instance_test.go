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
	"github.com/prometheus/prometheus/config"
)

type OpenstackSDInstanceTestSuite struct {
	suite.Suite
	Mock *SDMock
}

func (s *OpenstackSDInstanceTestSuite) TearDownSuite() {
	s.Mock.ShutdownServer()
}

func (s *OpenstackSDInstanceTestSuite) SetupTest() {
	s.Mock = NewSDMock(s.T())
	s.Mock.Setup()

	s.Mock.HandleServerListSuccessfully()
	s.Mock.HandleFloatingIPListSuccessfully()

	s.Mock.HandleVersionsSuccessfully()
	s.Mock.HandleAuthSuccessfully()
}

func TestOpenstackSDInstanceSuite(t *testing.T) {
	suite.Run(t, new(OpenstackSDInstanceTestSuite))
}

func (s *OpenstackSDInstanceTestSuite) openstackAuthSuccess() (Discovery, error) {
	conf := config.OpenstackSDConfig{
		IdentityEndpoint: s.Mock.Endpoint(),
		Password:         "test",
		Username:         "test",
		DomainName:       "12345",
		Region:           "RegionOne",
		Role:             "instance",
	}
	return NewDiscovery(&conf, nil)
}

func (s *OpenstackSDInstanceTestSuite) TestOpenstackSDInstanceRefresh() {
	instance, _ := s.openstackAuthSuccess()
	tg, err := instance.refresh()

	assert.Nil(s.T(), err)
	require.NotNil(s.T(), tg)
	require.NotNil(s.T(), tg.Targets)
	require.Len(s.T(), tg.Targets, 3)

	assert.Equal(s.T(), tg.Targets[0]["__address__"], model.LabelValue("10.0.0.32:0"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_instance_flavor"], model.LabelValue("1"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_instance_id"], model.LabelValue("ef079b0c-e610-4dfb-b1aa-b49f07ac48e5"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_instance_name"], model.LabelValue("herp"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_instance_status"], model.LabelValue("ACTIVE"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_private_ip"], model.LabelValue("10.0.0.32"))
	assert.Equal(s.T(), tg.Targets[0]["__meta_openstack_public_ip"], model.LabelValue("10.10.10.2"))

	assert.Equal(s.T(), tg.Targets[1]["__address__"], model.LabelValue("10.0.0.31:0"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_instance_flavor"], model.LabelValue("1"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_instance_id"], model.LabelValue("9e5476bd-a4ec-4653-93d6-72c93aa682ba"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_instance_name"], model.LabelValue("derp"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_instance_status"], model.LabelValue("ACTIVE"))
	assert.Equal(s.T(), tg.Targets[1]["__meta_openstack_private_ip"], model.LabelValue("10.0.0.31"))
}
