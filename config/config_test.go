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
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/discovery/azure"
	"github.com/prometheus/prometheus/discovery/consul"
	"github.com/prometheus/prometheus/discovery/dns"
	"github.com/prometheus/prometheus/discovery/ec2"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/marathon"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/discovery/triton"
	"github.com/prometheus/prometheus/discovery/zookeeper"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	sd_config "github.com/prometheus/prometheus/discovery/config"
	"github.com/prometheus/prometheus/util/testutil"
	"gopkg.in/yaml.v2"
)

func mustParseURL(u string) *config_util.URL {
	parsed, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	return &config_util.URL{URL: parsed}
}

var expectedConf = &Config{
	GlobalConfig: GlobalConfig{
		ScrapeInterval:     model.Duration(15 * time.Second),
		ScrapeTimeout:      DefaultGlobalConfig.ScrapeTimeout,
		EvaluationInterval: model.Duration(30 * time.Second),

		ExternalLabels: model.LabelSet{
			"monitor": "codelab",
			"foo":     "bar",
		},
	},

	RuleFiles: []string{
		filepath.FromSlash("testdata/first.rules"),
		filepath.FromSlash("testdata/my/*.rules"),
	},

	RemoteWriteConfigs: []*RemoteWriteConfig{
		{
			URL:           mustParseURL("http://remote1/push"),
			RemoteTimeout: model.Duration(30 * time.Second),
			WriteRelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: model.LabelNames{"__name__"},
					Separator:    ";",
					Regex:        MustNewRegexp("expensive.*"),
					Replacement:  "$1",
					Action:       RelabelDrop,
				},
			},
			QueueConfig: DefaultQueueConfig,
		},
		{
			URL:           mustParseURL("http://remote2/push"),
			RemoteTimeout: model.Duration(30 * time.Second),
			QueueConfig:   DefaultQueueConfig,
		},
	},

	RemoteReadConfigs: []*RemoteReadConfig{
		{
			URL:           mustParseURL("http://remote1/read"),
			RemoteTimeout: model.Duration(1 * time.Minute),
			ReadRecent:    true,
		},
		{
			URL:              mustParseURL("http://remote3/read"),
			RemoteTimeout:    model.Duration(1 * time.Minute),
			ReadRecent:       false,
			RequiredMatchers: model.LabelSet{"job": "special"},
		},
	},

	ScrapeConfigs: []*ScrapeConfig{
		{
			JobName: "prometheus",

			HonorLabels:    true,
			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			HTTPClientConfig: config_util.HTTPClientConfig{
				BearerTokenFile: filepath.FromSlash("testdata/valid_token_file"),
			},

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				StaticConfigs: []*targetgroup.Group{
					{
						Targets: []model.LabelSet{
							{model.AddressLabel: "localhost:9090"},
							{model.AddressLabel: "localhost:9191"},
						},
						Labels: model.LabelSet{
							"my":   "label",
							"your": "label",
						},
						Source: "0",
					},
				},

				FileSDConfigs: []*file.SDConfig{
					{
						Files:           []string{"testdata/foo/*.slow.json", "testdata/foo/*.slow.yml", "testdata/single/file.yml"},
						RefreshInterval: model.Duration(10 * time.Minute),
					},
					{
						Files:           []string{"testdata/bar/*.yaml"},
						RefreshInterval: model.Duration(5 * time.Minute),
					},
				},
			},

			RelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: model.LabelNames{"job", "__meta_dns_name"},
					TargetLabel:  "job",
					Separator:    ";",
					Regex:        MustNewRegexp("(.*)some-[regex]"),
					Replacement:  "foo-${1}",
					Action:       RelabelReplace,
				}, {
					SourceLabels: model.LabelNames{"abc"},
					TargetLabel:  "cde",
					Separator:    ";",
					Regex:        DefaultRelabelConfig.Regex,
					Replacement:  DefaultRelabelConfig.Replacement,
					Action:       RelabelReplace,
				}, {
					TargetLabel: "abc",
					Separator:   ";",
					Regex:       DefaultRelabelConfig.Regex,
					Replacement: "static",
					Action:      RelabelReplace,
				}, {
					TargetLabel: "abc",
					Separator:   ";",
					Regex:       MustNewRegexp(""),
					Replacement: "static",
					Action:      RelabelReplace,
				},
			},
		},
		{

			JobName: "service-x",

			ScrapeInterval: model.Duration(50 * time.Second),
			ScrapeTimeout:  model.Duration(5 * time.Second),
			SampleLimit:    1000,

			HTTPClientConfig: config_util.HTTPClientConfig{
				BasicAuth: &config_util.BasicAuth{
					Username: "admin_name",
					Password: "multiline\nmysecret\ntest",
				},
			},
			MetricsPath: "/my_path",
			Scheme:      "https",

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				DNSSDConfigs: []*dns.SDConfig{
					{
						Names: []string{
							"first.dns.address.domain.com",
							"second.dns.address.domain.com",
						},
						RefreshInterval: model.Duration(15 * time.Second),
						Type:            "SRV",
					},
					{
						Names: []string{
							"first.dns.address.domain.com",
						},
						RefreshInterval: model.Duration(30 * time.Second),
						Type:            "SRV",
					},
				},
			},

			RelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: model.LabelNames{"job"},
					Regex:        MustNewRegexp("(.*)some-[regex]"),
					Separator:    ";",
					Replacement:  DefaultRelabelConfig.Replacement,
					Action:       RelabelDrop,
				},
				{
					SourceLabels: model.LabelNames{"__address__"},
					TargetLabel:  "__tmp_hash",
					Regex:        DefaultRelabelConfig.Regex,
					Replacement:  DefaultRelabelConfig.Replacement,
					Modulus:      8,
					Separator:    ";",
					Action:       RelabelHashMod,
				},
				{
					SourceLabels: model.LabelNames{"__tmp_hash"},
					Regex:        MustNewRegexp("1"),
					Separator:    ";",
					Replacement:  DefaultRelabelConfig.Replacement,
					Action:       RelabelKeep,
				},
				{
					Regex:       MustNewRegexp("1"),
					Separator:   ";",
					Replacement: DefaultRelabelConfig.Replacement,
					Action:      RelabelLabelMap,
				},
				{
					Regex:       MustNewRegexp("d"),
					Separator:   ";",
					Replacement: DefaultRelabelConfig.Replacement,
					Action:      RelabelLabelDrop,
				},
				{
					Regex:       MustNewRegexp("k"),
					Separator:   ";",
					Replacement: DefaultRelabelConfig.Replacement,
					Action:      RelabelLabelKeep,
				},
			},
			MetricRelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: model.LabelNames{"__name__"},
					Regex:        MustNewRegexp("expensive_metric.*"),
					Separator:    ";",
					Replacement:  DefaultRelabelConfig.Replacement,
					Action:       RelabelDrop,
				},
			},
		},
		{
			JobName: "service-y",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				ConsulSDConfigs: []*consul.SDConfig{
					{
						Server:          "localhost:1234",
						Token:           "mysecret",
						Services:        []string{"nginx", "cache", "mysql"},
						ServiceTag:      "canary",
						NodeMeta:        map[string]string{"rack": "123"},
						TagSeparator:    consul.DefaultSDConfig.TagSeparator,
						Scheme:          "https",
						RefreshInterval: consul.DefaultSDConfig.RefreshInterval,
						AllowStale:      true,
						TLSConfig: config_util.TLSConfig{
							CertFile:           filepath.FromSlash("testdata/valid_cert_file"),
							KeyFile:            filepath.FromSlash("testdata/valid_key_file"),
							CAFile:             filepath.FromSlash("testdata/valid_ca_file"),
							InsecureSkipVerify: false,
						},
					},
				},
			},

			RelabelConfigs: []*RelabelConfig{
				{
					SourceLabels: model.LabelNames{"__meta_sd_consul_tags"},
					Regex:        MustNewRegexp("label:([^=]+)=([^,]+)"),
					Separator:    ",",
					TargetLabel:  "${1}",
					Replacement:  "${2}",
					Action:       RelabelReplace,
				},
			},
		},
		{
			JobName: "service-z",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  model.Duration(10 * time.Second),

			MetricsPath: "/metrics",
			Scheme:      "http",

			HTTPClientConfig: config_util.HTTPClientConfig{
				TLSConfig: config_util.TLSConfig{
					CertFile: filepath.FromSlash("testdata/valid_cert_file"),
					KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
				},

				BearerToken: "mysecret",
			},
		},
		{
			JobName: "service-kubernetes",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				KubernetesSDConfigs: []*kubernetes.SDConfig{
					{
						APIServer: kubernetesSDHostURL(),
						Role:      kubernetes.RoleEndpoint,
						BasicAuth: &config_util.BasicAuth{
							Username: "myusername",
							Password: "mysecret",
						},
						NamespaceDiscovery: kubernetes.NamespaceDiscovery{},
					},
				},
			},
		},
		{
			JobName: "service-kubernetes-namespaces",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				KubernetesSDConfigs: []*kubernetes.SDConfig{
					{
						APIServer: kubernetesSDHostURL(),
						Role:      kubernetes.RoleEndpoint,
						NamespaceDiscovery: kubernetes.NamespaceDiscovery{
							Names: []string{
								"default",
							},
						},
					},
				},
			},
		},
		{
			JobName: "service-marathon",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				MarathonSDConfigs: []*marathon.SDConfig{
					{
						Servers: []string{
							"https://marathon.example.com:443",
						},
						RefreshInterval: model.Duration(30 * time.Second),
						AuthToken:       config_util.Secret("mysecret"),
						HTTPClientConfig: config_util.HTTPClientConfig{
							TLSConfig: config_util.TLSConfig{
								CertFile: filepath.FromSlash("testdata/valid_cert_file"),
								KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
							},
						},
					},
				},
			},
		},
		{
			JobName: "service-ec2",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				EC2SDConfigs: []*ec2.SDConfig{
					{
						Region:          "us-east-1",
						AccessKey:       "access",
						SecretKey:       "mysecret",
						Profile:         "profile",
						RefreshInterval: model.Duration(60 * time.Second),
						Port:            80,
						Filters: []*ec2.Filter{
							{
								Name:   "tag:environment",
								Values: []string{"prod"},
							},
							{
								Name:   "tag:service",
								Values: []string{"web", "db"},
							},
						},
					},
				},
			},
		},
		{
			JobName: "service-azure",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				AzureSDConfigs: []*azure.SDConfig{
					{
						Environment:     "AzurePublicCloud",
						SubscriptionID:  "11AAAA11-A11A-111A-A111-1111A1111A11",
						TenantID:        "BBBB222B-B2B2-2B22-B222-2BB2222BB2B2",
						ClientID:        "333333CC-3C33-3333-CCC3-33C3CCCCC33C",
						ClientSecret:    "mysecret",
						RefreshInterval: model.Duration(5 * time.Minute),
						Port:            9100,
					},
				},
			},
		},
		{
			JobName: "service-nerve",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				NerveSDConfigs: []*zookeeper.NerveSDConfig{
					{
						Servers: []string{"localhost"},
						Paths:   []string{"/monitoring"},
						Timeout: model.Duration(10 * time.Second),
					},
				},
			},
		},
		{
			JobName: "0123service-xxx",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				StaticConfigs: []*targetgroup.Group{
					{
						Targets: []model.LabelSet{
							{model.AddressLabel: "localhost:9090"},
						},
						Source: "0",
					},
				},
			},
		},
		{
			JobName: "測試",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				StaticConfigs: []*targetgroup.Group{
					{
						Targets: []model.LabelSet{
							{model.AddressLabel: "localhost:9090"},
						},
						Source: "0",
					},
				},
			},
		},
		{
			JobName: "service-triton",

			ScrapeInterval: model.Duration(15 * time.Second),
			ScrapeTimeout:  DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				TritonSDConfigs: []*triton.SDConfig{
					{

						Account:         "testAccount",
						DNSSuffix:       "triton.example.com",
						Endpoint:        "triton.example.com",
						Port:            9163,
						RefreshInterval: model.Duration(60 * time.Second),
						Version:         1,
						TLSConfig: config_util.TLSConfig{
							CertFile: "testdata/valid_cert_file",
							KeyFile:  "testdata/valid_key_file",
						},
					},
				},
			},
		},
	},
	AlertingConfig: AlertingConfig{
		AlertmanagerConfigs: []*AlertmanagerConfig{
			{
				Scheme:  "https",
				Timeout: model.Duration(10 * time.Second),
				ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
					StaticConfigs: []*targetgroup.Group{
						{
							Targets: []model.LabelSet{
								{model.AddressLabel: "1.2.3.4:9093"},
								{model.AddressLabel: "1.2.3.5:9093"},
								{model.AddressLabel: "1.2.3.6:9093"},
							},
							Source: "0",
						},
					},
				},
			},
		},
	},
	original: "",
}

