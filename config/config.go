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
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	yaml "gopkg.in/yaml.v2"

	sd_config "github.com/prometheus/prometheus/discovery/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/relabel"
)

var (
	patRulePath = regexp.MustCompile(`^[^*]*(\*[^/]*)?$`)
)

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{}
	// If the entire config body is empty the UnmarshalYAML method is
	// never called. We thus have to set the DefaultConfig at the entry
	// point as well.
	*cfg = DefaultConfig

	err := yaml.UnmarshalStrict([]byte(s), cfg)
	if err != nil {
		return nil, err
	}
	cfg.original = s
	return cfg, nil
}

// LoadFile parses the given YAML file into a Config.
func LoadFile(filename string) (*Config, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(string(content))
	if err != nil {
		return nil, errors.Wrapf(err, "parsing YAML file %s", filename)
	}
	resolveFilepaths(filepath.Dir(filename), cfg)
	return cfg, nil
}

// The defaults applied before parsing the respective config sections.
var (
	// DefaultConfig is the default top-level configuration.
	DefaultConfig = Config{
		GlobalConfig: DefaultGlobalConfig,
	}

	// DefaultGlobalConfig is the default global configuration.
	DefaultGlobalConfig = GlobalConfig{
		ScrapeInterval:     model.Duration(1 * time.Minute),
		ScrapeTimeout:      model.Duration(10 * time.Second),
		EvaluationInterval: model.Duration(1 * time.Minute),
	}

	// DefaultScrapeConfig is the default scrape configuration.
	DefaultScrapeConfig = ScrapeConfig{
		// ScrapeTimeout and ScrapeInterval default to the
		// configured globals.
		MetricsPath:     "/metrics",
		Scheme:          "http",
		HonorLabels:     false,
		HonorTimestamps: true,
	}

	// DefaultAlertmanagerConfig is the default alertmanager configuration.
	DefaultAlertmanagerConfig = AlertmanagerConfig{
		Scheme:     "http",
		Timeout:    model.Duration(10 * time.Second),
		APIVersion: AlertmanagerAPIVersionV1,
	}

	// DefaultRemoteWriteConfig is the default remote write configuration.
	DefaultRemoteWriteConfig = RemoteWriteConfig{
		RemoteTimeout: model.Duration(30 * time.Second),
		QueueConfig:   DefaultQueueConfig,
	}

	// DefaultQueueConfig is the default remote queue configuration.
	DefaultQueueConfig = QueueConfig{
		// With a maximum of 1000 shards, assuming an average of 100ms remote write
		// time and 100 samples per batch, we will be able to push 1M samples/s.
		MaxShards:         1000,
		MinShards:         1,
		MaxSamplesPerSend: 100,

		// Each shard will have a max of 500 samples pending in it's channel, plus the pending
		// samples that have been enqueued. Theoretically we should only ever have about 600 samples
		// per shard pending. At 1000 shards that's 600k.
		Capacity:          500,
		BatchSendDeadline: model.Duration(5 * time.Second),

		// Backoff times for retrying a batch of samples on recoverable errors.
		MinBackoff: model.Duration(30 * time.Millisecond),
		MaxBackoff: model.Duration(100 * time.Millisecond),
	}

	// DefaultRemoteReadConfig is the default remote read configuration.
	DefaultRemoteReadConfig = RemoteReadConfig{
		RemoteTimeout: model.Duration(1 * time.Minute),
	}
)

// Config is the top-level configuration for Prometheus's config files.
type Config struct {
	GlobalConfig   GlobalConfig    `yaml:"global"`
	AlertingConfig AlertingConfig  `yaml:"alerting,omitempty"`
	RuleFiles      []string        `yaml:"rule_files,omitempty"`
	ScrapeConfigs  []*ScrapeConfig `yaml:"scrape_configs,omitempty"`

	RemoteWriteConfigs []*RemoteWriteConfig `yaml:"remote_write,omitempty"`
	RemoteReadConfigs  []*RemoteReadConfig  `yaml:"remote_read,omitempty"`

	// original is the input from which the config was parsed.
	original string
}

