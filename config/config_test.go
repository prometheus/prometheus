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

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery/azure"
	sd_config "github.com/prometheus/prometheus/discovery/config"
	"github.com/prometheus/prometheus/discovery/consul"
	"github.com/prometheus/prometheus/discovery/dns"
	"github.com/prometheus/prometheus/discovery/ec2"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/marathon"
	"github.com/prometheus/prometheus/discovery/openstack"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/discovery/triton"
	"github.com/prometheus/prometheus/discovery/zookeeper"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/relabel"
	"github.com/prometheus/prometheus/util/testutil"
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

		ExternalLabels: labels.Labels{
			{Name: "foo", Value: "bar"},
			{Name: "monitor", Value: "codelab"},
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
			WriteRelabelConfigs: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"__name__"},
					Separator:    ";",
					Regex:        relabel.MustNewRegexp("expensive.*"),
					Replacement:  "$1",
					Action:       relabel.Drop,
				},
			},
			QueueConfig: DefaultQueueConfig,
		},
		{
			URL:           mustParseURL("http://remote2/push"),
			RemoteTimeout: model.Duration(30 * time.Second),
			QueueConfig:   DefaultQueueConfig,
			HTTPClientConfig: config_util.HTTPClientConfig{
				TLSConfig: config_util.TLSConfig{
					CertFile: filepath.FromSlash("testdata/valid_cert_file"),
					KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
				},
			},
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
			HTTPClientConfig: config_util.HTTPClientConfig{
				TLSConfig: config_util.TLSConfig{
					CertFile: filepath.FromSlash("testdata/valid_cert_file"),
					KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
				},
			},
		},
	},

	ScrapeConfigs: []*ScrapeConfig{
		{
			JobName: "prometheus",

			HonorLabels:     true,
			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

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

			RelabelConfigs: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"job", "__meta_dns_name"},
					TargetLabel:  "job",
					Separator:    ";",
					Regex:        relabel.MustNewRegexp("(.*)some-[regex]"),
					Replacement:  "foo-${1}",
					Action:       relabel.Replace,
				}, {
					SourceLabels: model.LabelNames{"abc"},
					TargetLabel:  "cde",
					Separator:    ";",
					Regex:        relabel.DefaultRelabelConfig.Regex,
					Replacement:  relabel.DefaultRelabelConfig.Replacement,
					Action:       relabel.Replace,
				}, {
					TargetLabel: "abc",
					Separator:   ";",
					Regex:       relabel.DefaultRelabelConfig.Regex,
					Replacement: "static",
					Action:      relabel.Replace,
				}, {
					TargetLabel: "abc",
					Separator:   ";",
					Regex:       relabel.MustNewRegexp(""),
					Replacement: "static",
					Action:      relabel.Replace,
				},
			},
		},
		{

			JobName: "service-x",

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(50 * time.Second),
			ScrapeTimeout:   model.Duration(5 * time.Second),
			SampleLimit:     1000,

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

			RelabelConfigs: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"job"},
					Regex:        relabel.MustNewRegexp("(.*)some-[regex]"),
					Separator:    ";",
					Replacement:  relabel.DefaultRelabelConfig.Replacement,
					Action:       relabel.Drop,
				},
				{
					SourceLabels: model.LabelNames{"__address__"},
					TargetLabel:  "__tmp_hash",
					Regex:        relabel.DefaultRelabelConfig.Regex,
					Replacement:  relabel.DefaultRelabelConfig.Replacement,
					Modulus:      8,
					Separator:    ";",
					Action:       relabel.HashMod,
				},
				{
					SourceLabels: model.LabelNames{"__tmp_hash"},
					Regex:        relabel.MustNewRegexp("1"),
					Separator:    ";",
					Replacement:  relabel.DefaultRelabelConfig.Replacement,
					Action:       relabel.Keep,
				},
				{
					Regex:       relabel.MustNewRegexp("1"),
					Separator:   ";",
					Replacement: relabel.DefaultRelabelConfig.Replacement,
					Action:      relabel.LabelMap,
				},
				{
					Regex:       relabel.MustNewRegexp("d"),
					Separator:   ";",
					Replacement: relabel.DefaultRelabelConfig.Replacement,
					Action:      relabel.LabelDrop,
				},
				{
					Regex:       relabel.MustNewRegexp("k"),
					Separator:   ";",
					Replacement: relabel.DefaultRelabelConfig.Replacement,
					Action:      relabel.LabelKeep,
				},
			},
			MetricRelabelConfigs: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"__name__"},
					Regex:        relabel.MustNewRegexp("expensive_metric.*"),
					Separator:    ";",
					Replacement:  relabel.DefaultRelabelConfig.Replacement,
					Action:       relabel.Drop,
				},
			},
		},
		{
			JobName: "service-y",

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				ConsulSDConfigs: []*consul.SDConfig{
					{
						Server:          "localhost:1234",
						Token:           "mysecret",
						Services:        []string{"nginx", "cache", "mysql"},
						ServiceTags:     []string{"canary", "v1"},
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

			RelabelConfigs: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"__meta_sd_consul_tags"},
					Regex:        relabel.MustNewRegexp("label:([^=]+)=([^,]+)"),
					Separator:    ",",
					TargetLabel:  "${1}",
					Replacement:  "${2}",
					Action:       relabel.Replace,
				},
			},
		},
		{
			JobName: "service-z",

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   model.Duration(10 * time.Second),

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

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				KubernetesSDConfigs: []*kubernetes.SDConfig{
					{
						APIServer: kubernetesSDHostURL(),
						Role:      kubernetes.RoleEndpoint,
						HTTPClientConfig: config_util.HTTPClientConfig{
							BasicAuth: &config_util.BasicAuth{
								Username: "myusername",
								Password: "mysecret",
							},
							TLSConfig: config_util.TLSConfig{
								CertFile: filepath.FromSlash("testdata/valid_cert_file"),
								KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
							},
						},
						NamespaceDiscovery: kubernetes.NamespaceDiscovery{},
					},
				},
			},
		},
		{
			JobName: "service-kubernetes-namespaces",

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config_util.HTTPClientConfig{
				BasicAuth: &config_util.BasicAuth{
					Username:     "myusername",
					PasswordFile: filepath.FromSlash("testdata/valid_password_file"),
				},
			},

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

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

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

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

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

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				AzureSDConfigs: []*azure.SDConfig{
					{
						Environment:          "AzurePublicCloud",
						SubscriptionID:       "11AAAA11-A11A-111A-A111-1111A1111A11",
						TenantID:             "BBBB222B-B2B2-2B22-B222-2BB2222BB2B2",
						ClientID:             "333333CC-3C33-3333-CCC3-33C3CCCCC33C",
						ClientSecret:         "mysecret",
						AuthenticationMethod: "OAuth",
						RefreshInterval:      model.Duration(5 * time.Minute),
						Port:                 9100,
					},
				},
			},
		},
		{
			JobName: "service-nerve",

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

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

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

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
			JobName: "badfederation",

			HonorTimestamps: false,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: "/federate",
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

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

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

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

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
		{
			JobName: "service-openstack",

			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   DefaultGlobalConfig.ScrapeTimeout,

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfig: sd_config.ServiceDiscoveryConfig{
				OpenstackSDConfigs: []*openstack.SDConfig{
					{
						Role:            "instance",
						Region:          "RegionOne",
						Port:            80,
						RefreshInterval: model.Duration(60 * time.Second),
						TLSConfig: config_util.TLSConfig{
							CAFile:   "testdata/valid_ca_file",
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
				Scheme:     "https",
				Timeout:    model.Duration(10 * time.Second),
				APIVersion: AlertmanagerAPIVersionV1,
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

func TestScrapeIntervalLarger(t *testing.T) {
	c, err := LoadFile("testdata/scrape_interval_larger.good.yml")
	testutil.Ok(t, err)
	testutil.Equals(t, 1, len(c.ScrapeConfigs))
	for _, sc := range c.ScrapeConfigs {
		testutil.Equals(t, true, sc.ScrapeInterval >= sc.ScrapeTimeout)
	}
}

// YAML marshaling must not reveal authentication credentials.
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

func TestKubernetesEmptyAPIServer(t *testing.T) {
	_, err := LoadFile("testdata/kubernetes_empty_apiserver.good.yml")
	testutil.Ok(t, err)
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
		filename: "labelvalue.bad.yml",
		errMsg:   `"\xff" is not a valid label value`,
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
		filename: "kubernetes_http_config_without_api_server.bad.yml",
		errMsg:   "to use custom HTTP client configuration please provide the 'api_server' URL explicitly",
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
		filename: "openstack_role.bad.yml",
		errMsg:   "unknown OpenStack SD role",
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
	{
		filename: "azure_client_id_missing.bad.yml",
		errMsg:   "azure SD configuration requires a client_id",
	},
	{
		filename: "azure_client_secret_missing.bad.yml",
		errMsg:   "azure SD configuration requires a client_secret",
	},
	{
		filename: "azure_subscription_id_missing.bad.yml",
		errMsg:   "azure SD configuration requires a subscription_id",
	},
	{
		filename: "azure_tenant_id_missing.bad.yml",
		errMsg:   "azure SD configuration requires a tenant_id",
	},
	{
		filename: "azure_authentication_method.bad.yml",
		errMsg:   "unknown authentication_type \"invalid\". Supported types are \"OAuth\" or \"ManagedIdentity\"",
	},
	{
		filename: "empty_scrape_config.bad.yml",
		errMsg:   "empty or null scrape config section",
	},
	{
		filename: "empty_rw_config.bad.yml",
		errMsg:   "empty or null remote write config section",
	},
	{
		filename: "empty_rr_config.bad.yml",
		errMsg:   "empty or null remote read config section",
	},
	{
		filename: "empty_target_relabel_config.bad.yml",
		errMsg:   "empty or null target relabeling rule",
	},
	{
		filename: "empty_metric_relabel_config.bad.yml",
		errMsg:   "empty or null metric relabeling rule",
	},
	{
		filename: "empty_alert_relabel_config.bad.yml",
		errMsg:   "empty or null alert relabeling rule",
	},
	{
		filename: "empty_alertmanager_relabel_config.bad.yml",
		errMsg:   "empty or null Alertmanager target relabeling rule",
	},
	{
		filename: "empty_rw_relabel_config.bad.yml",
		errMsg:   "empty or null relabeling rule in remote write config",
	},
	{
		filename: "empty_static_config.bad.yml",
		errMsg:   "empty or null section in static_configs",
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
	testutil.NotOk(t, err)
}

func TestBadStaticConfigsYML(t *testing.T) {
	content, err := ioutil.ReadFile("testdata/static_config.bad.yml")
	testutil.Ok(t, err)
	var tg targetgroup.Group
	err = yaml.UnmarshalStrict(content, &tg)
	testutil.NotOk(t, err)
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

func kubernetesSDHostURL() config_util.URL {
	tURL, _ := url.Parse("https://localhost:1234")
	return config_util.URL{URL: tURL}
}
