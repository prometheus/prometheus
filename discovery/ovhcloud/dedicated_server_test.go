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

type DedicatedServerData struct {
	DedicatedServer DedicatedServer
	Err             error
	IPs             []string
	IPsErr          error
}

func initMockErrorDedicatedServerList(err error) {
	initMockAuth(123456)
	gock.New(mockURL).
		MatchHeader("Accept", "application/json").
		Get("/dedicated/server").
		ReplyError(err)
}

func initMockDedicatedServer(dedicatedServerList map[string]DedicatedServerData) {
	initMockAuth(123456)

	var dedicatedServerListName []string
	for dedicatedServerName := range dedicatedServerList {
		dedicatedServerListName = append(dedicatedServerListName, dedicatedServerName)
	}

	gock.New(mockURL).
		MatchHeader("Accept", "application/json").
		Get("/dedicated/server").
		Reply(200).
		JSON(dedicatedServerListName)

	for dedicatedServerName := range dedicatedServerList {
		if dedicatedServerList[dedicatedServerName].Err != nil {
			gock.New(mockURL).
				MatchHeader("Accept", "application/json").
				Get(fmt.Sprintf("/dedicated/server/%s", dedicatedServerName)).
				ReplyError(dedicatedServerList[dedicatedServerName].Err)
		} else {
			gock.New(mockURL).
				MatchHeader("Accept", "application/json").
				Get(fmt.Sprintf("/dedicated/server/%s", dedicatedServerName)).
				Reply(200).
				JSON(dedicatedServerList[dedicatedServerName].DedicatedServer)
			if dedicatedServerList[dedicatedServerName].IPsErr != nil {
				gock.New(mockURL).
					MatchHeader("Accept", "application/json").
					Get(fmt.Sprintf("/dedicated/server/%s/ips", dedicatedServerName)).
					ReplyError(dedicatedServerList[dedicatedServerName].IPsErr)
			} else {
				gock.New(mockURL).
					MatchHeader("Accept", "application/json").
					Get(fmt.Sprintf("/dedicated/server/%s/ips", dedicatedServerName)).
					Reply(200).
					JSON(dedicatedServerList[dedicatedServerName].IPs)
			}
		}
	}
}

func TestDedicatedServerWithBadConf(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	conf, err := getMockConf()
	require.NoError(t, err)

	conf.ApplicationKey = ""
	logger := testutil.NewLogger(t)
	d := newDedicatedServerDiscovery(&conf, logger)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.ErrorContains(t, err, "missing application key")

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestErrorDedicatedServerList(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})
	errTest := errors.New("error on get dedicated server list")
	initMockErrorDedicatedServerList(errTest)

	conf, err := getMockConf()
	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d := newDedicatedServerDiscovery(&conf, logger)

	ctx := context.Background()
	_, err = d.refresh(ctx)
	require.ErrorIs(t, err, errTest)
	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestErrorDedicatedServerDetail(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	dedicatedServer := DedicatedServer{
		State: "test",
	}

	errTest := errors.New("error on get dedicated server detail")
	initMockDedicatedServer(map[string]DedicatedServerData{"abcde": {DedicatedServer: dedicatedServer, IPs: []string{"1.2.3.5", "2001:0db8:0000:0000:0000:0000:0000:0001"}}, "errorTest": {Err: errTest}})
	conf, err := getMockConf()
	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d := newDedicatedServerDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tgDedicatedServer := tgs[0]
	require.NotNil(t, tgDedicatedServer)
	require.NotNil(t, tgDedicatedServer.Targets)
	require.Equal(t, 1, len(tgDedicatedServer.Targets))

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestBadIpDedicatedServer(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	dedicatedServer := DedicatedServer{
		State: "test",
	}

	initMockDedicatedServer(map[string]DedicatedServerData{"abcde": {DedicatedServer: dedicatedServer, IPs: []string{"1.2.3.5.6"}}})
	conf, err := getMockConf()
	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d := newDedicatedServerDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tgDedicatedServer := tgs[0]
	require.NotNil(t, tgDedicatedServer)
	require.Nil(t, tgDedicatedServer.Targets)
}

func TestDedicatedServerCallWithoutIPv4(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	dedicatedServer := DedicatedServer{
		State:           "test",
		CommercialRange: "Advance-1 Gen 2",
		LinkSpeed:       123,
		Rack:            "TESTRACK",
		NoIntervention:  false,
		Os:              "debian11_64",
		SupportLevel:    "pro",
		ServerID:        1234,
		Reverse:         "abcde-rev",
		Datacenter:      "gra3",
		Name:            "abcde",
	}

	initMockDedicatedServer(map[string]DedicatedServerData{"abcde": {DedicatedServer: dedicatedServer, IPs: []string{"2001:0db8:0000:0000:0000:0000:0000:0001"}}})
	conf, err := getMockConf()
	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d := newDedicatedServerDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(tgs))

	tgDedicatedServer := tgs[0]
	require.NotNil(t, tgDedicatedServer)
	require.NotNil(t, tgDedicatedServer.Targets)
	require.Equal(t, 1, len(tgDedicatedServer.Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__": "2001:0db8:0000:0000:0000:0000:0000:0001",
			"__meta_ovhcloud_dedicatedServer_commercialRange": "Advance-1 Gen 2",
			"__meta_ovhcloud_dedicatedServer_datacenter":      "gra3",
			"__meta_ovhcloud_dedicatedServer_ipv4":            "",
			"__meta_ovhcloud_dedicatedServer_ipv6":            "2001:0db8:0000:0000:0000:0000:0000:0001",
			"__meta_ovhcloud_dedicatedServer_linkSpeed":       "123",
			"__meta_ovhcloud_dedicatedServer_name":            "abcde",
			"__meta_ovhcloud_dedicatedServer_noIntervention":  "false",
			"__meta_ovhcloud_dedicatedServer_os":              "debian11_64",
			"__meta_ovhcloud_dedicatedServer_rack":            "TESTRACK",
			"__meta_ovhcloud_dedicatedServer_reverse":         "abcde-rev",
			"__meta_ovhcloud_dedicatedServer_serverId":        "1234",
			"__meta_ovhcloud_dedicatedServer_state":           "test",
			"__meta_ovhcloud_dedicatedServer_supportLevel":    "pro",
			"instance": "abcde",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tgDedicatedServer.Targets[i])
		})
	}

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}

