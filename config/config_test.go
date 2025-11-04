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
	"crypto/tls"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/units"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/grafana/regexp"
	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/otlptranslator"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/aws"
	"github.com/prometheus/prometheus/discovery/azure"
	"github.com/prometheus/prometheus/discovery/consul"
	"github.com/prometheus/prometheus/discovery/digitalocean"
	"github.com/prometheus/prometheus/discovery/dns"
	"github.com/prometheus/prometheus/discovery/eureka"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/hetzner"
	"github.com/prometheus/prometheus/discovery/http"
	"github.com/prometheus/prometheus/discovery/ionos"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/linode"
	"github.com/prometheus/prometheus/discovery/marathon"
	"github.com/prometheus/prometheus/discovery/moby"
	"github.com/prometheus/prometheus/discovery/nomad"
	"github.com/prometheus/prometheus/discovery/openstack"
	"github.com/prometheus/prometheus/discovery/ovhcloud"
	"github.com/prometheus/prometheus/discovery/puppetdb"
	"github.com/prometheus/prometheus/discovery/scaleway"
	"github.com/prometheus/prometheus/discovery/stackit"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/discovery/triton"
	"github.com/prometheus/prometheus/discovery/uyuni"
	"github.com/prometheus/prometheus/discovery/vultr"
	"github.com/prometheus/prometheus/discovery/xds"
	"github.com/prometheus/prometheus/discovery/zookeeper"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/util/testutil"
)

func mustParseURL(u string) *config.URL {
	parsed, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	return &config.URL{URL: parsed}
}

func boolPtr(b bool) *bool {
	return &b
}

const (
	globBodySizeLimit         = 15 * units.MiB
	globSampleLimit           = 1500
	globTargetLimit           = 30
	globLabelLimit            = 30
	globLabelNameLengthLimit  = 200
	globLabelValueLengthLimit = 200
	globalGoGC                = 42
	globScrapeFailureLogFile  = "testdata/fail.log"
)

