// Copyright 2013 The Prometheus Authors
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

package retrieval

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
)

func mustNewRegexp(s string) config.Regexp {
	re, err := config.NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

func TestPopulateLabels(t *testing.T) {
	cases := []struct {
		in      labels.Labels
		cfg     *config.ScrapeConfig
		res     labels.Labels
		resOrig labels.Labels
		err     error
	}{
		// Regular population of scrape config options.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "value",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4:1000",
				model.InstanceLabel:    "1.2.3.4:1000",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "value",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4:1000",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "value",
			}),
		},
		// Pre-define/overwrite scrape config labels.
		// Leave out port and expect it to be defaulted to scheme.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4:80",
				model.InstanceLabel:    "1.2.3.4:80",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:     "1.2.3.4",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			}),
		},
		// Provide instance label. HTTPS port default for IPv6.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel:  "[::1]",
				model.InstanceLabel: "custom-instance",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "[::1]:443",
				model.InstanceLabel:    "custom-instance",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.AddressLabel:     "[::1]",
				model.InstanceLabel:    "custom-instance",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
			}),
		},
		// Address label missing.
		{
			in: labels.FromStrings("custom", "value"),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res:     nil,
			resOrig: nil,
			err:     fmt.Errorf("no address"),
		},
		// Address label missing, but added in relabelling.
		{
			in: labels.FromStrings("custom", "host:1234"),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
				RelabelConfigs: []*config.RelabelConfig{
					{
						Action:       config.RelabelReplace,
						Regex:        mustNewRegexp("(.*)"),
						SourceLabels: model.LabelNames{"custom"},
						Replacement:  "${1}",
						TargetLabel:  string(model.AddressLabel),
					},
				},
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "host:1234",
				model.InstanceLabel:    "host:1234",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "host:1234",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "host:1234",
			}),
		},
		// Address label missing, but added in relabelling.
		{
			in: labels.FromStrings("custom", "host:1234"),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
				RelabelConfigs: []*config.RelabelConfig{
					{
						Action:       config.RelabelReplace,
						Regex:        mustNewRegexp("(.*)"),
						SourceLabels: model.LabelNames{"custom"},
						Replacement:  "${1}",
						TargetLabel:  string(model.AddressLabel),
					},
				},
			},
			res: labels.FromMap(map[string]string{
				model.AddressLabel:     "host:1234",
				model.InstanceLabel:    "host:1234",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "host:1234",
			}),
			resOrig: labels.FromMap(map[string]string{
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "host:1234",
			}),
		},
		// Invalid UTF-8 in label.
		{
			in: labels.FromMap(map[string]string{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "\xbd",
			}),
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res:     nil,
			resOrig: nil,
			err:     fmt.Errorf("invalid label value for \"custom\": \"\\xbd\""),
		},
	}
	for i, c := range cases {
		in := c.in.Copy()

		res, orig, err := populateLabels(c.in, c.cfg)
		if !reflect.DeepEqual(err, c.err) {
			t.Fatalf("case %d: wanted %v error, got %v", i, c.err, err)
		}
		if !reflect.DeepEqual(c.in, in) {
			t.Errorf("case %d: input lset was changed was\n\t%+v\n now\n\t%+v", i, in, c.in)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Errorf("case %d: expected res\n\t%+v\n got\n\t%+v", i, c.res, res)
		}
		if !reflect.DeepEqual(orig, c.resOrig) {
			t.Errorf("case %d: expected resOrig\n\t%+v\n got\n\t%+v", i, c.resOrig, orig)
		}
	}
}