func TestDedicatedServerCall(t *testing.T) {
	defer gock.Off()

	initMockAuthDetails(12345, map[string]string{"name": "test_name"})

	dedicatedServer := DedicatedServer{
		State:           "test",
		CommercialRange: "Advance-1 Gen 2",
		LinkSpeed:       123,
		Rack:            "TESTRACK",
		NoIntervention:  false,
		Os:              "debian11_64",
		SupportLevel:    "pro",
		ServerID:        1234,
		Reverse:         "abcde-rev",
		Datacenter:      "gra3",
		Name:            "abcde",
	}

	initMockDedicatedServer(map[string]DedicatedServerData{"abcde": {DedicatedServer: dedicatedServer, IPs: []string{"1.2.3.5", "2001:0db8:0000:0000:0000:0000:0000:0001"}}})
	conf, err := getMockConf()
	require.NoError(t, err)

	//  conf.Endpoint = mockURL
	logger := testutil.NewLogger(t)
	d := newDedicatedServerDiscovery(&conf, logger)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tgDedicatedServer := tgs[0]
	require.NotNil(t, tgDedicatedServer)
	require.NotNil(t, tgDedicatedServer.Targets)
	require.Equal(t, 1, len(tgDedicatedServer.Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__": "1.2.3.5",
			"__meta_ovhcloud_dedicatedServer_commercialRange": "Advance-1 Gen 2",
			"__meta_ovhcloud_dedicatedServer_datacenter":      "gra3",
			"__meta_ovhcloud_dedicatedServer_ipv4":            "1.2.3.5",
			"__meta_ovhcloud_dedicatedServer_ipv6":            "2001:0db8:0000:0000:0000:0000:0000:0001",
			"__meta_ovhcloud_dedicatedServer_linkSpeed":       "123",
			"__meta_ovhcloud_dedicatedServer_name":            "abcde",
			"__meta_ovhcloud_dedicatedServer_noIntervention":  "false",
			"__meta_ovhcloud_dedicatedServer_os":              "debian11_64",
			"__meta_ovhcloud_dedicatedServer_rack":            "TESTRACK",
			"__meta_ovhcloud_dedicatedServer_reverse":         "abcde-rev",
			"__meta_ovhcloud_dedicatedServer_serverId":        "1234",
			"__meta_ovhcloud_dedicatedServer_state":           "test",
			"__meta_ovhcloud_dedicatedServer_supportLevel":    "pro",
			"instance": "abcde",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tgDedicatedServer.Targets[i])
		})
	}

	// Verify that we don't have pending mocks
	require.Equal(t, gock.IsDone(), true)
}
