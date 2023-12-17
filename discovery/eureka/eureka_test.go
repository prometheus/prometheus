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

package eureka

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func testUpdateServices(respHandler http.HandlerFunc) ([]*targetgroup.Group, error) {
	// Create a test server with mock HTTP handler.
	ts := httptest.NewServer(respHandler)
	defer ts.Close()

	conf := SDConfig{
		Server: ts.URL,
	}

	md, err := NewDiscovery(&conf, nil, prometheus.NewRegistry())
	if err != nil {
		return nil, err
	}

	return md.refresh(context.Background())
}

func TestEurekaSDHandleError(t *testing.T) {
	var (
		errTesting  = "non 2xx status '500' response during eureka service discovery"
		respHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, ``)
		}
	)
	tgs, err := testUpdateServices(respHandler)

	require.EqualError(t, err, errTesting)
	require.Empty(t, tgs)
}

func TestEurekaSDEmptyList(t *testing.T) {
	var (
		appsXML = `<applications>
<versions__delta>1</versions__delta>
<apps__hashcode/>
</applications>`
		respHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, appsXML)
		}
	)
	tgs, err := testUpdateServices(respHandler)
	require.NoError(t, err)
	require.Len(t, tgs, 1)
}

func TestEurekaSDSendGroup(t *testing.T) {
	var (
		appsXML = `<applications>
  <versions__delta>1</versions__delta>
  <apps__hashcode>UP_4_</apps__hashcode>
  <application>
    <name>CONFIG-SERVICE</name>
    <instance>
      <instanceId>config-service001.test.com:config-service:8080</instanceId>
      <hostName>config-service001.test.com</hostName>
      <app>CONFIG-SERVICE</app>
      <ipAddr>192.133.83.31</ipAddr>
      <status>UP</status>
      <overriddenstatus>UNKNOWN</overriddenstatus>
      <port enabled="true">8080</port>
      <securePort enabled="false">8080</securePort>
      <countryId>1</countryId>
      <dataCenterInfo class="com.netflix.appinfo.InstanceInfo$DefaultDataCenterInfo">
        <name>MyOwn</name>
      </dataCenterInfo>
      <leaseInfo>
        <renewalIntervalInSecs>30</renewalIntervalInSecs>
        <durationInSecs>90</durationInSecs>
        <registrationTimestamp>1596003469304</registrationTimestamp>
        <lastRenewalTimestamp>1596110179310</lastRenewalTimestamp>
        <evictionTimestamp>0</evictionTimestamp>
        <serviceUpTimestamp>1547190033103</serviceUpTimestamp>
      </leaseInfo>
      <metadata>
        <instanceId>config-service001.test.com:config-service:8080</instanceId>
      </metadata>
      <homePageUrl>http://config-service001.test.com:8080/</homePageUrl>
      <statusPageUrl>http://config-service001.test.com:8080/info</statusPageUrl>
      <healthCheckUrl>http://config-service001.test.com 8080/health</healthCheckUrl>
      <vipAddress>config-service</vipAddress>
      <isCoordinatingDiscoveryServer>false</isCoordinatingDiscoveryServer>
      <lastUpdatedTimestamp>1596003469304</lastUpdatedTimestamp>
      <lastDirtyTimestamp>1596003469304</lastDirtyTimestamp>
      <actionType>ADDED</actionType>
    </instance>
    <instance>
      <instanceId>config-service002.test.com:config-service:8080</instanceId>
      <hostName>config-service002.test.com</hostName>
      <app>CONFIG-SERVICE</app>
      <ipAddr>192.133.83.31</ipAddr>
      <status>UP</status>
      <overriddenstatus>UNKNOWN</overriddenstatus>
      <port enabled="true">8080</port>
      <securePort enabled="false">8080</securePort>
      <countryId>1</countryId>
      <dataCenterInfo class="com.netflix.appinfo.InstanceInfo$DefaultDataCenterInfo">
        <name>MyOwn</name>
      </dataCenterInfo>
      <leaseInfo>
        <renewalIntervalInSecs>30</renewalIntervalInSecs>
        <durationInSecs>90</durationInSecs>
        <registrationTimestamp>1596003469304</registrationTimestamp>
        <lastRenewalTimestamp>1596110179310</lastRenewalTimestamp>
        <evictionTimestamp>0</evictionTimestamp>
        <serviceUpTimestamp>1547190033103</serviceUpTimestamp>
      </leaseInfo>
      <metadata>
        <instanceId>config-service002.test.com:config-service:8080</instanceId>
      </metadata>
      <homePageUrl>http://config-service002.test.com:8080/</homePageUrl>
      <statusPageUrl>http://config-service002.test.com:8080/info</statusPageUrl>
      <healthCheckUrl>http://config-service002.test.com:8080/health</healthCheckUrl>
      <vipAddress>config-service</vipAddress>
      <isCoordinatingDiscoveryServer>false</isCoordinatingDiscoveryServer>
      <lastUpdatedTimestamp>1596003469304</lastUpdatedTimestamp>
      <lastDirtyTimestamp>1596003469304</lastDirtyTimestamp>
      <actionType>ADDED</actionType>
    </instance>
  </application>
  <application>
    <name>META-SERVICE</name>
    <instance>
      <instanceId>meta-service002.test.com:meta-service:8080</instanceId>
      <hostName>meta-service002.test.com</hostName>
      <app>META-SERVICE</app>
      <ipAddr>192.133.87.237</ipAddr>
      <status>UP</status>
      <overriddenstatus>UNKNOWN</overriddenstatus>
      <port enabled="true">8080</port>
      <securePort enabled="false">443</securePort>
      <countryId>1</countryId>
      <dataCenterInfo class="com.netflix.appinfo.InstanceInfo$DefaultDataCenterInfo">
        <name>MyOwn</name>
      </dataCenterInfo>
      <leaseInfo>
        <renewalIntervalInSecs>30</renewalIntervalInSecs>
        <durationInSecs>90</durationInSecs>
        <registrationTimestamp>1535444352472</registrationTimestamp>
        <lastRenewalTimestamp>1596110168846</lastRenewalTimestamp>
        <evictionTimestamp>0</evictionTimestamp>
        <serviceUpTimestamp>1535444352472</serviceUpTimestamp>
      </leaseInfo>
      <metadata>
        <project>meta-service</project>
        <management.port>8090</management.port>
      </metadata>
      <homePageUrl>http://meta-service002.test.com:8080/</homePageUrl>
      <statusPageUrl>http://meta-service002.test.com:8080/info</statusPageUrl>
      <healthCheckUrl>http://meta-service002.test.com:8080/health</healthCheckUrl>
      <vipAddress>meta-service</vipAddress>
      <secureVipAddress>meta-service</secureVipAddress>
      <isCoordinatingDiscoveryServer>false</isCoordinatingDiscoveryServer>
      <lastUpdatedTimestamp>1535444352472</lastUpdatedTimestamp>
      <lastDirtyTimestamp>1535444352398</lastDirtyTimestamp>
      <actionType>ADDED</actionType>
    </instance>
    <instance>
      <instanceId>meta-service001.test.com:meta-service:8080</instanceId>
      <hostName>meta-service001.test.com</hostName>
      <app>META-SERVICE</app>
      <ipAddr>192.133.87.236</ipAddr>
      <status>UP</status>
      <overriddenstatus>UNKNOWN</overriddenstatus>
      <port enabled="true">8080</port>
      <securePort enabled="false">443</securePort>
      <countryId>1</countryId>
      <dataCenterInfo class="com.netflix.appinfo.InstanceInfo$DefaultDataCenterInfo">
        <name>MyOwn</name>
      </dataCenterInfo>
      <leaseInfo>
        <renewalIntervalInSecs>30</renewalIntervalInSecs>
        <durationInSecs>90</durationInSecs>
        <registrationTimestamp>1535444352472</registrationTimestamp>
        <lastRenewalTimestamp>1596110168846</lastRenewalTimestamp>
        <evictionTimestamp>0</evictionTimestamp>
        <serviceUpTimestamp>1535444352472</serviceUpTimestamp>
      </leaseInfo>
      <metadata>
        <project>meta-service</project>
        <management.port>8090</management.port>
      </metadata>
      <homePageUrl>http://meta-service001.test.com:8080/</homePageUrl>
      <statusPageUrl>http://meta-service001.test.com:8080/info</statusPageUrl>
      <healthCheckUrl>http://meta-service001.test.com:8080/health</healthCheckUrl>
      <vipAddress>meta-service</vipAddress>
      <secureVipAddress>meta-service</secureVipAddress>
      <isCoordinatingDiscoveryServer>false</isCoordinatingDiscoveryServer>
      <lastUpdatedTimestamp>1535444352472</lastUpdatedTimestamp>
      <lastDirtyTimestamp>1535444352398</lastDirtyTimestamp>
      <actionType>ADDED</actionType>
    </instance>
  </application>
</applications>`
		respHandler = func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, appsXML)
		}
	)

	tgs, err := testUpdateServices(respHandler)
	require.NoError(t, err)
	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.Equal(t, "eureka", tg.Source)
	require.Len(t, tg.Targets, 4)

	tgt := tg.Targets[0]
	require.Equal(t, tgt[model.AddressLabel], model.LabelValue("config-service001.test.com:8080"))

	tgt = tg.Targets[2]
	require.Equal(t, tgt[model.AddressLabel], model.LabelValue("meta-service002.test.com:8080"))
}
