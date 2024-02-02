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

package hetzner

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type robotSDTestSuite struct {
	Mock *SDMock
}

func (s *robotSDTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleRobotServers()
}

func TestRobotSDRefresh(t *testing.T) {
	suite := &robotSDTestSuite{}
	suite.SetupTest(t)
	cfg := DefaultSDConfig
	cfg.HTTPClientConfig.BasicAuth = &config.BasicAuth{Username: robotTestUsername, Password: robotTestPassword}
	cfg.robotEndpoint = suite.Mock.Endpoint()

	d, err := newRobotDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	targetGroups, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, targetGroups, 1)

	targetGroup := targetGroups[0]
	require.NotNil(t, targetGroup, "targetGroup should not be nil")
	require.NotNil(t, targetGroup.Targets, "targetGroup.targets should not be nil")
	require.Len(t, targetGroup.Targets, 2)

	for i, labelSet := range []model.LabelSet{
		{
			"__address__":                        model.LabelValue("123.123.123.123:80"),
			"__meta_hetzner_role":                model.LabelValue("robot"),
			"__meta_hetzner_server_id":           model.LabelValue("321"),
			"__meta_hetzner_server_name":         model.LabelValue("server1"),
			"__meta_hetzner_server_status":       model.LabelValue("ready"),
			"__meta_hetzner_public_ipv4":         model.LabelValue("123.123.123.123"),
			"__meta_hetzner_public_ipv6_network": model.LabelValue("2a01:4f8:111:4221::/64"),
			"__meta_hetzner_datacenter":          model.LabelValue("nbg1-dc1"),
			"__meta_hetzner_robot_product":       model.LabelValue("DS 3000"),
			"__meta_hetzner_robot_cancelled":     model.LabelValue("false"),
		},
		{
			"__address__":                    model.LabelValue("123.123.123.124:80"),
			"__meta_hetzner_role":            model.LabelValue("robot"),
			"__meta_hetzner_server_id":       model.LabelValue("421"),
			"__meta_hetzner_server_name":     model.LabelValue("server2"),
			"__meta_hetzner_server_status":   model.LabelValue("in process"),
			"__meta_hetzner_public_ipv4":     model.LabelValue("123.123.123.124"),
			"__meta_hetzner_datacenter":      model.LabelValue("fsn1-dc10"),
			"__meta_hetzner_robot_product":   model.LabelValue("X5"),
			"__meta_hetzner_robot_cancelled": model.LabelValue("true"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, labelSet, targetGroup.Targets[i])
		})
	}
}

func TestRobotSDRefreshHandleError(t *testing.T) {
	suite := &robotSDTestSuite{}
	suite.SetupTest(t)
	cfg := DefaultSDConfig
	cfg.robotEndpoint = suite.Mock.Endpoint()

	d, err := newRobotDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	targetGroups, err := d.refresh(context.Background())
	require.Error(t, err)
	require.Equal(t, "non 2xx status '401' response during hetzner service discovery with role robot", err.Error())

	require.Empty(t, targetGroups)
}
