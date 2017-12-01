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
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/testutil"
)

func openstackInstanceAuthSuccess(mock *SDMock) (Discovery, error) {
	conf := config.OpenstackSDConfig{
		IdentityEndpoint: mock.Endpoint(),
		Password:         "test",
		Username:         "test",
		DomainName:       "12345",
		Region:           "RegionOne",
		Role:             "instance",
	}
	return NewDiscovery(&conf, nil)
}

func TestOpenstackSDInstanceSuite(t *testing.T) {
	mock := NewSDMock(t)

	mock.Setup()
	mock.HandleServerListSuccessfully()
	mock.HandleFloatingIPListSuccessfully()
	mock.HandleVersionsSuccessfully()
	mock.HandleAuthSuccessfully()

	instance, _ := openstackInstanceAuthSuccess(mock)

	tg, err := instance.refresh()

	testutil.Ok(t, err)
	testutil.Assert(t, tg != nil, testutil.NotNilErrorMessage)
	testutil.Assert(t, tg.Targets != nil, testutil.NotNilErrorMessage)

	testutil.Equals(t, 3, len(tg.Targets))
	testutil.Equals(t, model.LabelValue("10.0.0.32:0"), tg.Targets[0]["__address__"])
	testutil.Equals(t, model.LabelValue("1"), tg.Targets[0]["__meta_openstack_instance_flavor"])
	testutil.Equals(t, model.LabelValue("ef079b0c-e610-4dfb-b1aa-b49f07ac48e5"), tg.Targets[0]["__meta_openstack_instance_id"])
	testutil.Equals(t, model.LabelValue("herp"), tg.Targets[0]["__meta_openstack_instance_name"])
	testutil.Equals(t, model.LabelValue("ACTIVE"), tg.Targets[0]["__meta_openstack_instance_status"])
	testutil.Equals(t, model.LabelValue("10.0.0.32"), tg.Targets[0]["__meta_openstack_private_ip"])
	testutil.Equals(t, model.LabelValue("10.10.10.2"), tg.Targets[0]["__meta_openstack_public_ip"])

	testutil.Equals(t, model.LabelValue("10.0.0.31:0"), tg.Targets[1]["__address__"])
	testutil.Equals(t, model.LabelValue("1"), tg.Targets[1]["__meta_openstack_instance_flavor"])
	testutil.Equals(t, model.LabelValue("9e5476bd-a4ec-4653-93d6-72c93aa682ba"), tg.Targets[1]["__meta_openstack_instance_id"])
	testutil.Equals(t, model.LabelValue("derp"), tg.Targets[1]["__meta_openstack_instance_name"])
	testutil.Equals(t, model.LabelValue("ACTIVE"), tg.Targets[1]["__meta_openstack_instance_status"])
	testutil.Equals(t, model.LabelValue("10.0.0.31"), tg.Targets[1]["__meta_openstack_private_ip"])

	mock.ShutdownServer()
}
