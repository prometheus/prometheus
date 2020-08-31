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
	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/util/testutil"
	"testing"
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
	testutil.Ok(t, err)

	targetGroups, err := d.refresh(context.Background())
	testutil.Ok(t, err)
	testutil.Equals(t, 1, len(targetGroups))

	targetGroup := targetGroups[0]
	testutil.Assert(t, targetGroup != nil, "targetGroup should not be nil")
	testutil.Assert(t, targetGroup.Targets != nil, "targetGroup.targets should not be nil")
	testutil.Equals(t, 2, len(targetGroup.Targets))

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
			testutil.Equals(t, labelSet, targetGroup.Targets[i])
		})
	}
}

func TestRobotSDRefreshHandleError(t *testing.T) {
	suite := &robotSDTestSuite{}
	suite.SetupTest(t)
	cfg := DefaultSDConfig
	cfg.robotEndpoint = suite.Mock.Endpoint()

	d, err := newRobotDiscovery(&cfg, log.NewNopLogger())
	testutil.Ok(t, err)

	targetGroups, err := d.refresh(context.Background())
	testutil.NotOk(t, err)
	testutil.Equals(t, "non 2xx status '401' response during hetzner service discovery with role robot", err.Error())

	testutil.Equals(t, 0, len(targetGroups))
}
