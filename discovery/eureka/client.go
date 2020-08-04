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
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

type Applications struct {
	VersionsDelta int           `xml:"versions__delta"`
	AppsHashcode  string        `xml:"apps__hashcode"`
	Applications  []Application `xml:"application,omitempty"`
}

type Application struct {
	Name      string     `xml:"name"`
	Instances []Instance `xml:"instance"`
}

type Port struct {
	Port    int  `xml:",chardata" json:"$"`
	Enabled bool `xml:"enabled,attr" json:"@enabled"`
}

type Instance struct {
	HostName                      string          `xml:"hostName" json:"hostName"`
	HomePageURL                   string          `xml:"homePageUrl,omitempty" json:"homePageUrl,omitempty"`
	StatusPageURL                 string          `xml:"statusPageUrl" json:"statusPageUrl"`
	HealthCheckURL                string          `xml:"healthCheckUrl,omitempty" json:"healthCheckUrl,omitempty"`
	App                           string          `xml:"app" json:"app"`
	IPAddr                        string          `xml:"ipAddr" json:"ipAddr"`
	VipAddress                    string          `xml:"vipAddress" json:"vipAddress"`
	SecureVipAddress              string          `xml:"secureVipAddress,omitempty" json:"secureVipAddress,omitempty"`
	Status                        string          `xml:"status" json:"status"`
	Port                          *Port           `xml:"port,omitempty" json:"port,omitempty"`
	SecurePort                    *Port           `xml:"securePort,omitempty" json:"securePort,omitempty"`
	DataCenterInfo                *DataCenterInfo `xml:"dataCenterInfo" json:"dataCenterInfo"`
	LeaseInfo                     *LeaseInfo      `xml:"leaseInfo,omitempty" json:"leaseInfo,omitempty"`
	Metadata                      *MetaData       `xml:"metadata,omitempty" json:"metadata,omitempty"`
	IsCoordinatingDiscoveryServer bool            `xml:"isCoordinatingDiscoveryServer,omitempty" json:"isCoordinatingDiscoveryServer,omitempty"`
	LastUpdatedTimestamp          int             `xml:"lastUpdatedTimestamp,omitempty" json:"lastUpdatedTimestamp,omitempty"`
	LastDirtyTimestamp            int             `xml:"lastDirtyTimestamp,omitempty" json:"lastDirtyTimestamp,omitempty"`
	ActionType                    string          `xml:"actionType,omitempty" json:"actionType,omitempty"`
	Overriddenstatus              string          `xml:"overriddenstatus,omitempty" json:"overriddenstatus,omitempty"`
	CountryID                     int             `xml:"countryId,omitempty" json:"countryId,omitempty"`
	InstanceID                    string          `xml:"instanceId,omitempty" json:"instanceId,omitempty"`
}
type DataCenterInfo struct {
	Name     string              `xml:"name" json:"name"`
	Class    string              `xml:"class,attr" json:"@class"`
	Metadata *DataCenterMetadata `xml:"metadata,omitempty" json:"metadata,omitempty"`
}

type DataCenterMetadata struct {
	AmiLaunchIndex   string `xml:"ami-launch-index,omitempty" json:"ami-launch-index,omitempty"`
	LocalHostname    string `xml:"local-hostname,omitempty" json:"local-hostname,omitempty"`
	AvailabilityZone string `xml:"availability-zone,omitempty" json:"availability-zone,omitempty"`
	InstanceID       string `xml:"instance-id,omitempty" json:"instance-id,omitempty"`
	PublicIpv4       string `xml:"public-ipv4,omitempty" json:"public-ipv4,omitempty"`
	PublicHostname   string `xml:"public-hostname,omitempty" json:"public-hostname,omitempty"`
	AmiManifestPath  string `xml:"ami-manifest-path,omitempty" json:"ami-manifest-path,omitempty"`
	LocalIpv4        string `xml:"local-ipv4,omitempty" json:"local-ipv4,omitempty"`
	Hostname         string `xml:"hostname,omitempty" json:"hostname,omitempty"`
	AmiID            string `xml:"ami-id,omitempty" json:"ami-id,omitempty"`
	InstanceType     string `xml:"instance-type,omitempty" json:"instance-type,omitempty"`
}

type LeaseInfo struct {
	EvictionDurationInSecs uint `xml:"evictionDurationInSecs,omitempty" json:"evictionDurationInSecs,omitempty"`
	RenewalIntervalInSecs  int  `xml:"renewalIntervalInSecs,omitempty" json:"renewalIntervalInSecs,omitempty"`
	DurationInSecs         int  `xml:"durationInSecs,omitempty" json:"durationInSecs,omitempty"`
	RegistrationTimestamp  int  `xml:"registrationTimestamp,omitempty" json:"registrationTimestamp,omitempty"`
	LastRenewalTimestamp   int  `xml:"lastRenewalTimestamp,omitempty" json:"lastRenewalTimestamp,omitempty"`
	EvictionTimestamp      int  `xml:"evictionTimestamp,omitempty" json:"evictionTimestamp,omitempty"`
	ServiceUpTimestamp     int  `xml:"serviceUpTimestamp,omitempty" json:"serviceUpTimestamp,omitempty"`
}

const appListPath string = "/apps"

func fetchApps(ctx context.Context, server string, client *http.Client) (*Applications, error) {
	url := appsURL(server)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	request = request.WithContext(ctx)

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return nil, errors.Errorf("non 2xx status '%d' response during eureka service discovery", resp.StatusCode)
	}

	var apps Applications
	err = xml.NewDecoder(resp.Body).Decode(&apps)
	if err != nil {
		return nil, errors.Wrapf(err, "%q", url)
	}
	return &apps, nil
}

func appsURL(server string) string {
	return fmt.Sprintf("%s%s", server, appListPath)
}
