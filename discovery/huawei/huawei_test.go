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

package huawei

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"testing"
)

type HuaweiCloudSDTestSuite struct {
	Mock *SDMock
}

func (s *HuaweiCloudSDTestSuite) TearDownSuite() {
	s.Mock.ShutDownServer()
}

func (s *HuaweiCloudSDTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleListProjects()
	s.Mock.HandleListCatalog()
	s.Mock.HandleListServer()
	s.Mock.HandleGetToken()
}

func TestHuaweiSDRefreshByAKSK(t *testing.T) {
	sdmock := &HuaweiCloudSDTestSuite{}
	sdmock.SetupTest(t)
	t.Cleanup(sdmock.TearDownSuite)

	cfg := DefaultSDConfig
	cfg.Region = "ap-southeast-2"
	cfg.AccessKey = "access-key"
	cfg.SecretKey = "secret-key"
	cfg.AuthURL = sdmock.Mock.Endpoint()
	cfg.ProjectName = "ap-southeast-2"

	d := NewDiscovery(&cfg, log.NewNopLogger())
	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(tgs))
	require.Equal(t, 2, len(tgs[0].Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                      model.LabelValue("10.1.1.224:80"),
			"__meta_ecs_flavor_name":           model.LabelValue("c3.2xlarge.2"),
			"__meta_ecs_vpc_id":                model.LabelValue("bc8d0sd9-8b85-4ca0-a0ac-9f680b615208"),
			"__meta_ecs_id":                    model.LabelValue("ccbe39f1-26df-4443-b28a-002b547542ce"),
			"__meta_ecs_host_id":               model.LabelValue("3f88ca6718fc7484d835a346c310aaaaaasssdsd"),
			"__meta_ecs_status":                model.LabelValue("BUILD"),
			"__meta_ecs_flavor_ram":            model.LabelValue("16384"),
			"__meta_ecs_image_name":            model.LabelValue("CentOS 7.6 64bit"),
			"__meta_ecs_host_name":             model.LabelValue("test4fvik"),
			"__meta_ecs_name":                  model.LabelValue("test4FVIK"),
			"__meta_ecs_host_status":           model.LabelValue("UP"),
			"__meta_ecs_flavor_cpu":            model.LabelValue("8"),
			"__meta_ecs_instance_name":         model.LabelValue("instance-001ebf09"),
			"__meta_ecs_fixed_ip_0":            model.LabelValue("10.1.1.224"),
			"__meta_ecs_flavor_id":             model.LabelValue("c3.2xlarge.2"),
			"__meta_ecs_os_bit":                model.LabelValue("64"),
			"__meta_ecs_availability_zone":     model.LabelValue("ap-southeast-2a"),
		},
		{
			"__address__":                      model.LabelValue("10.1.1.3:80"),
			"__meta_ecs_flavor_name":           model.LabelValue("c3.large.2"),
			"__meta_ecs_vpc_id":                model.LabelValue("bc8dqw89-8b85-4ca0-a0ac-9f680b615208"),
			"__meta_ecs_id":                    model.LabelValue("77140bce-4f2e-454a-9221-940bebf5cf17"),
			"__meta_ecs_host_id":               model.LabelValue("3f88ca6718fc7484d835a346cqwe8c1cbfec8864baece143d43e4a63"),
			"__meta_ecs_status":                model.LabelValue("ACTIVE"),
			"__meta_ecs_flavor_ram":            model.LabelValue("4096"),
			"__meta_ecs_image_name":            model.LabelValue("CentOS 7.9 64bit"),
			"__meta_ecs_host_name":             model.LabelValue("onevm"),
			"__meta_ecs_name":                  model.LabelValue("onevm"),
			"__meta_ecs_host_status":           model.LabelValue("UP"),
			"__meta_ecs_flavor_cpu":            model.LabelValue("2"),
			"__meta_ecs_instance_name":         model.LabelValue("instance-001ea76f"),
			"__meta_ecs_fixed_ip_0":            model.LabelValue("10.1.1.3"),
			"__meta_ecs_flavor_id":             model.LabelValue("c3.large.2"),
			"__meta_ecs_os_bit":                model.LabelValue("64"),
			"__meta_ecs_availability_zone":     model.LabelValue("ap-southeast-2a"),
			"__meta_ecs_tag_identifier":        model.LabelValue("test"),
			"__meta_ecs_tag_1":                 model.LabelValue("2"),
			"__meta_ecs_tag_Name":              model.LabelValue("onevm"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tgs[0].Targets[i])
		})
	}
}

