// Copyright 2013 Prometheus Team
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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/utility"
	"time"
)

type Config struct {
	Global *GlobalConfig
	Jobs   []JobConfig
}

type GlobalConfig struct {
	ScrapeInterval     time.Duration
	EvaluationInterval time.Duration
	Labels             model.LabelSet
	RuleFiles          []string
}

type JobConfig struct {
	Name           string
	ScrapeInterval time.Duration
	Targets        []Targets
}

type Targets struct {
	Endpoints []string
	Labels    model.LabelSet
}

func New() *Config {
	return &Config{
		Global: &GlobalConfig{Labels: model.LabelSet{}},
	}
}

func (config *Config) AddJob(options map[string]string, targets []Targets) error {
	name, ok := options["name"]
	if !ok {
		return errors.New("Missing job name")
	}
	if len(targets) == 0 {
		return fmt.Errorf("No targets configured for job '%v'", name)
	}
	job := JobConfig{
		Targets: tmpJobTargets,
	}
	for option, value := range options {
		if err := job.SetOption(option, value); err != nil {
			return err
		}
	}
	config.Jobs = append(config.Jobs, job)
	return nil
}

func (config *Config) GetJobByName(name string) (jobConfig *JobConfig) {
	for _, job := range config.Jobs {
		if job.Name == name {
			jobConfig = &job
			break
		}
	}
	return
}

func (config *GlobalConfig) SetOption(option string, value string) (err error) {
	switch option {
	case "scrape_interval":
		config.ScrapeInterval, err = utility.StringToDuration(value)
		return nil
	case "evaluation_interval":
		config.EvaluationInterval, err = utility.StringToDuration(value)
		return err
	default:
		err = fmt.Errorf("Unrecognized global configuration option '%v'", option)
	}
	return
}

func (config *GlobalConfig) SetLabels(labels model.LabelSet) {
	for k, v := range labels {
		config.Labels[k] = v
	}
}

func (config *GlobalConfig) AddRuleFiles(ruleFiles []string) {
	for _, ruleFile := range ruleFiles {
		config.RuleFiles = append(config.RuleFiles, ruleFile)
	}
}

func (job *JobConfig) SetOption(option string, value string) (err error) {
	switch option {
	case "name":
		job.Name = value
	case "scrape_interval":
		job.ScrapeInterval, err = utility.StringToDuration(value)
	default:
		err = fmt.Errorf("Unrecognized job configuration option '%v'", option)
	}
	return
}

func (job *JobConfig) AddTargets(endpoints []string, labels model.LabelSet) {
	targets := Targets{
		Endpoints: endpoints,
		Labels:    labels,
	}
	job.Targets = append(job.Targets, targets)
}