func TestLoadConfig(t *testing.T) {
	// Parse a valid file that sets a global scrape timeout. This tests whether parsing
	// an overwritten default field in the global config permanently changes the default.
	_, err := LoadFile("testdata/global_timeout.good.yml")
	testutil.Ok(t, err)

	c, err := LoadFile("testdata/conf.good.yml")
	testutil.Ok(t, err)

	expectedConf.original = c.original
	testutil.Equals(t, expectedConf, c)
}

// YAML marshalling must not reveal authentication credentials.
func TestElideSecrets(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml")
	testutil.Ok(t, err)

	secretRe := regexp.MustCompile(`\\u003csecret\\u003e|<secret>`)

	config, err := yaml.Marshal(c)
	testutil.Ok(t, err)
	yamlConfig := string(config)

	matches := secretRe.FindAllStringIndex(yamlConfig, -1)
	testutil.Assert(t, len(matches) == 7, "wrong number of secret matches found")
	testutil.Assert(t, !strings.Contains(yamlConfig, "mysecret"),
		"yaml marshal reveals authentication credentials.")
}

func TestLoadConfigRuleFilesAbsolutePath(t *testing.T) {
	// Parse a valid file that sets a rule files with an absolute path
	c, err := LoadFile(ruleFilesConfigFile)
	testutil.Ok(t, err)

	ruleFilesExpectedConf.original = c.original
	testutil.Equals(t, ruleFilesExpectedConf, c)
}