// resolveFilepaths joins all relative paths in a configuration
// with a given base directory.
func resolveFilepaths(baseDir string, cfg *Config) {
	join := func(fp string) string {
		if len(fp) > 0 && !filepath.IsAbs(fp) {
			fp = filepath.Join(baseDir, fp)
		}
		return fp
	}

	for i, rf := range cfg.RuleFiles {
		cfg.RuleFiles[i] = join(rf)
	}

	tlsPaths := func(cfg *config_util.TLSConfig) {
		cfg.CAFile = join(cfg.CAFile)
		cfg.CertFile = join(cfg.CertFile)
		cfg.KeyFile = join(cfg.KeyFile)
	}
	clientPaths := func(scfg *config_util.HTTPClientConfig) {
		if scfg.BasicAuth != nil {
			scfg.BasicAuth.PasswordFile = join(scfg.BasicAuth.PasswordFile)
		}
		scfg.BearerTokenFile = join(scfg.BearerTokenFile)
		tlsPaths(&scfg.TLSConfig)
	}
	sdPaths := func(cfg *sd_config.ServiceDiscoveryConfig) {
		for _, kcfg := range cfg.KubernetesSDConfigs {
			clientPaths(&kcfg.HTTPClientConfig)
		}
		for _, mcfg := range cfg.MarathonSDConfigs {
			mcfg.AuthTokenFile = join(mcfg.AuthTokenFile)
			clientPaths(&mcfg.HTTPClientConfig)
		}
		for _, consulcfg := range cfg.ConsulSDConfigs {
			tlsPaths(&consulcfg.TLSConfig)
		}
		for _, cfg := range cfg.OpenstackSDConfigs {
			tlsPaths(&cfg.TLSConfig)
		}
		for _, cfg := range cfg.TritonSDConfigs {
			tlsPaths(&cfg.TLSConfig)
		}
		for _, filecfg := range cfg.FileSDConfigs {
			for i, fn := range filecfg.Files {
				filecfg.Files[i] = join(fn)
			}
		}
	}

	for _, cfg := range cfg.ScrapeConfigs {
		clientPaths(&cfg.HTTPClientConfig)
		sdPaths(&cfg.ServiceDiscoveryConfig)
	}
	for _, cfg := range cfg.AlertingConfig.AlertmanagerConfigs {
		clientPaths(&cfg.HTTPClientConfig)
		sdPaths(&cfg.ServiceDiscoveryConfig)
	}
	for _, cfg := range cfg.RemoteReadConfigs {
		clientPaths(&cfg.HTTPClientConfig)
	}
	for _, cfg := range cfg.RemoteWriteConfigs {
		clientPaths(&cfg.HTTPClientConfig)
	}
}

func (c Config) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultConfig
	// We want to set c to the defaults and then overwrite it with the input.
	// To make unmarshal fill the plain data struct rather than calling UnmarshalYAML
	// again, we have to hide it using a type indirection.
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// If a global block was open but empty the default global config is overwritten.
	// We have to restore it here.
	if c.GlobalConfig.isZero() {
		c.GlobalConfig = DefaultGlobalConfig
	}

	for _, rf := range c.RuleFiles {
		if !patRulePath.MatchString(rf) {
			return errors.Errorf("invalid rule file path %q", rf)
		}
	}
	// Do global overrides and validate unique names.
	jobNames := map[string]struct{}{}
	for _, scfg := range c.ScrapeConfigs {
		if scfg == nil {
			return errors.New("empty or null scrape config section")
		}
		// First set the correct scrape interval, then check that the timeout
		// (inferred or explicit) is not greater than that.
		if scfg.ScrapeInterval == 0 {
			scfg.ScrapeInterval = c.GlobalConfig.ScrapeInterval
		}
		if scfg.ScrapeTimeout > scfg.ScrapeInterval {
			return errors.Errorf("scrape timeout greater than scrape interval for scrape config with job name %q", scfg.JobName)
		}
		if scfg.ScrapeTimeout == 0 {
			if c.GlobalConfig.ScrapeTimeout > scfg.ScrapeInterval {
				scfg.ScrapeTimeout = scfg.ScrapeInterval
			} else {
				scfg.ScrapeTimeout = c.GlobalConfig.ScrapeTimeout
			}
		}

		if _, ok := jobNames[scfg.JobName]; ok {
			return errors.Errorf("found multiple scrape configs with job name %q", scfg.JobName)
		}
		jobNames[scfg.JobName] = struct{}{}
	}
	for _, rwcfg := range c.RemoteWriteConfigs {
		if rwcfg == nil {
			return errors.New("empty or null remote write config section")
		}
	}
	for _, rrcfg := range c.RemoteReadConfigs {
		if rrcfg == nil {
			return errors.New("empty or null remote read config section")
		}
	}
	return nil
}