var expectedConf = &Config{
	loaded: true,
	GlobalConfig: GlobalConfig{
		ScrapeInterval:       model.Duration(15 * time.Second),
		ScrapeTimeout:        DefaultGlobalConfig.ScrapeTimeout,
		EvaluationInterval:   model.Duration(30 * time.Second),
		QueryLogFile:         "testdata/query.log",
		ScrapeFailureLogFile: globScrapeFailureLogFile,

		ExternalLabels: labels.FromStrings("foo", "bar", "monitor", "codelab"),

		BodySizeLimit:                  globBodySizeLimit,
		SampleLimit:                    globSampleLimit,
		TargetLimit:                    globTargetLimit,
		LabelLimit:                     globLabelLimit,
		LabelNameLengthLimit:           globLabelNameLengthLimit,
		LabelValueLengthLimit:          globLabelValueLengthLimit,
		ScrapeNativeHistograms:         boolPtr(false),
		AlwaysScrapeClassicHistograms:  false,
		ConvertClassicHistogramsToNHCB: false,
		MetricNameValidationScheme:     model.UTF8Validation,
	},

	Runtime: RuntimeConfig{
		GoGC: globalGoGC,
	},

	RuleFiles: []string{
		filepath.FromSlash("testdata/first.rules"),
		filepath.FromSlash("testdata/my/*.rules"),
	},

	RemoteWriteConfigs: []*RemoteWriteConfig{
		{
			URL:             mustParseURL("http://remote1/push"),
			ProtobufMessage: remoteapi.WriteV1MessageType,
			RemoteTimeout:   model.Duration(30 * time.Second),
			Name:            "drop_expensive",
			WriteRelabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"__name__"},
					Separator:            ";",
					Regex:                relabel.MustNewRegexp("expensive.*"),
					Replacement:          "$1",
					Action:               relabel.Drop,
					NameValidationScheme: model.UTF8Validation,
				},
			},
			QueueConfig:    DefaultQueueConfig,
			MetadataConfig: DefaultMetadataConfig,
			HTTPClientConfig: config.HTTPClientConfig{
				OAuth2: &config.OAuth2{
					ClientID:     "123",
					ClientSecret: "456",
					TokenURL:     "http://remote1/auth",
					TLSConfig: config.TLSConfig{
						CertFile: filepath.FromSlash("testdata/valid_cert_file"),
						KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
					},
				},
				FollowRedirects: true,
				EnableHTTP2:     false,
			},
		},
		{
			URL:             mustParseURL("http://remote2/push"),
			ProtobufMessage: remoteapi.WriteV2MessageType,
			RemoteTimeout:   model.Duration(30 * time.Second),
			QueueConfig:     DefaultQueueConfig,
			MetadataConfig:  DefaultMetadataConfig,
			Name:            "rw_tls",
			HTTPClientConfig: config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					CertFile: filepath.FromSlash("testdata/valid_cert_file"),
					KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
				},
				FollowRedirects: true,
				EnableHTTP2:     false,
			},
			Headers: map[string]string{"name": "value"},
		},
	},

	OTLPConfig: OTLPConfig{
		PromoteResourceAttributes: []string{
			"k8s.cluster.name", "k8s.job.name", "k8s.namespace.name",
		},
		TranslationStrategy:                  otlptranslator.UnderscoreEscapingWithSuffixes,
		LabelNameUnderscoreSanitization:      true,
		LabelNamePreserveMultipleUnderscores: true,
	},

	RemoteReadConfigs: []*RemoteReadConfig{
		{
			URL:              mustParseURL("http://remote1/read"),
			RemoteTimeout:    model.Duration(1 * time.Minute),
			ChunkedReadLimit: DefaultChunkedReadLimit,
			ReadRecent:       true,
			Name:             "default",
			HTTPClientConfig: config.HTTPClientConfig{
				FollowRedirects: true,
				EnableHTTP2:     false,
			},
			FilterExternalLabels: true,
		},
		{
			URL:              mustParseURL("http://remote3/read"),
			RemoteTimeout:    model.Duration(1 * time.Minute),
			ChunkedReadLimit: DefaultChunkedReadLimit,
			ReadRecent:       false,
			Name:             "read_special",
			RequiredMatchers: model.LabelSet{"job": "special"},
			HTTPClientConfig: config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					CertFile: filepath.FromSlash("testdata/valid_cert_file"),
					KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
				},
				FollowRedirects: true,
				EnableHTTP2:     true,
			},
			FilterExternalLabels: true,
		},
	},

	ScrapeConfigs: []*ScrapeConfig{
		{
			JobName: "prometheus",

			HonorLabels:                    true,
			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFallbackProtocol:         PrometheusText0_0_4,
			ScrapeFailureLogFile:           "testdata/fail_prom.log",
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,

			HTTPClientConfig: config.HTTPClientConfig{
				Authorization: &config.Authorization{
					Type:            "Bearer",
					CredentialsFile: filepath.FromSlash("testdata/valid_token_file"),
				},
				FollowRedirects: true,
				EnableHTTP2:     true,
				TLSConfig: config.TLSConfig{
					MinVersion: config.TLSVersion(tls.VersionTLS10),
				},
				HTTPHeaders: &config.Headers{
					Headers: map[string]config.Header{
						"foo": {
							Values:  []string{"foobar"},
							Secrets: []config.Secret{"bar", "foo"},
							Files:   []string{filepath.FromSlash("testdata/valid_password_file")},
						},
					},
				},
			},

			ServiceDiscoveryConfigs: discovery.Configs{
				&file.SDConfig{
					Files:           []string{"testdata/foo/*.slow.json", "testdata/foo/*.slow.yml", "testdata/single/file.yml"},
					RefreshInterval: model.Duration(10 * time.Minute),
				},
				&file.SDConfig{
					Files:           []string{"testdata/bar/*.yaml"},
					RefreshInterval: model.Duration(5 * time.Minute),
				},
				discovery.StaticConfig{
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
			},

			RelabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"job", "__meta_dns_name"},
					TargetLabel:          "job",
					Separator:            ";",
					Regex:                relabel.MustNewRegexp("(.*)some-[regex]"),
					Replacement:          "foo-${1}",
					Action:               relabel.Replace,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					SourceLabels:         model.LabelNames{"abc"},
					TargetLabel:          "cde",
					Separator:            ";",
					Regex:                relabel.DefaultRelabelConfig.Regex,
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Action:               relabel.Replace,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					TargetLabel:          "abc",
					Separator:            ";",
					Regex:                relabel.DefaultRelabelConfig.Regex,
					Replacement:          "static",
					Action:               relabel.Replace,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					TargetLabel:          "abc",
					Separator:            ";",
					Regex:                relabel.MustNewRegexp(""),
					Replacement:          "static",
					Action:               relabel.Replace,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					SourceLabels:         model.LabelNames{"foo"},
					TargetLabel:          "abc",
					Action:               relabel.KeepEqual,
					Regex:                relabel.DefaultRelabelConfig.Regex,
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Separator:            relabel.DefaultRelabelConfig.Separator,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					SourceLabels:         model.LabelNames{"foo"},
					TargetLabel:          "abc",
					Action:               relabel.DropEqual,
					Regex:                relabel.DefaultRelabelConfig.Regex,
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Separator:            relabel.DefaultRelabelConfig.Separator,
					NameValidationScheme: model.UTF8Validation,
				},
			},
		},
		{
			JobName: "service-x",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(50 * time.Second),
			ScrapeTimeout:                  model.Duration(5 * time.Second),
			EnableCompression:              true,
			BodySizeLimit:                  10 * units.MiB,
			SampleLimit:                    1000,
			TargetLimit:                    35,
			LabelLimit:                     35,
			LabelNameLengthLimit:           210,
			LabelValueLengthLimit:          210,
			ScrapeProtocols:                []ScrapeProtocol{PrometheusText0_0_4},
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			HTTPClientConfig: config.HTTPClientConfig{
				BasicAuth: &config.BasicAuth{
					Username: "admin_name",
					Password: "multiline\nmysecret\ntest",
				},
				FollowRedirects: true,
				EnableHTTP2:     true,
			},
			MetricsPath: "/my_path",
			Scheme:      "https",

			ServiceDiscoveryConfigs: discovery.Configs{
				&dns.SDConfig{
					Names: []string{
						"first.dns.address.domain.com",
						"second.dns.address.domain.com",
					},
					RefreshInterval: model.Duration(15 * time.Second),
					Type:            "SRV",
				},
				&dns.SDConfig{
					Names: []string{
						"first.dns.address.domain.com",
					},
					RefreshInterval: model.Duration(30 * time.Second),
					Type:            "SRV",
				},
			},

			RelabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"job"},
					Regex:                relabel.MustNewRegexp("(.*)some-[regex]"),
					Separator:            ";",
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Action:               relabel.Drop,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					SourceLabels:         model.LabelNames{"__address__"},
					TargetLabel:          "__tmp_hash",
					Regex:                relabel.DefaultRelabelConfig.Regex,
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Modulus:              8,
					Separator:            ";",
					Action:               relabel.HashMod,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					SourceLabels:         model.LabelNames{"__tmp_hash"},
					Regex:                relabel.MustNewRegexp("1"),
					Separator:            ";",
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Action:               relabel.Keep,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					Regex:                relabel.MustNewRegexp("1"),
					Separator:            ";",
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Action:               relabel.LabelMap,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					Regex:                relabel.MustNewRegexp("d"),
					Separator:            ";",
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Action:               relabel.LabelDrop,
					NameValidationScheme: model.UTF8Validation,
				},
				{
					Regex:                relabel.MustNewRegexp("k"),
					Separator:            ";",
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Action:               relabel.LabelKeep,
					NameValidationScheme: model.UTF8Validation,
				},
			},
			MetricRelabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"__name__"},
					Regex:                relabel.MustNewRegexp("expensive_metric.*"),
					Separator:            ";",
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Action:               relabel.Drop,
					NameValidationScheme: model.UTF8Validation,
				},
			},
		},
		{
			JobName: "service-y",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&consul.SDConfig{
					Server:          "localhost:1234",
					PathPrefix:      "/consul",
					Token:           "mysecret",
					Services:        []string{"nginx", "cache", "mysql"},
					ServiceTags:     []string{"canary", "v1"},
					NodeMeta:        map[string]string{"rack": "123"},
					TagSeparator:    consul.DefaultSDConfig.TagSeparator,
					Scheme:          "https",
					RefreshInterval: consul.DefaultSDConfig.RefreshInterval,
					AllowStale:      true,
					HTTPClientConfig: config.HTTPClientConfig{
						TLSConfig: config.TLSConfig{
							CertFile:           filepath.FromSlash("testdata/valid_cert_file"),
							KeyFile:            filepath.FromSlash("testdata/valid_key_file"),
							CAFile:             filepath.FromSlash("testdata/valid_ca_file"),
							InsecureSkipVerify: false,
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
				},
			},

			RelabelConfigs: []*relabel.Config{
				{
					SourceLabels:         model.LabelNames{"__meta_sd_consul_tags"},
					Regex:                relabel.MustNewRegexp("label:([^=]+)=([^,]+)"),
					Separator:            ",",
					TargetLabel:          "${1}",
					Replacement:          "${2}",
					Action:               relabel.Replace,
					NameValidationScheme: model.UTF8Validation,
				},
			},
		},
		{
			JobName: "service-z",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  model.Duration(10 * time.Second),
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath: "/metrics",
			Scheme:      "http",

			HTTPClientConfig: config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					CertFile: filepath.FromSlash("testdata/valid_cert_file"),
					KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
				},

				Authorization: &config.Authorization{
					Type:        "Bearer",
					Credentials: "mysecret",
				},

				FollowRedirects: true,
				EnableHTTP2:     true,
			},
		},
		{
			JobName: "service-kubernetes",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&kubernetes.SDConfig{
					APIServer: kubernetesSDHostURL(),
					Role:      kubernetes.RoleEndpoint,
					HTTPClientConfig: config.HTTPClientConfig{
						BasicAuth: &config.BasicAuth{
							Username: "myusername",
							Password: "mysecret",
						},
						TLSConfig: config.TLSConfig{
							CertFile: filepath.FromSlash("testdata/valid_cert_file"),
							KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
					NamespaceDiscovery: kubernetes.NamespaceDiscovery{},
				},
			},
		},
		{
			JobName: "service-kubernetes-namespaces",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath: DefaultScrapeConfig.MetricsPath,
			Scheme:      DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.HTTPClientConfig{
				BasicAuth: &config.BasicAuth{
					Username:     "myusername",
					PasswordFile: filepath.FromSlash("testdata/valid_password_file"),
				},
				FollowRedirects: true,
				EnableHTTP2:     true,
			},

			ServiceDiscoveryConfigs: discovery.Configs{
				&kubernetes.SDConfig{
					APIServer: kubernetesSDHostURL(),
					Role:      kubernetes.RoleEndpoint,
					NamespaceDiscovery: kubernetes.NamespaceDiscovery{
						Names: []string{
							"default",
						},
					},
					HTTPClientConfig: config.DefaultHTTPClientConfig,
				},
			},
		},
		{
			JobName: "service-kuma",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&xds.KumaSDConfig{
					Server:           "http://kuma-control-plane.kuma-system.svc:5676",
					ClientID:         "main-prometheus",
					HTTPClientConfig: config.DefaultHTTPClientConfig,
					RefreshInterval:  model.Duration(15 * time.Second),
					FetchTimeout:     model.Duration(2 * time.Minute),
				},
			},
		},
		{
			JobName: "service-marathon",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&marathon.SDConfig{
					Servers: []string{
						"https://marathon.example.com:443",
					},
					RefreshInterval: model.Duration(30 * time.Second),
					AuthToken:       "mysecret",
					HTTPClientConfig: config.HTTPClientConfig{
						TLSConfig: config.TLSConfig{
							CertFile: filepath.FromSlash("testdata/valid_cert_file"),
							KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
				},
			},
		},
		{
			JobName: "service-nomad",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&nomad.SDConfig{
					AllowStale:      true,
					Namespace:       "default",
					RefreshInterval: model.Duration(60 * time.Second),
					Region:          "global",
					Server:          "http://localhost:4646",
					TagSeparator:    ",",
					HTTPClientConfig: config.HTTPClientConfig{
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
				},
			},
		},
		{
			JobName: "service-ec2",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&aws.EC2SDConfig{
					Region:          "us-east-1",
					AccessKey:       "access",
					SecretKey:       "mysecret",
					Profile:         "profile",
					RefreshInterval: model.Duration(60 * time.Second),
					Port:            80,
					Filters: []*aws.EC2Filter{
						{
							Name:   "tag:environment",
							Values: []string{"prod"},
						},
						{
							Name:   "tag:service",
							Values: []string{"web", "db"},
						},
					},
					HTTPClientConfig: config.DefaultHTTPClientConfig,
				},
			},
		},
		{
			JobName: "service-lightsail",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&aws.LightsailSDConfig{
					Region:           "us-east-1",
					AccessKey:        "access",
					SecretKey:        "mysecret",
					Profile:          "profile",
					RefreshInterval:  model.Duration(60 * time.Second),
					Port:             80,
					HTTPClientConfig: config.DefaultHTTPClientConfig,
				},
			},
		},
		{
			JobName: "service-azure",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&azure.SDConfig{
					Environment:          "AzurePublicCloud",
					SubscriptionID:       "11AAAA11-A11A-111A-A111-1111A1111A11",
					ResourceGroup:        "my-resource-group",
					TenantID:             "BBBB222B-B2B2-2B22-B222-2BB2222BB2B2",
					ClientID:             "333333CC-3C33-3333-CCC3-33C3CCCCC33C",
					ClientSecret:         "mysecret",
					AuthenticationMethod: "OAuth",
					RefreshInterval:      model.Duration(5 * time.Minute),
					Port:                 9100,
					HTTPClientConfig:     config.DefaultHTTPClientConfig,
				},
			},
		},
		{
			JobName: "service-nerve",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&zookeeper.NerveSDConfig{
					Servers: []string{"localhost"},
					Paths:   []string{"/monitoring"},
					Timeout: model.Duration(10 * time.Second),
				},
			},
		},
		{
			JobName: "0123service-xxx",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				discovery.StaticConfig{
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

			HonorTimestamps:                false,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      "/federate",
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				discovery.StaticConfig{
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

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				discovery.StaticConfig{
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
			JobName: "httpsd",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&http.SDConfig{
					HTTPClientConfig: config.DefaultHTTPClientConfig,
					URL:              "http://example.com/prometheus",
					RefreshInterval:  model.Duration(60 * time.Second),
				},
			},
		},
		{
			JobName: "service-triton",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&triton.SDConfig{
					Account:         "testAccount",
					Role:            "container",
					DNSSuffix:       "triton.example.com",
					Endpoint:        "triton.example.com",
					Port:            9163,
					RefreshInterval: model.Duration(60 * time.Second),
					Version:         1,
					TLSConfig: config.TLSConfig{
						CertFile: "testdata/valid_cert_file",
						KeyFile:  "testdata/valid_key_file",
					},
				},
			},
		},
		{
			JobName: "digitalocean-droplets",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&digitalocean.SDConfig{
					HTTPClientConfig: config.HTTPClientConfig{
						Authorization: &config.Authorization{
							Type:        "Bearer",
							Credentials: "abcdef",
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
					Port:            80,
					RefreshInterval: model.Duration(60 * time.Second),
				},
			},
		},
		{
			JobName: "docker",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&moby.DockerSDConfig{
					Filters:            []moby.Filter{},
					Host:               "unix:///var/run/docker.sock",
					Port:               80,
					HostNetworkingHost: "localhost",
					RefreshInterval:    model.Duration(60 * time.Second),
					HTTPClientConfig:   config.DefaultHTTPClientConfig,
					MatchFirstNetwork:  true,
				},
			},
		},
		{
			JobName: "dockerswarm",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&moby.DockerSwarmSDConfig{
					Filters:          []moby.Filter{},
					Host:             "http://127.0.0.1:2375",
					Role:             "nodes",
					Port:             80,
					RefreshInterval:  model.Duration(60 * time.Second),
					HTTPClientConfig: config.DefaultHTTPClientConfig,
				},
			},
		},
		{
			JobName: "service-openstack",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&openstack.SDConfig{
					Role:            "instance",
					Region:          "RegionOne",
					Port:            80,
					Availability:    "public",
					RefreshInterval: model.Duration(60 * time.Second),
					TLSConfig: config.TLSConfig{
						CAFile:   "testdata/valid_ca_file",
						CertFile: "testdata/valid_cert_file",
						KeyFile:  "testdata/valid_key_file",
					},
				},
			},
		},
		{
			JobName: "service-puppetdb",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&puppetdb.SDConfig{
					URL:               "https://puppetserver/",
					Query:             "resources { type = \"Package\" and title = \"httpd\" }",
					IncludeParameters: true,
					Port:              80,
					RefreshInterval:   model.Duration(60 * time.Second),
					HTTPClientConfig: config.HTTPClientConfig{
						FollowRedirects: true,
						EnableHTTP2:     true,
						TLSConfig: config.TLSConfig{
							CAFile:   "testdata/valid_ca_file",
							CertFile: "testdata/valid_cert_file",
							KeyFile:  "testdata/valid_key_file",
						},
					},
				},
			},
		},
		{
			JobName:                        "hetzner",
			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultProtoFirstScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(true),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			RelabelConfigs: []*relabel.Config{
				{
					Action:               relabel.Uppercase,
					Regex:                relabel.DefaultRelabelConfig.Regex,
					Replacement:          relabel.DefaultRelabelConfig.Replacement,
					Separator:            relabel.DefaultRelabelConfig.Separator,
					SourceLabels:         model.LabelNames{"instance"},
					TargetLabel:          "instance",
					NameValidationScheme: model.UTF8Validation,
				},
			},

			ServiceDiscoveryConfigs: discovery.Configs{
				&hetzner.SDConfig{
					HTTPClientConfig: config.HTTPClientConfig{
						Authorization: &config.Authorization{
							Type:        "Bearer",
							Credentials: "abcdef",
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
					Port:            80,
					RefreshInterval: model.Duration(60 * time.Second),
					Role:            "hcloud",
				},
				&hetzner.SDConfig{
					HTTPClientConfig: config.HTTPClientConfig{
						BasicAuth:       &config.BasicAuth{Username: "abcdef", Password: "abcdef"},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
					Port:            80,
					RefreshInterval: model.Duration(60 * time.Second),
					Role:            "robot",
				},
			},
		},
		{
			JobName: "service-eureka",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&eureka.SDConfig{
					Server:           "http://eureka.example.com:8761/eureka",
					RefreshInterval:  model.Duration(30 * time.Second),
					HTTPClientConfig: config.DefaultHTTPClientConfig,
				},
			},
		},
		{
			JobName: "ovhcloud",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			HTTPClientConfig: config.DefaultHTTPClientConfig,
			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfigs: discovery.Configs{
				&ovhcloud.SDConfig{
					Endpoint:          "ovh-eu",
					ApplicationKey:    "testAppKey",
					ApplicationSecret: "testAppSecret",
					ConsumerKey:       "testConsumerKey",
					RefreshInterval:   model.Duration(60 * time.Second),
					Service:           "vps",
				},
				&ovhcloud.SDConfig{
					Endpoint:          "ovh-eu",
					ApplicationKey:    "testAppKey",
					ApplicationSecret: "testAppSecret",
					ConsumerKey:       "testConsumerKey",
					RefreshInterval:   model.Duration(60 * time.Second),
					Service:           "dedicated_server",
				},
			},
		},
		{
			JobName: "scaleway",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			HTTPClientConfig: config.DefaultHTTPClientConfig,
			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,

			ServiceDiscoveryConfigs: discovery.Configs{
				&scaleway.SDConfig{
					APIURL:           "https://api.scaleway.com",
					AccessKey:        "SCWXXXXXXXXXXXXXXXXX",
					HTTPClientConfig: config.DefaultHTTPClientConfig,
					Port:             80,
					Project:          "11111111-1111-1111-1111-111111111112",
					RefreshInterval:  model.Duration(60 * time.Second),
					Role:             "instance",
					SecretKey:        "11111111-1111-1111-1111-111111111111",
					Zone:             "fr-par-1",
				},
				&scaleway.SDConfig{
					APIURL:           "https://api.scaleway.com",
					AccessKey:        "SCWXXXXXXXXXXXXXXXXX",
					HTTPClientConfig: config.DefaultHTTPClientConfig,
					Port:             80,
					Project:          "11111111-1111-1111-1111-111111111112",
					RefreshInterval:  model.Duration(60 * time.Second),
					Role:             "baremetal",
					SecretKey:        "11111111-1111-1111-1111-111111111111",
					Zone:             "fr-par-1",
				},
			},
		},
		{
			JobName: "linode-instances",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&linode.SDConfig{
					HTTPClientConfig: config.HTTPClientConfig{
						Authorization: &config.Authorization{
							Type:        "Bearer",
							Credentials: "abcdef",
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
					Port:            80,
					TagSeparator:    linode.DefaultSDConfig.TagSeparator,
					RefreshInterval: model.Duration(60 * time.Second),
				},
			},
		},
		{
			JobName:                        "stackit-servers",
			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,
			ServiceDiscoveryConfigs: discovery.Configs{
				&stackit.SDConfig{
					Project: "11111111-1111-1111-1111-111111111111",
					Region:  "eu01",
					HTTPClientConfig: config.HTTPClientConfig{
						Authorization: &config.Authorization{
							Type:        "Bearer",
							Credentials: "abcdef",
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
					Port:            80,
					RefreshInterval: model.Duration(60 * time.Second),
				},
			},
		},
		{
			JobName: "uyuni",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			HTTPClientConfig: config.DefaultHTTPClientConfig,
			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			ServiceDiscoveryConfigs: discovery.Configs{
				&uyuni.SDConfig{
					Server:           "https://localhost:1234",
					Username:         "gopher",
					Password:         "hole",
					Entitlement:      "monitoring_entitled",
					Separator:        ",",
					RefreshInterval:  model.Duration(60 * time.Second),
					HTTPClientConfig: config.DefaultHTTPClientConfig,
				},
			},
		},
		{
			JobName: "ionos",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&ionos.SDConfig{
					DatacenterID: "8feda53f-15f0-447f-badf-ebe32dad2fc0",
					HTTPClientConfig: config.HTTPClientConfig{
						Authorization:   &config.Authorization{Type: "Bearer", Credentials: "abcdef"},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
					Port:            80,
					RefreshInterval: model.Duration(60 * time.Second),
				},
			},
		},
		{
			JobName: "vultr",

			HonorTimestamps:                true,
			ScrapeInterval:                 model.Duration(15 * time.Second),
			ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
			EnableCompression:              true,
			BodySizeLimit:                  globBodySizeLimit,
			SampleLimit:                    globSampleLimit,
			TargetLimit:                    globTargetLimit,
			LabelLimit:                     globLabelLimit,
			LabelNameLengthLimit:           globLabelNameLengthLimit,
			LabelValueLengthLimit:          globLabelValueLengthLimit,
			ScrapeProtocols:                DefaultScrapeProtocols,
			ScrapeFailureLogFile:           globScrapeFailureLogFile,
			MetricNameValidationScheme:     DefaultGlobalConfig.MetricNameValidationScheme,
			MetricNameEscapingScheme:       DefaultGlobalConfig.MetricNameEscapingScheme,
			ScrapeNativeHistograms:         boolPtr(false),
			AlwaysScrapeClassicHistograms:  boolPtr(false),
			ConvertClassicHistogramsToNHCB: boolPtr(false),

			MetricsPath:      DefaultScrapeConfig.MetricsPath,
			Scheme:           DefaultScrapeConfig.Scheme,
			HTTPClientConfig: config.DefaultHTTPClientConfig,

			ServiceDiscoveryConfigs: discovery.Configs{
				&vultr.SDConfig{
					HTTPClientConfig: config.HTTPClientConfig{
						Authorization: &config.Authorization{
							Type:        "Bearer",
							Credentials: "abcdef",
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},
					Port:            80,
					RefreshInterval: model.Duration(60 * time.Second),
				},
			},
		},
	},
	AlertingConfig: AlertingConfig{
		AlertmanagerConfigs: []*AlertmanagerConfig{
			{
				Scheme:           "https",
				Timeout:          model.Duration(10 * time.Second),
				APIVersion:       AlertmanagerAPIVersionV2,
				HTTPClientConfig: config.DefaultHTTPClientConfig,
				ServiceDiscoveryConfigs: discovery.Configs{
					discovery.StaticConfig{
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
	StorageConfig: StorageConfig{
		TSDBConfig: &TSDBConfig{
			OutOfOrderTimeWindow:     30 * time.Minute.Milliseconds(),
			OutOfOrderTimeWindowFlag: model.Duration(30 * time.Minute),
			Retention: &TSDBRetentionConfig{
				Time: model.Duration(24 * time.Hour),
				Size: 1 * units.GiB,
			},
		},
	},
	TracingConfig: TracingConfig{
		Endpoint:    "localhost:4317",
		ClientType:  TracingClientGRPC,
		Insecure:    false,
		Compression: "gzip",
		Timeout:     model.Duration(5 * time.Second),
		Headers:     map[string]string{"foo": "bar"},
		TLSConfig: config.TLSConfig{
			CertFile:           "testdata/valid_cert_file",
			KeyFile:            "testdata/valid_key_file",
			InsecureSkipVerify: true,
		},
	},
}

func TestYAMLNotLongerSupportedAMApi(t *testing.T) {
	_, err := LoadFile("testdata/config_with_no_longer_supported_am_api_config.yml", false, promslog.NewNopLogger())
	require.Error(t, err)
}

func TestYAMLRoundtrip(t *testing.T) {
	want, err := LoadFile("testdata/roundtrip.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)

	out, err := yaml.Marshal(want)
	require.NoError(t, err)

	got, err := Load(string(out), promslog.NewNopLogger())
	require.NoError(t, err)

	require.Equal(t, want, got)
}

func TestRemoteWriteRetryOnRateLimit(t *testing.T) {
	want, err := LoadFile("testdata/remote_write_retry_on_rate_limit.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)

	out, err := yaml.Marshal(want)
	require.NoError(t, err)

	got, err := Load(string(out), promslog.NewNopLogger())
	require.NoError(t, err)

	require.True(t, got.RemoteWriteConfigs[0].QueueConfig.RetryOnRateLimit)
	require.False(t, got.RemoteWriteConfigs[1].QueueConfig.RetryOnRateLimit)
}

func TestOTLPSanitizeResourceAttributes(t *testing.T) {
	t.Run("good config - default resource attributes", func(t *testing.T) {
		want, err := LoadFile(filepath.Join("testdata", "otlp_sanitize_default_resource_attributes.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		out, err := yaml.Marshal(want)
		require.NoError(t, err)
		var got Config
		require.NoError(t, yaml.UnmarshalStrict(out, &got))

		require.False(t, got.OTLPConfig.PromoteAllResourceAttributes)
		require.Empty(t, got.OTLPConfig.IgnoreResourceAttributes)
		require.Empty(t, got.OTLPConfig.PromoteResourceAttributes)
	})

	t.Run("good config - promote resource attributes", func(t *testing.T) {
		want, err := LoadFile(filepath.Join("testdata", "otlp_sanitize_promote_resource_attributes.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		out, err := yaml.Marshal(want)
		require.NoError(t, err)
		var got Config
		require.NoError(t, yaml.UnmarshalStrict(out, &got))

		require.False(t, got.OTLPConfig.PromoteAllResourceAttributes)
		require.Empty(t, got.OTLPConfig.IgnoreResourceAttributes)
		require.Equal(t, []string{"k8s.cluster.name", "k8s.job.name", "k8s.namespace.name"}, got.OTLPConfig.PromoteResourceAttributes)
	})

	t.Run("bad config - promote resource attributes", func(t *testing.T) {
		_, err := LoadFile(filepath.Join("testdata", "otlp_sanitize_promote_resource_attributes.bad.yml"), false, promslog.NewNopLogger())
		require.ErrorContains(t, err, `invalid 'promote_resource_attributes'`)
		require.ErrorContains(t, err, `duplicated promoted OTel resource attribute "k8s.job.name"`)
		require.ErrorContains(t, err, `empty promoted OTel resource attribute`)
	})

	t.Run("good config - promote all resource attributes", func(t *testing.T) {
		want, err := LoadFile(filepath.Join("testdata", "otlp_sanitize_resource_attributes_promote_all.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		out, err := yaml.Marshal(want)
		require.NoError(t, err)
		var got Config
		require.NoError(t, yaml.UnmarshalStrict(out, &got))
		require.True(t, got.OTLPConfig.PromoteAllResourceAttributes)
		require.Empty(t, got.OTLPConfig.PromoteResourceAttributes)
		require.Empty(t, got.OTLPConfig.IgnoreResourceAttributes)
	})

	t.Run("good config - ignore resource attributes", func(t *testing.T) {
		want, err := LoadFile(filepath.Join("testdata", "otlp_sanitize_ignore_resource_attributes.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		out, err := yaml.Marshal(want)
		require.NoError(t, err)
		var got Config
		require.NoError(t, yaml.UnmarshalStrict(out, &got))
		require.True(t, got.OTLPConfig.PromoteAllResourceAttributes)
		require.Empty(t, got.OTLPConfig.PromoteResourceAttributes)
		require.Equal(t, []string{"k8s.cluster.name", "k8s.job.name", "k8s.namespace.name"}, got.OTLPConfig.IgnoreResourceAttributes)
	})

	t.Run("bad config - ignore resource attributes", func(t *testing.T) {
		_, err := LoadFile(filepath.Join("testdata", "otlp_sanitize_ignore_resource_attributes.bad.yml"), false, promslog.NewNopLogger())
		require.ErrorContains(t, err, `invalid 'ignore_resource_attributes'`)
		require.ErrorContains(t, err, `duplicated ignored OTel resource attribute "k8s.job.name"`)
		require.ErrorContains(t, err, `empty ignored OTel resource attribute`)
	})

	t.Run("bad config - conflict between promote all and promote specific resource attributes", func(t *testing.T) {
		_, err := LoadFile(filepath.Join("testdata", "otlp_promote_all_resource_attributes.bad.yml"), false, promslog.NewNopLogger())
		require.ErrorContains(t, err, `'promote_all_resource_attributes' and 'promote_resource_attributes' cannot be configured simultaneously`)
	})

	t.Run("bad config - configuring ignoring of resource attributes without also enabling promotion of all resource attributes", func(t *testing.T) {
		_, err := LoadFile(filepath.Join("testdata", "otlp_ignore_resource_attributes_without_promote_all.bad.yml"), false, promslog.NewNopLogger())
		require.ErrorContains(t, err, `'ignore_resource_attributes' cannot be configured unless 'promote_all_resource_attributes' is true`)
	})
}

func TestOTLPAllowServiceNameInTargetInfo(t *testing.T) {
	t.Run("good config", func(t *testing.T) {
		want, err := LoadFile(filepath.Join("testdata", "otlp_allow_keep_identifying_resource_attributes.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		out, err := yaml.Marshal(want)
		require.NoError(t, err)
		var got Config
		require.NoError(t, yaml.UnmarshalStrict(out, &got))

		require.True(t, got.OTLPConfig.KeepIdentifyingResourceAttributes)
	})
}

func TestOTLPConvertHistogramsToNHCB(t *testing.T) {
	t.Run("good config", func(t *testing.T) {
		want, err := LoadFile(filepath.Join("testdata", "otlp_convert_histograms_to_nhcb.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		out, err := yaml.Marshal(want)
		require.NoError(t, err)
		var got Config
		require.NoError(t, yaml.UnmarshalStrict(out, &got))

		require.True(t, got.OTLPConfig.ConvertHistogramsToNHCB)
	})
}

func TestOTLPPromoteScopeMetadata(t *testing.T) {
	t.Run("good config", func(t *testing.T) {
		want, err := LoadFile(filepath.Join("testdata", "otlp_promote_scope_metadata.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		out, err := yaml.Marshal(want)
		require.NoError(t, err)
		var got Config
		require.NoError(t, yaml.UnmarshalStrict(out, &got))

		require.True(t, got.OTLPConfig.PromoteScopeMetadata)
	})
}

func TestOTLPLabelUnderscoreSanitization(t *testing.T) {
	t.Run("defaults to true", func(t *testing.T) {
		conf, err := LoadFile(filepath.Join("testdata", "otlp_label_underscore_sanitization_defaults.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		// Test that default values are true
		require.True(t, conf.OTLPConfig.LabelNameUnderscoreSanitization)
		require.True(t, conf.OTLPConfig.LabelNamePreserveMultipleUnderscores)
	})

	t.Run("explicitly enabled", func(t *testing.T) {
		conf, err := LoadFile(filepath.Join("testdata", "otlp_label_underscore_sanitization_enabled.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		out, err := yaml.Marshal(conf)
		require.NoError(t, err)
		var got Config
		require.NoError(t, yaml.UnmarshalStrict(out, &got))

		require.True(t, got.OTLPConfig.LabelNameUnderscoreSanitization)
		require.True(t, got.OTLPConfig.LabelNamePreserveMultipleUnderscores)
	})

	t.Run("explicitly disabled", func(t *testing.T) {
		conf, err := LoadFile(filepath.Join("testdata", "otlp_label_underscore_sanitization_disabled.good.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		// When explicitly set to false, they should be false
		require.False(t, conf.OTLPConfig.LabelNameUnderscoreSanitization)
		require.False(t, conf.OTLPConfig.LabelNamePreserveMultipleUnderscores)
	})

	t.Run("empty config uses defaults", func(t *testing.T) {
		conf, err := LoadFile(filepath.Join("testdata", "otlp_empty.yml"), false, promslog.NewNopLogger())
		require.NoError(t, err)

		// Empty config should use default values (true)
		require.True(t, conf.OTLPConfig.LabelNameUnderscoreSanitization)
		require.True(t, conf.OTLPConfig.LabelNamePreserveMultipleUnderscores)
	})
}

func TestOTLPAllowUTF8(t *testing.T) {
	t.Run("good config - NoUTF8EscapingWithSuffixes", func(t *testing.T) {
		fpath := filepath.Join("testdata", "otlp_allow_utf8.good.yml")
		verify := func(t *testing.T, conf *Config, err error) {
			t.Helper()
			require.NoError(t, err)
			require.Equal(t, otlptranslator.NoUTF8EscapingWithSuffixes, conf.OTLPConfig.TranslationStrategy)
		}

		t.Run("LoadFile", func(t *testing.T) {
			conf, err := LoadFile(fpath, false, promslog.NewNopLogger())
			verify(t, conf, err)
		})
		t.Run("Load", func(t *testing.T) {
			content, err := os.ReadFile(fpath)
			require.NoError(t, err)
			conf, err := Load(string(content), promslog.NewNopLogger())
			verify(t, conf, err)
		})
	})

	t.Run("incompatible config - NoUTF8EscapingWithSuffixes", func(t *testing.T) {
		fpath := filepath.Join("testdata", "otlp_allow_utf8.incompatible.yml")
		verify := func(t *testing.T, err error) {
			t.Helper()
			require.ErrorContains(t, err, `OTLP translation strategy "NoUTF8EscapingWithSuffixes" is not allowed when UTF8 is disabled`)
		}

		t.Run("LoadFile", func(t *testing.T) {
			_, err := LoadFile(fpath, false, promslog.NewNopLogger())
			verify(t, err)
		})
		t.Run("Load", func(t *testing.T) {
			content, err := os.ReadFile(fpath)
			require.NoError(t, err)
			_, err = Load(string(content), promslog.NewNopLogger())
			t.Log("err", err)
			verify(t, err)
		})
	})

	t.Run("good config - NoTranslation", func(t *testing.T) {
		fpath := filepath.Join("testdata", "otlp_no_translation.good.yml")
		verify := func(t *testing.T, conf *Config, err error) {
			t.Helper()
			require.NoError(t, err)
			require.Equal(t, otlptranslator.NoTranslation, conf.OTLPConfig.TranslationStrategy)
		}

		t.Run("LoadFile", func(t *testing.T) {
			conf, err := LoadFile(fpath, false, promslog.NewNopLogger())
			verify(t, conf, err)
		})
		t.Run("Load", func(t *testing.T) {
			content, err := os.ReadFile(fpath)
			require.NoError(t, err)
			conf, err := Load(string(content), promslog.NewNopLogger())
			verify(t, conf, err)
		})
	})

	t.Run("incompatible config - NoTranslation", func(t *testing.T) {
		fpath := filepath.Join("testdata", "otlp_no_translation.incompatible.yml")
		verify := func(t *testing.T, err error) {
			t.Helper()
			require.ErrorContains(t, err, `OTLP translation strategy "NoTranslation" is not allowed when UTF8 is disabled`)
		}

		t.Run("LoadFile", func(t *testing.T) {
			_, err := LoadFile(fpath, false, promslog.NewNopLogger())
			verify(t, err)
		})
		t.Run("Load", func(t *testing.T) {
			content, err := os.ReadFile(fpath)
			require.NoError(t, err)
			_, err = Load(string(content), promslog.NewNopLogger())
			t.Log("err", err)
			verify(t, err)
		})
	})

	t.Run("bad config", func(t *testing.T) {
		fpath := filepath.Join("testdata", "otlp_allow_utf8.bad.yml")
		verify := func(t *testing.T, err error) {
			t.Helper()
			require.ErrorContains(t, err, `unsupported OTLP translation strategy "Invalid"`)
		}

		t.Run("LoadFile", func(t *testing.T) {
			_, err := LoadFile(fpath, false, promslog.NewNopLogger())
			verify(t, err)
		})
		t.Run("Load", func(t *testing.T) {
			content, err := os.ReadFile(fpath)
			require.NoError(t, err)
			_, err = Load(string(content), promslog.NewNopLogger())
			verify(t, err)
		})
	})

	t.Run("good config - missing otlp config uses default", func(t *testing.T) {
		fpath := filepath.Join("testdata", "otlp_empty.yml")
		verify := func(t *testing.T, conf *Config, err error) {
			t.Helper()
			require.NoError(t, err)
			require.Equal(t, otlptranslator.UnderscoreEscapingWithSuffixes, conf.OTLPConfig.TranslationStrategy)
		}

		t.Run("LoadFile", func(t *testing.T) {
			conf, err := LoadFile(fpath, false, promslog.NewNopLogger())
			verify(t, conf, err)
		})
		t.Run("Load", func(t *testing.T) {
			content, err := os.ReadFile(fpath)
			require.NoError(t, err)
			conf, err := Load(string(content), promslog.NewNopLogger())
			verify(t, conf, err)
		})
	})
}

func TestLoadConfig(t *testing.T) {
	// Parse a valid file that sets a global scrape timeout. This tests whether parsing
	// an overwritten default field in the global config permanently changes the default.
	_, err := LoadFile("testdata/global_timeout.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)

	c, err := LoadFile("testdata/conf.good.yml", false, promslog.NewNopLogger())

	require.NoError(t, err)
	testutil.RequireEqualWithOptions(t, expectedConf, c, []cmp.Option{
		cmpopts.IgnoreUnexported(config.ProxyConfig{}),
		cmpopts.IgnoreUnexported(ionos.SDConfig{}),
		cmpopts.IgnoreUnexported(stackit.SDConfig{}),
		cmpopts.IgnoreUnexported(regexp.Regexp{}),
		cmpopts.IgnoreUnexported(hetzner.SDConfig{}),
		cmpopts.IgnoreUnexported(Config{}),
	})
}

func TestScrapeIntervalLarger(t *testing.T) {
	c, err := LoadFile("testdata/scrape_interval_larger.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
	require.Len(t, c.ScrapeConfigs, 1)
	for _, sc := range c.ScrapeConfigs {
		require.GreaterOrEqual(t, sc.ScrapeInterval, sc.ScrapeTimeout)
	}
}

// YAML marshaling must not reveal authentication credentials.
func TestElideSecrets(t *testing.T) {
	c, err := LoadFile("testdata/conf.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)

	secretRe := regexp.MustCompile(`\\u003csecret\\u003e|<secret>`)

	config, err := yaml.Marshal(c)
	require.NoError(t, err)
	yamlConfig := string(config)

	matches := secretRe.FindAllStringIndex(yamlConfig, -1)
	require.Len(t, matches, 25, "wrong number of secret matches found")
	require.NotContains(t, yamlConfig, "mysecret",
		"yaml marshal reveals authentication credentials.")
}

func TestLoadConfigRuleFilesAbsolutePath(t *testing.T) {
	// Parse a valid file that sets a rule files with an absolute path
	c, err := LoadFile(ruleFilesConfigFile, false, promslog.NewNopLogger())
	require.NoError(t, err)
	require.Equal(t, ruleFilesExpectedConf, c)
}

func TestKubernetesEmptyAPIServer(t *testing.T) {
	_, err := LoadFile("testdata/kubernetes_empty_apiserver.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
}

func TestKubernetesWithKubeConfig(t *testing.T) {
	_, err := LoadFile("testdata/kubernetes_kubeconfig_without_apiserver.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
}

func TestKubernetesSelectors(t *testing.T) {
	_, err := LoadFile("testdata/kubernetes_selectors_endpoints.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
	_, err = LoadFile("testdata/kubernetes_selectors_node.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
	_, err = LoadFile("testdata/kubernetes_selectors_ingress.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
	_, err = LoadFile("testdata/kubernetes_selectors_pod.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
	_, err = LoadFile("testdata/kubernetes_selectors_service.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
}

var expectedErrors = []struct {
	filename string
	errMsg   string
}{
	{
		filename: "jobname.bad.yml",
		errMsg:   `job_name is empty`,
	},
	{
		filename: "jobname_dup.bad.yml",
		errMsg:   `found multiple scrape configs with job name "prometheus"`,
	},
	{
		filename: "scrape_interval.bad.yml",
		errMsg:   `scrape timeout greater than scrape interval`,
	},
	{
		filename: "labelname.bad.yml",
		errMsg:   `"\xff" is not a valid label name`,
	},
	{
		filename: "labelvalue.bad.yml",
		errMsg:   `"\xff" is not a valid label value`,
	},
	{
		filename: "regex.bad.yml",
		errMsg:   "error parsing regexp",
	},
	{
		filename: "modulus_missing.bad.yml",
		errMsg:   "relabel configuration for hashmod requires non-zero modulus",
	},
	{
		filename: "labelkeep.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	},
	{
		filename: "labelkeep2.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	},
	{
		filename: "labelkeep3.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	},
	{
		filename: "labelkeep4.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	},
	{
		filename: "labelkeep5.bad.yml",
		errMsg:   "labelkeep action requires only 'regex', and no other fields",
	},
	{
		filename: "labeldrop.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	},
	{
		filename: "labeldrop2.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	},
	{
		filename: "labeldrop3.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	},
	{
		filename: "labeldrop4.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	},
	{
		filename: "labeldrop5.bad.yml",
		errMsg:   "labeldrop action requires only 'regex', and no other fields",
	},
	{
		filename: "dropequal.bad.yml",
		errMsg:   "relabel configuration for dropequal action requires 'target_label' value",
	},
	{
		filename: "dropequal1.bad.yml",
		errMsg:   "dropequal action requires only 'source_labels' and `target_label`, and no other fields",
	},
	{
		filename: "keepequal.bad.yml",
		errMsg:   "relabel configuration for keepequal action requires 'target_label' value",
	},
	{
		filename: "keepequal1.bad.yml",
		errMsg:   "keepequal action requires only 'source_labels' and `target_label`, and no other fields",
	},
	{
		filename: "labelmap.bad.yml",
		errMsg:   "!!binary value contains invalid base64 data",
	},
	{
		filename: "lowercase.bad.yml",
		errMsg:   "relabel configuration for lowercase action requires 'target_label' value",
	},
	{
		filename: "lowercase3.bad.yml",
		errMsg:   "'replacement' can not be set for lowercase action",
	},
	{
		filename: "uppercase.bad.yml",
		errMsg:   "relabel configuration for uppercase action requires 'target_label' value",
	},
	{
		filename: "uppercase3.bad.yml",
		errMsg:   "'replacement' can not be set for uppercase action",
	},
	{
		filename: "rules.bad.yml",
		errMsg:   "invalid rule file path",
	},
	{
		filename: "unknown_attr.bad.yml",
		errMsg:   "field consult_sd_configs not found in type",
	},
	{
		filename: "bearertoken.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	},
	{
		filename: "bearertoken_basicauth.bad.yml",
		errMsg:   "at most one of basic_auth, oauth2, bearer_token & bearer_token_file must be configured",
	},
	{
		filename: "kubernetes_http_config_without_api_server.bad.yml",
		errMsg:   "to use custom HTTP client configuration please provide the 'api_server' URL explicitly",
	},
	{
		filename: "kubernetes_kubeconfig_with_own_namespace.bad.yml",
		errMsg:   "cannot use 'kubeconfig_file' and 'namespaces.own_namespace' simultaneously",
	},
	{
		filename: "kubernetes_api_server_with_own_namespace.bad.yml",
		errMsg:   "cannot use 'api_server' and 'namespaces.own_namespace' simultaneously",
	},
	{
		filename: "kubernetes_kubeconfig_with_apiserver.bad.yml",
		errMsg:   "cannot use 'kubeconfig_file' and 'api_server' simultaneously",
	},
	{
		filename: "kubernetes_kubeconfig_with_http_config.bad.yml",
		errMsg:   "cannot use a custom HTTP client configuration together with 'kubeconfig_file'",
	},
	{
		filename: "kubernetes_bearertoken.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	},
	{
		filename: "kubernetes_role.bad.yml",
		errMsg:   "role",
	},
	{
		filename: "kubernetes_selectors_endpoints.bad.yml",
		errMsg:   "endpoints role supports only pod, service, endpoints selectors",
	},
	{
		filename: "kubernetes_selectors_ingress.bad.yml",
		errMsg:   "ingress role supports only ingress selectors",
	},
	{
		filename: "kubernetes_selectors_node.bad.yml",
		errMsg:   "node role supports only node selectors",
	},
	{
		filename: "kubernetes_selectors_pod.bad.yml",
		errMsg:   "pod role supports only pod selectors",
	},
	{
		filename: "kubernetes_selectors_service.bad.yml",
		errMsg:   "service role supports only service selectors",
	},
	{
		filename: "kubernetes_namespace_discovery.bad.yml",
		errMsg:   "field foo not found in type kubernetes.plain",
	},
	{
		filename: "kubernetes_selectors_duplicated_role.bad.yml",
		errMsg:   "duplicated selector role: pod",
	},
	{
		filename: "kubernetes_selectors_incorrect_selector.bad.yml",
		errMsg:   "invalid selector: 'metadata.status-Running'; can't understand 'metadata.status-Running'",
	},
	{
		filename: "kubernetes_bearertoken_basicauth.bad.yml",
		errMsg:   "at most one of basic_auth, oauth2, bearer_token & bearer_token_file must be configured",
	},
	{
		filename: "kubernetes_authorization_basicauth.bad.yml",
		errMsg:   "at most one of basic_auth, oauth2 & authorization must be configured",
	},
	{
		filename: "marathon_no_servers.bad.yml",
		errMsg:   "marathon_sd: must contain at least one Marathon server",
	},
	{
		filename: "marathon_authtoken_authtokenfile.bad.yml",
		errMsg:   "marathon_sd: at most one of auth_token & auth_token_file must be configured",
	},
	{
		filename: "marathon_authtoken_basicauth.bad.yml",
		errMsg:   "marathon_sd: at most one of basic_auth, auth_token & auth_token_file must be configured",
	},
	{
		filename: "marathon_authtoken_bearertoken.bad.yml",
		errMsg:   "marathon_sd: at most one of bearer_token, bearer_token_file, auth_token & auth_token_file must be configured",
	},
	{
		filename: "marathon_authtoken_authorization.bad.yml",
		errMsg:   "marathon_sd: at most one of auth_token, auth_token_file & authorization must be configured",
	},
	{
		filename: "openstack_role.bad.yml",
		errMsg:   "unknown OpenStack SD role",
	},
	{
		filename: "openstack_availability.bad.yml",
		errMsg:   "unknown availability invalid, must be one of admin, internal or public",
	},
	{
		filename: "url_in_targetgroup.bad.yml",
		errMsg:   "\"http://bad\" is not a valid hostname",
	},
	{
		filename: "target_label_missing.bad.yml",
		errMsg:   "relabel configuration for replace action requires 'target_label' value",
	},
	{
		filename: "target_label_hashmod_missing.bad.yml",
		errMsg:   "relabel configuration for hashmod action requires 'target_label' value",
	},
	{
		filename: "unknown_global_attr.bad.yml",
		errMsg:   "field nonexistent_field not found in type config.plain",
	},
	{
		filename: "remote_read_url_missing.bad.yml",
		errMsg:   `url for remote_read is empty`,
	},
	{
		filename: "remote_write_header.bad.yml",
		errMsg:   `x-prometheus-remote-write-version is a reserved header. It must not be changed`,
	},
	{
		filename: "remote_read_header.bad.yml",
		errMsg:   `x-prometheus-remote-write-version is a reserved header. It must not be changed`,
	},
	{
		filename: "remote_write_authorization_header.bad.yml",
		errMsg:   `authorization header must be changed via the basic_auth, authorization, oauth2, sigv4, azuread or google_iam parameter`,
	},
	{
		filename: "remote_write_wrong_msg.bad.yml",
		errMsg:   `invalid protobuf_message value: unknown type for remote write protobuf message io.prometheus.writet.v2.Request, supported: prometheus.WriteRequest, io.prometheus.write.v2.Request`,
	},
	{
		filename: "remote_write_url_missing.bad.yml",
		errMsg:   `url for remote_write is empty`,
	},
	{
		filename: "remote_write_dup.bad.yml",
		errMsg:   `found multiple remote write configs with job name "queue1"`,
	},
	{
		filename: "remote_read_dup.bad.yml",
		errMsg:   `found multiple remote read configs with job name "queue1"`,
	},
	{
		filename: "ec2_filters_empty_values.bad.yml",
		errMsg:   `EC2 SD configuration filter values cannot be empty`,
	},
	{
		filename: "ec2_token_file.bad.yml",
		errMsg:   `at most one of bearer_token & bearer_token_file must be configured`,
	},
	{
		filename: "lightsail_token_file.bad.yml",
		errMsg:   `at most one of bearer_token & bearer_token_file must be configured`,
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
		errMsg:   "unknown authentication_type \"invalid\". Supported types are \"OAuth\", \"ManagedIdentity\" or \"SDK\"",
	},
	{
		filename: "azure_bearertoken_basicauth.bad.yml",
		errMsg:   "at most one of basic_auth, oauth2, bearer_token & bearer_token_file must be configured",
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
	{
		filename: "puppetdb_no_query.bad.yml",
		errMsg:   "query missing",
	},
	{
		filename: "puppetdb_no_url.bad.yml",
		errMsg:   "URL is missing",
	},
	{
		filename: "puppetdb_bad_url.bad.yml",
		errMsg:   "host is missing in URL",
	},
	{
		filename: "puppetdb_no_scheme.bad.yml",
		errMsg:   "URL scheme must be 'http' or 'https'",
	},
	{
		filename: "puppetdb_token_file.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	},
	{
		filename: "hetzner_role.bad.yml",
		errMsg:   "unknown role",
	},
	{
		filename: "eureka_no_server.bad.yml",
		errMsg:   "empty or null eureka server",
	},
	{
		filename: "eureka_invalid_server.bad.yml",
		errMsg:   "invalid eureka server URL",
	},
	{
		filename: "scaleway_role.bad.yml",
		errMsg:   `unknown role "invalid"`,
	},
	{
		filename: "scaleway_no_secret.bad.yml",
		errMsg:   "one of secret_key & secret_key_file must be configured",
	},
	{
		filename: "scaleway_two_secrets.bad.yml",
		errMsg:   "at most one of secret_key & secret_key_file must be configured",
	},
	{
		filename: "scrape_body_size_limit.bad.yml",
		errMsg:   "units: unknown unit  in 100",
	},
	{
		filename: "http_url_no_scheme.bad.yml",
		errMsg:   "URL scheme must be 'http' or 'https'",
	},
	{
		filename: "http_url_no_host.bad.yml",
		errMsg:   "host is missing in URL",
	},
	{
		filename: "http_token_file.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	},
	{
		filename: "http_url_bad_scheme.bad.yml",
		errMsg:   "URL scheme must be 'http' or 'https'",
	},
	{
		filename: "empty_scrape_config_action.bad.yml",
		errMsg:   "relabel action cannot be empty",
	},
	{
		filename: "tracing_missing_endpoint.bad.yml",
		errMsg:   "tracing endpoint must be set",
	},
	{
		filename: "tracing_invalid_header.bad.yml",
		errMsg:   "x-prometheus-remote-write-version is a reserved header. It must not be changed",
	},
	{
		filename: "tracing_invalid_authorization_header.bad.yml",
		errMsg:   "authorization header configuration is not yet supported",
	},
	{
		filename: "tracing_invalid_compression.bad.yml",
		errMsg:   "invalid compression type foo provided, valid options: gzip",
	},
	{
		filename: "uyuni_no_server.bad.yml",
		errMsg:   "Uyuni SD configuration requires server host",
	},
	{
		filename: "uyuni_token_file.bad.yml",
		errMsg:   "at most one of bearer_token & bearer_token_file must be configured",
	},
	{
		filename: "ionos_datacenter.bad.yml",
		errMsg:   "datacenter id can't be empty",
	},
	{
		filename: "ovhcloud_no_secret.bad.yml",
		errMsg:   "application secret can not be empty",
	},
	{
		filename: "ovhcloud_bad_service.bad.yml",
		errMsg:   "unknown service: fakeservice",
	},
	{
		filename: "scrape_config_files_glob.bad.yml",
		errMsg:   `parsing YAML file testdata/scrape_config_files_glob.bad.yml: invalid scrape config file path "scrape_configs/*/*"`,
	},
	{
		filename: "scrape_config_files_scrape_protocols.bad.yml",
		errMsg:   `parsing YAML file testdata/scrape_config_files_scrape_protocols.bad.yml: scrape_protocols: unknown scrape protocol prometheusproto, supported: [OpenMetricsText0.0.1 OpenMetricsText1.0.0 PrometheusProto PrometheusText0.0.4 PrometheusText1.0.0] for scrape config with job name "node"`,
	},
	{
		filename: "scrape_config_files_scrape_protocols2.bad.yml",
		errMsg:   `parsing YAML file testdata/scrape_config_files_scrape_protocols2.bad.yml: duplicated protocol in scrape_protocols, got [OpenMetricsText1.0.0 PrometheusProto OpenMetricsText1.0.0] for scrape config with job name "node"`,
	},
	{
		filename: "scrape_config_files_fallback_scrape_protocol1.bad.yml",
		errMsg:   `parsing YAML file testdata/scrape_config_files_fallback_scrape_protocol1.bad.yml: invalid fallback_scrape_protocol for scrape config with job name "node": unknown scrape protocol prometheusproto, supported: [OpenMetricsText0.0.1 OpenMetricsText1.0.0 PrometheusProto PrometheusText0.0.4 PrometheusText1.0.0]`,
	},
	{
		filename: "scrape_config_files_fallback_scrape_protocol2.bad.yml",
		errMsg:   `unmarshal errors`,
	},
	{
		filename: "scrape_config_utf8_conflicting.bad.yml",
		errMsg:   `utf8 metric names requested but validation scheme is not set to UTF8`,
	},
	{
		filename: "stackit_endpoint.bad.yml",
		errMsg:   "invalid endpoint",
	},
}

func TestBadConfigs(t *testing.T) {
	for _, ee := range expectedErrors {
		_, err := LoadFile("testdata/"+ee.filename, false, promslog.NewNopLogger())
		require.ErrorContains(t, err, ee.errMsg,
			"Expected error for %s to contain %q but got: %s", ee.filename, ee.errMsg, err)
	}
}

func TestBadStaticConfigsYML(t *testing.T) {
	content, err := os.ReadFile("testdata/static_config.bad.yml")
	require.NoError(t, err)
	var tg targetgroup.Group
	err = yaml.UnmarshalStrict(content, &tg)
	require.Error(t, err)
}

func TestEmptyConfig(t *testing.T) {
	c, err := Load("", promslog.NewNopLogger())
	require.NoError(t, err)
	exp := DefaultConfig
	exp.loaded = true
	require.Equal(t, exp, *c)
	require.Equal(t, 75, c.Runtime.GoGC)
}

func TestExpandExternalLabels(t *testing.T) {
	// Cleanup ant TEST env variable that could exist on the system.
	os.Setenv("TEST", "")

	c, err := LoadFile("testdata/external_labels.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
	testutil.RequireEqual(t, labels.FromStrings("bar", "foo", "baz", "foobar", "foo", "", "qux", "foo${TEST}", "xyz", "foo$bar"), c.GlobalConfig.ExternalLabels)

	os.Setenv("TEST", "TestValue")
	c, err = LoadFile("testdata/external_labels.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)
	testutil.RequireEqual(t, labels.FromStrings("bar", "foo", "baz", "fooTestValuebar", "foo", "TestValue", "qux", "foo${TEST}", "xyz", "foo$bar"), c.GlobalConfig.ExternalLabels)
}

func TestAgentMode(t *testing.T) {
	_, err := LoadFile("testdata/agent_mode.with_alert_manager.yml", true, promslog.NewNopLogger())
	require.ErrorContains(t, err, "field alerting is not allowed in agent mode")

	_, err = LoadFile("testdata/agent_mode.with_alert_relabels.yml", true, promslog.NewNopLogger())
	require.ErrorContains(t, err, "field alerting is not allowed in agent mode")

	_, err = LoadFile("testdata/agent_mode.with_rule_files.yml", true, promslog.NewNopLogger())
	require.ErrorContains(t, err, "field rule_files is not allowed in agent mode")

	_, err = LoadFile("testdata/agent_mode.with_remote_reads.yml", true, promslog.NewNopLogger())
	require.ErrorContains(t, err, "field remote_read is not allowed in agent mode")

	c, err := LoadFile("testdata/agent_mode.without_remote_writes.yml", true, promslog.NewNopLogger())
	require.NoError(t, err)
	require.Empty(t, c.RemoteWriteConfigs)

	c, err = LoadFile("testdata/agent_mode.good.yml", true, promslog.NewNopLogger())
	require.NoError(t, err)
	require.Len(t, c.RemoteWriteConfigs, 1)
	require.Equal(
		t,
		"http://remote1/push",
		c.RemoteWriteConfigs[0].URL.String(),
	)
}

func TestEmptyGlobalBlock(t *testing.T) {
	c, err := Load("global:\n", promslog.NewNopLogger())
	require.NoError(t, err)
	exp := DefaultConfig
	exp.loaded = true
	require.Equal(t, exp, *c)
}

// ScrapeConfigOptions contains options for creating a scrape config.
type ScrapeConfigOptions struct {
	JobName                       string
	ScrapeInterval                model.Duration
	ScrapeTimeout                 model.Duration
	ScrapeProtocols               []ScrapeProtocol // Set to DefaultScrapeProtocols by default.
	ScrapeNativeHistograms        bool
	AlwaysScrapeClassicHistograms bool
	ConvertClassicHistToNHCB      bool
}

func TestGetScrapeConfigs(t *testing.T) {
	// Helper function to create a scrape config with the given options.
	sc := func(opts ScrapeConfigOptions) *ScrapeConfig {
		sc := ScrapeConfig{
			JobName:                    opts.JobName,
			HonorTimestamps:            true,
			ScrapeInterval:             opts.ScrapeInterval,
			ScrapeTimeout:              opts.ScrapeTimeout,
			ScrapeProtocols:            opts.ScrapeProtocols,
			MetricNameValidationScheme: model.UTF8Validation,
			MetricNameEscapingScheme:   model.AllowUTF8,

			MetricsPath:       "/metrics",
			Scheme:            "http",
			EnableCompression: true,
			HTTPClientConfig:  config.DefaultHTTPClientConfig,
			ServiceDiscoveryConfigs: discovery.Configs{
				discovery.StaticConfig{
					{
						Targets: []model.LabelSet{
							{
								model.AddressLabel: "localhost:8080",
							},
						},
						Source: "0",
					},
				},
			},
			ScrapeNativeHistograms:         boolPtr(opts.ScrapeNativeHistograms),
			AlwaysScrapeClassicHistograms:  boolPtr(opts.AlwaysScrapeClassicHistograms),
			ConvertClassicHistogramsToNHCB: boolPtr(opts.ConvertClassicHistToNHCB),
		}
		if opts.ScrapeProtocols == nil {
			sc.ScrapeProtocols = DefaultScrapeProtocols
		}
		return &sc
	}

	testCases := []struct {
		name           string
		configFile     string
		expectedResult []*ScrapeConfig
		expectedError  string
	}{
		{
			name:       "An included config file should be a valid global config.",
			configFile: "testdata/scrape_config_files.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{
				JobName:                       "prometheus",
				ScrapeInterval:                model.Duration(60 * time.Second),
				ScrapeTimeout:                 model.Duration(10 * time.Second),
				ScrapeNativeHistograms:        false,
				AlwaysScrapeClassicHistograms: false,
				ConvertClassicHistToNHCB:      false,
			})},
		},
		{
			name:       "A global config that only include a scrape config file.",
			configFile: "testdata/scrape_config_files_only.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{
				JobName:                       "prometheus",
				ScrapeInterval:                model.Duration(60 * time.Second),
				ScrapeTimeout:                 model.Duration(10 * time.Second),
				ScrapeNativeHistograms:        false,
				AlwaysScrapeClassicHistograms: false,
				ConvertClassicHistToNHCB:      false,
			})},
		},
		{
			name:       "A global config that combine scrape config files and scrape configs.",
			configFile: "testdata/scrape_config_files_combined.good.yml",
			expectedResult: []*ScrapeConfig{
				sc(ScrapeConfigOptions{
					JobName:                       "node",
					ScrapeInterval:                model.Duration(60 * time.Second),
					ScrapeTimeout:                 model.Duration(10 * time.Second),
					ScrapeNativeHistograms:        false,
					AlwaysScrapeClassicHistograms: false,
					ConvertClassicHistToNHCB:      false,
				}),
				sc(ScrapeConfigOptions{
					JobName:                       "prometheus",
					ScrapeInterval:                model.Duration(60 * time.Second),
					ScrapeTimeout:                 model.Duration(10 * time.Second),
					ScrapeNativeHistograms:        false,
					AlwaysScrapeClassicHistograms: false,
					ConvertClassicHistToNHCB:      false,
				}),
				sc(ScrapeConfigOptions{
					JobName:                       "alertmanager",
					ScrapeInterval:                model.Duration(60 * time.Second),
					ScrapeTimeout:                 model.Duration(10 * time.Second),
					ScrapeNativeHistograms:        false,
					AlwaysScrapeClassicHistograms: false,
					ConvertClassicHistToNHCB:      false,
				}),
			},
		},
		{
			name:       "A global config that includes a scrape config file with globs",
			configFile: "testdata/scrape_config_files_glob.good.yml",
			expectedResult: []*ScrapeConfig{
				{
					JobName: "prometheus",

					HonorTimestamps:                true,
					ScrapeInterval:                 model.Duration(60 * time.Second),
					ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
					ScrapeProtocols:                DefaultScrapeProtocols,
					MetricNameValidationScheme:     model.UTF8Validation,
					MetricNameEscapingScheme:       model.AllowUTF8,
					ScrapeNativeHistograms:         boolPtr(false),
					AlwaysScrapeClassicHistograms:  boolPtr(false),
					ConvertClassicHistogramsToNHCB: boolPtr(false),

					MetricsPath: DefaultScrapeConfig.MetricsPath,
					Scheme:      DefaultScrapeConfig.Scheme,

					EnableCompression: true,

					HTTPClientConfig: config.HTTPClientConfig{
						TLSConfig: config.TLSConfig{
							CertFile: filepath.FromSlash("testdata/scrape_configs/valid_cert_file"),
							KeyFile:  filepath.FromSlash("testdata/scrape_configs/valid_key_file"),
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},

					ServiceDiscoveryConfigs: discovery.Configs{
						discovery.StaticConfig{
							{
								Targets: []model.LabelSet{
									{model.AddressLabel: "localhost:8080"},
								},
								Source: "0",
							},
						},
					},
				},
				{
					JobName: "node",

					HonorTimestamps:                true,
					ScrapeInterval:                 model.Duration(15 * time.Second),
					ScrapeTimeout:                  DefaultGlobalConfig.ScrapeTimeout,
					ScrapeProtocols:                DefaultScrapeProtocols,
					MetricNameValidationScheme:     model.UTF8Validation,
					MetricNameEscapingScheme:       model.AllowUTF8,
					ScrapeNativeHistograms:         boolPtr(false),
					AlwaysScrapeClassicHistograms:  boolPtr(false),
					ConvertClassicHistogramsToNHCB: boolPtr(false),

					HTTPClientConfig: config.HTTPClientConfig{
						TLSConfig: config.TLSConfig{
							CertFile: filepath.FromSlash("testdata/valid_cert_file"),
							KeyFile:  filepath.FromSlash("testdata/valid_key_file"),
						},
						FollowRedirects: true,
						EnableHTTP2:     true,
					},

					MetricsPath: DefaultScrapeConfig.MetricsPath,
					Scheme:      DefaultScrapeConfig.Scheme,

					EnableCompression: true,

					ServiceDiscoveryConfigs: discovery.Configs{
						&vultr.SDConfig{
							HTTPClientConfig: config.HTTPClientConfig{
								Authorization: &config.Authorization{
									Type:        "Bearer",
									Credentials: "abcdef",
								},
								FollowRedirects: true,
								EnableHTTP2:     true,
							},
							Port:            80,
							RefreshInterval: model.Duration(60 * time.Second),
						},
					},
				},
			},
		},
		{
			name:          "A global config that includes twice the same scrape configs.",
			configFile:    "testdata/scrape_config_files_double_import.bad.yml",
			expectedError: `found multiple scrape configs with job name "prometheus"`,
		},
		{
			name:          "A global config that includes a scrape config identical to a scrape config in the main file.",
			configFile:    "testdata/scrape_config_files_duplicate.bad.yml",
			expectedError: `found multiple scrape configs with job name "prometheus"`,
		},
		{
			name:          "A global config that includes a scrape config file with errors.",
			configFile:    "testdata/scrape_config_files_global.bad.yml",
			expectedError: `scrape timeout greater than scrape interval for scrape config with job name "prometheus"`,
		},
		{
			name:           "A global config that enables convert classic histograms to nhcb.",
			configFile:     "testdata/global_convert_classic_hist_to_nhcb.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), AlwaysScrapeClassicHistograms: false, ConvertClassicHistToNHCB: true})},
		},
		{
			name:           "A global config that enables convert classic histograms to nhcb and scrape config that disables the conversion",
			configFile:     "testdata/local_disable_convert_classic_hist_to_nhcb.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), AlwaysScrapeClassicHistograms: false, ConvertClassicHistToNHCB: false})},
		},
		{
			name:           "A global config that disables convert classic histograms to nhcb and scrape config that enables the conversion",
			configFile:     "testdata/local_convert_classic_hist_to_nhcb.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), AlwaysScrapeClassicHistograms: false, ConvertClassicHistToNHCB: true})},
		},
		{
			name:           "A global config that enables always scrape classic histograms",
			configFile:     "testdata/global_enable_always_scrape_classic_hist.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), AlwaysScrapeClassicHistograms: true, ConvertClassicHistToNHCB: false})},
		},
		{
			name:           "A global config that disables always scrape classic histograms",
			configFile:     "testdata/global_disable_always_scrape_classic_hist.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), AlwaysScrapeClassicHistograms: false, ConvertClassicHistToNHCB: false})},
		},
		{
			name:           "A global config that disables always scrape classic histograms and scrape config that enables it",
			configFile:     "testdata/local_enable_always_scrape_classic_hist.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), AlwaysScrapeClassicHistograms: true, ConvertClassicHistToNHCB: false})},
		},
		{
			name:           "A global config that enables always scrape classic histograms and scrape config that disables it",
			configFile:     "testdata/local_disable_always_scrape_classic_hist.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), AlwaysScrapeClassicHistograms: false, ConvertClassicHistToNHCB: false})},
		},
		{
			name:           "A global config that enables scrape native histograms",
			configFile:     "testdata/global_enable_scrape_native_hist.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), ScrapeNativeHistograms: true, ScrapeProtocols: DefaultProtoFirstScrapeProtocols})},
		},
		{
			name:           "A global config that enables scrape native histograms and sets scrape protocols explicitly",
			configFile:     "testdata/global_enable_scrape_native_hist_and_scrape_protocols.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), ScrapeNativeHistograms: true, ScrapeProtocols: []ScrapeProtocol{PrometheusText0_0_4}})},
		},
		{
			name:           "A local config that enables scrape native histograms",
			configFile:     "testdata/local_enable_scrape_native_hist.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), ScrapeNativeHistograms: true, ScrapeProtocols: DefaultProtoFirstScrapeProtocols})},
		},
		{
			name:           "A local config that enables scrape native histograms and sets scrape protocols explicitly",
			configFile:     "testdata/local_enable_scrape_native_hist_and_scrape_protocols.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), ScrapeNativeHistograms: true, ScrapeProtocols: []ScrapeProtocol{PrometheusText0_0_4}})},
		},
		{
			name:           "A global config that enables scrape native histograms and scrape config that disables it",
			configFile:     "testdata/local_disable_scrape_native_hist.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), ScrapeNativeHistograms: false, ScrapeProtocols: DefaultScrapeProtocols})},
		},
		{
			name:           "A global config that enables scrape native histograms and scrape protocols and scrape config that disables scrape native histograms but does not change scrape protocols",
			configFile:     "testdata/global_scrape_protocols_and_local_disable_scrape_native_hist.good.yml",
			expectedResult: []*ScrapeConfig{sc(ScrapeConfigOptions{JobName: "prometheus", ScrapeInterval: model.Duration(60 * time.Second), ScrapeTimeout: model.Duration(10 * time.Second), ScrapeNativeHistograms: false, ScrapeProtocols: []ScrapeProtocol{PrometheusText0_0_4}})},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := LoadFile(tc.configFile, false, promslog.NewNopLogger())
			require.NoError(t, err)

			scfgs, err := c.GetScrapeConfigs()
			if len(tc.expectedError) > 0 {
				require.ErrorContains(t, err, tc.expectedError)
			}
			require.Equal(t, tc.expectedResult, scfgs)
		})
	}
}

func kubernetesSDHostURL() config.URL {
	tURL, _ := url.Parse("https://localhost:1234")
	return config.URL{URL: tURL}
}

func TestScrapeConfigDisableCompression(t *testing.T) {
	want, err := LoadFile("testdata/scrape_config_disable_compression.good.yml", false, promslog.NewNopLogger())
	require.NoError(t, err)

	out, err := yaml.Marshal(want)

	require.NoError(t, err)
	got := &Config{}
	require.NoError(t, yaml.UnmarshalStrict(out, got))

	require.False(t, got.ScrapeConfigs[0].EnableCompression)
}

func TestScrapeConfigNameValidationSettings(t *testing.T) {
	tests := []struct {
		name           string
		inputFile      string
		expectScheme   model.ValidationScheme
		expectEscaping model.EscapingScheme
	}{
		{
			name:           "blank config implies default",
			inputFile:      "scrape_config_default_validation_mode",
			expectScheme:   model.UTF8Validation,
			expectEscaping: model.NoEscaping,
		},
		{
			name:           "global setting implies local settings",
			inputFile:      "scrape_config_global_validation_mode",
			expectScheme:   model.LegacyValidation,
			expectEscaping: model.DotsEscaping,
		},
		{
			name:           "local setting",
			inputFile:      "scrape_config_local_validation_mode",
			expectScheme:   model.LegacyValidation,
			expectEscaping: model.ValueEncodingEscaping,
		},
		{
			name:           "local setting overrides global setting",
			inputFile:      "scrape_config_local_global_validation_mode",
			expectScheme:   model.UTF8Validation,
			expectEscaping: model.DotsEscaping,
		},
		{
			name:           "local validation implies underscores escaping",
			inputFile:      "scrape_config_local_infer_escaping",
			expectScheme:   model.LegacyValidation,
			expectEscaping: model.UnderscoreEscaping,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want, err := LoadFile(fmt.Sprintf("testdata/%s.yml", tc.inputFile), false, promslog.NewNopLogger())
			require.NoError(t, err)

			out, err := yaml.Marshal(want)

			require.NoError(t, err)
			got := &Config{}
			require.NoError(t, yaml.UnmarshalStrict(out, got))

			require.Equal(t, tc.expectScheme, got.ScrapeConfigs[0].MetricNameValidationScheme)

			escaping, err := model.ToEscapingScheme(got.ScrapeConfigs[0].MetricNameEscapingScheme)
			require.NoError(t, err)
			require.Equal(t, tc.expectEscaping, escaping)
		})
	}
}

func TestScrapeConfigNameEscapingSettings(t *testing.T) {
	tests := []struct {
		name                   string
		inputFile              string
		expectValidationScheme model.ValidationScheme
		expectEscapingScheme   string
	}{
		{
			name:                   "blank config implies default",
			inputFile:              "scrape_config_default_validation_mode",
			expectValidationScheme: model.UTF8Validation,
			expectEscapingScheme:   "allow-utf-8",
		},
		{
			name:                   "global setting implies local settings",
			inputFile:              "scrape_config_global_validation_mode",
			expectValidationScheme: model.LegacyValidation,
			expectEscapingScheme:   "dots",
		},
		{
			name:                   "local setting",
			inputFile:              "scrape_config_local_validation_mode",
			expectValidationScheme: model.LegacyValidation,
			expectEscapingScheme:   "values",
		},
		{
			name:                   "local setting overrides global setting",
			inputFile:              "scrape_config_local_global_validation_mode",
			expectValidationScheme: model.UTF8Validation,
			expectEscapingScheme:   "dots",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want, err := LoadFile(fmt.Sprintf("testdata/%s.yml", tc.inputFile), false, promslog.NewNopLogger())
			require.NoError(t, err)

			out, err := yaml.Marshal(want)

			require.NoError(t, err)
			got := &Config{}
			require.NoError(t, yaml.UnmarshalStrict(out, got))

			require.Equal(t, tc.expectValidationScheme, got.ScrapeConfigs[0].MetricNameValidationScheme)
			require.Equal(t, tc.expectEscapingScheme, got.ScrapeConfigs[0].MetricNameEscapingScheme)
		})
	}
}

func TestScrapeProtocolHeader(t *testing.T) {
	tests := []struct {
		name          string
		proto         ScrapeProtocol
		expectedValue string
	}{
		{
			name:          "blank",
			proto:         ScrapeProtocol(""),
			expectedValue: "",
		},
		{
			name:          "invalid",
			proto:         ScrapeProtocol("invalid"),
			expectedValue: "",
		},
		{
			name:          "prometheus protobuf",
			proto:         PrometheusProto,
			expectedValue: "application/vnd.google.protobuf",
		},
		{
			name:          "prometheus text 0.0.4",
			proto:         PrometheusText0_0_4,
			expectedValue: "text/plain",
		},
		{
			name:          "prometheus text 1.0.0",
			proto:         PrometheusText1_0_0,
			expectedValue: "text/plain",
		},
		{
			name:          "openmetrics 0.0.1",
			proto:         OpenMetricsText0_0_1,
			expectedValue: "application/openmetrics-text",
		},
		{
			name:          "openmetrics 1.0.0",
			proto:         OpenMetricsText1_0_0,
			expectedValue: "application/openmetrics-text",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mediaType := tc.proto.HeaderMediaType()

			require.Equal(t, tc.expectedValue, mediaType)
		})
	}
}

// Regression test against https://github.com/prometheus/prometheus/issues/15538
func TestGetScrapeConfigs_Loaded(t *testing.T) {
	t.Run("without load", func(t *testing.T) {
		c := &Config{}
		_, err := c.GetScrapeConfigs()
		require.EqualError(t, err, "scrape config cannot be fetched, main config was not validated and loaded correctly; should not happen")
	})
	t.Run("with load", func(t *testing.T) {
		c, err := Load("", promslog.NewNopLogger())
		require.NoError(t, err)
		_, err = c.GetScrapeConfigs()
		require.NoError(t, err)
	})
}
