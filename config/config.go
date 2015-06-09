package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/util/strutil"
)

var (
	patJobName    = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)
	patFileSDName = regexp.MustCompile(`^[^*]*(\*[^/]*)?\.(json|yml|yaml|JSON|YML|YAML)$`)
	patRulePath   = regexp.MustCompile(`^[^*]*(\*[^/]*)?$`)
)

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
	DefaultConfig = Config{
		GlobalConfig: DefaultGlobalConfig,
	}

	// The default global configuration.
	DefaultGlobalConfig = GlobalConfig{
		ScrapeInterval:     Duration(1 * time.Minute),
		ScrapeTimeout:      Duration(10 * time.Second),
		EvaluationInterval: Duration(1 * time.Minute),
	}

	// The default scrape configuration.
	DefaultScrapeConfig = ScrapeConfig{
		// ScrapeTimeout and ScrapeInterval default to the
		// configured globals.
		MetricsPath: "/metrics",
		Scheme:      "http",
	}

	// The default Relabel configuration.
	DefaultRelabelConfig = RelabelConfig{
		Action:    RelabelReplace,
		Separator: ";",
	}

	// The default DNS SD configuration.
	DefaultDNSSDConfig = DNSSDConfig{
		RefreshInterval: Duration(30 * time.Second),
	}

	// The default file SD configuration.
	DefaultFileSDConfig = FileSDConfig{
		RefreshInterval: Duration(30 * time.Second),
	}

	// The default Consul SD configuration.
	DefaultConsulSDConfig = ConsulSDConfig{
		TagSeparator: ",",
		Scheme:       "http",
	}

	// The default Serverset SD configuration.
	DefaultServersetSDConfig = ServersetSDConfig{
		Timeout: Duration(10 * time.Second),
	}
)

