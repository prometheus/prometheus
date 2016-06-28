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
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

var (
	patJobName    = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)
	patFileSDName = regexp.MustCompile(`^[^*]*(\*[^/]*)?\.(json|yml|yaml|JSON|YML|YAML)$`)
	patRulePath   = regexp.MustCompile(`^[^*]*(\*[^/]*)?$`)
	patAuthLine   = regexp.MustCompile(`((?:password|bearer_token|secret_key|client_secret):\s+)(".+"|'.+'|[^\s]+)`)
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
		return nil, err
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

	// DefaultRelabelConfig is the default Relabel configuration.
	DefaultRelabelConfig = RelabelConfig{
		Action:      RelabelReplace,
		Separator:   ";",
		Regex:       MustNewRegexp("(.*)"),
		Replacement: "$1",
	}

	// DefaultDNSSDConfig is the default DNS SD configuration.
	DefaultDNSSDConfig = DNSSDConfig{
		RefreshInterval: model.Duration(30 * time.Second),
		Type:            "SRV",
	}

	// DefaultFileSDConfig is the default file SD configuration.
	DefaultFileSDConfig = FileSDConfig{
		RefreshInterval: model.Duration(5 * time.Minute),
	}

	// DefaultConsulSDConfig is the default Consul SD configuration.
	DefaultConsulSDConfig = ConsulSDConfig{
		TagSeparator: ",",
		Scheme:       "http",
	}

	// DefaultServersetSDConfig is the default Serverset SD configuration.
	DefaultServersetSDConfig = ServersetSDConfig{
		Timeout: model.Duration(10 * time.Second),
	}

	// DefaultNerveSDConfig is the default Nerve SD configuration.
	DefaultNerveSDConfig = NerveSDConfig{
		Timeout: model.Duration(10 * time.Second),
	}

	// DefaultMarathonSDConfig is the default Marathon SD configuration.
	DefaultMarathonSDConfig = MarathonSDConfig{
		RefreshInterval: model.Duration(30 * time.Second),
	}

	// DefaultKubernetesSDConfig is the default Kubernetes SD configuration
	DefaultKubernetesSDConfig = KubernetesSDConfig{
		RequestTimeout: model.Duration(10 * time.Second),
		RetryInterval:  model.Duration(1 * time.Second),
	}

	// DefaultGCESDConfig is the default EC2 SD configuration.
	DefaultGCESDConfig = GCESDConfig{
		Port:            80,
		TagSeparator:    ",",
		RefreshInterval: model.Duration(60 * time.Second),
	}

	// DefaultEC2SDConfig is the default EC2 SD configuration.
	DefaultEC2SDConfig = EC2SDConfig{
		Port:            80,
		RefreshInterval: model.Duration(60 * time.Second),
	}

	// DefaultAzureSDConfig is the default Azure SD configuration.
	DefaultAzureSDConfig = AzureSDConfig{
		Port:            80,
		RefreshInterval: model.Duration(5 * time.Minute),
	}
)

