// Copyright 2015 The Prometheus Authors
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
	"fmt"
	"time"
	"net"
	"net/http"
	"bytes"
	"encoding/json"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
)

var (
	eurekaSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_eureka_refresh_failures_total",
			Help: "The number of Eureka scrape failures.",
		})
	eurekaSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_eureka_refresh_duration_seconds",
			Help: "The duration of a Eureka refresh in seconds.",
		})
	// DefaultSDConfig is the default EC2 SD configuration.
	DefaultSDConfig = SDConfig{
		Eureka:	"127.0.0.1:8761",
		RefreshInterval: model.Duration(60 * time.Second),
	}
)

// Filter is the configuration for filtering EC2 instances.
type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// SDConfig is the configuration for EC2 based service discovery.
type SDConfig struct {
	Eureka        string             `yaml:"eureka"`
	RefreshInterval model.Duration     `yaml:"refresh_interval,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return nil
}

func init() {
	prometheus.MustRegister(eurekaSDRefreshFailuresCount)
	prometheus.MustRegister(eurekaSDRefreshDuration)
}

// Discovery periodically performs EC2-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	eureka   string
	interval time.Duration
	logger   log.Logger
}

type EurekaResult struct {
	ApplicationList EurekaApplicationList `json:"applications"`
}

type EurekaApplicationList struct {
	Application []EurekaApplication `json:"application"`
}

type EurekaApplication struct {
	Name string `json:"name"`
	Instances []EurekaInstances `json:"instance"`
}

type EurekaInstances struct {
	App string `json:"app"`
	IpAddr string `json:"ipAddr"`
	Port Port `json:"port"`
	SecurePort Port `json:"securePort"`
	CountryId int `json:"countryId"`
	InstanceId string `json:"instanceId"`
	HomepageURL string `json:"homePageUrl"`
}

type Port struct {
	Port int64 `json:"$"`
}


// NewDiscovery returns a new EurekaDiscovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &Discovery{
		interval: time.Duration(conf.RefreshInterval),
		eureka:   conf.Eureka,
		logger:   logger,
	}
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		level.Error(d.logger).Log("msg", "Refresh failed", "err", err)
	} else {
		select {
		case ch <- []*targetgroup.Group{tg}:
		case <-ctx.Done():
			return
		}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := d.refresh()
			if err != nil {
				level.Error(d.logger).Log("msg", "Refresh failed", "err", err)
				continue
			}

			select {
			case ch <- []*targetgroup.Group{tg}:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *Discovery) refresh() (tg *targetgroup.Group, err error) {
	t0 := time.Now()
	defer func() {
		eurekaSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			eurekaSDRefreshFailuresCount.Inc()
		}
	}()

	client := &http.Client{}
	req, err := http.NewRequest("GET", d.eureka, nil)
	if(err != nil)  {
		return nil, fmt.Errorf("could create new request to eureka %s", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if(err != nil)  {
		return nil, fmt.Errorf("could create new request to eureka %s", err)
	}

	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	
	// parsing JSON	
	var er EurekaResult
	err = json.Unmarshal([]byte(buf.String()), &er)
	if(err != nil) {
		return nil, fmt.Errorf("could not parse JSON response from eureka (%s)", err)
	}

	bytes, _ := json.Marshal(er)
	
	tg = &targetgroup.Group{
		Source: d.eureka,
	}

	for i := 0; i < len(er.ApplicationList.Application); i++ {
		for k := 0; k < len(er.ApplicationList.Application[i].Instances); k++ {
			
			labels := model.LabelSet{
				"EurekaInstanceID" : model.LabelValue(er.ApplicationList.Application[i].Instances[k].InstanceId),
			}

			addr := net.JoinHostPort(er.ApplicationList.Application[i].Instances[k].IpAddr,  fmt.Sprintf("%d", er.ApplicationList.Application[i].Instances[k].Port.Port))

			labels[model.AddressLabel] = model.LabelValue(addr)
			labels["app"] = model.LabelValue(er.ApplicationList.Application[i].Name)

			tg.Targets = append(tg.Targets, labels)

		}
	}

	return tg, nil
}