func TestHuaweiSDRefreshByPassowrd(t *testing.T) {
	sdmock := &HuaweiCloudSDTestSuite{}
	sdmock.SetupTest(t)
	t.Cleanup(sdmock.TearDownSuite)

	cfg := DefaultSDConfig
	cfg.Region = "ap-southeast-2"
	cfg.UserName = "username"
	cfg.Password = "password"
	cfg.DomainName = "domain_name"
	cfg.AuthURL = sdmock.Mock.Endpoint()
	cfg.ProjectName = "ap-southeast-2"

	d := NewDiscovery(&cfg, log.NewNopLogger())
	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(tgs))
	require.Equal(t, 2, len(tgs[0].Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                      model.LabelValue("10.1.1.224:80"),
			"__meta_ecs_flavor_name":           model.LabelValue("c3.2xlarge.2"),
			"__meta_ecs_vpc_id":                model.LabelValue("bc8d0sd9-8b85-4ca0-a0ac-9f680b615208"),
			"__meta_ecs_id":                    model.LabelValue("ccbe39f1-26df-4443-b28a-002b547542ce"),
			"__meta_ecs_host_id":               model.LabelValue("3f88ca6718fc7484d835a346c310aaaaaasssdsd"),
			"__meta_ecs_status":                model.LabelValue("BUILD"),
			"__meta_ecs_flavor_ram":            model.LabelValue("16384"),
			"__meta_ecs_image_name":            model.LabelValue("CentOS 7.6 64bit"),
			"__meta_ecs_host_name":             model.LabelValue("test4fvik"),
			"__meta_ecs_name":                  model.LabelValue("test4FVIK"),
			"__meta_ecs_host_status":           model.LabelValue("UP"),
			"__meta_ecs_flavor_cpu":            model.LabelValue("8"),
			"__meta_ecs_instance_name":         model.LabelValue("instance-001ebf09"),
			"__meta_ecs_fixed_ip_0":            model.LabelValue("10.1.1.224"),
			"__meta_ecs_flavor_id":             model.LabelValue("c3.2xlarge.2"),
			"__meta_ecs_os_bit":                model.LabelValue("64"),
			"__meta_ecs_availability_zone":     model.LabelValue("ap-southeast-2a"),
		},
		{
			"__address__":                      model.LabelValue("10.1.1.3:80"),
			"__meta_ecs_flavor_name":           model.LabelValue("c3.large.2"),
			"__meta_ecs_vpc_id":                model.LabelValue("bc8dqw89-8b85-4ca0-a0ac-9f680b615208"),
			"__meta_ecs_id":                    model.LabelValue("77140bce-4f2e-454a-9221-940bebf5cf17"),
			"__meta_ecs_host_id":               model.LabelValue("3f88ca6718fc7484d835a346cqwe8c1cbfec8864baece143d43e4a63"),
			"__meta_ecs_status":                model.LabelValue("ACTIVE"),
			"__meta_ecs_flavor_ram":            model.LabelValue("4096"),
			"__meta_ecs_image_name":            model.LabelValue("CentOS 7.9 64bit"),
			"__meta_ecs_host_name":             model.LabelValue("onevm"),
			"__meta_ecs_name":                  model.LabelValue("onevm"),
			"__meta_ecs_host_status":           model.LabelValue("UP"),
			"__meta_ecs_flavor_cpu":            model.LabelValue("2"),
			"__meta_ecs_instance_name":         model.LabelValue("instance-001ea76f"),
			"__meta_ecs_fixed_ip_0":            model.LabelValue("10.1.1.3"),
			"__meta_ecs_flavor_id":             model.LabelValue("c3.large.2"),
			"__meta_ecs_os_bit":                model.LabelValue("64"),
			"__meta_ecs_availability_zone":     model.LabelValue("ap-southeast-2a"),
			"__meta_ecs_tag_identifier":        model.LabelValue("test"),
			"__meta_ecs_tag_1":                 model.LabelValue("2"),
			"__meta_ecs_tag_Name":              model.LabelValue("onevm"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tgs[0].Targets[i])
		})
	}

}

