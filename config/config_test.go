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

package config

import (
	"encoding/json"
	"io/ioutil"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

var expectedConf = &Config{
	GlobalConfig: GlobalConfig{
		ScrapeInterval:     Duration(15 * time.Second),
		ScrapeTimeout:      DefaultGlobalConfig.ScrapeTimeout,
		EvaluationInterval: Duration(30 * time.Second),

		Labels: model.LabelSet{
			"monitor": "codelab",
			"foo":     "bar",
		},
	},

	RuleFiles: []string{
		"testdata/first.rules",
		"/absolute/second.rules",
		"testdata/my/*.rules",
	},

	ScrapeConfigs: []*ScrapeConfig{
		{
			JobName: "prometheus",

			HonorLabels:    true,
			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			BearerTokenFile: "testdata/valid_token_file",

			TargetGroups: []*TargetGroup{
				{
					Targets: []model.LabelSet{
						{model.AddressLabel: "localhost:9090"},
						{model.AddressLabel: "localhost:9191"},
					},
					Labels: model.LabelSet{
						"my":   "label",
						"your": "label",
					},
				},
			},

			FileSDConfigs: []*FileSDConfig{
				{
					Names:           []string{"foo/*.slow.json", "foo/*.slow.yml", "single/file.yml"},
					RefreshInterval: Duration(10 * time.Minute),
				},
				{
					Names:           []string{"bar/*.yaml"},
					RefreshInterval: Duration(5 * time.Minute),
				},
			},

			RelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: model.LabelNames{"job", "__meta_dns_srv_name"},
					TargetLabel:  "job",
					Separator:    ";",
					Regex:        MustNewRegexp("(.*)some-[regex]"),
					Replacement:  "foo-${1}",
					Action:       RelabelReplace,
				},
			},
		},
		{
			JobName: "service-x",

			ScrapeInterval: Duration(50 * time.Second),
			ScrapeTimeout:  Duration(5 * time.Second),

			BasicAuth: &BasicAuth{
				Username: "admin_name",
				Password: "admin_password",
			},
			MetricsPath: "/my_path",
			Scheme:      "https",

			DNSSDConfigs: []*DNSSDConfig{
				{
					Names: []string{
						"first.dns.address.domain.com",
						"second.dns.address.domain.com",
					},
					RefreshInterval: Duration(15 * time.Second),
					Type:            "SRV",
				},
				{
					Names: []string{
						"first.dns.address.domain.com",
					},
					RefreshInterval: Duration(30 * time.Second),
					Type:            "SRV",
				},
			},

			RelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: model.LabelNames{"job"},
					Regex:        MustNewRegexp("(.*)some-[regex]"),
					Separator:    ";",
					Action:       RelabelDrop,
				},
				{
					SourceLabels: model.LabelNames{"__address__"},
					TargetLabel:  "__tmp_hash",
					Modulus:      8,
					Separator:    ";",
					Action:       RelabelHashMod,
				},
				{
					SourceLabels: model.LabelNames{"__tmp_hash"},
					Regex:        MustNewRegexp("1"),
					Separator:    ";",
					Action:       RelabelKeep,
				},
			},
			MetricRelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: model.LabelNames{"__name__"},
					Regex:        MustNewRegexp("expensive_metric.*"),
					Separator:    ";",
					Action:       RelabelDrop,
				},
			},
		},
		{
			JobName: "service-y",

			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ConsulSDConfigs: []*ConsulSDConfig{
				{
					Server:       "localhost:1234",
					Services:     []string{"nginx", "cache", "mysql"},
					TagSeparator: DefaultConsulSDConfig.TagSeparator,
					Scheme:       DefaultConsulSDConfig.Scheme,
				},
			},
		},
		{
			JobName: "service-z",

			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  Duration(10 * time.Second),

			MetricsPath: "/metrics",
			Scheme:      "http",

			TLSConfig: TLSConfig{
				CertFile: "testdata/valid_cert_file",
				KeyFile:  "testdata/valid_key_file",
			},

			BearerToken: "avalidtoken",
		},
		{
			JobName: "service-kubernetes",

			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			KubernetesSDConfigs: []*KubernetesSDConfig{
				{
					Masters:        []URL{kubernetesSDHostURL()},
					Username:       "myusername",
					Password:       "mypassword",
					KubeletPort:    10255,
					RequestTimeout: Duration(10 * time.Second),
					RetryInterval:  Duration(1 * time.Second),
				},
			},
		},
		{
			JobName: "service-marathon",

			ScrapeInterval: Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			MarathonSDConfigs: []*MarathonSDConfig{
				{
					Servers: []string{
						"http://marathon.example.com:8080",
					},
					RefreshInterval: Duration(30 * time.Second),
				},
			},
		},
	},
	original: "",
}

