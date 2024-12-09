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
	"net/http"

	"github.com/prometheus/common/version"
)

var userAgent = version.PrometheusUserAgent()

type Applications struct {
	VersionsDelta int           `xml:"versions__delta"`
	AppsHashcode  string        `xml:"apps__hashcode"`
	Applications  []Application `xml:"application"`
}

type Application struct {
	Name      string     `xml:"name"`
	Instances []Instance `xml:"instance"`
}

type Port struct {
	Port    int  `xml:",chardata"`
	Enabled bool `xml:"enabled,attr"`
}

type Instance struct {
	HostName                      string          `xml:"hostName"`
	HomePageURL                   string          `xml:"homePageUrl"`
	StatusPageURL                 string          `xml:"statusPageUrl"`
	HealthCheckURL                string          `xml:"healthCheckUrl"`
	App                           string          `xml:"app"`
	IPAddr                        string          `xml:"ipAddr"`
	VipAddress                    string          `xml:"vipAddress"`
	SecureVipAddress              string          `xml:"secureVipAddress"`
	Status                        string          `xml:"status"`
	Port                          *Port           `xml:"port"`
	SecurePort                    *Port           `xml:"securePort"`
	DataCenterInfo                *DataCenterInfo `xml:"dataCenterInfo"`
	Metadata                      *MetaData       `xml:"metadata"`
	IsCoordinatingDiscoveryServer bool            `xml:"isCoordinatingDiscoveryServer"`
	ActionType                    string          `xml:"actionType"`
	CountryID                     int             `xml:"countryId"`
	InstanceID                    string          `xml:"instanceId"`
}

type MetaData struct {
	Items []Tag `xml:",any"`
}

type Tag struct {
	XMLName xml.Name
	Content string `xml:",innerxml"`
}

type DataCenterInfo struct {
	Name     string    `xml:"name"`
	Class    string    `xml:"class,attr"`
	Metadata *MetaData `xml:"metadata"`
}

const appListPath string = "/apps"

func fetchApps(ctx context.Context, server string, client *http.Client) (*Applications, error) {
	url := fmt.Sprintf("%s%s", server, appListPath)

	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request = request.WithContext(ctx)
	request.Header.Add("User-Agent", userAgent)

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("non 2xx status '%d' response during eureka service discovery", resp.StatusCode)
	}

	var apps Applications
	err = xml.NewDecoder(resp.Body).Decode(&apps)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", url, err)
	}
	return &apps, nil
}
