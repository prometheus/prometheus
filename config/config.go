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

	"github.com/prometheus/prometheus/discovery/azure"
	"github.com/prometheus/prometheus/discovery/consul"
	"github.com/prometheus/prometheus/discovery/dns"
	"github.com/prometheus/prometheus/discovery/ec2"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/gce"
	"github.com/prometheus/prometheus/discovery/marathon"
	"github.com/prometheus/prometheus/discovery/triton"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/targetgroup"
	configUtil "github.com/prometheus/prometheus/util/config"
	yamlUtil "github.com/prometheus/prometheus/util/yaml"
	yaml "gopkg.in/yaml.v2"
)

var (
	patRulePath   = regexp.MustCompile(`^[^*]*(\*[^/]*)?$`)
	relabelTarget = regexp.MustCompile(`^(?:(?:[a-zA-Z_]|\$(?:\{\w+\}|\w+))+\w*)+$`)
)

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{}
	// If the entire config body is empty the UnmarshalYAML method is
	// never called. We thus have to set the DefaultConfig at the entry
	// point as well.
	*cfg = DefaultConfig

	err := yaml.Unmarshal([]byte(s), cfg)
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
		return nil, fmt.Errorf("parsing YAML file %s: %v", filename, err)
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
		MetricsPath: "/metrics",
		Scheme:      "http",
		HonorLabels: false,
	}

	// DefaultAlertmanagerConfig is the default alertmanager configuration.
	DefaultAlertmanagerConfig = AlertmanagerConfig{
		Scheme:  "http",
		Timeout: 10 * time.Second,
	}

	// DefaultRelabelConfig is the default Relabel configuration.
	DefaultRelabelConfig = RelabelConfig{
		Action:      RelabelReplace,
		Separator:   ";",
		Regex:       MustNewRegexp("(.*)"),
		Replacement: "$1",
	}

	// DefaultServersetSDConfig is the default Serverset SD configuration.
	DefaultServersetSDConfig = ServersetSDConfig{
		Timeout: model.Duration(10 * time.Second),
	}

	// DefaultNerveSDConfig is the default Nerve SD configuration.
	DefaultNerveSDConfig = NerveSDConfig{
		Timeout: model.Duration(10 * time.Second),
	}

	// DefaultKubernetesSDConfig is the default Kubernetes SD configuration
	DefaultKubernetesSDConfig = KubernetesSDConfig{}

	// DefaultOpenstackSDConfig is the default OpenStack SD configuration.
	DefaultOpenstackSDConfig = OpenstackSDConfig{
		Port:            80,
		RefreshInterval: model.Duration(60 * time.Second),
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
		MaxSamplesPerSend: 100,

		// By default, buffer 1000 batches, which at 100ms per batch is 1:40mins. At
		// 1000 shards, this will buffer 100M samples total.
		Capacity:          100 * 1000,
		BatchSendDeadline: 5 * time.Second,

		// Max number of times to retry a batch on recoverable errors.
		MaxRetries: 10,
		MinBackoff: 30 * time.Millisecond,
		MaxBackoff: 100 * time.Millisecond,
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

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`

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

	clientPaths := func(scfg *configUtil.HTTPClientConfig) {
		scfg.BearerTokenFile = join(scfg.BearerTokenFile)
		scfg.TLSConfig.CAFile = join(scfg.TLSConfig.CAFile)
		scfg.TLSConfig.CertFile = join(scfg.TLSConfig.CertFile)
		scfg.TLSConfig.KeyFile = join(scfg.TLSConfig.KeyFile)
	}
	sdPaths := func(cfg *ServiceDiscoveryConfig) {
		for _, kcfg := range cfg.KubernetesSDConfigs {
			kcfg.BearerTokenFile = join(kcfg.BearerTokenFile)
			kcfg.TLSConfig.CAFile = join(kcfg.TLSConfig.CAFile)
			kcfg.TLSConfig.CertFile = join(kcfg.TLSConfig.CertFile)
			kcfg.TLSConfig.KeyFile = join(kcfg.TLSConfig.KeyFile)
		}
		for _, mcfg := range cfg.MarathonSDConfigs {
			mcfg.BearerTokenFile = join(mcfg.BearerTokenFile)
			mcfg.TLSConfig.CAFile = join(mcfg.TLSConfig.CAFile)
			mcfg.TLSConfig.CertFile = join(mcfg.TLSConfig.CertFile)
			mcfg.TLSConfig.KeyFile = join(mcfg.TLSConfig.KeyFile)
		}
		for _, consulcfg := range cfg.ConsulSDConfigs {
			consulcfg.TLSConfig.CAFile = join(consulcfg.TLSConfig.CAFile)
			consulcfg.TLSConfig.CertFile = join(consulcfg.TLSConfig.CertFile)
			consulcfg.TLSConfig.KeyFile = join(consulcfg.TLSConfig.KeyFile)
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
	if err := yamlUtil.CheckOverflow(c.XXX, "config"); err != nil {
		return err
	}
	// If a global block was open but empty the default global config is overwritten.
	// We have to restore it here.
	if c.GlobalConfig.isZero() {
		c.GlobalConfig = DefaultGlobalConfig
	}

	for _, rf := range c.RuleFiles {
		if !patRulePath.MatchString(rf) {
			return fmt.Errorf("invalid rule file path %q", rf)
		}
	}
	// Do global overrides and validate unique names.
	jobNames := map[string]struct{}{}
	for _, scfg := range c.ScrapeConfigs {
		// First set the correct scrape interval, then check that the timeout
		// (inferred or explicit) is not greater than that.
		if scfg.ScrapeInterval == 0 {
			scfg.ScrapeInterval = c.GlobalConfig.ScrapeInterval
		}
		if scfg.ScrapeTimeout > scfg.ScrapeInterval {
			return fmt.Errorf("scrape timeout greater than scrape interval for scrape config with job name %q", scfg.JobName)
		}
		if scfg.ScrapeTimeout == 0 {
			if c.GlobalConfig.ScrapeTimeout > scfg.ScrapeInterval {
				scfg.ScrapeTimeout = scfg.ScrapeInterval
			} else {
				scfg.ScrapeTimeout = c.GlobalConfig.ScrapeTimeout
			}
		}

		if _, ok := jobNames[scfg.JobName]; ok {
			return fmt.Errorf("found multiple scrape configs with job name %q", scfg.JobName)
		}
		jobNames[scfg.JobName] = struct{}{}
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
	ExternalLabels model.LabelSet `yaml:"external_labels,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
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
	if err := yamlUtil.CheckOverflow(gc.XXX, "global config"); err != nil {
		return err
	}
	// First set the correct scrape interval, then check that the timeout
	// (inferred or explicit) is not greater than that.
	if gc.ScrapeInterval == 0 {
		gc.ScrapeInterval = DefaultGlobalConfig.ScrapeInterval
	}
	if gc.ScrapeTimeout > gc.ScrapeInterval {
		return fmt.Errorf("global scrape timeout greater than scrape interval")
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

// ServiceDiscoveryConfig configures lists of different service discovery mechanisms.
type ServiceDiscoveryConfig struct {
	// List of labeled target groups for this job.
	StaticConfigs []*targetgroup.Group `yaml:"static_configs,omitempty"`
	// List of DNS service discovery configurations.
	DNSSDConfigs []*dns.SDConfig `yaml:"dns_sd_configs,omitempty"`
	// List of file service discovery configurations.
	FileSDConfigs []*file.SDConfig `yaml:"file_sd_configs,omitempty"`
	// List of Consul service discovery configurations.
	ConsulSDConfigs []*consul.SDConfig `yaml:"consul_sd_configs,omitempty"`
	// List of Serverset service discovery configurations.
	ServersetSDConfigs []*ServersetSDConfig `yaml:"serverset_sd_configs,omitempty"`
	// NerveSDConfigs is a list of Nerve service discovery configurations.
	NerveSDConfigs []*NerveSDConfig `yaml:"nerve_sd_configs,omitempty"`
	// MarathonSDConfigs is a list of Marathon service discovery configurations.
	MarathonSDConfigs []*marathon.SDConfig `yaml:"marathon_sd_configs,omitempty"`
	// List of Kubernetes service discovery configurations.
	KubernetesSDConfigs []*KubernetesSDConfig `yaml:"kubernetes_sd_configs,omitempty"`
	// List of GCE service discovery configurations.
	GCESDConfigs []*gce.SDConfig `yaml:"gce_sd_configs,omitempty"`
	// List of EC2 service discovery configurations.
	EC2SDConfigs []*ec2.SDConfig `yaml:"ec2_sd_configs,omitempty"`
	// List of OpenStack service discovery configurations.
	OpenstackSDConfigs []*OpenstackSDConfig `yaml:"openstack_sd_configs,omitempty"`
	// List of Azure service discovery configurations.
	AzureSDConfigs []*azure.SDConfig `yaml:"azure_sd_configs,omitempty"`
	// List of Triton service discovery configurations.
	TritonSDConfigs []*triton.SDConfig `yaml:"triton_sd_configs,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ServiceDiscoveryConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain ServiceDiscoveryConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return yamlUtil.CheckOverflow(c.XXX, "service discovery config")
}

// ScrapeConfig configures a scraping unit for Prometheus.
type ScrapeConfig struct {
	// The job name to which the job label is set by default.
	JobName string `yaml:"job_name"`
	// Indicator whether the scraped metrics should remain unmodified.
	HonorLabels bool `yaml:"honor_labels,omitempty"`
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

	ServiceDiscoveryConfig ServiceDiscoveryConfig      `yaml:",inline"`
	HTTPClientConfig       configUtil.HTTPClientConfig `yaml:",inline"`

	// List of target relabel configurations.
	RelabelConfigs []*RelabelConfig `yaml:"relabel_configs,omitempty"`
	// List of metric relabel configurations.
	MetricRelabelConfigs []*RelabelConfig `yaml:"metric_relabel_configs,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ScrapeConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultScrapeConfig
	type plain ScrapeConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err = yamlUtil.CheckOverflow(c.XXX, "scrape_config"); err != nil {
		return err
	}
	if len(c.JobName) == 0 {
		return fmt.Errorf("job_name is empty")
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err = c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	// Check for users putting URLs in target groups.
	if len(c.RelabelConfigs) == 0 {
		for _, tg := range c.ServiceDiscoveryConfig.StaticConfigs {
			for _, t := range tg.Targets {
				if err = CheckTargetAddress(t[model.AddressLabel]); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// AlertingConfig configures alerting and alertmanager related configs.
type AlertingConfig struct {
	AlertRelabelConfigs []*RelabelConfig      `yaml:"alert_relabel_configs,omitempty"`
	AlertmanagerConfigs []*AlertmanagerConfig `yaml:"alertmanagers,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
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
	return yamlUtil.CheckOverflow(c.XXX, "alerting config")
}

// AlertmanagerConfig configures how Alertmanagers can be discovered and communicated with.
type AlertmanagerConfig struct {
	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.

	ServiceDiscoveryConfig ServiceDiscoveryConfig      `yaml:",inline"`
	HTTPClientConfig       configUtil.HTTPClientConfig `yaml:",inline"`

	// The URL scheme to use when talking to Alertmanagers.
	Scheme string `yaml:"scheme,omitempty"`
	// Path prefix to add in front of the push endpoint path.
	PathPrefix string `yaml:"path_prefix,omitempty"`
	// The timeout used when sending alerts.
	Timeout time.Duration `yaml:"timeout,omitempty"`

	// List of Alertmanager relabel configurations.
	RelabelConfigs []*RelabelConfig `yaml:"relabel_configs,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *AlertmanagerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultAlertmanagerConfig
	type plain AlertmanagerConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if err := yamlUtil.CheckOverflow(c.XXX, "alertmanager config"); err != nil {
		return err
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
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
	return nil
}

// CheckTargetAddress checks if target address is valid.
func CheckTargetAddress(address model.LabelValue) error {
	// For now check for a URL, we may want to expand this later.
	if strings.Contains(string(address), "/") {
		return fmt.Errorf("%q is not a valid hostname", address)
	}
	return nil
}

// ClientCert contains client cert credentials.
type ClientCert struct {
	Cert string            `yaml:"cert"`
	Key  configUtil.Secret `yaml:"key"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// FileSDConfig is the configuration for file based discovery.
type FileSDConfig struct {
	Files           []string       `yaml:"files"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// ServersetSDConfig is the configuration for Twitter serversets in Zookeeper based discovery.
type ServersetSDConfig struct {
	Servers []string       `yaml:"servers"`
	Paths   []string       `yaml:"paths"`
	Timeout model.Duration `yaml:"timeout,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ServersetSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultServersetSDConfig
	type plain ServersetSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := yamlUtil.CheckOverflow(c.XXX, "serverset_sd_config"); err != nil {
		return err
	}
	if len(c.Servers) == 0 {
		return fmt.Errorf("serverset SD config must contain at least one Zookeeper server")
	}
	if len(c.Paths) == 0 {
		return fmt.Errorf("serverset SD config must contain at least one path")
	}
	for _, path := range c.Paths {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("serverset SD config paths must begin with '/': %s", path)
		}
	}
	return nil
}

// NerveSDConfig is the configuration for AirBnB's Nerve in Zookeeper based discovery.
type NerveSDConfig struct {
	Servers []string       `yaml:"servers"`
	Paths   []string       `yaml:"paths"`
	Timeout model.Duration `yaml:"timeout,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *NerveSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultNerveSDConfig
	type plain NerveSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := yamlUtil.CheckOverflow(c.XXX, "nerve_sd_config"); err != nil {
		return err
	}
	if len(c.Servers) == 0 {
		return fmt.Errorf("nerve SD config must contain at least one Zookeeper server")
	}
	if len(c.Paths) == 0 {
		return fmt.Errorf("nerve SD config must contain at least one path")
	}
	for _, path := range c.Paths {
		if !strings.HasPrefix(path, "/") {
			return fmt.Errorf("nerve SD config paths must begin with '/': %s", path)
		}
	}
	return nil
}

// KubernetesRole is role of the service in Kubernetes.
type KubernetesRole string

// The valid options for KubernetesRole.
const (
	KubernetesRoleNode     KubernetesRole = "node"
	KubernetesRolePod      KubernetesRole = "pod"
	KubernetesRoleService  KubernetesRole = "service"
	KubernetesRoleEndpoint KubernetesRole = "endpoints"
	KubernetesRoleIngress  KubernetesRole = "ingress"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *KubernetesRole) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case KubernetesRoleNode, KubernetesRolePod, KubernetesRoleService, KubernetesRoleEndpoint, KubernetesRoleIngress:
		return nil
	default:
		return fmt.Errorf("Unknown Kubernetes SD role %q", *c)
	}
}

// KubernetesSDConfig is the configuration for Kubernetes service discovery.
type KubernetesSDConfig struct {
	APIServer          configUtil.URL               `yaml:"api_server"`
	Role               KubernetesRole               `yaml:"role"`
	BasicAuth          *configUtil.BasicAuth        `yaml:"basic_auth,omitempty"`
	BearerToken        configUtil.Secret            `yaml:"bearer_token,omitempty"`
	BearerTokenFile    string                       `yaml:"bearer_token_file,omitempty"`
	TLSConfig          configUtil.TLSConfig         `yaml:"tls_config,omitempty"`
	NamespaceDiscovery KubernetesNamespaceDiscovery `yaml:"namespaces"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *KubernetesSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = KubernetesSDConfig{}
	type plain KubernetesSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := yamlUtil.CheckOverflow(c.XXX, "kubernetes_sd_config"); err != nil {
		return err
	}
	if c.Role == "" {
		return fmt.Errorf("role missing (one of: pod, service, endpoints, node)")
	}
	if len(c.BearerToken) > 0 && len(c.BearerTokenFile) > 0 {
		return fmt.Errorf("at most one of bearer_token & bearer_token_file must be configured")
	}
	if c.BasicAuth != nil && (len(c.BearerToken) > 0 || len(c.BearerTokenFile) > 0) {
		return fmt.Errorf("at most one of basic_auth, bearer_token & bearer_token_file must be configured")
	}
	if c.APIServer.URL == nil &&
		(c.BasicAuth != nil || c.BearerToken != "" || c.BearerTokenFile != "" ||
			c.TLSConfig.CAFile != "" || c.TLSConfig.CertFile != "" || c.TLSConfig.KeyFile != "") {
		return fmt.Errorf("to use custom authentication please provide the 'api_server' URL explicitly")
	}
	return nil
}

// KubernetesNamespaceDiscovery is the configuration for discovering
// Kubernetes namespaces.
type KubernetesNamespaceDiscovery struct {
	Names []string `yaml:"names"`
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *KubernetesNamespaceDiscovery) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = KubernetesNamespaceDiscovery{}
	type plain KubernetesNamespaceDiscovery
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return yamlUtil.CheckOverflow(c.XXX, "namespaces")
}

// OpenstackSDConfig is the configuration for OpenStack based service discovery.
type OpenstackSDConfig struct {
	IdentityEndpoint string            `yaml:"identity_endpoint"`
	Username         string            `yaml:"username"`
	UserID           string            `yaml:"userid"`
	Password         configUtil.Secret `yaml:"password"`
	ProjectName      string            `yaml:"project_name"`
	ProjectID        string            `yaml:"project_id"`
	DomainName       string            `yaml:"domain_name"`
	DomainID         string            `yaml:"domain_id"`
	Role             OpenStackRole     `yaml:"role"`
	Region           string            `yaml:"region"`
	RefreshInterval  model.Duration    `yaml:"refresh_interval,omitempty"`
	Port             int               `yaml:"port"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// OpenStackRole is role of the target in OpenStack.
type OpenStackRole string

// The valid options for OpenStackRole.
const (
	// OpenStack document reference
	// https://docs.openstack.org/nova/pike/admin/arch.html#hypervisors
	OpenStackRoleHypervisor OpenStackRole = "hypervisor"
	// OpenStack document reference
	// https://docs.openstack.org/horizon/pike/user/launch-instances.html
	OpenStackRoleInstance OpenStackRole = "instance"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OpenStackRole) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case OpenStackRoleHypervisor, OpenStackRoleInstance:
		return nil
	default:
		return fmt.Errorf("Unknown OpenStack SD role %q", *c)
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OpenstackSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultOpenstackSDConfig
	type plain OpenstackSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Role == "" {
		return fmt.Errorf("role missing (one of: instance, hypervisor)")
	}
	return yamlUtil.CheckOverflow(c.XXX, "openstack_sd_config")
}

// RelabelAction is the action to be performed on relabeling.
type RelabelAction string

const (
	// RelabelReplace performs a regex replacement.
	RelabelReplace RelabelAction = "replace"
	// RelabelKeep drops targets for which the input does not match the regex.
	RelabelKeep RelabelAction = "keep"
	// RelabelDrop drops targets for which the input does match the regex.
	RelabelDrop RelabelAction = "drop"
	// RelabelHashMod sets a label to the modulus of a hash of labels.
	RelabelHashMod RelabelAction = "hashmod"
	// RelabelLabelMap copies labels to other labelnames based on a regex.
	RelabelLabelMap RelabelAction = "labelmap"
	// RelabelLabelDrop drops any label matching the regex.
	RelabelLabelDrop RelabelAction = "labeldrop"
	// RelabelLabelKeep drops any label not matching the regex.
	RelabelLabelKeep RelabelAction = "labelkeep"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *RelabelAction) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	switch act := RelabelAction(strings.ToLower(s)); act {
	case RelabelReplace, RelabelKeep, RelabelDrop, RelabelHashMod, RelabelLabelMap, RelabelLabelDrop, RelabelLabelKeep:
		*a = act
		return nil
	}
	return fmt.Errorf("unknown relabel action %q", s)
}

// RelabelConfig is the configuration for relabeling of target label sets.
type RelabelConfig struct {
	// A list of labels from which values are taken and concatenated
	// with the configured separator in order.
	SourceLabels model.LabelNames `yaml:"source_labels,flow,omitempty"`
	// Separator is the string between concatenated values from the source labels.
	Separator string `yaml:"separator,omitempty"`
	// Regex against which the concatenation is matched.
	Regex Regexp `yaml:"regex,omitempty"`
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64 `yaml:"modulus,omitempty"`
	// TargetLabel is the label to which the resulting string is written in a replacement.
	// Regexp interpolation is allowed for the replace action.
	TargetLabel string `yaml:"target_label,omitempty"`
	// Replacement is the regex replacement pattern to be used.
	Replacement string `yaml:"replacement,omitempty"`
	// Action is the action to be performed for the relabeling.
	Action RelabelAction `yaml:"action,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RelabelConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultRelabelConfig
	type plain RelabelConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if err := yamlUtil.CheckOverflow(c.XXX, "relabel_config"); err != nil {
		return err
	}
	if c.Regex.Regexp == nil {
		c.Regex = MustNewRegexp("")
	}
	if c.Modulus == 0 && c.Action == RelabelHashMod {
		return fmt.Errorf("relabel configuration for hashmod requires non-zero modulus")
	}
	if (c.Action == RelabelReplace || c.Action == RelabelHashMod) && c.TargetLabel == "" {
		return fmt.Errorf("relabel configuration for %s action requires 'target_label' value", c.Action)
	}
	if c.Action == RelabelReplace && !relabelTarget.MatchString(c.TargetLabel) {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}
	if c.Action == RelabelHashMod && !model.LabelName(c.TargetLabel).IsValid() {
		return fmt.Errorf("%q is invalid 'target_label' for %s action", c.TargetLabel, c.Action)
	}

	if c.Action == RelabelLabelDrop || c.Action == RelabelLabelKeep {
		if c.SourceLabels != nil ||
			c.TargetLabel != DefaultRelabelConfig.TargetLabel ||
			c.Modulus != DefaultRelabelConfig.Modulus ||
			c.Separator != DefaultRelabelConfig.Separator ||
			c.Replacement != DefaultRelabelConfig.Replacement {
			return fmt.Errorf("%s action requires only 'regex', and no other fields", c.Action)
		}
	}

	return nil
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshallable.
type Regexp struct {
	*regexp.Regexp
	original string
}

// NewRegexp creates a new anchored Regexp and returns an error if the
// passed-in regular expression does not compile.
func NewRegexp(s string) (Regexp, error) {
	regex, err := regexp.Compile("^(?:" + s + ")$")
	return Regexp{
		Regexp:   regex,
		original: s,
	}, err
}

// MustNewRegexp works like NewRegexp, but panics if the regular expression does not compile.
func MustNewRegexp(s string) Regexp {
	re, err := NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	r, err := NewRegexp(s)
	if err != nil {
		return err
	}
	*re = r
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (re Regexp) MarshalYAML() (interface{}, error) {
	if re.original != "" {
		return re.original, nil
	}
	return nil, nil
}

// RemoteWriteConfig is the configuration for writing to remote storage.
type RemoteWriteConfig struct {
	URL                 *configUtil.URL  `yaml:"url"`
	RemoteTimeout       model.Duration   `yaml:"remote_timeout,omitempty"`
	WriteRelabelConfigs []*RelabelConfig `yaml:"write_relabel_configs,omitempty"`

	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.
	HTTPClientConfig configUtil.HTTPClientConfig `yaml:",inline"`
	QueueConfig      QueueConfig                 `yaml:"queue_config,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RemoteWriteConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultRemoteWriteConfig
	type plain RemoteWriteConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == nil {
		return fmt.Errorf("url for remote_write is empty")
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	return yamlUtil.CheckOverflow(c.XXX, "remote_write")
}

// QueueConfig is the configuration for the queue used to write to remote
// storage.
type QueueConfig struct {
	// Number of samples to buffer per shard before we start dropping them.
	Capacity int `yaml:"capacity,omitempty"`

	// Max number of shards, i.e. amount of concurrency.
	MaxShards int `yaml:"max_shards,omitempty"`

	// Maximum number of samples per send.
	MaxSamplesPerSend int `yaml:"max_samples_per_send,omitempty"`

	// Maximum time sample will wait in buffer.
	BatchSendDeadline time.Duration `yaml:"batch_send_deadline,omitempty"`

	// Max number of times to retry a batch on recoverable errors.
	MaxRetries int `yaml:"max_retries,omitempty"`

	// On recoverable errors, backoff exponentially.
	MinBackoff time.Duration `yaml:"min_backoff,omitempty"`
	MaxBackoff time.Duration `yaml:"max_backoff,omitempty"`
}

// RemoteReadConfig is the configuration for reading from remote storage.
type RemoteReadConfig struct {
	URL           *configUtil.URL `yaml:"url"`
	RemoteTimeout model.Duration  `yaml:"remote_timeout,omitempty"`
	ReadRecent    bool            `yaml:"read_recent,omitempty"`
	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.
	HTTPClientConfig configUtil.HTTPClientConfig `yaml:",inline"`

	// RequiredMatchers is an optional list of equality matchers which have to
	// be present in a selector to query the remote read endpoint.
	RequiredMatchers model.LabelSet `yaml:"required_matchers,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RemoteReadConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultRemoteReadConfig
	type plain RemoteReadConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == nil {
		return fmt.Errorf("url for remote_read is empty")
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	return yamlUtil.CheckOverflow(c.XXX, "remote_read")
}
