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

//go:build noazure
// +build noazure

package azure

import (
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/disabled"
)

type SDConfig struct {
	disabled.SDConfig
}

func (*SDConfig) Name() string {
	return "azure"
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (s *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return s.Error(s.Name())
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}
