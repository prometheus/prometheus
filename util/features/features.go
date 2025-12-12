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

package features

import (
	"maps"
	"sync"
)

// Category constants define the standard feature flag categories used in Prometheus.
const (
	API                       = "api"
	OTLPReceiver              = "otlp_receiver"
	Prometheus                = "prometheus"
	PromQL                    = "promql"
	PromQLFunctions           = "promql_functions"
	PromQLOperators           = "promql_operators"
	Rules                     = "rules"
	Scrape                    = "scrape"
	ServiceDiscoveryProviders = "service_discovery_providers"
	TemplatingFunctions       = "templating_functions"
	TSDB                      = "tsdb"
	UI                        = "ui"
)

// Collector defines the interface for collecting and managing feature flags.
// It provides methods to enable, disable, and retrieve feature states.
type Collector interface {
	// Enable marks a feature as enabled in the registry.
	// The category and name should use snake_case naming convention.
	Enable(category, name string)

	// Disable marks a feature as disabled in the registry.
	// The category and name should use snake_case naming convention.
	Disable(category, name string)

	// Set sets a feature to the specified enabled state.
	// The category and name should use snake_case naming convention.
	Set(category, name string, enabled bool)

	// Get returns a copy of all registered features organized by category.
	// Returns a map where the keys are category names and values are maps
	// of feature names to their enabled status.
	Get() map[string]map[string]bool
}

// registry is the private implementation of the Collector interface.
// It stores feature information organized by category.
type registry struct {
	mu       sync.RWMutex
	features map[string]map[string]bool
}

// DefaultRegistry is the package-level registry used by Prometheus.
var DefaultRegistry = NewRegistry()

// NewRegistry creates a new feature registry.
func NewRegistry() Collector {
	return &registry{
		features: make(map[string]map[string]bool),
	}
}

// Enable marks a feature as enabled in the registry.
func (r *registry) Enable(category, name string) {
	r.Set(category, name, true)
}

// Disable marks a feature as disabled in the registry.
func (r *registry) Disable(category, name string) {
	r.Set(category, name, false)
}

// Set sets a feature to the specified enabled state.
func (r *registry) Set(category, name string, enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.features[category] == nil {
		r.features[category] = make(map[string]bool)
	}
	r.features[category][name] = enabled
}

// Get returns a copy of all registered features organized by category.
func (r *registry) Get() map[string]map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]map[string]bool, len(r.features))
	for category, features := range r.features {
		result[category] = make(map[string]bool, len(features))
		maps.Copy(result[category], features)
	}
	return result
}

// Enable marks a feature as enabled in the default registry.
func Enable(category, name string) {
	DefaultRegistry.Enable(category, name)
}

// Disable marks a feature as disabled in the default registry.
func Disable(category, name string) {
	DefaultRegistry.Disable(category, name)
}

// Set sets a feature to the specified enabled state in the default registry.
func Set(category, name string, enabled bool) {
	DefaultRegistry.Set(category, name, enabled)
}

// Get returns all features from the default registry.
func Get() map[string]map[string]bool {
	return DefaultRegistry.Get()
}
