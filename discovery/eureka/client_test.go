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

	"github.com/stretchr/testify/require"
)

func TestFetchApps(t *testing.T) {
	appsXML := `<applications>
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

	// Simulate apps with a valid XML response.
	respHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, appsXML)
	}
	// Create a test server with mock HTTP handler.
	ts := httptest.NewServer(http.HandlerFunc(respHandler))
	defer ts.Close()

	apps, err := fetchApps(context.TODO(), ts.URL, &http.Client{})
	require.NoError(t, err)

	require.Len(t, apps.Applications, 2)
	require.Equal(t, "CONFIG-SERVICE", apps.Applications[0].Name)
	require.Equal(t, "META-SERVICE", apps.Applications[1].Name)

	require.Len(t, apps.Applications[1].Instances, 2)
	require.Equal(t, "meta-service002.test.com:meta-service:8080", apps.Applications[1].Instances[0].InstanceID)
	require.Equal(t, "project", apps.Applications[1].Instances[0].Metadata.Items[0].XMLName.Local)
	require.Equal(t, "meta-service", apps.Applications[1].Instances[0].Metadata.Items[0].Content)
	require.Equal(t, "management.port", apps.Applications[1].Instances[0].Metadata.Items[1].XMLName.Local)
	require.Equal(t, "8090", apps.Applications[1].Instances[0].Metadata.Items[1].Content)
	require.Equal(t, "meta-service001.test.com:meta-service:8080", apps.Applications[1].Instances[1].InstanceID)
}

func Test500ErrorHttpResponse(t *testing.T) {
	// Simulate 500 error.
	respHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, ``)
	}
	// Create a test server with mock HTTP handler.
	ts := httptest.NewServer(http.HandlerFunc(respHandler))
	defer ts.Close()

	_, err := fetchApps(context.TODO(), ts.URL, &http.Client{})
	require.Error(t, err, "5xx HTTP response")
}
