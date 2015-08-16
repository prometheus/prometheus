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

	"gopkg.in/yaml.v2"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/util/strutil"
)

var (
	patJobName    = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)
	patFileSDName = regexp.MustCompile(`^[^*]*(\*[^/]*)?\.(json|yml|yaml|JSON|YML|YAML)$`)
	patRulePath   = regexp.MustCompile(`^[^*]*(\*[^/]*)?$`)
	patAuthLine   = regexp.MustCompile(`((?:username|password|bearer_token):\s+)(".+"|'.+'|[^\s]+)`)
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
		HonorLabels: false,
	}

	// The default Relabel configuration.
	DefaultRelabelConfig = RelabelConfig{
		Action:    RelabelReplace,
		Separator: ";",
	}

	// The default DNS SD configuration.
	DefaultDNSSDConfig = DNSSDConfig{
		RefreshInterval: Duration(30 * time.Second),
		Type:            "SRV",
	}

	// The default file SD configuration.
	DefaultFileSDConfig = FileSDConfig{
		RefreshInterval: Duration(5 * time.Minute),
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

	// DefaultMarathonSDConfig is the default Marathon SD configuration.
	DefaultMarathonSDConfig = MarathonSDConfig{
		RefreshInterval: Duration(30 * time.Second),
	}

	// The default Kubernetes SD configuration
	DefaultKubernetesSDConfig = KubernetesSDConfig{
		KubeletPort:    10255,
		RequestTimeout: Duration(10 * time.Second),
		RetryInterval:  Duration(1 * time.Second),
	}
)

