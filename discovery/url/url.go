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

package url

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"io/ioutil"
	"net/http"
	"time"
)

var (
	DefaultSDConfig = SDConfig{
		RefreshInterval: model.Duration(5 * time.Second),
	}

	urlSDReadSuccessful = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "prometheus_sd_url_last_read_successful",
		Help: "Last url service discovery successful",
	}, []string{"url"})

	urlSDReadDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "prometheus_sd_url_read_duration_seconds",
		Help: "Url service discovery duration in seconds",
	}, []string{"url"})
)

func init() {
	prometheus.MustRegister(urlSDReadSuccessful)
	prometheus.MustRegister(urlSDReadDuration)
}

type SDConfig struct {
	Url             string         `yaml:"url"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
}

func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	return unmarshal((*plain)(c))
}

type Discovery struct {
	url      string
	interval time.Duration
	logger   log.Logger
}

func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	disc := &Discovery{
		url:      conf.Url,
		interval: time.Duration(conf.RefreshInterval),
		logger:   logger,
	}
	return disc
}

func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {

	refresh := func() {
		tg, err := d.readTargetGroups()
		if err != nil {
			urlSDReadSuccessful.WithLabelValues(d.url).Set(0)
			level.Error(d.logger).Log("msg", "Error reading targets from url", "url", d.url, "err", err)
		} else {
			ch <- tg
			urlSDReadSuccessful.WithLabelValues(d.url).Set(1)
		}
	}

	refresh()

	th := time.NewTicker(d.interval)
	defer th.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-th.C:
			refresh()
		}
	}
}

func (d *Discovery) readTargetGroups() ([]*targetgroup.Group, error) {

	start := time.Now()
	defer func() {
		urlSDReadDuration.WithLabelValues(d.url).Observe(time.Since(start).Seconds())
	}()

	resp, err := http.Get(d.url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if nil != err {
		return nil, err
	}
	tg := make([]*targetgroup.Group, 0)
	if err := json.Unmarshal(body, &tg); err != nil {
		return nil, err
	}

	for i, tg := range tg {
		if tg == nil {
			err = errors.New("nil target group item found")
			return nil, err
		}

		tg.Source = urlSource(d.url, i)
		if tg.Labels == nil {
			tg.Labels = model.LabelSet{}
		}
	}

	return tg, nil
}

func urlSource(url string, i int) string {
	return fmt.Sprintf("%s:%d", url, i)
}