var expectedErrors = []struct {
	filename string
	errMsg   string
}{
	{
		filename: "jobname.bad.yml",
		errMsg:   `job_name is empty`,
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
		filename: "modulus_missing.bad.yml",
		errMsg:   "relabel configuration for hashmod requires non-zero modulus",
	}, {
		filename: "labelkeep.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	}, {
		filename: "labelkeep2.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	}, {
		filename: "labelkeep3.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	}, {
		filename: "labelkeep4.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	}, {
		filename: "labelkeep5.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	}, {
		filename: "labeldrop.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	}, {
		filename: "labeldrop2.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	}, {
		filename: "labeldrop3.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	}, {
		filename: "labeldrop4.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	}, {
		filename: "labeldrop5.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	}, {
		filename: "labelmap.bad.yml",
		errMsg:   "\"l-$1\" is invalid 'replacement' for labelmap action",
	}, {
		filename: "rules.bad.yml",
		errMsg:   "invalid rule file path",
	}, {
		filename: "unknown_attr.bad.yml",
		errMsg:   "field consult_sd_configs not found in type config.plain",
	}, {
		filename: "bearertoken.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	}, {
		filename: "bearertoken_basicauth.bad.yml",
		errMsg:   "at most one of basic_auth, bearer_token & bearer_token_file must be configured",
	}, {
		filename: "kubernetes_bearertoken.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	}, {
		filename: "kubernetes_role.bad.yml",
		errMsg:   "role",
	}, {
		filename: "kubernetes_namespace_discovery.bad.yml",
		errMsg:   "field foo not found in type kubernetes.plain",
	}, {
		filename: "kubernetes_bearertoken_basicauth.bad.yml",
		errMsg:   "at most one of basic_auth, bearer_token & bearer_token_file must be configured",
	}, {
		filename: "marathon_no_servers.bad.yml",
		errMsg:   "marathon_sd: must contain at least one Marathon server",
	}, {
		filename: "marathon_authtoken_authtokenfile.bad.yml",
		errMsg:   "marathon_sd: at most one of auth_token & auth_token_file must be configured",
	}, {
		filename: "marathon_authtoken_basicauth.bad.yml",
		errMsg:   "marathon_sd: at most one of basic_auth, auth_token & auth_token_file must be configured",
	}, {
		filename: "marathon_authtoken_bearertoken.bad.yml",
		errMsg:   "marathon_sd: at most one of bearer_token, bearer_token_file, auth_token & auth_token_file must be configured",
	}, {
		filename: "url_in_targetgroup.bad.yml",
		errMsg:   "\"http://bad\" is not a valid hostname",
	}, {
		filename: "target_label_missing.bad.yml",
		errMsg:   "relabel configuration for replace action requires 'target_label' value",
	}, {
		filename: "target_label_hashmod_missing.bad.yml",
		errMsg:   "relabel configuration for hashmod action requires 'target_label' value",
	}, {
		filename: "unknown_global_attr.bad.yml",
		errMsg:   "field nonexistent_field not found in type config.plain",
	}, {
		filename: "remote_read_url_missing.bad.yml",
		errMsg:   `url for remote_read is empty`,
	}, {
		filename: "remote_write_url_missing.bad.yml",
		errMsg:   `url for remote_write is empty`,
	},
	{
		filename: "ec2_filters_empty_values.bad.yml",
		errMsg:   `EC2 SD configuration filter values cannot be empty`,
	},
	{
		filename: "section_key_dup.bad.yml",
		errMsg:   "field scrape_configs already set in type config.plain",
	},
}

