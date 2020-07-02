// Copyright 2020 The Prometheus Authors
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
	"os"
	"strconv"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"
)

const defaultInterval = model.Duration(12 * 3600 * time.Second)

// ReadConfig gets the GlobalConfig from a configFile (that is a path to the file).
func ReadConfig(configFile string) (*Config, error) {
	if len(configFile) == 0 {
		fmt.Fprintln(os.Stderr, "No config file provided, configuration is reading from System environment")
		return readConfigFromENV()
	}
	fmt.Fprintln(os.Stderr, "Configuration is reading from configuration file")
	return readConfigFromYAML(configFile)
}

func readConfigFromYAML(configFile string) (*Config, error) {
	b, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	res := new(Config)
	err = yaml.Unmarshal(b, res)
	return res, err
}

func readConfigFromENV() (*Config, error) {
	res := new(Config)
	err := res.unmarshalENV()
	return res, err
}

// LogFormat is the type used for describing the format of logs.
type LogFormat string

const (
	// JSONFormat is used for JSON logs.
	JSONFormat LogFormat = "json"
	// TextFormat is used of structured text logs.
	TextFormat LogFormat = "text"
)

var mapLogFormat = map[LogFormat]bool{ // nolint: gochecknoglobals
	JSONFormat: true,
	TextFormat: true,
}

// Config contains the configuration for a server.
type Config struct {
	ActivateRPCLog bool      `yaml:"activate_rpc_log"`
	LogFormat      LogFormat `yaml:"log_format"`
	PrometheusURL  string    `yaml:"prometheus_url"`
	RESTAPIPort    uint64    `yaml:"rest_api_port"`
	// MetadataLookbackInterval is the time in second used to retrieve label and metrics from Prometheus
	MetadataLookbackInterval model.Duration `yaml:"metadata_lookback_interval"`
}

// UnmarshalYAML overrides a function used internally by the yaml.v3 lib.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	tmp := &Config{}
	type plain Config
	if err := unmarshal((*plain)(tmp)); err != nil {
		return err
	}
	if err := tmp.Validate(); err != nil {
		return err
	}
	*c = *tmp
	return nil
}

func (c *Config) unmarshalENV() error {
	prefix := "LANGSERVER"
	conf := &struct {
		ActivateRPCLog bool
		LogFormat      string
		PrometheusURL  string
		// the envconfig lib is not able to convert an empty string to the value 0
		// so we have to convert it manually
		RESTAPIPort              string
		MetadataLookbackInterval string
	}{}
	if err := envconfig.Process(prefix, conf); err != nil {
		return err
	}
	if len(conf.RESTAPIPort) > 0 {
		var parseError error
		c.RESTAPIPort, parseError = strconv.ParseUint(conf.RESTAPIPort, 10, 64)
		if parseError != nil {
			return parseError
		}
	}
	if len(conf.MetadataLookbackInterval) > 0 {
		var parseError error
		c.MetadataLookbackInterval, parseError = model.ParseDuration(conf.MetadataLookbackInterval)
		if parseError != nil {
			return parseError
		}
	}
	c.ActivateRPCLog = conf.ActivateRPCLog
	c.PrometheusURL = conf.PrometheusURL
	c.LogFormat = LogFormat(conf.LogFormat)
	return c.Validate()
}

// Validate returns an error if the config is not valid.
func (c *Config) Validate() error {
	if len(c.PrometheusURL) > 0 {
		if _, err := url.Parse(c.PrometheusURL); err != nil {
			return err
		}
	}

	if len(c.LogFormat) > 0 {
		if !mapLogFormat[c.LogFormat] {
			return fmt.Errorf(`invalid value for logFormat. "%s" Valid values are  "%s" or "%s"`, c.LogFormat, TextFormat, JSONFormat)
		}
	} else {
		// default value
		c.LogFormat = TextFormat
	}

	if c.MetadataLookbackInterval <= 0 {
		c.MetadataLookbackInterval = defaultInterval
	}

	return nil
}
