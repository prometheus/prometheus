// Copyright The Prometheus Authors
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
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/units"
	"github.com/grafana/regexp"
	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/otlptranslator"
	"github.com/prometheus/sigv4"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/storage/remote/azuread"
	"github.com/prometheus/prometheus/storage/remote/googleiam"
)

var (
	patRulePath     = regexp.MustCompile(`^[^*]*(\*[^/]*)?$`)
	reservedHeaders = map[string]struct{}{
		// NOTE: authorization is checked specially,
		// see RemoteWriteConfig.UnmarshalYAML.
		// "authorization":                  {},
		"host":                              {},
		"content-encoding":                  {},
		"content-length":                    {},
		"content-type":                      {},
		"user-agent":                        {},
		"connection":                        {},
		"keep-alive":                        {},
		"proxy-authenticate":                {},
		"proxy-authorization":               {},
		"www-authenticate":                  {},
		"accept-encoding":                   {},
		"x-prometheus-remote-write-version": {},
		"x-prometheus-remote-read-version":  {},

		// Added by SigV4.
		"x-amz-date":           {},
		"x-amz-security-token": {},
		"x-amz-content-sha256": {},
	}
)

// Load parses the YAML input s into a Config.
func Load(s string, logger *slog.Logger) (*Config, error) {
	cfg := &Config{}
	// If the entire config body is empty the UnmarshalYAML method is
	// never called. We thus have to set the DefaultConfig at the entry
	// point as well.
	*cfg = DefaultConfig

	err := yaml.UnmarshalStrict([]byte(s), cfg)
	if err != nil {
		return nil, err
	}

	b := labels.NewScratchBuilder(0)
	cfg.GlobalConfig.ExternalLabels.Range(func(v labels.Label) {
		newV := os.Expand(v.Value, func(s string) string {
			if s == "$" {
				return "$"
			}
			if v := os.Getenv(s); v != "" {
				return v
			}
			logger.Warn("Empty environment variable", "name", s)
			return ""
		})
		if newV != v.Value {
			logger.Debug("External label replaced", "label", v.Name, "input", v.Value, "output", newV)
		}
		// Note newV can be blank. https://github.com/prometheus/prometheus/issues/11024
		b.Add(v.Name, newV)
	})
	if !b.Labels().IsEmpty() {
		cfg.GlobalConfig.ExternalLabels = b.Labels()
	}

	switch cfg.OTLPConfig.TranslationStrategy {
	case otlptranslator.UnderscoreEscapingWithSuffixes, otlptranslator.UnderscoreEscapingWithoutSuffixes:
	case "":
	case otlptranslator.NoTranslation, otlptranslator.NoUTF8EscapingWithSuffixes:
		if cfg.GlobalConfig.MetricNameValidationScheme == model.LegacyValidation {
			return nil, fmt.Errorf("OTLP translation strategy %q is not allowed when UTF8 is disabled", cfg.OTLPConfig.TranslationStrategy)
		}
	default:
		return nil, fmt.Errorf("unsupported OTLP translation strategy %q", cfg.OTLPConfig.TranslationStrategy)
	}
	cfg.loaded = true
	return cfg, nil
}

// LoadFile parses and validates the given YAML file into a read-only Config.
// Callers should never write to or shallow copy the returned Config.
func LoadFile(filename string, agentMode bool, logger *slog.Logger) (*Config, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(string(content), logger)
	if err != nil {
		return nil, fmt.Errorf("parsing YAML file %s: %w", filename, err)
	}

	if agentMode {
		if len(cfg.AlertingConfig.AlertmanagerConfigs) > 0 || len(cfg.AlertingConfig.AlertRelabelConfigs) > 0 {
			return nil, errors.New("field alerting is not allowed in agent mode")
		}

		if len(cfg.RuleFiles) > 0 {
			return nil, errors.New("field rule_files is not allowed in agent mode")
		}

		if len(cfg.RemoteReadConfigs) > 0 {
			return nil, errors.New("field remote_read is not allowed in agent mode")
		}
	}

	cfg.SetDirectory(filepath.Dir(filename))
	return cfg, nil
}

func boolPtr(b bool) *bool {
	return &b
}

// The defaults applied before parsing the respective config sections.
var (
	// DefaultConfig is the default top-level configuration.
	DefaultConfig = Config{
		GlobalConfig: DefaultGlobalConfig,
		Runtime:      DefaultRuntimeConfig,
		OTLPConfig:   DefaultOTLPConfig,
	}

	// DefaultGlobalConfig is the default global configuration.
	DefaultGlobalConfig = GlobalConfig{
		ScrapeInterval:     model.Duration(1 * time.Minute),
		ScrapeTimeout:      model.Duration(10 * time.Second),
		EvaluationInterval: model.Duration(1 * time.Minute),
		RuleQueryOffset:    model.Duration(0 * time.Minute),
		// This is nil to be able to distinguish between the case when
		// the normal default should be used and the case when a
		// new default is needed due to an enabled feature flag.
		// E.g. set to `DefaultProtoFirstScrapeProtocols` when
		// the feature flag `created-timestamp-zero-ingestion` is set.
		ScrapeProtocols: nil,
		// When the native histogram feature flag is enabled,
		// ScrapeNativeHistograms default changes to true.
		ScrapeNativeHistograms:         boolPtr(false),
		ConvertClassicHistogramsToNHCB: false,
		AlwaysScrapeClassicHistograms:  false,
		ExtraScrapeMetrics:             boolPtr(false),
		MetricNameValidationScheme:     model.UTF8Validation,
		MetricNameEscapingScheme:       model.AllowUTF8,
	}

	DefaultRuntimeConfig = RuntimeConfig{
		// Go runtime tuning.
		GoGC: getGoGC(),
	}

	// DefaultScrapeConfig is the default scrape configuration. Users of this
	// default MUST call Validate() on the config after creation, even if it's
	// used unaltered, to check for parameter correctness and fill out default
	// values that can't be set inline in this declaration.
	DefaultScrapeConfig = ScrapeConfig{
		// ScrapeTimeout, ScrapeInterval, ScrapeProtocols, AlwaysScrapeClassicHistograms, and ConvertClassicHistogramsToNHCB default to the configured globals.
		MetricsPath:       "/metrics",
		Scheme:            "http",
		HonorLabels:       false,
		HonorTimestamps:   true,
		HTTPClientConfig:  config.DefaultHTTPClientConfig,
		EnableCompression: true,
	}

	// DefaultAlertmanagerConfig is the default alertmanager configuration.
	DefaultAlertmanagerConfig = AlertmanagerConfig{
		Scheme:           "http",
		Timeout:          model.Duration(10 * time.Second),
		APIVersion:       AlertmanagerAPIVersionV2,
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}

	DefaultRemoteWriteHTTPClientConfig = config.HTTPClientConfig{
		FollowRedirects: true,
		EnableHTTP2:     false,
	}

	// DefaultRemoteWriteConfig is the default remote write configuration.
	DefaultRemoteWriteConfig = RemoteWriteConfig{
		RemoteTimeout:    model.Duration(30 * time.Second),
		ProtobufMessage:  remoteapi.WriteV1MessageType,
		QueueConfig:      DefaultQueueConfig,
		MetadataConfig:   DefaultMetadataConfig,
		HTTPClientConfig: DefaultRemoteWriteHTTPClientConfig,
	}

	// DefaultQueueConfig is the default remote queue configuration.
	DefaultQueueConfig = QueueConfig{
		// With a maximum of 50 shards, assuming an average of 100ms remote write
		// time and 2000 samples per batch, we will be able to push 1M samples/s.
		MaxShards:         50,
		MinShards:         1,
		MaxSamplesPerSend: 2000,

		// Each shard will have a max of 10,000 samples pending in its channel, plus the pending
		// samples that have been enqueued. Theoretically we should only ever have about 12,000 samples
		// per shard pending. At 50 shards that's 600k.
		Capacity:          10000,
		BatchSendDeadline: model.Duration(5 * time.Second),

		// Backoff times for retrying a batch of samples on recoverable errors.
		MinBackoff: model.Duration(30 * time.Millisecond),
		MaxBackoff: model.Duration(5 * time.Second),
	}

	// DefaultMetadataConfig is the default metadata configuration for a remote write endpoint.
	DefaultMetadataConfig = MetadataConfig{
		Send:              true,
		SendInterval:      model.Duration(1 * time.Minute),
		MaxSamplesPerSend: 2000,
	}

	// DefaultRemoteReadConfig is the default remote read configuration.
	DefaultRemoteReadConfig = RemoteReadConfig{
		RemoteTimeout:        model.Duration(1 * time.Minute),
		ChunkedReadLimit:     DefaultChunkedReadLimit,
		HTTPClientConfig:     config.DefaultHTTPClientConfig,
		FilterExternalLabels: true,
	}

	// DefaultStorageConfig is the default TSDB/Exemplar storage configuration.
	DefaultStorageConfig = StorageConfig{
		ExemplarsConfig: &DefaultExemplarsConfig,
	}

	DefaultExemplarsConfig = ExemplarsConfig{
		MaxExemplars: 100000,
	}

	// DefaultOTLPConfig is the default OTLP configuration.
	DefaultOTLPConfig = OTLPConfig{
		TranslationStrategy: otlptranslator.UnderscoreEscapingWithSuffixes,
		// For backwards compatibility.
		LabelNameUnderscoreSanitization: true,
		// For backwards compatibility.
		LabelNamePreserveMultipleUnderscores: true,
	}
)