// GlobalConfig configures values that are used across other configuration
// objects.
type GlobalConfig struct {
	// How frequently to scrape targets by default.
	ScrapeInterval model.Duration `yaml:"scrape_interval,omitempty"`
	// The default timeout when scraping targets.
	ScrapeTimeout model.Duration `yaml:"scrape_timeout,omitempty"`
	// How frequently to evaluate rules by default.
	EvaluationInterval model.Duration `yaml:"evaluation_interval,omitempty"`
	// The labels to add to any timeseries that this Prometheus instance scrapes.
	ExternalLabels labels.Labels `yaml:"external_labels,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *GlobalConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Create a clean global config as the previous one was already populated
	// by the default due to the YAML parser behavior for empty blocks.
	gc := &GlobalConfig{}
	type plain GlobalConfig
	if err := unmarshal((*plain)(gc)); err != nil {
		return err
	}

	for _, l := range gc.ExternalLabels {
		if !model.LabelName(l.Name).IsValid() {
			return errors.Errorf("%q is not a valid label name", l.Name)
		}
		if !model.LabelValue(l.Value).IsValid() {
			return errors.Errorf("%q is not a valid label value", l.Value)
		}
	}

	// First set the correct scrape interval, then check that the timeout
	// (inferred or explicit) is not greater than that.
	if gc.ScrapeInterval == 0 {
		gc.ScrapeInterval = DefaultGlobalConfig.ScrapeInterval
	}
	if gc.ScrapeTimeout > gc.ScrapeInterval {
		return errors.New("global scrape timeout greater than scrape interval")
	}
	if gc.ScrapeTimeout == 0 {
		if DefaultGlobalConfig.ScrapeTimeout > gc.ScrapeInterval {
			gc.ScrapeTimeout = gc.ScrapeInterval
		} else {
			gc.ScrapeTimeout = DefaultGlobalConfig.ScrapeTimeout
		}
	}
	if gc.EvaluationInterval == 0 {
		gc.EvaluationInterval = DefaultGlobalConfig.EvaluationInterval
	}
	*c = *gc
	return nil
}

// isZero returns true iff the global config is the zero value.
func (c *GlobalConfig) isZero() bool {
	return c.ExternalLabels == nil &&
		c.ScrapeInterval == 0 &&
		c.ScrapeTimeout == 0 &&
		c.EvaluationInterval == 0
}

// ScrapeConfig configures a scraping unit for Prometheus.
type ScrapeConfig struct {
	// The job name to which the job label is set by default.
	JobName string `yaml:"job_name"`
	// Indicator whether the scraped metrics should remain unmodified.
	HonorLabels bool `yaml:"honor_labels,omitempty"`
	// Indicator whether the scraped timestamps should be respected.
	HonorTimestamps bool `yaml:"honor_timestamps"`
	// A set of query parameters with which the target is scraped.
	Params url.Values `yaml:"params,omitempty"`
	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval model.Duration `yaml:"scrape_interval,omitempty"`
	// The timeout for scraping targets of this config.
	ScrapeTimeout model.Duration `yaml:"scrape_timeout,omitempty"`
	// The HTTP resource path on which to fetch metrics from targets.
	MetricsPath string `yaml:"metrics_path,omitempty"`
	// The URL scheme with which to fetch metrics from targets.
	Scheme string `yaml:"scheme,omitempty"`
	// More than this many samples post metric-relabelling will cause the scrape to fail.
	SampleLimit uint `yaml:"sample_limit,omitempty"`

	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.

	ServiceDiscoveryConfig sd_config.ServiceDiscoveryConfig `yaml:",inline"`
	HTTPClientConfig       config_util.HTTPClientConfig     `yaml:",inline"`

	// List of target relabel configurations.
	RelabelConfigs []*relabel.Config `yaml:"relabel_configs,omitempty"`
	// List of metric relabel configurations.
	MetricRelabelConfigs []*relabel.Config `yaml:"metric_relabel_configs,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ScrapeConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultScrapeConfig
	type plain ScrapeConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if len(c.JobName) == 0 {
		return errors.New("job_name is empty")
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	// The UnmarshalYAML method of ServiceDiscoveryConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.ServiceDiscoveryConfig.Validate(); err != nil {
		return err
	}

	// Check for users putting URLs in target groups.
	if len(c.RelabelConfigs) == 0 {
		for _, tg := range c.ServiceDiscoveryConfig.StaticConfigs {
			for _, t := range tg.Targets {
				if err := CheckTargetAddress(t[model.AddressLabel]); err != nil {
					return err
				}
			}
		}
	}

	for _, rlcfg := range c.RelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null target relabeling rule in scrape config")
		}
	}
	for _, rlcfg := range c.MetricRelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null metric relabeling rule in scrape config")
		}
	}

	// Add index to the static config target groups for unique identification
	// within scrape pool.
	for i, tg := range c.ServiceDiscoveryConfig.StaticConfigs {
		tg.Source = fmt.Sprintf("%d", i)
	}

	return nil
}

