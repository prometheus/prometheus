package config

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

var jobNameRE = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_-]*$")

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{
		original: s,
	}
	err := yaml.Unmarshal([]byte(s), cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadFromFile parses the given YAML file into a Config.
func LoadFromFile(filename string) (*Config, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return Load(string(content))
}

// The defaults applied before parsing the respective config sections.
var (
	// The default top-level configuration.
	DefaultConfig = DefaultedConfig{
		GlobalConfig: &GlobalConfig{DefaultGlobalConfig},
	}

	// The default global configuration.
	DefaultGlobalConfig = DefaultedGlobalConfig{
		ScrapeInterval:     Duration(10 * time.Second),
		ScrapeTimeout:      Duration(10 * time.Second),
		EvaluationInterval: Duration(1 * time.Minute),
	}

	// Te default scrape configuration.
	DefaultScrapeConfig = DefaultedScrapeConfig{
		// ScrapeTimeout and ScrapeInterval default to the
		// configured globals.
		MetricsPath: "/metrics",
		Scheme:      "http",
	}

	// The default Relabel configuration.
	DefaultRelabelConfig = DefaultedRelabelConfig{
		Action:    RelabelReplace,
		Separator: ";",
	}

	// The default DNS SD configuration.
	DefaultDNSSDConfig = DefaultedDNSSDConfig{
		RefreshInterval: Duration(30 * time.Second),
	}
)

// Config is the top-level configuration for Prometheus's config files.
type Config struct {
	// DefaultedConfig contains the actual fields of Config.
	DefaultedConfig `yaml:",inline"`

	// original is the input from which the config was parsed.
	original string
}

func (c Config) String() string {
	if c.original != "" {
		return c.original
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c.DefaultedConfig = DefaultConfig
	if err := unmarshal(&c.DefaultedConfig); err != nil {
		return err
	}
	// Do global overrides and validate unique names.
	jobNames := map[string]struct{}{}
	for _, scfg := range c.ScrapeConfigs {
		if scfg.ScrapeInterval == 0 {
			scfg.ScrapeInterval = c.GlobalConfig.ScrapeInterval
		}
		if scfg.ScrapeTimeout == 0 {
			scfg.ScrapeTimeout = c.GlobalConfig.ScrapeTimeout
		}

		if _, ok := jobNames[scfg.JobName]; ok {
			return fmt.Errorf("found multiple scrape configs with job name %q", scfg.JobName)
		}
		jobNames[scfg.JobName] = struct{}{}
	}
	return nil
}

// DefaultedConfig is a proxy type for Config.
type DefaultedConfig struct {
	GlobalConfig  *GlobalConfig   `yaml:"global"`
	RuleFiles     []string        `yaml:"rule_files,omitempty"`
	ScrapeConfigs []*ScrapeConfig `yaml:"scrape_configs,omitempty"`
}

// GlobalConfig configures values that are used across other configuration
// objects.
type GlobalConfig struct {
	// DefaultedGlobalConfig contains the actual fields for GlobalConfig.
	DefaultedGlobalConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (c *GlobalConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c.DefaultedGlobalConfig = DefaultGlobalConfig
	if err := unmarshal(&c.DefaultedGlobalConfig); err != nil {
		return err
	}
	return nil
}

// DefaultedGlobalConfig is a proxy type for GlobalConfig.
type DefaultedGlobalConfig struct {
	// How frequently to scrape targets by default.
	ScrapeInterval Duration `yaml:"scrape_interval,omitempty"`
	// The default timeout when scraping targets.
	ScrapeTimeout Duration `yaml:"scrape_timeout,omitempty"`
	// How frequently to evaluate rules by default.
	EvaluationInterval Duration `yaml:"evaluation_interval,omitempty"`

	// The labels to add to any timeseries that this Prometheus instance scrapes.
	Labels clientmodel.LabelSet `yaml:"labels,omitempty"`
}

// ScrapeConfig configures a scraping unit for Prometheus.
type ScrapeConfig struct {
	// DefaultedScrapeConfig contains the actual fields for ScrapeConfig.
	DefaultedScrapeConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (c *ScrapeConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c.DefaultedScrapeConfig = DefaultScrapeConfig
	err := unmarshal(&c.DefaultedScrapeConfig)
	if err != nil {
		return err
	}
	if !jobNameRE.MatchString(c.JobName) {
		return fmt.Errorf("%q is not a valid job name", c.JobName)
	}
	return nil
}

// DefaultedScrapeConfig is a proxy type for ScrapeConfig.
type DefaultedScrapeConfig struct {
	// The job name to which the job label is set by default.
	JobName string `yaml:"job_name"`
	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval Duration `yaml:"scrape_interval,omitempty"`
	// The timeout for scraping targets of this config.
	ScrapeTimeout Duration `yaml:"scrape_timeout,omitempty"`
	// The HTTP resource path on which to fetch metrics from targets.
	MetricsPath string `yaml:"metrics_path,omitempty"`
	// The URL scheme with which to fetch metrics from targets.
	Scheme string `yaml:"scheme,omitempty"`
	// The HTTP basic authentication credentials for the targets.
	BasicAuth *BasicAuth `yaml:"basic_auth"`

	// List of labeled target groups for this job.
	TargetGroups []*TargetGroup `yaml:"target_groups,omitempty"`
	// List of DNS service discovery configurations.
	DNSSDConfigs []*DNSSDConfig `yaml:"dns_sd_configs,omitempty"`
	// List of relabel configurations.
	RelabelConfigs []*RelabelConfig `yaml:"relabel_configs,omitempty"`
}

// BasicAuth contains basic HTTP authentication credentials.
type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// TargetGroup is a set of targets with a common label set.
type TargetGroup struct {
	// Targets is a list of targets identified by a label set. Each target is
	// uniquely identifiable in the group by its address label.
	Targets []clientmodel.LabelSet `yaml:"targets,omitempty" json:"targets,omitempty"`
	// Labels is a set of labels that is common across all targets in the group.
	Labels clientmodel.LabelSet `yaml:"labels,omitempty" json:"labels,omitempty"`

	// Source is an identifier that describes a group of targets.
	Source string `yaml:"-", json:"-"`
}

func (tg TargetGroup) String() string {
	return tg.Source
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (tg *TargetGroup) UnmarshalYAML(unmarshal func(interface{}) error) error {
	g := struct {
		Targets []string             `yaml:"targets"`
		Labels  clientmodel.LabelSet `yaml:"labels"`
	}{}
	if err := unmarshal(&g); err != nil {
		return err
	}
	tg.Targets = make([]clientmodel.LabelSet, 0, len(g.Targets))
	for _, t := range g.Targets {
		if strings.Contains(t, "/") {
			return fmt.Errorf("%q is not a valid hostname", t)
		}
		tg.Targets = append(tg.Targets, clientmodel.LabelSet{
			clientmodel.AddressLabel: clientmodel.LabelValue(t),
		})
	}
	tg.Labels = g.Labels
	return nil
}

// MarshalYAML implements the yaml.Marshaller interface.
func (tg TargetGroup) MarshalYAML() (interface{}, error) {
	g := &struct {
		Targets []string             `yaml:"targets"`
		Labels  clientmodel.LabelSet `yaml:"labels,omitempty"`
	}{
		Targets: make([]string, 0, len(tg.Targets)),
		Labels:  tg.Labels,
	}
	for _, t := range tg.Targets {
		g.Targets = append(g.Targets, string(t[clientmodel.AddressLabel]))
	}
	return g, nil
}

// DNSSDConfig is the configuration for DNS based service discovery.
type DNSSDConfig struct {
	// DefaultedDNSSDConfig contains the actual fields for DNSSDConfig.
	DefaultedDNSSDConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (c *DNSSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c.DefaultedDNSSDConfig = DefaultDNSSDConfig
	err := unmarshal(&c.DefaultedDNSSDConfig)
	if err != nil {
		return err
	}
	if len(c.Names) == 0 {
		return fmt.Errorf("DNS config must contain at least one SRV record name")
	}
	return nil
}

// DefaultedDNSSDConfig is a proxy type for DNSSDConfig.
type DefaultedDNSSDConfig struct {
	Names           []string `yaml:"names"`
	RefreshInterval Duration `yaml:"refresh_interval,omitempty"`
}

// RelabelAction is the action to be performed on relabeling.
type RelabelAction string

const (
	// Performs a regex replacement.
	RelabelReplace RelabelAction = "replace"
	// Drops targets for which the input does not match the regex.
	RelabelKeep = "keep"
	// Drops targets for which the input does match the regex.
	RelabelDrop = "drop"
)

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (a *RelabelAction) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	switch act := RelabelAction(strings.ToLower(s)); act {
	case RelabelReplace, RelabelKeep, RelabelDrop:
		*a = act
		return nil
	}
	return fmt.Errorf("unknown relabel action %q", s)
}

// RelabelConfig is the configuration for relabeling of target label sets.
type RelabelConfig struct {
	// DefaultedRelabelConfig contains the actual fields for RelabelConfig.
	DefaultedRelabelConfig `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (c *RelabelConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	c.DefaultedRelabelConfig = DefaultRelabelConfig
	return unmarshal(&c.DefaultedRelabelConfig)
}

// DefaultedRelabelConfig is a proxy type for RelabelConfig.
type DefaultedRelabelConfig struct {
	// A list of labels from which values are taken and concatenated
	// with the configured separator in order.
	SourceLabels clientmodel.LabelNames `yaml:"source_labels,flow"`
	// Separator is the string between concatenated values from the source labels.
	Separator string `yaml:"separator,omitempty"`
	// Regex against which the concatenation is matched.
	Regex *Regexp `yaml:"regex"`
	// The label to which the resulting string is written in a replacement.
	TargetLabel clientmodel.LabelName `yaml:"target_label,omitempty"`
	// Replacement is the regex replacement pattern to be used.
	Replacement string `yaml:"replacement,omitempty"`
	// Action is the action to be performed for the relabeling.
	Action RelabelAction `yaml:"action,omitempty"`
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshallable.
type Regexp struct {
	regexp.Regexp
}

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	regex, err := regexp.Compile(s)
	if err != nil {
		return err
	}
	re.Regexp = *regex
	return nil
}

// MarshalYAML implements the yaml.Marshaller interface.
func (re Regexp) MarshalYAML() (interface{}, error) {
	return re.String(), nil
}

// Duration encapsulates a time.Duration and makes it YAML marshallable.
//
// TODO(fabxc): Since we have custom types for most things, including timestamps,
// we might want to move this into our model as well, eventually.
type Duration time.Duration

// UnmarshalYAML implements the yaml.Unmarshaller interface.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	dur, err := utility.StringToDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

// MarshalYAML implements the yaml.Marshaller interface.
func (d Duration) MarshalYAML() (interface{}, error) {
	return utility.DurationToString(time.Duration(d)), nil
}