// This custom URL type allows validating at configuration load time.
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
	GlobalConfig  GlobalConfig    `yaml:"global"`
	RuleFiles     []string        `yaml:"rule_files,omitempty"`
	ScrapeConfigs []*ScrapeConfig `yaml:"scrape_configs,omitempty"`

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

		if scfg.ClientCert != nil {
			scfg.ClientCert.Cert = join(scfg.ClientCert.Cert)
			scfg.ClientCert.Key = join(scfg.ClientCert.Key)
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

// isZero returns true iff the global config is the zero value.
func (c *GlobalConfig) isZero() bool {
	return c.Labels == nil &&
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
	// A set of query parameters with which the target is scraped.
	Params url.Values `yaml:"params,omitempty"`
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
	// The bearer token for the targets.
	BearerToken string `yaml:"bearer_token,omitempty"`
	// The bearer token file for the targets.
	BearerTokenFile string `yaml:"bearer_token_file,omitempty"`
	// The ca cert to use for the targets.
	CACert string `yaml:"ca_cert,omitempty"`
	// The client cert authentication credentials for the targets.
	ClientCert *ClientCert `yaml:"client_cert,omitempty"`
	// HTTP proxy server to use to connect to the targets.
	ProxyURL URL `yaml:"proxy_url,omitempty"`

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
	// MarathonSDConfigs is a list of Marathon service discovery configurations.
	MarathonSDConfigs []*MarathonSDConfig `yaml:"marathon_sd_configs,omitempty"`
	// List of Kubernetes service discovery configurations.
	KubernetesSDConfigs []*KubernetesSDConfig `yaml:"kubernetes_sd_configs,omitempty"`

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
	if len(c.BearerToken) > 0 && len(c.BearerTokenFile) > 0 {
		return fmt.Errorf("at most one of bearer_token & bearer_token_file must be configured")
	}
	if c.BasicAuth != nil && (len(c.BearerToken) > 0 || len(c.BearerTokenFile) > 0) {
		return fmt.Errorf("at most one of basic_auth, bearer_token & bearer_token_file must be configured")
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
	Type            string   `yaml:"type"`
	Port            int      `yaml:"port"` // Ignored for SRV records
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
	switch strings.ToUpper(c.Type) {
	case "SRV":
	case "A", "AAAA":
		if c.Port == 0 {
			return fmt.Errorf("a port is required in DNS-SD configs for all record types except SRV")
		}
	default:
		return fmt.Errorf("invalid DNS-SD records type %s", c.Type)
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
	if strings.TrimSpace(c.Server) == "" {
		return fmt.Errorf("Consul SD configuration requires a server address")
	}
	return checkOverflow(c.XXX, "consul_sd_config")
}

// ServersetSDConfig is the configuration for Twitter serversets in Zookeeper based discovery.
type ServersetSDConfig struct {
	Servers []string `yaml:"servers"`
	Paths   []string `yaml:"paths"`
	Timeout Duration `yaml:"timeout,omitempty"`

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
	return checkOverflow(c.XXX, "serverset_sd_config")
}

// MarathonSDConfig is the configuration for services running on Marathon.
type MarathonSDConfig struct {
	Servers         []string `yaml:"servers,omitempty"`
	RefreshInterval Duration `yaml:"refresh_interval,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// KubernetesSDConfig is the configuration for Kubernetes service discovery.
type KubernetesSDConfig struct {
	Server          string   `yaml:"server"`
	KubeletPort     int      `yaml:"kubelet_port,omitempty"`
	InCluster       bool     `yaml:"in_cluster,omitempty"`
	BearerTokenFile string   `yaml:"bearer_token_file,omitempty"`
	Username        string   `yaml:"username,omitempty"`
	Password        string   `yaml:"password,omitempty"`
	Insecure        bool     `yaml:"insecure,omitempty"`
	CertFile        string   `yaml:"cert_file,omitempty"`
	KeyFile         string   `yaml:"key_file,omitempty"`
	CAFile          string   `yaml:"ca_file,omitempty"`
	RetryInterval   Duration `yaml:"retry_interval,omitempty"`
	RequestTimeout  Duration `yaml:"request_timeout,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *MarathonSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultMarathonSDConfig
	type plain MarathonSDConfig
	if len(c.Servers) == 0 {
		return fmt.Errorf("Marathon SD config must contain at least one Marathon server")
	}

	return checkOverflow(c.XXX, "marathon_sd_config")
}

func (c *KubernetesSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultKubernetesSDConfig
	type plain KubernetesSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Server) == "" {
		return fmt.Errorf("Kubernetes SD configuration requires a server address")
	}
	// Make sure server ends in trailing slash - simpler URL building later
	if !strings.HasSuffix(c.Server, "/") {
		c.Server += "/"
	}

	return checkOverflow(c.XXX, "kubernetes_sd_config")
}

// RelabelAction is the action to be performed on relabeling.
type RelabelAction string

const (
	// Performs a regex replacement.
	RelabelReplace RelabelAction = "replace"
	// Drops targets for which the input does not match the regex.
	RelabelKeep RelabelAction = "keep"
	// Drops targets for which the input does match the regex.
	RelabelDrop RelabelAction = "drop"
	// Sets a label to the modulus of a hash of labels.
	RelabelHashMod RelabelAction = "hashmod"
	// Copy labels to other labelnames based on a regex.
	RelabelLabelMap RelabelAction = "labelmap"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (a *RelabelAction) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	switch act := RelabelAction(strings.ToLower(s)); act {
	case RelabelReplace, RelabelKeep, RelabelDrop, RelabelHashMod:
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
	Regex *Regexp `yaml:"regex,omitempty"`
	// Modulus to take of the hash of concatenated values from the source labels.
	Modulus uint64 `yaml:"modulus,omitempty"`
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
	if c.Regex == nil && c.Action != RelabelHashMod {
		return fmt.Errorf("relabel configuration requires a regular expression")
	}
	if c.Modulus == 0 && c.Action == RelabelHashMod {
		return fmt.Errorf("relabel configuration for hashmod requires non-zero modulus")
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
func (re *Regexp) MarshalYAML() (interface{}, error) {
	if re != nil {
		return re.String(), nil
	}
	return nil, nil
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
