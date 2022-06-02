// Copyright 2021 The Prometheus Authors
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

package ovhcloud

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"

	"github.com/prometheus/prometheus/util/testutil"
)

type VpsData struct {
	Vps    Vps
	Err    error
	IPs    []string
	IPsErr error
}

func initMockErrorVpsList(err error) {
	initMockAuth(123562)
	gock.New(mockURL).
		MatchHeader("Accept", "application/json").
		Get("/vps").
		ReplyError(err)
}

func initMockVps(vpsList map[string]VpsData) {
	initMockAuth(123562)

	var vpsListName []string
	for vpsName := range vpsList {
		vpsListName = append(vpsListName, vpsName)
	}

	gock.New(mockURL).
		MatchHeader("Accept", "application/json").
		Get("/vps").
		Reply(200).
		JSON(vpsListName)

	for vpsName := range vpsList {
		if vpsList[vpsName].Err != nil {
			gock.New(mockURL).
				MatchHeader("Accept", "application/json").
				Get(fmt.Sprintf("/vps/%s", vpsName)).
				ReplyError(vpsList[vpsName].Err)
		} else {
			gock.New(mockURL).
				MatchHeader("Accept", "application/json").
				Get(fmt.Sprintf("/vps/%s", vpsName)).
				Reply(200).
				JSON(vpsList[vpsName].Vps)
			if vpsList[vpsName].IPsErr != nil {
				gock.New(mockURL).
					MatchHeader("Accept", "application/json").
					Get(fmt.Sprintf("/vps/%s/ips", vpsName)).
					ReplyError(vpsList[vpsName].IPsErr)
			} else {
				gock.New(mockURL).
					MatchHeader("Accept", "application/json").
					Get(fmt.Sprintf("/vps/%s/ips", vpsName)).
					Reply(200).
					JSON(vpsList[vpsName].IPs)
			}
		}
	}
}