// AlertingConfig configures alerting and alertmanager related configs.
type AlertingConfig struct {
	AlertRelabelConfigs []*relabel.Config     `yaml:"alert_relabel_configs,omitempty"`
	AlertmanagerConfigs []*AlertmanagerConfig `yaml:"alertmanagers,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *AlertingConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Create a clean global config as the previous one was already populated
	// by the default due to the YAML parser behavior for empty blocks.
	*c = AlertingConfig{}
	type plain AlertingConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	for _, rlcfg := range c.AlertRelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null alert relabeling rule")
		}
	}
	return nil
}

// AlertmanagerAPIVersion represents a version of the
// github.com/prometheus/alertmanager/api, e.g. 'v1' or 'v2'.
type AlertmanagerAPIVersion string

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (v *AlertmanagerAPIVersion) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*v = AlertmanagerAPIVersion("")
	type plain AlertmanagerAPIVersion
	if err := unmarshal((*plain)(v)); err != nil {
		return err
	}

	for _, supportedVersion := range SupportedAlertmanagerAPIVersions {
		if *v == supportedVersion {
			return nil
		}
	}

	return fmt.Errorf("expected Alertmanager api version to be one of %v but got %v", SupportedAlertmanagerAPIVersions, *v)
}

const (
	// AlertmanagerAPIVersionV1 represents
	// github.com/prometheus/alertmanager/api/v1.
	AlertmanagerAPIVersionV1 AlertmanagerAPIVersion = "v1"
	// AlertmanagerAPIVersionV2 represents
	// github.com/prometheus/alertmanager/api/v2.
	AlertmanagerAPIVersionV2 AlertmanagerAPIVersion = "v2"
)

var SupportedAlertmanagerAPIVersions = []AlertmanagerAPIVersion{
	AlertmanagerAPIVersionV1, AlertmanagerAPIVersionV2,
}

// AlertmanagerConfig configures how Alertmanagers can be discovered and communicated with.
type AlertmanagerConfig struct {
	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.

	ServiceDiscoveryConfig sd_config.ServiceDiscoveryConfig `yaml:",inline"`
	HTTPClientConfig       config_util.HTTPClientConfig     `yaml:",inline"`

	// The URL scheme to use when talking to Alertmanagers.
	Scheme string `yaml:"scheme,omitempty"`
	// Path prefix to add in front of the push endpoint path.
	PathPrefix string `yaml:"path_prefix,omitempty"`
	// The timeout used when sending alerts.
	Timeout model.Duration `yaml:"timeout,omitempty"`

	// The api version of Alertmanager.
	APIVersion AlertmanagerAPIVersion `yaml:"api_version"`

	// List of Alertmanager relabel configurations.
	RelabelConfigs []*relabel.Config `yaml:"relabel_configs,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *AlertmanagerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultAlertmanagerConfig
	type plain AlertmanagerConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	// The UnmarshalYAML method of ServiceDiscoveryConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.ServiceDiscoveryConfig.Validate(); err != nil {
		return err
	}

	// Check for users putting URLs in target groups.
	if len(c.RelabelConfigs) == 0 {
		for _, tg := range c.ServiceDiscoveryConfig.StaticConfigs {
			for _, t := range tg.Targets {
				if err := CheckTargetAddress(t[model.AddressLabel]); err != nil {
					return err
				}
			}
		}
	}

	for _, rlcfg := range c.RelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null Alertmanager target relabeling rule")
		}
	}

	// Add index to the static config target groups for unique identification
	// within scrape pool.
	for i, tg := range c.ServiceDiscoveryConfig.StaticConfigs {
		tg.Source = fmt.Sprintf("%d", i)
	}

	return nil
}

