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

func openstackHypervisorAuthSuccess(mock *SDMock) (Discovery, error) {
	conf := config.OpenstackSDConfig{
		IdentityEndpoint: mock.Endpoint(),
		Password:         "test",
		Username:         "test",
		DomainName:       "12345",
		Region:           "RegionOne",
		Role:             "hypervisor",
	}
	return NewDiscovery(&conf, nil)
}

func TestOpenstackSDHypervisorRefresh(t *testing.T) {
	mock := NewSDMock(t)
	mock.Setup()

	mock.HandleHypervisorListSuccessfully()

	mock.HandleVersionsSuccessfully()
	mock.HandleAuthSuccessfully()

	hypervisor, _ := openstackHypervisorAuthSuccess(mock)

	tg, err := hypervisor.refresh()

	testutil.Ok(t, err)
	testutil.Assert(t, tg != nil, testutil.NotNilErrorMessage)
	testutil.Assert(t, tg.Targets != nil, testutil.NotNilErrorMessage)
	testutil.Equals(t, 2, len(tg.Targets))

	testutil.Equals(t, model.LabelValue("172.16.70.14:0"), tg.Targets[0]["__address__"])
	testutil.Equals(t, model.LabelValue("nc14.cloud.com"), tg.Targets[0]["__meta_openstack_hypervisor_hostname"])
	testutil.Equals(t, model.LabelValue("QEMU"), tg.Targets[0]["__meta_openstack_hypervisor_type"])
	testutil.Equals(t, model.LabelValue("172.16.70.14"), tg.Targets[0]["__meta_openstack_hypervisor_host_ip"])
	testutil.Equals(t, model.LabelValue("up"), tg.Targets[0]["__meta_openstack_hypervisor_state"])
	testutil.Equals(t, model.LabelValue("enabled"), tg.Targets[0]["__meta_openstack_hypervisor_status"])

	testutil.Equals(t, model.LabelValue("172.16.70.13:0"), tg.Targets[1]["__address__"])
	testutil.Equals(t, model.LabelValue("cc13.cloud.com"), tg.Targets[1]["__meta_openstack_hypervisor_hostname"])
	testutil.Equals(t, model.LabelValue("QEMU"), tg.Targets[1]["__meta_openstack_hypervisor_type"])
	testutil.Equals(t, model.LabelValue("172.16.70.13"), tg.Targets[1]["__meta_openstack_hypervisor_host_ip"])
	testutil.Equals(t, model.LabelValue("up"), tg.Targets[1]["__meta_openstack_hypervisor_state"])
	testutil.Equals(t, model.LabelValue("enabled"), tg.Targets[1]["__meta_openstack_hypervisor_status"])

	mock.ShutdownServer()
}