// Config is the top-level configuration for Prometheus's config files.
type Config struct {
	GlobalConfig  GlobalConfig    `yaml:"global"`
	RuleFiles     []string        `yaml:"rule_files,omitempty"`
	ScrapeConfigs []*ScrapeConfig `yaml:"scrape_configs,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`

	// original is the input from which the config was parsed.
	original string
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
	if c.original != "" {
		return c.original
	}
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
	for _, rf := range c.RuleFiles {
		if !patRulePath.MatchString(rf) {
			return fmt.Errorf("invalid rule file path %q", rf)
		}
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
	return checkOverflow(c.XXX, "config")
}

// GlobalConfig configures values that are used across other configuration
// objects.
type GlobalConfig struct {
	// How frequently to scrape targets by default.
	ScrapeInterval Duration `yaml:"scrape_interval,omitempty"`
	// The default timeout when scraping targets.
	ScrapeTimeout Duration `yaml:"scrape_timeout,omitempty"`
	// How frequently to evaluate rules by default.
	EvaluationInterval Duration `yaml:"evaluation_interval,omitempty"`
	// The labels to add to any timeseries that this Prometheus instance scrapes.
	Labels clientmodel.LabelSet `yaml:"labels,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *GlobalConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultGlobalConfig
	type plain GlobalConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return checkOverflow(c.XXX, "global config")
}

// ScrapeConfig configures a scraping unit for Prometheus.
type ScrapeConfig struct {
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
	BasicAuth *BasicAuth `yaml:"basic_auth,omitempty"`

	// List of labeled target groups for this job.
	TargetGroups []*TargetGroup `yaml:"target_groups,omitempty"`
	// List of DNS service discovery configurations.
	DNSSDConfigs []*DNSSDConfig `yaml:"dns_sd_configs,omitempty"`
	// List of file service discovery configurations.
	FileSDConfigs []*FileSDConfig `yaml:"file_sd_configs,omitempty"`
	// List of Consul service discovery configurations.
	ConsulSDConfigs []*ConsulSDConfig `yaml:"consul_sd_configs,omitempty"`
	// List of Serverset service discovery configurations.
	ServersetSDConfigs []*ServersetSDConfig `yaml:"serverset_sd_configs,omitempty"`

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
	if !patJobName.MatchString(c.JobName) {
		return fmt.Errorf("%q is not a valid job name", c.JobName)
	}
	return checkOverflow(c.XXX, "scrape_config")
}

// BasicAuth contains basic HTTP authentication credentials.
type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`

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
	Targets []clientmodel.LabelSet
	// Labels is a set of labels that is common across all targets in the group.
	Labels clientmodel.LabelSet

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
		Labels  clientmodel.LabelSet   `yaml:"labels"`
		XXX     map[string]interface{} `yaml:",inline"`
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
	return checkOverflow(g.XXX, "target_group")
}

// MarshalYAML implements the yaml.Marshaler interface.
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

// UnmarshalJSON implements the json.Unmarshaler interface.
func (tg *TargetGroup) UnmarshalJSON(b []byte) error {
	g := struct {
		Targets []string             `json:"targets"`
		Labels  clientmodel.LabelSet `json:"labels"`
	}{}
	if err := json.Unmarshal(b, &g); err != nil {
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

// DNSSDConfig is the configuration for DNS based service discovery.
type DNSSDConfig struct {
	Names           []string `yaml:"names"`
	RefreshInterval Duration `yaml:"refresh_interval,omitempty"`

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
	if len(c.Names) == 0 {
		return fmt.Errorf("DNS-SD config must contain at least one SRV record name")
	}
	return checkOverflow(c.XXX, "dns_sd_config")
}

// FileSDConfig is the configuration for file based discovery.
type FileSDConfig struct {
	Names           []string `yaml:"names"`
	RefreshInterval Duration `yaml:"refresh_interval,omitempty"`

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
	if len(c.Names) == 0 {
		return fmt.Errorf("file service discovery config must contain at least one path name")
	}
	for _, name := range c.Names {
		if !patFileSDName.MatchString(name) {
			return fmt.Errorf("path name %q is not valid for file discovery", name)
		}
	}
	return checkOverflow(c.XXX, "file_sd_config")
}

// ConsulSDConfig is the configuration for Consul service discovery.
type ConsulSDConfig struct {
	Server       string `yaml:"server"`
	Token        string `yaml:"token"`
	Datacenter   string `yaml:"datacenter"`
	TagSeparator string `yaml:"tag_separator"`
	Scheme       string `yaml:"scheme"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	// The list of services for which targets are discovered.
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
	if strings.TrimSpace(c.Server) == "" {
		return fmt.Errorf("Consul SD configuration requires a server address")
	}
	if len(c.Services) == 0 {
		return fmt.Errorf("Consul SD configuration requires at least one service name")
	}
	return checkOverflow(c.XXX, "consul_sd_config")
}

// ServersetSDConfig is the configuration for Twitter serversets in Zookeeper based discovery.
type ServersetSDConfig struct {
	Servers []string `yaml:"servers"`
	Paths   []string `yaml:"paths"`
	Timeout Duration `yaml:"timeout,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ServersetSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultServersetSDConfig
	type plain ServersetSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
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

// UnmarshalYAML implements the yaml.Unmarshaler interface.
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
	if c.Regex == nil {
		return fmt.Errorf("relabel configuration requires a regular expression")
	}
	return checkOverflow(c.XXX, "relabel_config")
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshallable.
type Regexp struct {
	regexp.Regexp
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
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

// MarshalYAML implements the yaml.Marshaler interface.
func (re Regexp) MarshalYAML() (interface{}, error) {
	return re.String(), nil
}

// Duration encapsulates a time.Duration and makes it YAML marshallable.
//
// TODO(fabxc): Since we have custom types for most things, including timestamps,
// we might want to move this into our model as well, eventually.
type Duration time.Duration

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	dur, err := strutil.StringToDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (d Duration) MarshalYAML() (interface{}, error) {
	return strutil.DurationToString(time.Duration(d)), nil
}