func TestVpsErrorOnList(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	errTest := errors.New("error on get list")
	initMockErrorVpsList(errTest)

	conf, err := getMockConf("vps")
	require.NoError(t, err)

	logger := testutil.NewLogger(t)
	d := newVpsDiscovery(&conf, logger)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.ErrorIs(t, err, errTest)

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestVpsWithBadConf(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	conf, err := getMockConf("vps")
	require.NoError(t, err)

	conf.ApplicationKey = ""
	logger := testutil.NewLogger(t)
	d := newVpsDiscovery(&conf, logger)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.ErrorContains(t, err, "missing application key")

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestVpsErrorOnDetail(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	errTest := errors.New("error on get detail")
	modelV := Model{
		Name:                "model name test",
		Disk:                40,
		MaximumAdditionalIP: 1,
		Memory:              2048,
		Offer:               "VPS abc",
		Version:             "2019v1",
		Vcore:               1,
	}

	vps := Vps{
		Model:       modelV,
		Zone:        "zone",
		Cluster:     "cluster_test",
		DisplayName: "test_name",
		Name:        "abc",
		NetbootMode: "local",
		State:       "running",
		MemoryLimit: 2048,
		OfferType:   "ssd",
	}

	vpsMap := map[string]VpsData{"abc": {Vps: vps, IPs: []string{"192.0.2.1", "2001:0db8:0000:0000:0000:0000:0000:0001"}}, "def": {Err: errTest}}
	initMockVps(vpsMap)

	conf, err := getMockConf("vps")
	require.NoError(t, err)

	logger := testutil.NewLogger(t)
	d := newVpsDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	// When we have an error on a detail, we continue and the server is skipped
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tgVps := tgs[0]

	require.NotNil(t, tgVps)
	require.NotNil(t, tgVps.Targets)
	require.Equal(t, 1, len(tgVps.Targets))

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestVpsErrorOnIps(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	errTest := errors.New("error on get ips")
	modelV := Model{
		Name:                "model name test",
		Disk:                40,
		MaximumAdditionalIP: 1,
		Memory:              2048,
		Offer:               "VPS abc",
		Version:             "2019v1",
		Vcore:               1,
	}

	vps := Vps{
		Model:       modelV,
		Zone:        "zone",
		Cluster:     "cluster_test",
		DisplayName: "test_name",
		Name:        "abc",
		NetbootMode: "local",
		State:       "running",
		MemoryLimit: 2048,
		OfferType:   "ssd",
	}

	vpsMap := map[string]VpsData{"abc": {Vps: vps, IPs: []string{"192.0.2.1", "2001:0db8:0000:0000:0000:0000:0000:0001"}}, "def": {Vps: vps, IPsErr: errTest}}
	initMockVps(vpsMap)

	conf, err := getMockConf("vps")
	require.NoError(t, err)

	logger := testutil.NewLogger(t)
	d := newVpsDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	// When we have an error on a detail, we continue and the server is skipped
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tgVps := tgs[0]

	require.NotNil(t, tgVps)
	require.NotNil(t, tgVps.Targets)
	require.Equal(t, 1, len(tgVps.Targets))

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestBadIpVps(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	modelV := Model{
		Name:                "model name test",
		Disk:                40,
		MaximumAdditionalIP: 1,
		Memory:              2048,
		Offer:               "VPS abc",
		Version:             "2019v1",
		Vcore:               1,
	}

	vps := Vps{
		Model:       modelV,
		Zone:        "zone",
		Cluster:     "cluster_test",
		DisplayName: "test_name",
		Name:        "abc",
		NetbootMode: "local",
		State:       "running",
		MemoryLimit: 2048,
		OfferType:   "ssd",
	}

	vpsMap := map[string]VpsData{"abc": {Vps: vps, IPs: []string{"192.0.2.1.6"}}}
	initMockVps(vpsMap)

	conf, err := getMockConf("vps")
	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d := newVpsDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(tgs))

	tgVps := tgs[0]
	require.NotNil(t, tgVps)
	require.Nil(t, tgVps.Targets)

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestVpsRefreshWithoutIPv4(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	modelV := Model{
		Name:                "model name test",
		Disk:                40,
		MaximumAdditionalIP: 1,
		Memory:              2048,
		Offer:               "VPS abc",
		Version:             "2019v1",
		Vcore:               1,
	}

	vps := Vps{
		Model:       modelV,
		Zone:        "zone",
		Cluster:     "cluster_test",
		DisplayName: "test_name",
		Name:        "abc",
		NetbootMode: "local",
		State:       "running",
		MemoryLimit: 2048,
		OfferType:   "ssd",
	}

	vpsMap := map[string]VpsData{"abc": {Vps: vps, IPs: []string{"2001:0db8:0000:0000:0000:0000:0000:0001"}}}
	initMockVps(vpsMap)

	conf, err := getMockConf("vps")
	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d := newVpsDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tgVps := tgs[0]
	require.NotNil(t, tgVps)
	require.NotNil(t, tgVps.Targets)
	require.Equal(t, 1, len(tgVps.Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                             "2001:0db8:0000:0000:0000:0000:0000:0001",
			"__meta_ovhcloud_vps_ipv4":                "",
			"__meta_ovhcloud_vps_ipv6":                "2001:0db8:0000:0000:0000:0000:0000:0001",
			"__meta_ovhcloud_vps_cluster":             "cluster_test",
			"__meta_ovhcloud_vps_datacenter":          "[]",
			"__meta_ovhcloud_vps_disk":                "40",
			"__meta_ovhcloud_vps_displayName":         "test_name",
			"__meta_ovhcloud_vps_maximumAdditionalIp": "1",
			"__meta_ovhcloud_vps_memory":              "2048",
			"__meta_ovhcloud_vps_memoryLimit":         "2048",
			"__meta_ovhcloud_vps_model_name":          "model name test",
			"__meta_ovhcloud_vps_name":                "abc",
			"__meta_ovhcloud_vps_netbootMode":         "local",
			"__meta_ovhcloud_vps_offer":               "VPS abc",
			"__meta_ovhcloud_vps_offerType":           "ssd",
			"__meta_ovhcloud_vps_state":               "running",
			"__meta_ovhcloud_vps_vcore":               "1",
			"__meta_ovhcloud_vps_version":             "2019v1",
			"__meta_ovhcloud_vps_zone":                "zone",
			"instance":                                "abc",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tgVps.Targets[i])
		})
	}

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestVpsRefresh(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	modelV := Model{
		Name:                "model name test",
		Disk:                40,
		MaximumAdditionalIP: 1,
		Memory:              2048,
		Offer:               "VPS abc",
		Version:             "2019v1",
		Vcore:               1,
	}

	vps := Vps{
		Model:       modelV,
		Zone:        "zone",
		Cluster:     "cluster_test",
		DisplayName: "test_name",
		Name:        "abc",
		NetbootMode: "local",
		State:       "running",
		MemoryLimit: 2048,
		OfferType:   "ssd",
	}

	vpsMap := map[string]VpsData{"abc": {Vps: vps, IPs: []string{"192.0.2.1", "2001:0db8:0000:0000:0000:0000:0000:0001"}}}
	initMockVps(vpsMap)

	conf, err := getMockConf("vps")
	require.NoError(t, err)

	//	conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d := newVpsDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tgVps := tgs[0]
	require.NotNil(t, tgVps)
	require.NotNil(t, tgVps.Targets)
	require.Equal(t, 1, len(tgVps.Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                             "192.0.2.1",
			"__meta_ovhcloud_vps_ipv4":                "192.0.2.1",
			"__meta_ovhcloud_vps_ipv6":                "2001:0db8:0000:0000:0000:0000:0000:0001",
			"__meta_ovhcloud_vps_cluster":             "cluster_test",
			"__meta_ovhcloud_vps_datacenter":          "[]",
			"__meta_ovhcloud_vps_disk":                "40",
			"__meta_ovhcloud_vps_displayName":         "test_name",
			"__meta_ovhcloud_vps_maximumAdditionalIp": "1",
			"__meta_ovhcloud_vps_memory":              "2048",
			"__meta_ovhcloud_vps_memoryLimit":         "2048",
			"__meta_ovhcloud_vps_model_name":          "model name test",
			"__meta_ovhcloud_vps_name":                "abc",
			"__meta_ovhcloud_vps_netbootMode":         "local",
			"__meta_ovhcloud_vps_offer":               "VPS abc",
			"__meta_ovhcloud_vps_offerType":           "ssd",
			"__meta_ovhcloud_vps_state":               "running",
			"__meta_ovhcloud_vps_vcore":               "1",
			"__meta_ovhcloud_vps_version":             "2019v1",
			"__meta_ovhcloud_vps_zone":                "zone",
			"instance":                                "abc",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tgVps.Targets[i])
		})
	}

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}
