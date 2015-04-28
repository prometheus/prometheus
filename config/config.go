// Copyright 2013 The Prometheus Authors
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
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"

	pb "github.com/prometheus/prometheus/config/generated"
)

var jobNameRE = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_-]*$")
var labelNameRE = regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")

// Config encapsulates the configuration of a Prometheus instance. It wraps the
// raw configuration protocol buffer to be able to add custom methods to it.
type Config struct {
	// The protobuf containing the actual configuration values.
	pb.PrometheusConfig
}

// String returns an ASCII serialization of the loaded configuration protobuf.
func (c *Config) String() string {
	return proto.MarshalTextString(&c.PrometheusConfig)
}

// validateLabels validates whether label names have the correct format.
func validateLabels(labels *pb.LabelPairs) error {
	if labels == nil {
		return nil
	}
	for _, label := range labels.Label {
		if !labelNameRE.MatchString(label.GetName()) {
			return fmt.Errorf("invalid label name '%s'", label.GetName())
		}
	}
	return nil
}

// validateHosts validates whether a target group contains valid hosts.
func validateHosts(hosts []string) error {
	if hosts == nil {
		return nil
	}
	for _, host := range hosts {
		// Make sure that this does not contain any paths or schemes.
		// This ensures that old configurations error.
		if strings.Contains(host, "/") {
			return fmt.Errorf("invalid host '%s', no schemes or paths allowed", host)
		}
	}
	return nil
}

// Validate checks an entire parsed Config for the validity of its fields.
func (c *Config) Validate() error {
	// Check the global configuration section for validity.
	global := c.Global
	if _, err := utility.StringToDuration(global.GetScrapeInterval()); err != nil {
		return fmt.Errorf("invalid global scrape interval: %s", err)
	}
	if _, err := utility.StringToDuration(global.GetEvaluationInterval()); err != nil {
		return fmt.Errorf("invalid rule evaluation interval: %s", err)
	}
	if err := validateLabels(global.Labels); err != nil {
		return fmt.Errorf("invalid global labels: %s", err)
	}

	// Check each scrape configuration for validity.
	jobNames := map[string]struct{}{}
	for _, sc := range c.ScrapeConfigs() {
		name := sc.GetJobName()

		if _, ok := jobNames[name]; ok {
			return fmt.Errorf("found multiple scrape configs configured with the same job name: %q", name)
		}
		jobNames[name] = struct{}{}

		if err := sc.Validate(); err != nil {
			return fmt.Errorf("error in scrape config %q: %s", name, err)
		}
	}

	return nil
}

// GlobalLabels returns the global labels as a LabelSet.
func (c *Config) GlobalLabels() clientmodel.LabelSet {
	labels := clientmodel.LabelSet{}
	if c.Global != nil && c.Global.Labels != nil {
		for _, label := range c.Global.Labels.Label {
			labels[clientmodel.LabelName(label.GetName())] = clientmodel.LabelValue(label.GetValue())
		}
	}
	return labels
}

// ScrapeConfigs returns all scrape configurations.
func (c *Config) ScrapeConfigs() (cfgs []*ScrapeConfig) {
	for _, sc := range c.GetScrapeConfig() {
		cfgs = append(cfgs, &ScrapeConfig{*sc})
	}
	return
}

// stringToDuration converts a string to a duration and dies on invalid format.
func stringToDuration(intervalStr string) time.Duration {
	duration, err := utility.StringToDuration(intervalStr)
	if err != nil {
		panic(err)
	}
	return duration
}

// ScrapeInterval gets the default scrape interval for a Config.
func (c *Config) ScrapeInterval() time.Duration {
	return stringToDuration(c.Global.GetScrapeInterval())
}

// EvaluationInterval gets the default evaluation interval for a Config.
func (c *Config) EvaluationInterval() time.Duration {
	return stringToDuration(c.Global.GetEvaluationInterval())
}

// ScrapeConfig encapsulates a protobuf scrape configuration.
type ScrapeConfig struct {
	pb.ScrapeConfig
}

