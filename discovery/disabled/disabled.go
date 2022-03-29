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

package disabled

import (
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/discovery"
)

// SDConfig is an empty config used to error out when unmarshalling.
type SDConfig struct {
}

// Error returns an error. This is used to have consistent error messages
// between disabled SD's. It should be called during UnmarshalYAML.
func (s *SDConfig) Error(name string) error {
	return fmt.Errorf("service discovery %s has been disabled at build time", name)
}

// SetDirectory joins any relative file paths with dir.
func (s *SDConfig) SetDirectory(dir string) {
}

// UnmarshalYAML implements the Unmarshaler interface and should be overriden by
// disabled SD's.
func (s *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return s.Error("n/a")
}

// NewDiscoverer implementes the Discoverer interface.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return nil, errors.New("service discovery has been disabled.")
}