// Config is the top-level configuration for Prometheus's config files.
type Config struct {
	GlobalConfig      GlobalConfig    `yaml:"global"`
	Runtime           RuntimeConfig   `yaml:"runtime,omitempty"`
	AlertingConfig    AlertingConfig  `yaml:"alerting,omitempty"`
	RuleFiles         []string        `yaml:"rule_files,omitempty"`
	ScrapeConfigFiles []string        `yaml:"scrape_config_files,omitempty"`
	ScrapeConfigs     []*ScrapeConfig `yaml:"scrape_configs,omitempty"`
	StorageConfig     StorageConfig   `yaml:"storage,omitempty"`
	TracingConfig     TracingConfig   `yaml:"tracing,omitempty"`

	RemoteWriteConfigs []*RemoteWriteConfig `yaml:"remote_write,omitempty"`
	RemoteReadConfigs  []*RemoteReadConfig  `yaml:"remote_read,omitempty"`
	OTLPConfig         OTLPConfig           `yaml:"otlp,omitempty"`

	loaded bool // Certain methods require configuration to use Load validation.
}

// SetDirectory joins any relative file paths with dir.
// This method writes to config, and it's not concurrency safe.
func (c *Config) SetDirectory(dir string) {
	c.GlobalConfig.SetDirectory(dir)
	c.AlertingConfig.SetDirectory(dir)
	c.TracingConfig.SetDirectory(dir)
	for i, file := range c.RuleFiles {
		c.RuleFiles[i] = config.JoinDir(dir, file)
	}
	for i, file := range c.ScrapeConfigFiles {
		c.ScrapeConfigFiles[i] = config.JoinDir(dir, file)
	}
	for _, c := range c.ScrapeConfigs {
		c.SetDirectory(dir)
	}
	for _, c := range c.RemoteWriteConfigs {
		c.SetDirectory(dir)
	}
	for _, c := range c.RemoteReadConfigs {
		c.SetDirectory(dir)
	}
}