// URL is a custom URL type that allows validation at configuration load time.
type URL struct {
	*url.URL
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for URLs.
func (u *URL) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	urlp, err := url.Parse(s)
	if err != nil {
		return err
	}
	u.URL = urlp
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface for URLs.
func (u URL) MarshalYAML() (interface{}, error) {
	if u.URL != nil {
		return u.String(), nil
	}
	return nil, nil
}

// Config is the top-level configuration for Prometheus's config files.
type Config struct {
	GlobalConfig   GlobalConfig    `yaml:"global"`
	AlertingConfig AlertingConfig  `yaml:"alerting,omitempty"`
	RuleFiles      []string        `yaml:"rule_files,omitempty"`
	ScrapeConfigs  []*ScrapeConfig `yaml:"scrape_configs,omitempty"`

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

	for _, scfg := range cfg.ScrapeConfigs {
		scfg.BearerTokenFile = join(scfg.BearerTokenFile)
		scfg.TLSConfig.CAFile = join(scfg.TLSConfig.CAFile)
		scfg.TLSConfig.CertFile = join(scfg.TLSConfig.CertFile)
		scfg.TLSConfig.KeyFile = join(scfg.TLSConfig.KeyFile)

		for _, kcfg := range scfg.KubernetesSDConfigs {
			kcfg.BearerTokenFile = join(kcfg.BearerTokenFile)
			kcfg.TLSConfig.CAFile = join(kcfg.TLSConfig.CAFile)
			kcfg.TLSConfig.CertFile = join(kcfg.TLSConfig.CertFile)
			kcfg.TLSConfig.KeyFile = join(kcfg.TLSConfig.KeyFile)
		}
	}
}

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

func (c Config) String() string {
	var s string
	if c.original != "" {
		s = c.original
	} else {
		b, err := yaml.Marshal(c)
		if err != nil {
			return fmt.Sprintf("<error creating config string: %s>", err)
		}
		s = string(b)
	}
	return patAuthLine.ReplaceAllString(s, "${1}<hidden>")
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
	if err := checkOverflow(c.XXX, "config"); err != nil {
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

// AlertingConfig configures alerting and alertmanager related configs
type AlertingConfig struct {
	AlertRelabelConfigs []*RelabelConfig `yaml:"alert_relabel_configs,omitempty"`
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
	if err := checkOverflow(c.XXX, "global config"); err != nil {
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

// TLSConfig configures the options for TLS connections.
type TLSConfig struct {
	// The CA cert to use for the targets.
	CAFile string `yaml:"ca_file,omitempty"`
	// The client cert file for the targets.
	CertFile string `yaml:"cert_file,omitempty"`
	// The client key file for the targets.
	KeyFile string `yaml:"key_file,omitempty"`
	// Used to verify the hostname for the targets.
	ServerName string `yaml:"server_name,omitempty"`
	// Disable target certificate validation.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *TLSConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain TLSConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if err := checkOverflow(c.XXX, "TLS config"); err != nil {
		return err
	}
	return nil
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
	// The HTTP basic authentication credentials for the targets.
	BasicAuth *BasicAuth `yaml:"basic_auth,omitempty"`
	// The bearer token for the targets.
	BearerToken string `yaml:"bearer_token,omitempty"`
	// The bearer token file for the targets.
	BearerTokenFile string `yaml:"bearer_token_file,omitempty"`
	// HTTP proxy server to use to connect to the targets.
	ProxyURL URL `yaml:"proxy_url,omitempty"`
	// TLSConfig to use to connect to the targets.
	TLSConfig TLSConfig `yaml:"tls_config,omitempty"`

	// List of labeled target groups for this job.
	StaticConfigs []*TargetGroup `yaml:"static_configs,omitempty"`
	// List of DNS service discovery configurations.
	DNSSDConfigs []*DNSSDConfig `yaml:"dns_sd_configs,omitempty"`
	// List of file service discovery configurations.
	FileSDConfigs []*FileSDConfig `yaml:"file_sd_configs,omitempty"`
	// List of Consul service discovery configurations.
	ConsulSDConfigs []*ConsulSDConfig `yaml:"consul_sd_configs,omitempty"`
	// List of Serverset service discovery configurations.
	ServersetSDConfigs []*ServersetSDConfig `yaml:"serverset_sd_configs,omitempty"`
	// NerveSDConfigs is a list of Nerve service discovery configurations.
	NerveSDConfigs []*NerveSDConfig `yaml:"nerve_sd_configs,omitempty"`
	// MarathonSDConfigs is a list of Marathon service discovery configurations.
	MarathonSDConfigs []*MarathonSDConfig `yaml:"marathon_sd_configs,omitempty"`
	// List of Kubernetes service discovery configurations.
	KubernetesSDConfigs []*KubernetesSDConfig `yaml:"kubernetes_sd_configs,omitempty"`
	// List of GCE service discovery configurations.
	GCESDConfigs []*GCESDConfig `yaml:"gce_sd_configs,omitempty"`
	// List of EC2 service discovery configurations.
	EC2SDConfigs []*EC2SDConfig `yaml:"ec2_sd_configs,omitempty"`
	// List of Azure service discovery configurations.
	AzureSDConfigs []*AzureSDConfig `yaml:"azure_sd_configs,omitempty"`

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
	if err := checkOverflow(c.XXX, "scrape_config"); err != nil {
		return err
	}
	if !patJobName.MatchString(c.JobName) {
		return fmt.Errorf("%q is not a valid job name", c.JobName)
	}
	if len(c.BearerToken) > 0 && len(c.BearerTokenFile) > 0 {
		return fmt.Errorf("at most one of bearer_token & bearer_token_file must be configured")
	}
	if c.BasicAuth != nil && (len(c.BearerToken) > 0 || len(c.BearerTokenFile) > 0) {
		return fmt.Errorf("at most one of basic_auth, bearer_token & bearer_token_file must be configured")
	}
	// Check for users putting URLs in target groups.
	if len(c.RelabelConfigs) == 0 {
		for _, tg := range c.StaticConfigs {
			for _, t := range tg.Targets {
				if err = CheckTargetAddress(t[model.AddressLabel]); err != nil {
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

// BasicAuth contains basic HTTP authentication credentials.
type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// ClientCert contains client cert credentials.
type ClientCert struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *BasicAuth) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain BasicAuth
	err := unmarshal((*plain)(a))
	if err != nil {
		return err
	}
	return checkOverflow(a.XXX, "basic_auth")
}

// TargetGroup is a set of targets with a common label set.
type TargetGroup struct {
	// Targets is a list of targets identified by a label set. Each target is
	// uniquely identifiable in the group by its address label.
	Targets []model.LabelSet
	// Labels is a set of labels that is common across all targets in the group.
	Labels model.LabelSet

	// Source is an identifier that describes a group of targets.
	Source string
}

func (tg TargetGroup) String() string {
	return tg.Source
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (tg *TargetGroup) UnmarshalYAML(unmarshal func(interface{}) error) error {
	g := struct {
		Targets []string               `yaml:"targets"`
		Labels  model.LabelSet         `yaml:"labels"`
		XXX     map[string]interface{} `yaml:",inline"`
	}{}
	if err := unmarshal(&g); err != nil {
		return err
	}
	tg.Targets = make([]model.LabelSet, 0, len(g.Targets))
	for _, t := range g.Targets {
		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel: model.LabelValue(t),
		})
	}
	tg.Labels = g.Labels
	return checkOverflow(g.XXX, "target_group")
}

// MarshalYAML implements the yaml.Marshaler interface.
func (tg TargetGroup) MarshalYAML() (interface{}, error) {
	g := &struct {
		Targets []string       `yaml:"targets"`
		Labels  model.LabelSet `yaml:"labels,omitempty"`
	}{
		Targets: make([]string, 0, len(tg.Targets)),
		Labels:  tg.Labels,
	}
	for _, t := range tg.Targets {
		g.Targets = append(g.Targets, string(t[model.AddressLabel]))
	}
	return g, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (tg *TargetGroup) UnmarshalJSON(b []byte) error {
	g := struct {
		Targets []string       `json:"targets"`
		Labels  model.LabelSet `json:"labels"`
	}{}
	if err := json.Unmarshal(b, &g); err != nil {
		return err
	}
	tg.Targets = make([]model.LabelSet, 0, len(g.Targets))
	for _, t := range g.Targets {
		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel: model.LabelValue(t),
		})
	}
	tg.Labels = g.Labels
	return nil
}

// DNSSDConfig is the configuration for DNS based service discovery.
type DNSSDConfig struct {
	Names           []string       `yaml:"names"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Type            string         `yaml:"type"`
	Port            int            `yaml:"port"` // Ignored for SRV records
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *DNSSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultDNSSDConfig
	type plain DNSSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := checkOverflow(c.XXX, "dns_sd_config"); err != nil {
		return err
	}
	if len(c.Names) == 0 {
		return fmt.Errorf("DNS-SD config must contain at least one SRV record name")
	}
	switch strings.ToUpper(c.Type) {
	case "SRV":
	case "A", "AAAA":
		if c.Port == 0 {
			return fmt.Errorf("a port is required in DNS-SD configs for all record types except SRV")
		}
	default:
		return fmt.Errorf("invalid DNS-SD records type %s", c.Type)
	}
	return nil
}

// FileSDConfig is the configuration for file based discovery.
type FileSDConfig struct {
	Files           []string       `yaml:"files"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *FileSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultFileSDConfig
	type plain FileSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := checkOverflow(c.XXX, "file_sd_config"); err != nil {
		return err
	}
	if len(c.Files) == 0 {
		return fmt.Errorf("file service discovery config must contain at least one path name")
	}
	for _, name := range c.Files {
		if !patFileSDName.MatchString(name) {
			return fmt.Errorf("path name %q is not valid for file discovery", name)
		}
	}
	return nil
}

// ConsulSDConfig is the configuration for Consul service discovery.
type ConsulSDConfig struct {
	Server       string `yaml:"server"`
	Token        string `yaml:"token,omitempty"`
	Datacenter   string `yaml:"datacenter,omitempty"`
	TagSeparator string `yaml:"tag_separator,omitempty"`
	Scheme       string `yaml:"scheme,omitempty"`
	Username     string `yaml:"username,omitempty"`
	Password     string `yaml:"password,omitempty"`
	// The list of services for which targets are discovered.
	// Defaults to all services if empty.
	Services []string `yaml:"services"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ConsulSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultConsulSDConfig
	type plain ConsulSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := checkOverflow(c.XXX, "consul_sd_config"); err != nil {
		return err
	}
	if strings.TrimSpace(c.Server) == "" {
		return fmt.Errorf("Consul SD configuration requires a server address")
	}
	return nil
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
	if err := checkOverflow(c.XXX, "serverset_sd_config"); err != nil {
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
	if err := checkOverflow(c.XXX, "nerve_sd_config"); err != nil {
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

// MarathonSDConfig is the configuration for services running on Marathon.
type MarathonSDConfig struct {
	Servers         []string       `yaml:"servers,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *MarathonSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultMarathonSDConfig
	type plain MarathonSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := checkOverflow(c.XXX, "marathon_sd_config"); err != nil {
		return err
	}
	if len(c.Servers) == 0 {
		return fmt.Errorf("Marathon SD config must contain at least one Marathon server")
	}

	return nil
}

// KubernetesSDConfig is the configuration for Kubernetes service discovery.
type KubernetesSDConfig struct {
	APIServers      []URL          `yaml:"api_servers"`
	Role            KubernetesRole `yaml:"role"`
	InCluster       bool           `yaml:"in_cluster,omitempty"`
	BasicAuth       *BasicAuth     `yaml:"basic_auth,omitempty"`
	BearerToken     string         `yaml:"bearer_token,omitempty"`
	BearerTokenFile string         `yaml:"bearer_token_file,omitempty"`
	RetryInterval   model.Duration `yaml:"retry_interval,omitempty"`
	RequestTimeout  model.Duration `yaml:"request_timeout,omitempty"`
	TLSConfig       TLSConfig      `yaml:"tls_config,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

type KubernetesRole string

const (
	KubernetesRoleNode      = "node"
	KubernetesRolePod       = "pod"
	KubernetesRoleContainer = "container"
	KubernetesRoleService   = "service"
	KubernetesRoleEndpoint  = "endpoint"
	KubernetesRoleAPIServer = "apiserver"
)

func (c *KubernetesRole) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case KubernetesRoleNode, KubernetesRolePod, KubernetesRoleContainer, KubernetesRoleService, KubernetesRoleEndpoint, KubernetesRoleAPIServer:
		return nil
	default:
		return fmt.Errorf("Unknown Kubernetes SD role %q", *c)
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *KubernetesSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultKubernetesSDConfig
	type plain KubernetesSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := checkOverflow(c.XXX, "kubernetes_sd_config"); err != nil {
		return err
	}
	if c.Role == "" {
		return fmt.Errorf("role missing (one of: container, pod, service, endpoint, node, apiserver)")
	}
	if len(c.APIServers) == 0 {
		return fmt.Errorf("Kubernetes SD configuration requires at least one Kubernetes API server")
	}
	if len(c.BearerToken) > 0 && len(c.BearerTokenFile) > 0 {
		return fmt.Errorf("at most one of bearer_token & bearer_token_file must be configured")
	}
	if c.BasicAuth != nil && (len(c.BearerToken) > 0 || len(c.BearerTokenFile) > 0) {
		return fmt.Errorf("at most one of basic_auth, bearer_token & bearer_token_file must be configured")
	}
	return nil
}

// GCESDConfig is the configuration for GCE based service discovery.
type GCESDConfig struct {
	// Project: The Google Cloud Project ID
	Project string `yaml:"project"`

	// Zone: The zone of the scrape targets.
	// If you need to configure multiple zones use multiple gce_sd_configs
	Zone string `yaml:"zone"`

	// Filter: Can be used optionally to filter the instance list by other criteria.
	// Syntax of this filter string is described here in the filter query parameter section:
	// https://cloud.google.com/compute/docs/reference/latest/instances/list
	Filter string `yaml:"filter,omitempty"`

	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Port            int            `yaml:"port"`
	TagSeparator    string         `yaml:"tag_separator,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *GCESDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultGCESDConfig
	type plain GCESDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := checkOverflow(c.XXX, "ec2_sd_config"); err != nil {
		return err
	}
	if c.Project == "" {
		return fmt.Errorf("GCE SD configuration requires a project")
	}
	if c.Zone == "" {
		return fmt.Errorf("GCE SD configuration requires a zone")
	}
	return nil
}

// EC2SDConfig is the configuration for EC2 based service discovery.
type EC2SDConfig struct {
	Region          string         `yaml:"region"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	SecretKey       string         `yaml:"secret_key,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Port            int            `yaml:"port"`
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *EC2SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultEC2SDConfig
	type plain EC2SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err := checkOverflow(c.XXX, "ec2_sd_config"); err != nil {
		return err
	}
	if c.Region == "" {
		return fmt.Errorf("EC2 SD configuration requires a region")
	}
	return nil
}

// AzureSDConfig is the configuration for Azure based service discovery.
type AzureSDConfig struct {
	Port            int            `yaml:"port"`
	SubscriptionID  string         `yaml:"subscription_id"`
	TenantID        string         `yaml:"tenant_id,omitempty"`
	ClientID        string         `yaml:"client_id,omitempty"`
	ClientSecret    string         `yaml:"client_secret,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *AzureSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultAzureSDConfig
	type plain AzureSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	return checkOverflow(c.XXX, "azure_sd_config")
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
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *RelabelAction) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	switch act := RelabelAction(strings.ToLower(s)); act {
	case RelabelReplace, RelabelKeep, RelabelDrop, RelabelHashMod, RelabelLabelMap:
		*a = act
		return nil
	}
	return fmt.Errorf("unknown relabel action %q", s)
}

// RelabelConfig is the configuration for relabeling of target label sets.
type RelabelConfig struct {
	// A list of labels from which values are taken and concatenated
	// with the configured separator in order.
	SourceLabels model.LabelNames `yaml:"source_labels,flow"`
	// Separator is the string between concatenated values from the source labels.
	Separator string `yaml:"separator,omitempty"`
	// Regex against which the concatenation is matched.
	Regex Regexp `yaml:"regex,omitempty"`
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64 `yaml:"modulus,omitempty"`
	// The label to which the resulting string is written in a replacement.
	TargetLabel model.LabelName `yaml:"target_label,omitempty"`
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
	if err := checkOverflow(c.XXX, "relabel_config"); err != nil {
		return err
	}
	if c.Modulus == 0 && c.Action == RelabelHashMod {
		return fmt.Errorf("relabel configuration for hashmod requires non-zero modulus")
	}
	if (c.Action == RelabelReplace || c.Action == RelabelHashMod) && c.TargetLabel == "" {
		return fmt.Errorf("relabel configuration for %s action requires 'target_label' value", c.Action)
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