// ScrapeInterval gets the scrape interval for the scrape config.
func (c *ScrapeConfig) ScrapeInterval() time.Duration {
	return stringToDuration(c.GetScrapeInterval())
}

// ScrapeTimeout gets the scrape timeout for the scrape config.
func (c *ScrapeConfig) ScrapeTimeout() time.Duration {
	return stringToDuration(c.GetScrapeTimeout())
}

// Labels returns a label set for the targets that is implied by the scrape config.
func (c *ScrapeConfig) Labels() clientmodel.LabelSet {
	return clientmodel.LabelSet{
		clientmodel.MetricsPathLabel: clientmodel.LabelValue(c.GetMetricsPath()),
		clientmodel.JobLabel:         clientmodel.LabelValue(c.GetJobName()),
	}
}

// Validate checks the ScrapeConfig for the validity of its fields
func (c *ScrapeConfig) Validate() error {
	name := c.GetJobName()

	if !jobNameRE.MatchString(name) {
		return fmt.Errorf("invalid job name %q", name)
	}
	if _, err := utility.StringToDuration(c.GetScrapeInterval()); err != nil {
		return fmt.Errorf("invalid scrape interval: %s", err)
	}
	if _, err := utility.StringToDuration(c.GetScrapeTimeout()); err != nil {
		return fmt.Errorf("invalid scrape timeout: %s", err)
	}
	for _, tgroup := range c.GetTargetGroup() {
		if err := validateLabels(tgroup.Labels); err != nil {
			return fmt.Errorf("invalid labels: %s", err)
		}
		if err := validateHosts(tgroup.Target); err != nil {
			return fmt.Errorf("invalid targets: %s", err)
		}
	}
	for _, dnscfg := range c.DNSConfigs() {
		if err := dnscfg.Validate(); err != nil {
			return fmt.Errorf("invalid DNS config: %s", err)
		}
	}
	for _, rlcfg := range c.RelabelConfigs() {
		if err := rlcfg.Validate(); err != nil {
			return fmt.Errorf("invalid relabelling config: %s", err)
		}
	}
	return nil
}

// DNSConfigs returns the list of DNS service discovery configurations
// for the scrape config.
func (c *ScrapeConfig) DNSConfigs() []*DNSConfig {
	var dnscfgs []*DNSConfig
	for _, dc := range c.GetDnsConfig() {
		dnscfgs = append(dnscfgs, &DNSConfig{*dc})
	}
	return dnscfgs
}

// RelabelConfigs returns the relabel configs of the scrape config.
func (c *ScrapeConfig) RelabelConfigs() []*RelabelConfig {
	var rlcfgs []*RelabelConfig
	for _, rc := range c.GetRelabelConfig() {
		rlcfgs = append(rlcfgs, &RelabelConfig{*rc})
	}
	return rlcfgs
}

// DNSConfig encapsulates the protobuf configuration object for DNS based
// service discovery.
type DNSConfig struct {
	pb.DNSConfig
}

// Validate checks the DNSConfig for the validity of its fields.
func (c *DNSConfig) Validate() error {
	if _, err := utility.StringToDuration(c.GetRefreshInterval()); err != nil {
		return fmt.Errorf("invalid refresh interval: %s", err)
	}
	return nil
}

// SDRefreshInterval gets the the SD refresh interval for the scrape config.
func (c *DNSConfig) RefreshInterval() time.Duration {
	return stringToDuration(c.GetRefreshInterval())
}

type RelabelConfig struct {
	pb.RelabelConfig
}

func (c *RelabelConfig) Validate() error {
	if len(c.GetSourceLabel()) == 0 {
		return errors.New("at least one source label is required")
	}
	return nil
}

// TargetGroup is derived from a protobuf TargetGroup and attaches a source to it
// that identifies the origin of the group.
type TargetGroup struct {
	// Source is an identifier that describes a group of targets.
	Source string
	// Labels is a set of labels that is common across all targets in the group.
	Labels clientmodel.LabelSet
	// Targets is a list of targets identified by a label set. Each target is
	// uniquely identifiable in the group by its address label.
	Targets []clientmodel.LabelSet
}

func (tg *TargetGroup) String() string {
	return tg.Source
}