func TestBadConfigs(t *testing.T) {
	for _, ee := range expectedErrors {
		_, err := LoadFile("testdata/" + ee.filename)
		testutil.NotOk(t, err, "%s", ee.filename)
		testutil.Assert(t, strings.Contains(err.Error(), ee.errMsg),
			"Expected error for %s to contain %q but got: %s", ee.filename, ee.errMsg, err)
	}
}

func TestBadStaticConfigsJSON(t *testing.T) {
	content, err := ioutil.ReadFile("testdata/static_config.bad.json")
	testutil.Ok(t, err)
	var tg targetgroup.Group
	err = json.Unmarshal(content, &tg)
	testutil.NotOk(t, err, "")
}

func TestBadStaticConfigsYML(t *testing.T) {
	content, err := ioutil.ReadFile("testdata/static_config.bad.yml")
	testutil.Ok(t, err)
	var tg targetgroup.Group
	err = yaml.UnmarshalStrict(content, &tg)
	testutil.NotOk(t, err, "")
}

func TestEmptyConfig(t *testing.T) {
	c, err := Load("")
	testutil.Ok(t, err)
	exp := DefaultConfig
	testutil.Equals(t, exp, *c)
}

func TestEmptyGlobalBlock(t *testing.T) {
	c, err := Load("global:\n")
	testutil.Ok(t, err)
	exp := DefaultConfig
	exp.original = "global:\n"
	testutil.Equals(t, exp, *c)
}

func TestTargetLabelValidity(t *testing.T) {
	tests := []struct {
		str   string
		valid bool
	}{
		{"-label", false},
		{"label", true},
		{"label${1}", true},
		{"${1}label", true},
		{"${1}", true},
		{"${1}label", true},
		{"${", false},
		{"$", false},
		{"${}", false},
		{"foo${", false},
		{"$1", true},
		{"asd$2asd", true},
		{"-foo${1}bar-", false},
		{"_${1}_", true},
		{"foo${bar}foo", true},
	}
	for _, test := range tests {
		testutil.Assert(t, relabelTarget.Match([]byte(test.str)) == test.valid,
			"Expected %q to be %v", test.str, test.valid)
	}
}

func kubernetesSDHostURL() config_util.URL {
	tURL, _ := url.Parse("https://localhost:1234")
	return config_util.URL{URL: tURL}
}