func (c Config) String() string {
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// GetScrapeConfigs returns the read-only, validated scrape configurations including
// the ones from the scrape_config_files.
// This method does not write to config, and it's concurrency safe (the pointer receiver is for efficiency).
// This method also assumes the Config was created by Load or LoadFile function, it returns error
// if it was not. We can't re-validate or apply globals here due to races,
// read more https://github.com/prometheus/prometheus/issues/15538.
func (c *Config) GetScrapeConfigs() ([]*ScrapeConfig, error) {
	if !c.loaded {
		// Programmatic error, we warn before more confusing errors would happen due to lack of the globalization.
		return nil, errors.New("scrape config cannot be fetched, main config was not validated and loaded correctly; should not happen")
	}

	scfgs := make([]*ScrapeConfig, len(c.ScrapeConfigs))
	jobNames := map[string]string{}
	for i, scfg := range c.ScrapeConfigs {
		jobNames[scfg.JobName] = "main config file"
		scfgs[i] = scfg
	}

	// Re-read and validate the dynamic scrape config rules.
	for _, pat := range c.ScrapeConfigFiles {
		fs, err := filepath.Glob(pat)
		if err != nil {
			// The only error can be a bad pattern.
			return nil, fmt.Errorf("error retrieving scrape config files for %q: %w", pat, err)
		}
		for _, filename := range fs {
			cfg := ScrapeConfigs{}
			content, err := os.ReadFile(filename)
			if err != nil {
				return nil, fileErr(filename, err)
			}
			err = yaml.UnmarshalStrict(content, &cfg)
			if err != nil {
				return nil, fileErr(filename, err)
			}
			for _, scfg := range cfg.ScrapeConfigs {
				if err := scfg.Validate(c.GlobalConfig); err != nil {
					return nil, fileErr(filename, err)
				}

				if f, ok := jobNames[scfg.JobName]; ok {
					return nil, fileErr(filename, fmt.Errorf("found multiple scrape configs with job name %q, first found in %s", scfg.JobName, f))
				}
				jobNames[scfg.JobName] = fmt.Sprintf("%q", filePath(filename))

				scfg.SetDirectory(filepath.Dir(filename))
				scfgs = append(scfgs, scfg)
			}
		}
	}
	return scfgs, nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
// NOTE: This method should not be used outside of this package. Use Load or LoadFile instead.
func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
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

	// If a runtime block was open but empty the default runtime config is overwritten.
	// We have to restore it here.
	if c.Runtime.isZero() {
		c.Runtime = DefaultRuntimeConfig
	}

	for _, rf := range c.RuleFiles {
		if !patRulePath.MatchString(rf) {
			return fmt.Errorf("invalid rule file path %q", rf)
		}
	}

	for _, sf := range c.ScrapeConfigFiles {
		if !patRulePath.MatchString(sf) {
			return fmt.Errorf("invalid scrape config file path %q", sf)
		}
	}

	// Do global overrides and validation.
	jobNames := map[string]struct{}{}
	for _, scfg := range c.ScrapeConfigs {
		if err := scfg.Validate(c.GlobalConfig); err != nil {
			return err
		}
		if _, ok := jobNames[scfg.JobName]; ok {
			return fmt.Errorf("found multiple scrape configs with job name %q", scfg.JobName)
		}
		jobNames[scfg.JobName] = struct{}{}
	}

	if err := c.AlertingConfig.Validate(c.GlobalConfig.MetricNameValidationScheme); err != nil {
		return err
	}

	rwNames := map[string]struct{}{}
	for _, rwcfg := range c.RemoteWriteConfigs {
		if rwcfg == nil {
			return errors.New("empty or null remote write config section")
		}
		// Skip empty names, we fill their name with their config hash in remote write code.
		if _, ok := rwNames[rwcfg.Name]; ok && rwcfg.Name != "" {
			return fmt.Errorf("found multiple remote write configs with job name %q", rwcfg.Name)
		}
		if err := rwcfg.Validate(c.GlobalConfig.MetricNameValidationScheme); err != nil {
			return err
		}
		rwNames[rwcfg.Name] = struct{}{}
	}
	rrNames := map[string]struct{}{}
	for _, rrcfg := range c.RemoteReadConfigs {
		if rrcfg == nil {
			return errors.New("empty or null remote read config section")
		}
		// Skip empty names, we fill their name with their config hash in remote read code.
		if _, ok := rrNames[rrcfg.Name]; ok && rrcfg.Name != "" {
			return fmt.Errorf("found multiple remote read configs with job name %q", rrcfg.Name)
		}
		rrNames[rrcfg.Name] = struct{}{}
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
	// The protocols to negotiate during a scrape. It tells clients what
	// protocol are accepted by Prometheus and with what weight (most wanted is first).
	// Supported values (case sensitive): PrometheusProto, OpenMetricsText0.0.1,
	// OpenMetricsText1.0.0, PrometheusText1.0.0, PrometheusText0.0.4
	ScrapeProtocols []ScrapeProtocol `yaml:"scrape_protocols,omitempty"`
	// How frequently to evaluate rules by default.
	EvaluationInterval model.Duration `yaml:"evaluation_interval,omitempty"`
	// Offset the rule evaluation timestamp of this particular group by the specified duration into the past to ensure the underlying metrics have been received.
	RuleQueryOffset model.Duration `yaml:"rule_query_offset,omitempty"`
	// File to which PromQL queries are logged.
	QueryLogFile string `yaml:"query_log_file,omitempty"`
	// File to which scrape failures are logged.
	ScrapeFailureLogFile string `yaml:"scrape_failure_log_file,omitempty"`
	// The labels to add to any timeseries that this Prometheus instance scrapes.
	ExternalLabels labels.Labels `yaml:"external_labels,omitempty"`
	// An uncompressed response body larger than this many bytes will cause the
	// scrape to fail. 0 means no limit.
	BodySizeLimit units.Base2Bytes `yaml:"body_size_limit,omitempty"`
	// More than this many samples post metric-relabeling will cause the scrape to
	// fail. 0 means no limit.
	SampleLimit uint `yaml:"sample_limit,omitempty"`
	// More than this many targets after the target relabeling will cause the
	// scrapes to fail. 0 means no limit.
	TargetLimit uint `yaml:"target_limit,omitempty"`
	// More than this many labels post metric-relabeling will cause the scrape to
	// fail. 0 means no limit.
	LabelLimit uint `yaml:"label_limit,omitempty"`
	// More than this label name length post metric-relabeling will cause the
	// scrape to fail. 0 means no limit.
	LabelNameLengthLimit uint `yaml:"label_name_length_limit,omitempty"`
	// More than this label value length post metric-relabeling will cause the
	// scrape to fail. 0 means no limit.
	LabelValueLengthLimit uint `yaml:"label_value_length_limit,omitempty"`
	// Keep no more than this many dropped targets per job.
	// 0 means no limit.
	KeepDroppedTargets uint `yaml:"keep_dropped_targets,omitempty"`
	// Allow UTF8 Metric and Label Names. Can be blank in config files but must
	// have a value if a GlobalConfig is created programmatically.
	MetricNameValidationScheme model.ValidationScheme `yaml:"metric_name_validation_scheme,omitempty"`
	// Metric name escaping mode to request through content negotiation. Can be
	// blank in config files but must have a value if a ScrapeConfig is created
	// programmatically.
	MetricNameEscapingScheme string `yaml:"metric_name_escaping_scheme,omitempty"`
	// Whether to scrape native histograms.
	ScrapeNativeHistograms *bool `yaml:"scrape_native_histograms,omitempty"`
	// Whether to convert all scraped classic histograms into native histograms with custom buckets.
	ConvertClassicHistogramsToNHCB bool `yaml:"convert_classic_histograms_to_nhcb,omitempty"`
	// Whether to scrape a classic histogram, even if it is also exposed as a native histogram.
	AlwaysScrapeClassicHistograms bool `yaml:"always_scrape_classic_histograms,omitempty"`
	// Whether to enable additional scrape metrics.
	// When enabled, Prometheus stores samples for scrape_timeout_seconds,
	// scrape_sample_limit, and scrape_body_size_bytes.
	ExtraScrapeMetrics *bool `yaml:"extra_scrape_metrics,omitempty"`
}

// ScrapeProtocol represents supported protocol for scraping metrics.
type ScrapeProtocol string

// Validate returns error if given scrape protocol is not supported.
func (s ScrapeProtocol) Validate() error {
	if _, ok := ScrapeProtocolsHeaders[s]; !ok {
		return fmt.Errorf("unknown scrape protocol %v, supported: %v",
			s, func() (ret []string) {
				for k := range ScrapeProtocolsHeaders {
					ret = append(ret, string(k))
				}
				sort.Strings(ret)
				return ret
			}())
	}
	return nil
}

// HeaderMediaType returns the MIME mediaType for a particular ScrapeProtocol.
func (s ScrapeProtocol) HeaderMediaType() string {
	if _, ok := ScrapeProtocolsHeaders[s]; !ok {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(ScrapeProtocolsHeaders[s])
	if err != nil {
		return ""
	}
	return mediaType
}

var (
	PrometheusProto      ScrapeProtocol = "PrometheusProto"
	PrometheusText0_0_4  ScrapeProtocol = "PrometheusText0.0.4"
	PrometheusText1_0_0  ScrapeProtocol = "PrometheusText1.0.0"
	OpenMetricsText0_0_1 ScrapeProtocol = "OpenMetricsText0.0.1"
	OpenMetricsText1_0_0 ScrapeProtocol = "OpenMetricsText1.0.0"
	UTF8NamesHeader      string         = model.EscapingKey + "=" + model.AllowUTF8

	ScrapeProtocolsHeaders = map[ScrapeProtocol]string{
		PrometheusProto:      "application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited",
		PrometheusText0_0_4:  "text/plain;version=0.0.4",
		PrometheusText1_0_0:  "text/plain;version=1.0.0",
		OpenMetricsText0_0_1: "application/openmetrics-text;version=0.0.1",
		OpenMetricsText1_0_0: "application/openmetrics-text;version=1.0.0",
	}

	// DefaultScrapeProtocols is the set of scrape protocols that will be proposed
	// to scrape target, ordered by priority.
	DefaultScrapeProtocols = []ScrapeProtocol{
		OpenMetricsText1_0_0,
		OpenMetricsText0_0_1,
		PrometheusText1_0_0,
		PrometheusText0_0_4,
	}

	// DefaultProtoFirstScrapeProtocols is like DefaultScrapeProtocols, but it
	// favors protobuf Prometheus exposition format.
	// Used by default by the "scrape_native_histograms" option and for certain
	// feature-flags like "created-timestamp-zero-ingestion".
	DefaultProtoFirstScrapeProtocols = []ScrapeProtocol{
		PrometheusProto,
		OpenMetricsText1_0_0,
		OpenMetricsText0_0_1,
		PrometheusText1_0_0,
		PrometheusText0_0_4,
	}
)

// validateAcceptScrapeProtocols return errors if we see problems with accept scrape protocols option.
func validateAcceptScrapeProtocols(sps []ScrapeProtocol) error {
	if len(sps) == 0 {
		return errors.New("scrape_protocols cannot be empty")
	}
	dups := map[string]struct{}{}
	for _, sp := range sps {
		if _, ok := dups[strings.ToLower(string(sp))]; ok {
			return fmt.Errorf("duplicated protocol in scrape_protocols, got %v", sps)
		}
		if err := sp.Validate(); err != nil {
			return fmt.Errorf("scrape_protocols: %w", err)
		}
		dups[strings.ToLower(string(sp))] = struct{}{}
	}
	return nil
}

// SetDirectory joins any relative file paths with dir.
func (c *GlobalConfig) SetDirectory(dir string) {
	c.QueryLogFile = config.JoinDir(dir, c.QueryLogFile)
	c.ScrapeFailureLogFile = config.JoinDir(dir, c.ScrapeFailureLogFile)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *GlobalConfig) UnmarshalYAML(unmarshal func(any) error) error {
	// Create a clean global config as the previous one was already populated
	// by the default due to the YAML parser behavior for empty blocks.
	gc := &GlobalConfig{}
	type plain GlobalConfig
	if err := unmarshal((*plain)(gc)); err != nil {
		return err
	}

	switch gc.MetricNameValidationScheme {
	case model.UTF8Validation, model.LegacyValidation:
	default:
		gc.MetricNameValidationScheme = DefaultGlobalConfig.MetricNameValidationScheme
	}

	if err := gc.ExternalLabels.Validate(func(l labels.Label) error {
		if !gc.MetricNameValidationScheme.IsValidLabelName(l.Name) {
			return fmt.Errorf("%q is not a valid label name", l.Name)
		}
		if !model.LabelValue(l.Value).IsValid() {
			return fmt.Errorf("%q is not a valid label value", l.Value)
		}
		return nil
	}); err != nil {
		return err
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
		gc.ScrapeTimeout = min(DefaultGlobalConfig.ScrapeTimeout, gc.ScrapeInterval)
	}
	if gc.EvaluationInterval == 0 {
		gc.EvaluationInterval = DefaultGlobalConfig.EvaluationInterval
	}
	if gc.ScrapeNativeHistograms == nil {
		gc.ScrapeNativeHistograms = DefaultGlobalConfig.ScrapeNativeHistograms
	}
	if gc.ExtraScrapeMetrics == nil {
		gc.ExtraScrapeMetrics = DefaultGlobalConfig.ExtraScrapeMetrics
	}
	if gc.ScrapeProtocols == nil {
		if DefaultGlobalConfig.ScrapeProtocols != nil {
			// This is the case where the defaults are set due to a feature flag.
			// E.g. if the created-timestamp-zero-ingestion feature flag is
			// used.
			gc.ScrapeProtocols = DefaultGlobalConfig.ScrapeProtocols
		}
		// Otherwise, we leave ScrapeProtocols at nil for now. In the
		// per-job scrape config, we have to recognize the unset case to
		// correctly set the default depending on the local value of
		// ScrapeNativeHistograms.
	}
	if gc.ScrapeProtocols != nil {
		// Only validate if not-nil at this point.
		if err := validateAcceptScrapeProtocols(gc.ScrapeProtocols); err != nil {
			return fmt.Errorf("%w for global config", err)
		}
	}

	*c = *gc
	return nil
}

// isZero returns true iff the global config is the zero value.
func (c *GlobalConfig) isZero() bool {
	return c.ExternalLabels.IsEmpty() &&
		c.ScrapeInterval == 0 &&
		c.ScrapeTimeout == 0 &&
		c.EvaluationInterval == 0 &&
		c.RuleQueryOffset == 0 &&
		c.QueryLogFile == "" &&
		c.ScrapeFailureLogFile == "" &&
		c.ScrapeProtocols == nil &&
		c.ScrapeNativeHistograms == nil &&
		!c.ConvertClassicHistogramsToNHCB &&
		!c.AlwaysScrapeClassicHistograms &&
		c.BodySizeLimit == 0 &&
		c.SampleLimit == 0 &&
		c.TargetLimit == 0 &&
		c.LabelLimit == 0 &&
		c.LabelNameLengthLimit == 0 &&
		c.LabelValueLengthLimit == 0 &&
		c.KeepDroppedTargets == 0 &&
		c.MetricNameValidationScheme == model.UnsetValidation &&
		c.MetricNameEscapingScheme == "" &&
		c.ExtraScrapeMetrics == nil
}

const DefaultGoGCPercentage = 75

// RuntimeConfig configures the values for the process behavior.
type RuntimeConfig struct {
	// The Go garbage collection target percentage.
	GoGC int `yaml:"gogc,omitempty"`

	// Below are guidelines for adding a new field:
	//
	// For config that shouldn't change after startup, you might want to use
	// flags https://prometheus.io/docs/prometheus/latest/command-line/prometheus/.
	//
	// Consider when the new field is first applied: at the very beginning of instance
	// startup, after the TSDB is loaded etc. See https://github.com/prometheus/prometheus/pull/16491
	// for an example.
	//
	// Provide a test covering various scenarios: empty config file, empty or incomplete runtime
	// config block, precedence over other inputs (e.g., env vars, if applicable) etc.
	// See TestRuntimeGOGCConfig (or https://github.com/prometheus/prometheus/pull/15238).
	// The test should also verify behavior on reloads, since this config should be
	// adjustable at runtime.
}

// isZero returns true iff the global config is the zero value.
func (c *RuntimeConfig) isZero() bool {
	return c.GoGC == 0
}

type ScrapeConfigs struct {
	ScrapeConfigs []*ScrapeConfig `yaml:"scrape_configs,omitempty"`
}

// ScrapeConfig configures a scraping unit for Prometheus.
type ScrapeConfig struct {
	// The job name to which the job label is set by default.
	JobName string `yaml:"job_name"`
	// Indicator whether the scraped metrics should remain unmodified.
	HonorLabels bool `yaml:"honor_labels,omitempty"`
	// Indicator whether the scraped timestamps should be respected.
	HonorTimestamps bool `yaml:"honor_timestamps"`
	// Indicator whether to track the staleness of the scraped timestamps.
	TrackTimestampsStaleness bool `yaml:"track_timestamps_staleness"`
	// A set of query parameters with which the target is scraped.
	Params url.Values `yaml:"params,omitempty"`
	// How frequently to scrape the targets of this scrape config.
	ScrapeInterval model.Duration `yaml:"scrape_interval,omitempty"`
	// The timeout for scraping targets of this config.
	ScrapeTimeout model.Duration `yaml:"scrape_timeout,omitempty"`
	// The protocols to negotiate during a scrape. It tells clients what
	// protocol are accepted by Prometheus and with what preference (most wanted is first).
	// Supported values (case sensitive): PrometheusProto, OpenMetricsText0.0.1,
	// OpenMetricsText1.0.0, PrometheusText1.0.0, PrometheusText0.0.4.
	ScrapeProtocols []ScrapeProtocol `yaml:"scrape_protocols,omitempty"`
	// The fallback protocol to use if the Content-Type provided by the target
	// is not provided, blank, or not one of the expected values.
	// Supported values (case sensitive): PrometheusProto, OpenMetricsText0.0.1,
	// OpenMetricsText1.0.0, PrometheusText1.0.0, PrometheusText0.0.4.
	ScrapeFallbackProtocol ScrapeProtocol `yaml:"fallback_scrape_protocol,omitempty"`
	// Whether to scrape native histograms.
	ScrapeNativeHistograms *bool `yaml:"scrape_native_histograms,omitempty"`
	// Whether to scrape a classic histogram, even if it is also exposed as a native histogram.
	AlwaysScrapeClassicHistograms *bool `yaml:"always_scrape_classic_histograms,omitempty"`
	// Whether to convert all scraped classic histograms into a native histogram with custom buckets.
	ConvertClassicHistogramsToNHCB *bool `yaml:"convert_classic_histograms_to_nhcb,omitempty"`
	// File to which scrape failures are logged.
	ScrapeFailureLogFile string `yaml:"scrape_failure_log_file,omitempty"`
	// The HTTP resource path on which to fetch metrics from targets.
	MetricsPath string `yaml:"metrics_path,omitempty"`
	// The URL scheme with which to fetch metrics from targets.
	Scheme string `yaml:"scheme,omitempty"`
	// Indicator whether to request compressed response from the target.
	EnableCompression bool `yaml:"enable_compression"`
	// An uncompressed response body larger than this many bytes will cause the
	// scrape to fail. 0 means no limit.
	BodySizeLimit units.Base2Bytes `yaml:"body_size_limit,omitempty"`
	// More than this many samples post metric-relabeling will cause the scrape to
	// fail. 0 means no limit.
	SampleLimit uint `yaml:"sample_limit,omitempty"`
	// More than this many targets after the target relabeling will cause the
	// scrapes to fail. 0 means no limit.
	TargetLimit uint `yaml:"target_limit,omitempty"`
	// More than this many labels post metric-relabeling will cause the scrape to
	// fail. 0 means no limit.
	LabelLimit uint `yaml:"label_limit,omitempty"`
	// More than this label name length post metric-relabeling will cause the
	// scrape to fail. 0 means no limit.
	LabelNameLengthLimit uint `yaml:"label_name_length_limit,omitempty"`
	// More than this label value length post metric-relabeling will cause the
	// scrape to fail. 0 means no limit.
	LabelValueLengthLimit uint `yaml:"label_value_length_limit,omitempty"`
	// If there are more than this many buckets in a native histogram,
	// buckets will be merged to stay within the limit.
	NativeHistogramBucketLimit uint `yaml:"native_histogram_bucket_limit,omitempty"`
	// If the growth factor of one bucket to the next is smaller than this,
	// buckets will be merged to increase the factor sufficiently.
	NativeHistogramMinBucketFactor float64 `yaml:"native_histogram_min_bucket_factor,omitempty"`
	// Keep no more than this many dropped targets per job.
	// 0 means no limit.
	KeepDroppedTargets uint `yaml:"keep_dropped_targets,omitempty"`
	// Allow UTF8 Metric and Label Names. Can be blank in config files but must
	// have a value if a ScrapeConfig is created programmatically.
	MetricNameValidationScheme model.ValidationScheme `yaml:"metric_name_validation_scheme,omitempty"`
	// Metric name escaping mode to request through content negotiation. Can be
	// blank in config files but must have a value if a ScrapeConfig is created
	// programmatically.
	MetricNameEscapingScheme string `yaml:"metric_name_escaping_scheme,omitempty"`
	// Whether to enable additional scrape metrics.
	// When enabled, Prometheus stores samples for scrape_timeout_seconds,
	// scrape_sample_limit, and scrape_body_size_bytes.
	// If not set (nil), inherits the value from the global configuration.
	ExtraScrapeMetrics *bool `yaml:"extra_scrape_metrics,omitempty"`

	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.

	ServiceDiscoveryConfigs discovery.Configs       `yaml:"-"`
	HTTPClientConfig        config.HTTPClientConfig `yaml:",inline"`

	// List of target relabel configurations.
	RelabelConfigs []*relabel.Config `yaml:"relabel_configs,omitempty"`
	// List of metric relabel configurations.
	MetricRelabelConfigs []*relabel.Config `yaml:"metric_relabel_configs,omitempty"`
}

// SetDirectory joins any relative file paths with dir.
func (c *ScrapeConfig) SetDirectory(dir string) {
	c.ServiceDiscoveryConfigs.SetDirectory(dir)
	c.HTTPClientConfig.SetDirectory(dir)
	c.ScrapeFailureLogFile = config.JoinDir(dir, c.ScrapeFailureLogFile)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ScrapeConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultScrapeConfig
	if err := discovery.UnmarshalYAMLWithInlineConfigs(c, unmarshal); err != nil {
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

	// Check for users putting URLs in target groups.
	if len(c.RelabelConfigs) == 0 {
		if err := checkStaticTargets(c.ServiceDiscoveryConfigs); err != nil {
			return err
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

	return nil
}

// Validate validates scrape config, but also fills relevant default values from global config if needed.
func (c *ScrapeConfig) Validate(globalConfig GlobalConfig) error {
	if c == nil {
		return errors.New("empty or null scrape config section")
	}
	// First set the correct scrape interval, then check that the timeout
	// (inferred or explicit) is not greater than that.
	if c.ScrapeInterval == 0 {
		c.ScrapeInterval = globalConfig.ScrapeInterval
	}
	if c.ScrapeTimeout > c.ScrapeInterval {
		return fmt.Errorf("scrape timeout greater than scrape interval for scrape config with job name %q", c.JobName)
	}
	if c.ScrapeTimeout == 0 {
		c.ScrapeTimeout = min(globalConfig.ScrapeTimeout, c.ScrapeInterval)
	}
	if c.BodySizeLimit == 0 {
		c.BodySizeLimit = globalConfig.BodySizeLimit
	}
	if c.SampleLimit == 0 {
		c.SampleLimit = globalConfig.SampleLimit
	}
	if c.TargetLimit == 0 {
		c.TargetLimit = globalConfig.TargetLimit
	}
	if c.LabelLimit == 0 {
		c.LabelLimit = globalConfig.LabelLimit
	}
	if c.LabelNameLengthLimit == 0 {
		c.LabelNameLengthLimit = globalConfig.LabelNameLengthLimit
	}
	if c.LabelValueLengthLimit == 0 {
		c.LabelValueLengthLimit = globalConfig.LabelValueLengthLimit
	}
	if c.KeepDroppedTargets == 0 {
		c.KeepDroppedTargets = globalConfig.KeepDroppedTargets
	}
	if c.ScrapeFailureLogFile == "" {
		c.ScrapeFailureLogFile = globalConfig.ScrapeFailureLogFile
	}
	if c.ScrapeNativeHistograms == nil {
		c.ScrapeNativeHistograms = globalConfig.ScrapeNativeHistograms
	}
	if c.ExtraScrapeMetrics == nil {
		c.ExtraScrapeMetrics = globalConfig.ExtraScrapeMetrics
	}

	if c.ScrapeProtocols == nil {
		switch {
		case globalConfig.ScrapeProtocols != nil:
			// global ScrapeProtocols either set explicitly or via a
			// default triggered by a feature flag. This overrides
			// the selection based on locally active scraping of
			// native histograms.
			c.ScrapeProtocols = globalConfig.ScrapeProtocols
		case c.ScrapeNativeHistogramsEnabled():
			c.ScrapeProtocols = DefaultProtoFirstScrapeProtocols
		default:
			c.ScrapeProtocols = DefaultScrapeProtocols
		}
	}
	if err := validateAcceptScrapeProtocols(c.ScrapeProtocols); err != nil {
		return fmt.Errorf("%w for scrape config with job name %q", err, c.JobName)
	}

	if c.ScrapeFallbackProtocol != "" {
		if err := c.ScrapeFallbackProtocol.Validate(); err != nil {
			return fmt.Errorf("invalid fallback_scrape_protocol for scrape config with job name %q: %w", c.JobName, err)
		}
	}

	//nolint:staticcheck
	if model.NameValidationScheme != model.UTF8Validation {
		return errors.New("model.NameValidationScheme must be set to UTF8")
	}

	switch globalConfig.MetricNameValidationScheme {
	case model.LegacyValidation, model.UTF8Validation:
	default:
		return errors.New("global name validation method must be set")
	}
	// Scrapeconfig validation scheme matches global if left blank.
	localValidationUnset := false
	switch c.MetricNameValidationScheme {
	case model.UnsetValidation:
		localValidationUnset = true
		c.MetricNameValidationScheme = globalConfig.MetricNameValidationScheme
	case model.LegacyValidation, model.UTF8Validation:
	default:
		return fmt.Errorf("unknown scrape config name validation method specified, must be either '', 'legacy' or 'utf8', got %s", c.MetricNameValidationScheme)
	}

	// Escaping scheme is based on the validation scheme if left blank.
	switch globalConfig.MetricNameEscapingScheme {
	case "":
		if globalConfig.MetricNameValidationScheme == model.LegacyValidation {
			globalConfig.MetricNameEscapingScheme = model.EscapeUnderscores
		} else {
			globalConfig.MetricNameEscapingScheme = model.AllowUTF8
		}
	case model.AllowUTF8, model.EscapeUnderscores, model.EscapeDots, model.EscapeValues:
	default:
		return fmt.Errorf("unknown global name escaping method specified, must be one of '%s', '%s', '%s', or '%s', got %q", model.AllowUTF8, model.EscapeUnderscores, model.EscapeDots, model.EscapeValues, globalConfig.MetricNameEscapingScheme)
	}

	// Similarly, if ScrapeConfig escaping scheme is blank, infer it from the
	// ScrapeConfig validation scheme if that was set, or the Global validation
	// scheme if the ScrapeConfig validation scheme was also not set. This ensures
	// that local ScrapeConfigs that only specify Legacy validation do not inherit
	// the global AllowUTF8 escaping setting, which is an error.
	if c.MetricNameEscapingScheme == "" {
		//nolint:gocritic
		if localValidationUnset {
			c.MetricNameEscapingScheme = globalConfig.MetricNameEscapingScheme
		} else if c.MetricNameValidationScheme == model.LegacyValidation {
			c.MetricNameEscapingScheme = model.EscapeUnderscores
		} else {
			c.MetricNameEscapingScheme = model.AllowUTF8
		}
	}

	switch c.MetricNameEscapingScheme {
	case model.AllowUTF8:
		if c.MetricNameValidationScheme != model.UTF8Validation {
			return errors.New("utf8 metric names requested but validation scheme is not set to UTF8")
		}
	case model.EscapeUnderscores, model.EscapeDots, model.EscapeValues:
	default:
		return fmt.Errorf("unknown scrape config name escaping method specified, must be one of '%s', '%s', '%s', or '%s', got %q", model.AllowUTF8, model.EscapeUnderscores, model.EscapeDots, model.EscapeValues, c.MetricNameEscapingScheme)
	}

	if c.ConvertClassicHistogramsToNHCB == nil {
		global := globalConfig.ConvertClassicHistogramsToNHCB
		c.ConvertClassicHistogramsToNHCB = &global
	}

	if c.AlwaysScrapeClassicHistograms == nil {
		global := globalConfig.AlwaysScrapeClassicHistograms
		c.AlwaysScrapeClassicHistograms = &global
	}

	for _, rc := range c.RelabelConfigs {
		if err := rc.Validate(c.MetricNameValidationScheme); err != nil {
			return err
		}
	}
	for _, rc := range c.MetricRelabelConfigs {
		if err := rc.Validate(c.MetricNameValidationScheme); err != nil {
			return err
		}
	}

	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (c *ScrapeConfig) MarshalYAML() (any, error) {
	return discovery.MarshalYAMLWithInlineConfigs(c)
}

// ToEscapingScheme wraps the equivalent common library function with the
// desired default behavior based on the given validation scheme. This is a
// workaround for third party exporters that don't set the escaping scheme.
func ToEscapingScheme(s string, v model.ValidationScheme) (model.EscapingScheme, error) {
	if s == "" {
		switch v {
		case model.UTF8Validation:
			return model.NoEscaping, nil
		case model.LegacyValidation:
			return model.UnderscoreEscaping, nil
		case model.UnsetValidation:
			return model.NoEscaping, fmt.Errorf("ValidationScheme is unset: %s", v)
		default:
			panic(fmt.Errorf("unhandled validation scheme: %s", v))
		}
	}
	return model.ToEscapingScheme(s)
}

// ScrapeNativeHistogramsEnabled returns whether to scrape native histograms.
func (c *ScrapeConfig) ScrapeNativeHistogramsEnabled() bool {
	return c.ScrapeNativeHistograms != nil && *c.ScrapeNativeHistograms
}

// ConvertClassicHistogramsToNHCBEnabled returns whether to convert classic histograms to NHCB.
func (c *ScrapeConfig) ConvertClassicHistogramsToNHCBEnabled() bool {
	return c.ConvertClassicHistogramsToNHCB != nil && *c.ConvertClassicHistogramsToNHCB
}

// AlwaysScrapeClassicHistogramsEnabled returns whether to always scrape classic histograms.
func (c *ScrapeConfig) AlwaysScrapeClassicHistogramsEnabled() bool {
	return c.AlwaysScrapeClassicHistograms != nil && *c.AlwaysScrapeClassicHistograms
}

// ExtraScrapeMetricsEnabled returns whether to enable extra scrape metrics.
func (c *ScrapeConfig) ExtraScrapeMetricsEnabled() bool {
	return c.ExtraScrapeMetrics != nil && *c.ExtraScrapeMetrics
}

// StorageConfig configures runtime reloadable configuration options.
type StorageConfig struct {
	TSDBConfig      *TSDBConfig      `yaml:"tsdb,omitempty"`
	ExemplarsConfig *ExemplarsConfig `yaml:"exemplars,omitempty"`
}

// TSDBRetentionConfig holds the configuration retention of data in storage storage.
type TSDBRetentionConfig struct {
	// How long to retain samples in storage.
	Time model.Duration `yaml:"time,omitempty"`

	// Maximum number of bytes that can be stored for blocks.
	Size units.Base2Bytes `yaml:"size,omitempty"`
}

// TSDBConfig configures runtime reloadable configuration options.
type TSDBConfig struct {
	// OutOfOrderTimeWindow sets how long back in time an out-of-order sample can be inserted
	// into the TSDB. This flag is typically set while unmarshaling the configuration file and translating
	// OutOfOrderTimeWindowFlag's duration. The unit of this flag is expected to be the same as any
	// other timestamp in the TSDB.
	OutOfOrderTimeWindow int64

	// OutOfOrderTimeWindowFlag holds the parsed duration from the config file.
	// During unmarshall, this is converted into milliseconds and stored in OutOfOrderTimeWindow.
	// This should not be used directly and must be converted into OutOfOrderTimeWindow.
	OutOfOrderTimeWindowFlag model.Duration `yaml:"out_of_order_time_window,omitempty"`

	Retention *TSDBRetentionConfig `yaml:"retention,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *TSDBConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*t = TSDBConfig{}
	type plain TSDBConfig
	if err := unmarshal((*plain)(t)); err != nil {
		return err
	}

	t.OutOfOrderTimeWindow = time.Duration(t.OutOfOrderTimeWindowFlag).Milliseconds()

	return nil
}

type TracingClientType string

const (
	TracingClientHTTP TracingClientType = "http"
	TracingClientGRPC TracingClientType = "grpc"

	GzipCompression = "gzip"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *TracingClientType) UnmarshalYAML(unmarshal func(any) error) error {
	*t = TracingClientType("")
	type plain TracingClientType
	if err := unmarshal((*plain)(t)); err != nil {
		return err
	}

	if *t != TracingClientHTTP && *t != TracingClientGRPC {
		return fmt.Errorf("expected tracing client type to be to be %s or %s, but got %s",
			TracingClientHTTP, TracingClientGRPC, *t,
		)
	}

	return nil
}

// TracingConfig configures the tracing options.
type TracingConfig struct {
	ClientType       TracingClientType `yaml:"client_type,omitempty"`
	Endpoint         string            `yaml:"endpoint,omitempty"`
	SamplingFraction float64           `yaml:"sampling_fraction,omitempty"`
	Insecure         bool              `yaml:"insecure,omitempty"`
	TLSConfig        config.TLSConfig  `yaml:"tls_config,omitempty"`
	Headers          map[string]string `yaml:"headers,omitempty"`
	Compression      string            `yaml:"compression,omitempty"`
	Timeout          model.Duration    `yaml:"timeout,omitempty"`
}

// SetDirectory joins any relative file paths with dir.
func (t *TracingConfig) SetDirectory(dir string) {
	t.TLSConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *TracingConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*t = TracingConfig{
		ClientType: TracingClientGRPC,
	}
	type plain TracingConfig
	if err := unmarshal((*plain)(t)); err != nil {
		return err
	}

	if err := validateHeadersForTracing(t.Headers); err != nil {
		return err
	}

	if t.Endpoint == "" {
		return errors.New("tracing endpoint must be set")
	}

	if t.Compression != "" && t.Compression != GzipCompression {
		return fmt.Errorf("invalid compression type %s provided, valid options: %s",
			t.Compression, GzipCompression)
	}

	return nil
}

// ExemplarsConfig configures runtime reloadable configuration options.
type ExemplarsConfig struct {
	// MaxExemplars sets the size, in # of exemplars stored, of the single circular buffer used to store exemplars in memory.
	// Use a value of 0 or less than 0 to disable the storage without having to restart Prometheus.
	MaxExemplars int64 `yaml:"max_exemplars,omitempty"`
}

// AlertingConfig configures alerting and alertmanager related configs.
type AlertingConfig struct {
	AlertRelabelConfigs []*relabel.Config   `yaml:"alert_relabel_configs,omitempty"`
	AlertmanagerConfigs AlertmanagerConfigs `yaml:"alertmanagers,omitempty"`
}

func (c *AlertingConfig) Validate(nameValidationScheme model.ValidationScheme) error {
	for _, rc := range c.AlertRelabelConfigs {
		if err := rc.Validate(nameValidationScheme); err != nil {
			return err
		}
	}
	for _, rc := range c.AlertmanagerConfigs {
		if err := rc.Validate(nameValidationScheme); err != nil {
			return err
		}
	}
	return nil
}

// SetDirectory joins any relative file paths with dir.
func (c *AlertingConfig) SetDirectory(dir string) {
	for _, c := range c.AlertmanagerConfigs {
		c.SetDirectory(dir)
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *AlertingConfig) UnmarshalYAML(unmarshal func(any) error) error {
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

// AlertmanagerConfigs is a slice of *AlertmanagerConfig.
type AlertmanagerConfigs []*AlertmanagerConfig

// ToMap converts a slice of *AlertmanagerConfig to a map.
func (a AlertmanagerConfigs) ToMap() map[string]*AlertmanagerConfig {
	ret := make(map[string]*AlertmanagerConfig)
	for i := range a {
		ret[fmt.Sprintf("config-%d", i)] = a[i]
	}
	return ret
}

// AlertmanagerAPIVersion represents a version of the
// github.com/prometheus/alertmanager/api, e.g. 'v1' or 'v2'.
// 'v1' is no longer supported.
type AlertmanagerAPIVersion string

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (v *AlertmanagerAPIVersion) UnmarshalYAML(unmarshal func(any) error) error {
	*v = AlertmanagerAPIVersion("")
	type plain AlertmanagerAPIVersion
	if err := unmarshal((*plain)(v)); err != nil {
		return err
	}

	if !slices.Contains(SupportedAlertmanagerAPIVersions, *v) {
		return fmt.Errorf("expected Alertmanager api version to be one of %v but got %v", SupportedAlertmanagerAPIVersions, *v)
	}

	return nil
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
	AlertmanagerAPIVersionV2,
}

// AlertmanagerConfig configures how Alertmanagers can be discovered and communicated with.
type AlertmanagerConfig struct {
	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.

	ServiceDiscoveryConfigs discovery.Configs       `yaml:"-"`
	HTTPClientConfig        config.HTTPClientConfig `yaml:",inline"`
	SigV4Config             *sigv4.SigV4Config      `yaml:"sigv4,omitempty"`

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
	// Relabel alerts before sending to the specific alertmanager.
	AlertRelabelConfigs []*relabel.Config `yaml:"alert_relabel_configs,omitempty"`
}

// SetDirectory joins any relative file paths with dir.
func (c *AlertmanagerConfig) SetDirectory(dir string) {
	c.ServiceDiscoveryConfigs.SetDirectory(dir)
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *AlertmanagerConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultAlertmanagerConfig
	if err := discovery.UnmarshalYAMLWithInlineConfigs(c, unmarshal); err != nil {
		return err
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	httpClientConfigAuthEnabled := c.HTTPClientConfig.BasicAuth != nil ||
		c.HTTPClientConfig.Authorization != nil || c.HTTPClientConfig.OAuth2 != nil

	if httpClientConfigAuthEnabled && c.SigV4Config != nil {
		return errors.New("at most one of basic_auth, authorization, oauth2, & sigv4 must be configured")
	}

	// Check for users putting URLs in target groups.
	if len(c.RelabelConfigs) == 0 {
		if err := checkStaticTargets(c.ServiceDiscoveryConfigs); err != nil {
			return err
		}
	}

	for _, rlcfg := range c.RelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null Alertmanager target relabeling rule")
		}
	}

	for _, rlcfg := range c.AlertRelabelConfigs {
		if rlcfg == nil {
			return errors.New("empty or null Alertmanager alert relabeling rule")
		}
	}

	return nil
}

func (c *AlertmanagerConfig) Validate(nameValidationScheme model.ValidationScheme) error {
	for _, rc := range c.AlertRelabelConfigs {
		if err := rc.Validate(nameValidationScheme); err != nil {
			return err
		}
	}
	for _, rc := range c.RelabelConfigs {
		if err := rc.Validate(nameValidationScheme); err != nil {
			return err
		}
	}
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (c *AlertmanagerConfig) MarshalYAML() (any, error) {
	return discovery.MarshalYAMLWithInlineConfigs(c)
}

func checkStaticTargets(configs discovery.Configs) error {
	for _, cfg := range configs {
		sc, ok := cfg.(discovery.StaticConfig)
		if !ok {
			continue
		}
		for _, tg := range sc {
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

// RemoteWriteConfig is the configuration for writing to remote storage.
type RemoteWriteConfig struct {
	URL                  *config.URL       `yaml:"url"`
	RemoteTimeout        model.Duration    `yaml:"remote_timeout,omitempty"`
	Headers              map[string]string `yaml:"headers,omitempty"`
	WriteRelabelConfigs  []*relabel.Config `yaml:"write_relabel_configs,omitempty"`
	Name                 string            `yaml:"name,omitempty"`
	SendExemplars        bool              `yaml:"send_exemplars,omitempty"`
	SendNativeHistograms bool              `yaml:"send_native_histograms,omitempty"`
	RoundRobinDNS        bool              `yaml:"round_robin_dns,omitempty"`
	// ProtobufMessage specifies the protobuf message to use against the remote
	// receiver as specified in https://prometheus.io/docs/specs/remote_write_spec_2_0/
	ProtobufMessage remoteapi.WriteMessageType `yaml:"protobuf_message,omitempty"`

	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	QueueConfig      QueueConfig             `yaml:"queue_config,omitempty"`
	MetadataConfig   MetadataConfig          `yaml:"metadata_config,omitempty"`
	SigV4Config      *sigv4.SigV4Config      `yaml:"sigv4,omitempty"`
	AzureADConfig    *azuread.AzureADConfig  `yaml:"azuread,omitempty"`
	GoogleIAMConfig  *googleiam.Config       `yaml:"google_iam,omitempty"`
}

// SetDirectory joins any relative file paths with dir.
func (c *RemoteWriteConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RemoteWriteConfig) UnmarshalYAML(unmarshal func(any) error) error {
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
	if err := validateHeaders(c.Headers); err != nil {
		return err
	}

	if err := c.ProtobufMessage.Validate(); err != nil {
		return fmt.Errorf("invalid protobuf_message value: %w", err)
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	return validateAuthConfigs(c)
}

func (c *RemoteWriteConfig) Validate(nameValidationScheme model.ValidationScheme) error {
	for _, rc := range c.WriteRelabelConfigs {
		if err := rc.Validate(nameValidationScheme); err != nil {
			return err
		}
	}

	return nil
}

// validateAuthConfigs validates that at most one of basic_auth, authorization, oauth2, sigv4, azuread or google_iam must be configured.
func validateAuthConfigs(c *RemoteWriteConfig) error {
	var authConfigured []string
	if c.HTTPClientConfig.BasicAuth != nil {
		authConfigured = append(authConfigured, "basic_auth")
	}
	if c.HTTPClientConfig.Authorization != nil {
		authConfigured = append(authConfigured, "authorization")
	}
	if c.HTTPClientConfig.OAuth2 != nil {
		authConfigured = append(authConfigured, "oauth2")
	}
	if c.SigV4Config != nil {
		authConfigured = append(authConfigured, "sigv4")
	}
	if c.AzureADConfig != nil {
		authConfigured = append(authConfigured, "azuread")
	}
	if c.GoogleIAMConfig != nil {
		authConfigured = append(authConfigured, "google_iam")
	}
	if len(authConfigured) > 1 {
		return fmt.Errorf("at most one of basic_auth, authorization, oauth2, sigv4, azuread or google_iam must be configured. Currently configured: %v", authConfigured)
	}
	return nil
}

func validateHeadersForTracing(headers map[string]string) error {
	for header := range headers {
		if strings.ToLower(header) == "authorization" {
			return errors.New("custom authorization header configuration is not yet supported")
		}
		if _, ok := reservedHeaders[strings.ToLower(header)]; ok {
			return fmt.Errorf("%s is a reserved header. It must not be changed", header)
		}
	}
	return nil
}

func validateHeaders(headers map[string]string) error {
	for header := range headers {
		if strings.ToLower(header) == "authorization" {
			return errors.New("authorization header must be changed via the basic_auth, authorization, oauth2, sigv4, azuread or google_iam parameter")
		}
		if _, ok := reservedHeaders[strings.ToLower(header)]; ok {
			return fmt.Errorf("%s is a reserved header. It must not be changed", header)
		}
	}
	return nil
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
	MinBackoff       model.Duration `yaml:"min_backoff,omitempty"`
	MaxBackoff       model.Duration `yaml:"max_backoff,omitempty"`
	RetryOnRateLimit bool           `yaml:"retry_on_http_429,omitempty"`

	// Samples older than the limit will be dropped.
	SampleAgeLimit model.Duration `yaml:"sample_age_limit,omitempty"`
}

// MetadataConfig is the configuration for sending metadata to remote
// storage.
type MetadataConfig struct {
	// Send controls whether we send metric metadata to remote storage.
	Send bool `yaml:"send"`
	// SendInterval controls how frequently we send metric metadata.
	SendInterval model.Duration `yaml:"send_interval"`
	// Maximum number of samples per send.
	MaxSamplesPerSend int `yaml:"max_samples_per_send,omitempty"`
}

const (
	// DefaultChunkedReadLimit is the default value for the maximum size of the protobuf frame client allows.
	// 50MB is the default. This is equivalent to ~100k full XOR chunks and average labelset.
	DefaultChunkedReadLimit = 5e+7
)

// RemoteReadConfig is the configuration for reading from remote storage.
type RemoteReadConfig struct {
	URL              *config.URL       `yaml:"url"`
	RemoteTimeout    model.Duration    `yaml:"remote_timeout,omitempty"`
	ChunkedReadLimit uint64            `yaml:"chunked_read_limit,omitempty"`
	Headers          map[string]string `yaml:"headers,omitempty"`
	ReadRecent       bool              `yaml:"read_recent,omitempty"`
	Name             string            `yaml:"name,omitempty"`

	// We cannot do proper Go type embedding below as the parser will then parse
	// values arbitrarily into the overflow maps of further-down types.
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`

	// RequiredMatchers is an optional list of equality matchers which have to
	// be present in a selector to query the remote read endpoint.
	RequiredMatchers model.LabelSet `yaml:"required_matchers,omitempty"`

	// Whether to use the external labels as selectors for the remote read endpoint.
	FilterExternalLabels bool `yaml:"filter_external_labels,omitempty"`
}

// SetDirectory joins any relative file paths with dir.
func (c *RemoteReadConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *RemoteReadConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultRemoteReadConfig
	type plain RemoteReadConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == nil {
		return errors.New("url for remote_read is empty")
	}
	if err := validateHeaders(c.Headers); err != nil {
		return err
	}
	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	return c.HTTPClientConfig.Validate()
}

func filePath(filename string) string {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return filename
	}
	return absPath
}

func fileErr(filename string, err error) error {
	return fmt.Errorf("%q: %w", filePath(filename), err)
}

func getGoGC() int {
	goGCEnv := os.Getenv("GOGC")
	// If the GOGC env var is set, use the same logic as upstream Go.
	if goGCEnv != "" {
		// Special case for GOGC=off.
		if strings.ToLower(goGCEnv) == "off" {
			return -1
		}
		i, err := strconv.Atoi(goGCEnv)
		if err == nil {
			return i
		}
	}
	return DefaultGoGCPercentage
}

// OTLPConfig is the configuration for writing to the OTLP endpoint.
type OTLPConfig struct {
	PromoteAllResourceAttributes      bool                                     `yaml:"promote_all_resource_attributes,omitempty"`
	PromoteResourceAttributes         []string                                 `yaml:"promote_resource_attributes,omitempty"`
	IgnoreResourceAttributes          []string                                 `yaml:"ignore_resource_attributes,omitempty"`
	TranslationStrategy               otlptranslator.TranslationStrategyOption `yaml:"translation_strategy,omitempty"`
	KeepIdentifyingResourceAttributes bool                                     `yaml:"keep_identifying_resource_attributes,omitempty"`
	ConvertHistogramsToNHCB           bool                                     `yaml:"convert_histograms_to_nhcb,omitempty"`
	// PromoteScopeMetadata controls whether to promote OTel scope metadata (i.e. name, version, schema URL, and attributes) to metric labels.
	// As per OTel spec, the aforementioned scope metadata should be identifying, i.e. made into metric labels.
	PromoteScopeMetadata bool `yaml:"promote_scope_metadata,omitempty"`
	// LabelNameUnderscoreSanitization controls whether to enable prepending of 'key_' to labels
	// starting with '_'. Reserved labels starting with `__` are not modified.
	// This is only relevant when AllowUTF8 is false (i.e., when using underscore escaping).
	LabelNameUnderscoreSanitization bool `yaml:"label_name_underscore_sanitization,omitempty"`
	// LabelNamePreserveMultipleUnderscores enables preserving of multiple consecutive underscores
	// in label names when AllowUTF8 is false. When false, multiple consecutive underscores are
	// collapsed to a single underscore during label name sanitization.
	LabelNamePreserveMultipleUnderscores bool `yaml:"label_name_preserve_multiple_underscores,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *OTLPConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultOTLPConfig
	type plain OTLPConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.PromoteAllResourceAttributes {
		if len(c.PromoteResourceAttributes) > 0 {
			return errors.New("'promote_all_resource_attributes' and 'promote_resource_attributes' cannot be configured simultaneously")
		}
		if err := sanitizeAttributes(c.IgnoreResourceAttributes, "ignored"); err != nil {
			return fmt.Errorf("invalid 'ignore_resource_attributes': %w", err)
		}
	} else {
		if len(c.IgnoreResourceAttributes) > 0 {
			return errors.New("'ignore_resource_attributes' cannot be configured unless 'promote_all_resource_attributes' is true")
		}
		if err := sanitizeAttributes(c.PromoteResourceAttributes, "promoted"); err != nil {
			return fmt.Errorf("invalid 'promote_resource_attributes': %w", err)
		}
	}

	return nil
}

func sanitizeAttributes(attributes []string, adjective string) error {
	seen := map[string]struct{}{}
	var err error
	for i, attr := range attributes {
		attr = strings.TrimSpace(attr)
		if attr == "" {
			err = errors.Join(err, fmt.Errorf("empty %s OTel resource attribute", adjective))
			continue
		}
		if _, exists := seen[attr]; exists {
			err = errors.Join(err, fmt.Errorf("duplicated %s OTel resource attribute %q", adjective, attr))
			continue
		}

		seen[attr] = struct{}{}
		attributes[i] = attr
	}
	return err
}