// CheckTargetAddress checks if target address is valid.
func CheckTargetAddress(address model.LabelValue) error {
	// For now check for a URL, we may want to expand this later.
	if strings.Contains(string(address), "/") {
		return errors.Errorf("%q is not a valid hostname", address)
	}
	return nil
}

// ClientCert contains client cert credentials.
type ClientCert struct {
	Cert string             `yaml:"cert"`
	Key  config_util.Secret `yaml:"key"`
}

// FileSDConfig is the configuration for file based discovery.
type FileSDConfig struct {
	Files           []string       `yaml:"files"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
}

// RemoteWriteConfig is the configuration for writing to remote storage.
type RemoteWriteConfig struct {
	URL                 *config_util.URL  `yaml:"url"`
	RemoteTimeout       model.Duration    `yaml:"remote_timeout,omitempty"`
	WriteRelabelConfigs []*relabel.Config `yaml:"write_relabel_configs,omitempty"`

	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.
	HTTPClientConfig config_util.HTTPClientConfig `yaml:",inline"`
	QueueConfig      QueueConfig                  `yaml:"queue_config,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RemoteWriteConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultRemoteWriteConfig
	type plain RemoteWriteConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == nil {
		return errors.New("url for remote_write is empty")
	}
	for _, rlcfg := range c.WriteRelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null relabeling rule in remote write config")
		}
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	return c.HTTPClientConfig.Validate()
}

// QueueConfig is the configuration for the queue used to write to remote
// storage.
type QueueConfig struct {
	// Number of samples to buffer per shard before we block. Defaults to
	// MaxSamplesPerSend.
	Capacity int `yaml:"capacity,omitempty"`

	// Max number of shards, i.e. amount of concurrency.
	MaxShards int `yaml:"max_shards,omitempty"`

	// Min number of shards, i.e. amount of concurrency.
	MinShards int `yaml:"min_shards,omitempty"`

	// Maximum number of samples per send.
	MaxSamplesPerSend int `yaml:"max_samples_per_send,omitempty"`

	// Maximum time sample will wait in buffer.
	BatchSendDeadline model.Duration `yaml:"batch_send_deadline,omitempty"`

	// On recoverable errors, backoff exponentially.
	MinBackoff model.Duration `yaml:"min_backoff,omitempty"`
	MaxBackoff model.Duration `yaml:"max_backoff,omitempty"`
}

// RemoteReadConfig is the configuration for reading from remote storage.
type RemoteReadConfig struct {
	URL           *config_util.URL `yaml:"url"`
	RemoteTimeout model.Duration   `yaml:"remote_timeout,omitempty"`
	ReadRecent    bool             `yaml:"read_recent,omitempty"`
	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.
	HTTPClientConfig config_util.HTTPClientConfig `yaml:",inline"`

	// RequiredMatchers is an optional list of equality matchers which have to
	// be present in a selector to query the remote read endpoint.
	RequiredMatchers model.LabelSet `yaml:"required_matchers,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RemoteReadConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultRemoteReadConfig
	type plain RemoteReadConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == nil {
		return errors.New("url for remote_read is empty")
	}
	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	return c.HTTPClientConfig.Validate()
}
