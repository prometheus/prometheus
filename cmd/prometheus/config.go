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

package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/tsdb"
	"github.com/prometheus/prometheus/web"
)

// cfg contains immutable configuration parameters for a running Prometheus
// server. It is populated by its flag set.
var cfg = struct {
	printVersion bool
	configFile   string

	localStoragePath   string
	localStorageEngine string
	notifier           notifier.Options
	notifierTimeout    model.Duration
	queryEngine        promql.EngineOptions
	web                web.Options
	tsdb               tsdb.Options
	lookbackDelta      model.Duration
	webTimeout         model.Duration
	queryTimeout       model.Duration

	alertmanagerURLs stringset
	prometheusURL    string

	logFormat string
	logLevel  string
}{
	// The defaults for model.Duration flag parsing.
	notifierTimeout: model.Duration(10 * time.Second),
	tsdb: tsdb.Options{
		MinBlockDuration: model.Duration(2 * time.Hour),
		Retention:        model.Duration(15 * 24 * time.Hour),
	},
	lookbackDelta: model.Duration(5 * time.Minute),
	webTimeout:    model.Duration(30 * time.Second),
	queryTimeout:  model.Duration(2 * time.Minute),

	alertmanagerURLs: stringset{},
	notifier: notifier.Options{
		Registerer: prometheus.DefaultRegisterer,
	},
}

func validate() error {
	if err := parsePrometheusURL(); err != nil {
		return errors.Wrapf(err, "parse external URL %q", cfg.prometheusURL)
	}

	cfg.web.ReadTimeout = time.Duration(cfg.webTimeout)
	// Default -web.route-prefix to path of -web.external-url.
	if cfg.web.RoutePrefix == "" {
		cfg.web.RoutePrefix = cfg.web.ExternalURL.Path
	}
	// RoutePrefix must always be at least '/'.
	cfg.web.RoutePrefix = "/" + strings.Trim(cfg.web.RoutePrefix, "/")

	for u := range cfg.alertmanagerURLs {
		if err := validateAlertmanagerURL(u); err != nil {
			return err
		}
	}

	if cfg.tsdb.MaxBlockDuration == 0 {
		cfg.tsdb.MaxBlockDuration = cfg.tsdb.Retention / 10
	}

	promql.LookbackDelta = time.Duration(cfg.lookbackDelta)

	cfg.queryEngine.Timeout = time.Duration(cfg.queryTimeout)

	return nil
}

func parsePrometheusURL() error {
	fmt.Println("promurl", cfg.prometheusURL)
	if cfg.prometheusURL == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return err
		}
		fmt.Println("listenaddr", cfg.web.ListenAddress)
		_, port, err := net.SplitHostPort(cfg.web.ListenAddress)
		if err != nil {
			return err
		}
		cfg.prometheusURL = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	if ok := govalidator.IsURL(cfg.prometheusURL); !ok {
		return fmt.Errorf("invalid Prometheus URL: %s", cfg.prometheusURL)
	}

	promURL, err := url.Parse(cfg.prometheusURL)
	if err != nil {
		return err
	}
	cfg.web.ExternalURL = promURL

	ppref := strings.TrimRight(cfg.web.ExternalURL.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	cfg.web.ExternalURL.Path = ppref
	return nil
}

func validateAlertmanagerURL(u string) error {
	if u == "" {
		return nil
	}
	if ok := govalidator.IsURL(u); !ok {
		return fmt.Errorf("invalid Alertmanager URL: %s", u)
	}
	url, err := url.Parse(u)
	if err != nil {
		return err
	}
	if url.Scheme == "" {
		return fmt.Errorf("missing scheme in Alertmanager URL: %s", u)
	}
	return nil
}

func parseAlertmanagerURLToConfig(us string) (*config.AlertmanagerConfig, error) {
	u, err := url.Parse(us)
	if err != nil {
		return nil, err
	}
	acfg := &config.AlertmanagerConfig{
		Scheme:     u.Scheme,
		PathPrefix: u.Path,
		Timeout:    time.Duration(cfg.notifierTimeout),
		ServiceDiscoveryConfig: config.ServiceDiscoveryConfig{
			StaticConfigs: []*config.TargetGroup{
				{
					Targets: []model.LabelSet{
						{
							model.AddressLabel: model.LabelValue(u.Host),
						},
					},
				},
			},
		},
	}

	if u.User != nil {
		acfg.HTTPClientConfig = config.HTTPClientConfig{
			BasicAuth: &config.BasicAuth{
				Username: u.User.Username(),
			},
		}

		if password, isSet := u.User.Password(); isSet {
			acfg.HTTPClientConfig.BasicAuth.Password = config.Secret(password)
		}
	}

	return acfg, nil
}

type stringset map[string]struct{}

func (ss stringset) Set(s string) error {
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			ss[v] = struct{}{}
		}
	}
	return nil
}

func (ss stringset) String() string {
	return strings.Join(ss.slice(), ",")
}

func (ss stringset) slice() []string {
	slice := make([]string, 0, len(ss))
	for k := range ss {
		slice = append(slice, k)
	}
	sort.Strings(slice)
	return slice
}
