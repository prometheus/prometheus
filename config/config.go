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
	"fmt"
	"regexp"
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
func (c Config) String() string {
	return proto.MarshalTextString(&c.PrometheusConfig)
}

// validateLabels validates whether label names have the correct format.
func (c Config) validateLabels(labels *pb.LabelPairs) error {
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

// Validate checks an entire parsed Config for the validity of its fields.
func (c Config) Validate() error {
	// Check the global configuration section for validity.
	global := c.Global
	if _, err := utility.StringToDuration(global.GetScrapeInterval()); err != nil {
		return fmt.Errorf("invalid global scrape interval: %s", err)
	}
	if _, err := utility.StringToDuration(global.GetEvaluationInterval()); err != nil {
		return fmt.Errorf("invalid rule evaluation interval: %s", err)
	}
	if err := c.validateLabels(global.Labels); err != nil {
		return fmt.Errorf("invalid global labels: %s", err)
	}

	// Check each job configuration for validity.
	jobNames := map[string]bool{}
	for _, job := range c.Job {
		if jobNames[job.GetName()] {
			return fmt.Errorf("found multiple jobs configured with the same name: '%s'", job.GetName())
		}
		jobNames[job.GetName()] = true

		if !jobNameRE.MatchString(job.GetName()) {
			return fmt.Errorf("invalid job name '%s'", job.GetName())
		}
		if _, err := utility.StringToDuration(job.GetScrapeInterval()); err != nil {
			return fmt.Errorf("invalid scrape interval for job '%s': %s", job.GetName(), err)
		}
		if _, err := utility.StringToDuration(job.GetSdRefreshInterval()); err != nil {
			return fmt.Errorf("invalid SD refresh interval for job '%s': %s", job.GetName(), err)
		}
		if _, err := utility.StringToDuration(job.GetScrapeTimeout()); err != nil {
			return fmt.Errorf("invalid scrape timeout for job '%s': %s", job.GetName(), err)
		}
		for _, targetGroup := range job.TargetGroup {
			if err := c.validateLabels(targetGroup.Labels); err != nil {
				return fmt.Errorf("invalid labels for job '%s': %s", job.GetName(), err)
			}
		}
		if job.SdName != nil && len(job.TargetGroup) > 0 {
			return fmt.Errorf("specified both DNS-SD name and target group for job: %s", job.GetName())
		}
	}

	return nil
}

// GetJobByName finds a job by its name in a Config object.
func (c Config) GetJobByName(name string) *JobConfig {
	for _, job := range c.Job {
		if job.GetName() == name {
			return &JobConfig{*job}
		}
	}
	return nil
}

// GlobalLabels returns the global labels as a LabelSet.
func (c Config) GlobalLabels() clientmodel.LabelSet {
	labels := clientmodel.LabelSet{}
	if c.Global.Labels != nil {
		for _, label := range c.Global.Labels.Label {
			labels[clientmodel.LabelName(label.GetName())] = clientmodel.LabelValue(label.GetValue())
		}
	}
	return labels
}

// Jobs returns all the jobs in a Config object.
func (c Config) Jobs() (jobs []JobConfig) {
	for _, job := range c.Job {
		jobs = append(jobs, JobConfig{*job})
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
func (c Config) ScrapeInterval() time.Duration {
	return stringToDuration(c.Global.GetScrapeInterval())
}

// EvaluationInterval gets the default evaluation interval for a Config.
func (c Config) EvaluationInterval() time.Duration {
	return stringToDuration(c.Global.GetEvaluationInterval())
}

// JobConfig encapsulates the configuration of a single job. It wraps the raw
// job protocol buffer to be able to add custom methods to it.
type JobConfig struct {
	pb.JobConfig
}

// ScrapeInterval gets the scrape interval for a job.
func (c JobConfig) ScrapeInterval() time.Duration {
	return stringToDuration(c.GetScrapeInterval())
}

// ScrapeTimeout gets the scrape timeout for a job.
func (c JobConfig) ScrapeTimeout() time.Duration {
	return stringToDuration(c.GetScrapeInterval())
}
