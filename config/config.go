package config

import (
	"errors"
	"fmt"
	"github.com/matttproud/prometheus/model"
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
		return errors.New(fmt.Sprintf("No targets configured for job '%v'", name))
	}
	job := &JobConfig{
		Targets: tmpJobTargets,
	}
	for option, value := range options {
		if err := job.SetOption(option, value); err != nil {
			return err
		}
	}
	config.Jobs = append(config.Jobs, *job)
	return nil
}

func (config *GlobalConfig) SetOption(option string, value string) error {
	switch option {
	case "scrape_interval":
		config.ScrapeInterval = stringToDuration(value)
		return nil
	case "evaluation_interval":
		config.EvaluationInterval = stringToDuration(value)
		return nil
	default:
		return errors.New(fmt.Sprintf("Unrecognized global configuration option '%v'", option))
	}
	return nil
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

func (job *JobConfig) SetOption(option string, value string) error {
	switch option {
	case "name":
		job.Name = value
		return nil
	case "scrape_interval":
		job.ScrapeInterval = stringToDuration(value)
		return nil
	default:
		return errors.New(fmt.Sprintf("Unrecognized job configuration option '%v'", option))
	}
	return nil
}

func (job *JobConfig) AddTargets(endpoints []string, labels model.LabelSet) {
	targets := Targets{
		Endpoints: endpoints,
		Labels:    labels,
	}
	job.Targets = append(job.Targets, targets)
}
