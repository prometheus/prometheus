// Copyright OpCore

package config_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/alecthomas/units"
	"github.com/go-kit/log"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	op_config "github.com/prometheus/prometheus/op-pkg/config"
)

type OpConfigSuite struct {
	suite.Suite

	cfg *config.OpRemoteWriteConfig
}

func TestOpRemoteWriteConfig(t *testing.T) {
	suite.Run(t, new(OpConfigSuite))
}

func (s *OpConfigSuite) SetupTest() {
	s.cfg = new(config.OpRemoteWriteConfig)
}

func (s *OpConfigSuite) TestRemoteWriteConfig() {
	raw := `url: http://remote1/push
remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop
name: drop_expensive
tls_config:
  server_name: server_name
bearer_token: auth_token`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().NoError(err)

	s.Require().NotEmpty(s.cfg.Destinations)
	s.Require().Equal(config.PrometheusProtocol, s.cfg.Protocol)
	s.Require().Equal("http://remote1/push", s.cfg.URL.String())
	s.Require().Equal("server_name", s.cfg.Destinations[0].HTTPClientConfig.TLSConfig.ServerName)
	s.Require().Equal("http://remote1/push", s.cfg.Destinations[0].URL.String())
	s.Require().Equal("drop_expensive", s.cfg.Destinations[0].Name)
	s.Require().Equal("auth_token", string(s.cfg.HTTPClientConfig.Authorization.Credentials))
}

func (s *OpConfigSuite) TestRemoteWriteConfigURLError() {
	raw := `remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop
name: drop_expensive
oauth2:
  client_id: "123"
  client_secret: "456"
  token_url: "http://remote1/auth"
  tls_config:
    cert_file: valid_cert_file
    key_file: valid_key_file`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().Error(err)
}

func (s *OpConfigSuite) TestOpDestinationConfigURLError() {
	raw := `destinations:
- name: dname
remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().Error(err)
}

func (s *OpConfigSuite) TestOpDestinationConfigTLSError() {
	raw := `destinations:
- name: dname
  url: https://host.com
remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().Error(err)
}

func (s *OpConfigSuite) TestOpDestinationConfigError() {
	raw := `destinations:
- name: dname
  url: https://host.com
  tls_config:
    server_name: server_name
remote_timeout: 30s
write_relabel_configs:
- source_labels: [__name__]
  regex: expensive.*
  action: drop`

	err := yaml.Unmarshal([]byte(raw), s.cfg)
	s.Require().NoError(err)
	s.Require().NotEmpty(s.cfg.Destinations)
	s.Require().Equal("server_name", s.cfg.Destinations[0].HTTPClientConfig.TLSConfig.ServerName)
}

func (s *OpConfigSuite) TestLoadConfig() {
	// Parse a valid file that sets a global scrape timeout. This tests whether parsing
	// an overwritten default field in the global config permanently changes the default.
	_, err := config.LoadFile("testdata/global_timeout.good.yml", false, false, log.NewNopLogger())
	s.Require().NoError(err)

	c, err := config.LoadFile("testdata/op.conf.good.yml", false, false, log.NewNopLogger())
	s.Require().NoError(err)
	s.Require().Equal(expectedConf, c)
}

func (s *OpConfigSuite) TestGetReceiverConfig() {
	// Parse a valid file that sets a global scrape timeout. This tests whether parsing
	// an overwritten default field in the global config permanently changes the default.
	_, err := config.LoadFile("testdata/global_timeout.good.yml", false, false, log.NewNopLogger())
	s.Require().NoError(err)

	c, err := config.LoadFile("testdata/op.conf.good.yml", false, false, log.NewNopLogger())
	s.Require().NoError(err)
	s.Require().Equal(expectedConf, c)

	rrCfg, err := c.GetReceiverConfig()
	s.Require().NoError(err)
	s.Require().Len(rrCfg.Configs, 2)
	s.Require().Len(rrCfg.Configs, 2)
	s.Require().Equal("scrape_service-x", rrCfg.Configs[1].Name)
	s.Require().Len(rrCfg.Configs[1].RelabelConfigs, 1)
}

func mustParseURL(u string) *common_config.URL {
	parsed, err := url.Parse(u)
	if err != nil {
		panic(err)
	}
	return &common_config.URL{URL: parsed}
}

var expectedConf = &config.Config{
	GlobalConfig: config.GlobalConfig{
		ScrapeInterval:     model.Duration(15 * time.Second),
		ScrapeTimeout:      config.DefaultGlobalConfig.ScrapeTimeout,
		EvaluationInterval: model.Duration(30 * time.Second),
		QueryLogFile:       "",

		ExternalLabels: labels.FromStrings("foo", "bar", "monitor", "codelab"),

		BodySizeLimit:         15 * units.MiB,
		SampleLimit:           1500,
		TargetLimit:           30,
		LabelLimit:            30,
		LabelNameLengthLimit:  200,
		LabelValueLengthLimit: 200,
		ScrapeProtocols:       config.DefaultGlobalConfig.ScrapeProtocols,
	},

	ScrapeConfigs: []*config.ScrapeConfig{
		{
			JobName:         "service-x",
			HonorTimestamps: true,
			ScrapeInterval:  model.Duration(15 * time.Second),
			ScrapeTimeout:   model.Duration(10 * time.Second),
			ScrapeProtocols: []config.ScrapeProtocol{
				"OpenMetricsText1.0.0",
				"OpenMetricsText0.0.1",
				"PrometheusText0.0.4",
			},
			MetricsPath:           "/metrics",
			Scheme:                "http",
			EnableCompression:     true,
			BodySizeLimit:         15728640,
			SampleLimit:           1500,
			TargetLimit:           30,
			LabelLimit:            30,
			LabelNameLengthLimit:  200,
			LabelValueLengthLimit: 200,
			HTTPClientConfig: common_config.HTTPClientConfig{
				FollowRedirects: true,
				EnableHTTP2:     true,
			},
			MetricRelabelConfigs: []*relabel.Config{
				{
					SourceLabels: model.LabelNames{"job"},
					Regex:        relabel.MustNewRegexp("(.*)some-[regex]"),
					Action:       "drop",
					Separator:    ";",
					Replacement:  "$1",
				},
			},
		},
	},

	RemoteWriteConfigs: []*config.OpRemoteWriteConfig{
		{
			Protocol: config.PrometheusProtocol,
			Destinations: []*config.OpDestinationConfig{
				{
					Name: "dname",
					URL:  mustParseURL("https://host.com"),
					HTTPClientConfig: common_config.HTTPClientConfig{
						TLSConfig: common_config.TLSConfig{
							ServerName: "server_name",
						},
					},
				},
			},
			RemoteWriteConfig: config.RemoteWriteConfig{
				RemoteTimeout:  model.Duration(30 * time.Second),
				QueueConfig:    config.DefaultQueueConfig,
				MetadataConfig: config.DefaultMetadataConfig,
				HTTPClientConfig: common_config.HTTPClientConfig{
					FollowRedirects: true,
					EnableHTTP2:     true,
				},
			},
		},
	},

	ReceiverConfig: op_config.RemoteWriteReceiverConfig{
		NumberOfShards: 2,
		Configs: []*relabeler.InputRelabelerConfig{
			{
				Name: "some_remote_write_receiver_1",
				RelabelConfigs: []*cppbridge.RelabelConfig{
					{
						SourceLabels: []string{"__name__"},
						Separator:    ";",
						Regex:        ".*",
						Replacement:  "$1",
						Action:       cppbridge.Keep,
					},
				},
			},
		},
	},
}
