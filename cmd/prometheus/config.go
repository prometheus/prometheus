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
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/asaskevich/govalidator"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/tsdb"
	"github.com/prometheus/prometheus/web"
	"github.com/spf13/pflag"
)

// cfg contains immutable configuration parameters for a running Prometheus
// server. It is populated by its flag set.
var cfg = struct {
	fs *pflag.FlagSet

	printVersion bool
	configFile   string

	localStoragePath   string
	localStorageEngine string
	notifier           notifier.Options
	notifierTimeout    time.Duration
	queryEngine        promql.EngineOptions
	web                web.Options
	tsdb               tsdb.Options

	alertmanagerURLs stringset
	prometheusURL    string

	// Deprecated storage flags, kept for backwards compatibility.
	deprecatedMemoryChunks       uint64
	deprecatedMaxChunksToPersist uint64
}{
	alertmanagerURLs: stringset{},
	notifier: notifier.Options{
		Registerer: prometheus.DefaultRegisterer,
	},
}

func parse(args []string) error {
	err := cfg.fs.Parse(args)
	if err != nil || len(cfg.fs.Args()) != 0 {
		if err != flag.ErrHelp {
			log.Errorf("Invalid command line arguments. Help: %s -h", os.Args[0])
		}
		if err == nil {
			err = fmt.Errorf("Non-flag argument on command line: %q", cfg.fs.Args()[0])
		}
		return err
	}

	if promql.StalenessDelta < 0 {
		return fmt.Errorf("negative staleness delta: %s", promql.StalenessDelta)
	}

	if err := parsePrometheusURL(); err != nil {
		return err
	}
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

	return nil
}

func parsePrometheusURL() error {
	if cfg.prometheusURL == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return err
		}
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
		Timeout:    cfg.notifierTimeout,
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

var helpTmpl = `
usage: prometheus [<args>]
{{ range $cat, $flags := . }}{{ if ne $cat "." }} == {{ $cat | upper }} =={{ end }}
  {{ range $flags }}
   -{{ .Name }} {{ .DefValue | quote }}
      {{ .Usage | wrap 80 6 }}
  {{ end }}
{{ end }}
`

func usage() {
	helpTmpl = strings.TrimSpace(helpTmpl)
	t := template.New("usage")
	t = t.Funcs(template.FuncMap{
		"wrap": func(width, indent int, s string) (ns string) {
			width = width - indent
			length := indent
			for _, w := range strings.SplitAfter(s, " ") {
				if length+len(w) > width {
					ns += "\n" + strings.Repeat(" ", indent)
					length = 0
				}
				ns += w
				length += len(w)
			}
			return strings.TrimSpace(ns)
		},
		"quote": func(s string) string {
			if len(s) == 0 || s == "false" || s == "true" || unicode.IsDigit(rune(s[0])) {
				return s
			}
			return fmt.Sprintf("%q", s)
		},
		"upper": strings.ToUpper,
	})
	t = template.Must(t.Parse(helpTmpl))

	groups := make(map[string][]*pflag.Flag)

	// Bucket flags into groups based on the first of their dot-separated levels.
	cfg.fs.VisitAll(func(fl *pflag.Flag) {
		parts := strings.SplitN(fl.Name, ".", 2)
		if len(parts) == 1 {
			groups["."] = append(groups["."], fl)
		} else {
			name := parts[0]
			groups[name] = append(groups[name], fl)
		}
	})
	for cat, fl := range groups {
		if len(fl) < 2 && cat != "." {
			groups["."] = append(groups["."], fl...)
			delete(groups, cat)
		}
	}

	if err := t.Execute(os.Stdout, groups); err != nil {
		panic(fmt.Errorf("error executing usage template: %s", err))
	}
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
