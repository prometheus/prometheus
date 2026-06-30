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

package discovery

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

// Supported values for ResolveAddressesConfig.Type.
const (
	resolveTypeA    = "A"
	resolveTypeAAAA = "AAAA"
	resolveTypeAuto = "auto"
)

// defaultMaxResolvedAddresses caps the number of addresses a single host may
// expand into when the configuration does not set an explicit limit.
const defaultMaxResolvedAddresses = 64

// DefaultResolveAddressesConfig is the default address resolution configuration.
var DefaultResolveAddressesConfig = ResolveAddressesConfig{
	Enabled:              false,
	RefreshInterval:      model.Duration(30 * time.Second),
	Type:                 resolveTypeAuto,
	MaxResolvedAddresses: defaultMaxResolvedAddresses,
}

// ResolveAddressesConfig configures the discovery Manager to resolve the FQDN
// found in each target's __address__ label into one or more IP addresses,
// expanding a single target into one target per resolved IP. Resolution runs
// before relabeling and is refreshed on RefreshInterval.
type ResolveAddressesConfig struct {
	// Enabled turns address resolution on for the target set.
	Enabled bool `yaml:"enabled"`
	// RefreshInterval is how often the resolved addresses are refreshed.
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	// Type selects which records to resolve: "A", "AAAA" or "auto" (both).
	Type string `yaml:"type,omitempty"`
	// MaxResolvedAddresses caps how many addresses a single host may expand
	// into, bounding the fan-out from untrusted DNS answers. A value of 0 means
	// use the default; negative values are rejected.
	MaxResolvedAddresses int `yaml:"max_resolved_addresses,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *ResolveAddressesConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultResolveAddressesConfig
	type plain ResolveAddressesConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	switch strings.ToUpper(c.Type) {
	case resolveTypeA:
		c.Type = resolveTypeA
	case resolveTypeAAAA:
		c.Type = resolveTypeAAAA
	case strings.ToUpper(resolveTypeAuto):
		c.Type = resolveTypeAuto
	default:
		return fmt.Errorf("invalid resolve_addresses type %q (must be A, AAAA or auto)", c.Type)
	}
	if c.Enabled && c.RefreshInterval <= 0 {
		return errors.New("resolve_addresses refresh_interval must be greater than 0")
	}
	if c.MaxResolvedAddresses < 0 {
		return fmt.Errorf("resolve_addresses max_resolved_addresses must not be negative, got %d", c.MaxResolvedAddresses)
	}
	if c.MaxResolvedAddresses == 0 {
		c.MaxResolvedAddresses = defaultMaxResolvedAddresses
	}
	return nil
}