func TestLoadConfig(t *testing.T) {
	// Parse a valid file that sets a global scrape timeout. This tests whether parsing
	// an overwritten default field in the global config permanently changes the default.
	if _, err := LoadFile("testdata/global_timeout.good.yml"); err != nil {
		t.Errorf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	c, err := LoadFile("testdata/conf.good.yml")
	if err != nil {
		t.Fatalf("Error parsing %s: %s", "testdata/conf.good.yml", err)
	}

	bgot, err := yaml.Marshal(c)
	if err != nil {
		t.Fatalf("%s", err)
	}

	bexp, err := yaml.Marshal(expectedConf)
	if err != nil {
		t.Fatalf("%s", err)
	}
	expectedConf.original = c.original

	if !reflect.DeepEqual(c, expectedConf) {
		t.Fatalf("%s: unexpected config result: \n\n%s\n expected\n\n%s", "testdata/conf.good.yml", bgot, bexp)
	}

	// String method must not reveal authentication credentials.
	s := c.String()
	if strings.Contains(s, "admin_name") || strings.Contains(s, "admin_password") {
		t.Fatalf("config's String method reveals authentication credentials.")
	}
}

var expectedErrors = []struct {
	filename string
	errMsg   string
}{
	{
		filename: "jobname.bad.yml",
		errMsg:   `"prom^etheus" is not a valid job name`,
	}, {
		filename: "jobname_dup.bad.yml",
		errMsg:   `found multiple scrape configs with job name "prometheus"`,
	}, {
		filename: "scrape_interval.bad.yml",
		errMsg:   `scrape timeout greater than scrape interval`,
	}, {
		filename: "labelname.bad.yml",
		errMsg:   `"not$allowed" is not a valid label name`,
	}, {
		filename: "labelname2.bad.yml",
		errMsg:   `"not:allowed" is not a valid label name`,
	}, {
		filename: "regex.bad.yml",
		errMsg:   "error parsing regexp",
	}, {
		filename: "regex_missing.bad.yml",
		errMsg:   "relabel configuration requires a regular expression",
	}, {
		filename: "modulus_missing.bad.yml",
		errMsg:   "relabel configuration for hashmod requires non-zero modulus",
	}, {
		filename: "rules.bad.yml",
		errMsg:   "invalid rule file path",
	}, {
		filename: "unknown_attr.bad.yml",
		errMsg:   "unknown fields in scrape_config: consult_sd_configs",
	}, {
		filename: "bearertoken.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	}, {
		filename: "bearertoken_basicauth.bad.yml",
		errMsg:   "at most one of basic_auth, bearer_token & bearer_token_file must be configured",
	}, {
		filename: "marathon_no_servers.bad.yml",
		errMsg:   "Marathon SD config must contain at least one Marathon server",
	},
}

func TestBadConfigs(t *testing.T) {
	for _, ee := range expectedErrors {
		_, err := LoadFile("testdata/" + ee.filename)
		if err == nil {
			t.Errorf("Expected error parsing %s but got none", ee.filename)
			continue
		}
		if !strings.Contains(err.Error(), ee.errMsg) {
			t.Errorf("Expected error for %s to contain %q but got: %s", ee.filename, ee.errMsg, err)
		}
	}
}

func TestBadTargetGroup(t *testing.T) {
	content, err := ioutil.ReadFile("testdata/tgroup.bad.json")
	if err != nil {
		t.Fatal(err)
	}
	var tg TargetGroup
	err = json.Unmarshal(content, &tg)
	if err == nil {
		t.Errorf("Expected unmarshal error but got none.")
	}
}

func TestEmptyConfig(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Unexpected error parsing empty config file: %s", err)
	}
	exp := DefaultConfig

	if !reflect.DeepEqual(*c, exp) {
		t.Fatalf("want %v, got %v", exp, c)
	}
}

func TestEmptyGlobalBlock(t *testing.T) {
	c, err := Load("global:\n")
	if err != nil {
		t.Fatalf("Unexpected error parsing empty config file: %s", err)
	}
	exp := DefaultConfig
	exp.original = "global:\n"

	if !reflect.DeepEqual(*c, exp) {
		t.Fatalf("want %v, got %v", exp, c)
	}
}

func kubernetesSDHostURL() URL {
	tURL, _ := url.Parse("https://localhost:1234")
	return URL{URL: tURL}
}
