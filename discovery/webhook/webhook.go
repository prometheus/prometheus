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

package webhook

import (
	"github.com/prometheus/common/model"
	"time"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"io/ioutil"
	"net/http"
	"encoding/json"
	"errors"
	"github.com/go-kit/kit/log/level"
)

var (
	tr = http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: false,
		DisableKeepAlives:  false,
	}
	client = http.Client{
		Transport: &tr,
	}
)

var (
	// DefaultSDConfig is the default webhook SD configuration.
	DefaultSDConfig = SDConfig{
		RefreshInterval: model.Duration(5 * time.Second),
	}
)

// SDConfig is the configuration for file based discovery.
type SDConfig struct {
	Url             string         `yaml:"url"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return nil
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

	th := time.NewTicker(d.interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-th.C:
			tg, err := fetchTargets(d.url)
			if err != nil {
				level.Error(d.logger).Log("msg", "Error reading url", "url", d.url, "err", err)
			} else {
				ch <- tg
			}
		}
	}
}

func fetchTargets(url string) ([]*targetgroup.Group, error) {

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if nil != err {
		return nil, err
	}
	tg := make([]*targetgroup.Group, 0)
	json.Unmarshal(body, &tg)

	for i, tg := range tg {
		if tg == nil {
			err = errors.New("nil target group item found")
			return nil, err
		}

		tg.Source = fileSource(url, i)
		if tg.Labels == nil {
			tg.Labels = model.LabelSet{}
		}
	}

	return tg, nil
}

func fileSource(url string, i int) string {
	return fmt.Sprintf("%s:%d", url, i)
}
