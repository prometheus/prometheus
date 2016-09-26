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

package discovery

import (
	"fmt"
	"net/http"
	"time"
	"io/ioutil"
	"crypto/tls"
	"encoding/json"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
)

const (
	ambariLabelPrefix         = model.MetaLabelPrefix + "ambari_"

)

var (
	ambariSDScrapesCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ambari_sd_scrapes_total",
			Help:      "The number of Ambari-SD scrapes.",
		})
	ambariSDScrapeFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "ambari_sd_scrape_failures_total",
			Help:      "The number of Ambari-SD scrape failures.",
		})
	ambariSDScrapeDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "ambari_sd_scrape_duration",
			Help:      "The duration of a Ambari-SD scrape in seconds.",
		})
)

func init() {
	prometheus.MustRegister(ambariSDScrapesCount)
	prometheus.MustRegister(ambariSDScrapeFailuresCount)
	prometheus.MustRegister(ambariSDScrapeDuration)
}

type AmbariCluster struct {
	Href string `json:"href"`
	Items []struct {
		Href string `json:"href"`
		Clusters struct {
			     ClusterName string `json:"cluster_name"`
			     Version string `json:"version"`
		     } `json:"Clusters"`
	} `json:"items"`
}


// AmbariDiscovery periodically performs Ambari-SD requests. It implements
// the TargetProvider interface.
type AmbariDiscovery struct {
	cfg	 *config.AmbariSDConfig
	interval time.Duration
}

// NewAmbariDiscovery returns a new AmbariDiscovery which periodically refreshes its targets.
func NewAmbariDiscovery(conf *config.AmbariSDConfig) (*AmbariDiscovery, error) {
	ad := &AmbariDiscovery{
		cfg:      conf,
		interval: time.Duration(conf.RefreshInterval),
	}
	resp, err := ad.makeAmbariRequest("api/v1/clusters") //To test API connection
	clusters := new(AmbariCluster)
	if err = json.Unmarshal(resp, &clusters); err != nil {
		return nil, fmt.Errorf("error comminicating with Ambari API. API Did not return a valid JSON: %s", err)
	}
	for _,element := range clusters.Items {
		if element.Clusters.ClusterName == conf.Cluster{
			return ad, nil
		}
	}
	return nil, fmt.Errorf("error comminicating with Ambari API. Invalid cluster defined in config: %s", conf.Cluster)


}

func (ad *AmbariDiscovery) makeAmbariRequest(apiendpoint string) ([]uint8, error) {
	var tr *http.Transport
	if ad.cfg.ValidateSSL != true {
		tr = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	} else {
		tr = &http.Transport{
		}
	}
	client := &http.Client{Transport: tr}
	ambariUrl := fmt.Sprintf("%s://%s:%d/%s", ad.cfg.Proto, ad.cfg.Host, ad.cfg.AmbariPort, apiendpoint)
	req, err := http.NewRequest("GET", ambariUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("error communicating with ambari API: %s", err)
	}
	req.SetBasicAuth(ad.cfg.Username, ad.cfg.Password)
	resp, err := client.Do(req)
	if err != nil{
		return nil, fmt.Errorf("error communicating with ambari API: %s", err)
	}
	bodyText, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error comminicating with Ambari API. Status code was: %s", resp.Status)
	}
	return bodyText, err
}

// Run implements the TargetProvider interface.
func (ad *AmbariDiscovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

	// Get an initial set right away.
	tg, err := ad.refresh()
	if err != nil {
		log.Error(err)
	} else {
		ch <- []*config.TargetGroup{tg}
	}

	ticker := time.NewTicker(ad.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tg, err := ad.refresh()
			if err != nil {
				log.Error(err)
			} else {
				ch <- []*config.TargetGroup{tg}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (ad *AmbariDiscovery) refresh() (tg *config.TargetGroup, err error) {
	t0 := time.Now()
	defer func() {
		ambariSDScrapeDuration.Observe(time.Since(t0).Seconds())
		ambariSDScrapesCount.Inc()
		if err != nil {
			ambariSDScrapeFailuresCount.Inc()
		}
	}()

	tg = &config.TargetGroup{
		Source: fmt.Sprintf("AMBARI_%s_%s", ad.cfg.Host, ad.cfg.Proto),
	}

	if err != nil {
		return tg, fmt.Errorf("error retrieving scrape targets from ambari: %s", err)
	}
	return tg, nil
}
