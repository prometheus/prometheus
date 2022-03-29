// Copyright 2022 The Prometheus Authors
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

//go:build noaws
// +build noaws

package aws

import (
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/disabled"
)

type EC2SDConfig struct {
	disabled.SDConfig
}

func (*EC2SDConfig) Name() string {
	return "ec2"
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *EC2SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return s.Error(s.Name())
}

type LightsailSDConfig struct {
	disabled.SDConfig
}

func (*LightsailSDConfig) Name() string {
	return "lightsail"
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *LightsailSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return s.Error(s.Name())
}

func init() {
	discovery.RegisterConfig(&LightsailSDConfig{})
	discovery.RegisterConfig(&EC2SDConfig{})
}
