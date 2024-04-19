// Copyright 2024 The Prometheus Authors
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

package secrets

import "fmt"

var globalRegistry = NewRegistry()

// Registry contains all secret providers.
type Registry struct {
	configMap map[string]NewProviderFunc
}

// NewRegistry creates a new registry.
func NewRegistry() Registry {
	return Registry{
		configMap: make(map[string]NewProviderFunc),
	}
}

// Get returns the secret provider constructor with the given name.
func (r *Registry) Get(name string) NewProviderFunc {
	return r.configMap[name]
}

// RegisterProvider registers the given Provider for YAML marshalling and unmarshalling.
func RegisterProvider(name string, provider NewProviderFunc) {
	RegisterProviderExact(name+"_sp_config", provider)
}

// RegisterProviderExact registers the given Provider for YAML marshalling and unmarshalling
// using the exact name given. This method should only be called for core providers.
func RegisterProviderExact(name string, provider NewProviderFunc) {
	if err := globalRegistry.RegisterExact(name, provider); err != nil {
		panic(err)
	}
}

// Register registers the given Provider for YAML marshalling and unmarshalling.
func (r *Registry) Register(name string, provider NewProviderFunc) error {
	return r.RegisterExact(name+"_sp_config", provider)
}

// Register registers the given Provider for YAML marshalling and unmarshalling.
func (r *Registry) RegisterExact(name string, provider NewProviderFunc) error {
	if provider == nil {
		return fmt.Errorf("secrets provider must not be nil")
	}
	if _, ok := r.configMap[name]; ok {
		return fmt.Errorf("secrets provider named %q is already registered", name)
	}
	r.configMap[name] = provider
	return nil
}
