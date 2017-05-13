package openstack

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
)

type OpenstackSDTestSuite struct {
	suite.Suite
	Mock *OpenstackSDMock
}

func (s *OpenstackSDTestSuite) TearDownSuite() {
	s.Mock.ShutdownServer()
}

func (s *OpenstackSDTestSuite) SetupTest() {
	s.Mock = NewOpenstackSDMock(s.T())
	s.Mock.Setup()

	s.Mock.HandleServerListSuccessfully()
	s.Mock.HandleFloatingIPListSuccessfully()

	s.Mock.HandleVersionsSuccessfully()
	s.Mock.HandleAuthSuccessfully()
}

func TestOpenstackSDSuite(t *testing.T) {
	suite.Run(t, new(OpenstackSDTestSuite))
}

func (s *OpenstackSDTestSuite) openstackAuthSuccess() (*Discovery, error) {
	conf := config.OpenstackSDConfig{
		IdentityEndpoint: s.Mock.Endpoint(),
		Password:         "test",
		Username:         "test",
		DomainName:       "12345",
		Region:           "RegionOne",
	}

	return NewDiscovery(&conf)
}

func (s *OpenstackSDTestSuite) TestOpenstackSDRefresh() {
	d, _ := s.openstackAuthSuccess()

	tg, err := d.refresh()
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
