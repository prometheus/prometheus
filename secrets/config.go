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

import (
	"context"
	"fmt"
	"slices"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
)

// Secret represents a sensitive value.
type Secret interface {
	// Fetch fetches the secret content. The context is expected to live
	// the duration of a scrape. It may be cancelled to indicate a
	// timeout. The returned error will be visible to users when they
	// query scrape targets.
	Fetch(ctx context.Context) (string, error)
}

// SecretFunc wraps a function to make it a Secret.
func SecretFunc(f func(ctx context.Context) (string, error)) Secret {
	s := secretFunc(f)
	return &s
}

type secretFunc func(ctx context.Context) (string, error)

// Fetch implements Secret.Fetch.
func (f *secretFunc) Fetch(ctx context.Context) (string, error) {
	return (*f)(ctx)
}

// Config is the secret configuration.
type Config[T any] struct {
	// Name is the name of the secret, used in "ref" fields.
	Name string `yaml:"name"`
	// Data contains the secret configuration.
	Data T `yaml:",inline"`
}

// ProviderConfig flattens all secret configurations for a single secret
// provider.
type ProviderConfig struct {
	// Name is the name of the provider.
	Name string
	// Configs are the secret configurations.
	Configs []Config[*yaml.Node]
}

// Configs is an nested collection of Config. For custom marshalling and
// unmarshalling, use a pointer to this type.
type Configs []ProviderConfig

// UnmarshalYAML implements yaml.v2.Unmarshaler.
func (c *Configs) UnmarshalYAML(unmarshal func(interface{}) error) error {
	data := []Config[map[string]any]{}
	if err := unmarshal(&data); err != nil {
		return fmt.Errorf("unable to unmarshal secret configurations: %w", err)
	}
	m := map[string]*ProviderConfig{}
	for _, configMap := range data {
		if secretProviderCount := len(configMap.Data); secretProviderCount == 0 {
			return fmt.Errorf("expected secret provider but found none")
		} else if secretProviderCount > 1 {
			return fmt.Errorf("expected single secret provider but found %d", len(configMap.Data))
		}
		// For loop, but only 1 entry ever.
		for name := range configMap.Data {
			node := &yaml.Node{}
			if err := node.Encode(configMap.Data[name]); err != nil {
				return fmt.Errorf("unable to decode secret configuration: %w", err)
			}

			// Since we decode from any, we could potentially have a
			// non-deterministic map.
			nodeMappingSort(node)

			providerList, ok := m[name]
			if !ok {
				*c = append(*c, ProviderConfig{
					Name:    name,
					Configs: []Config[*yaml.Node]{},
				})
				// Get reference to latest entry.
				providerList = &((*c)[len(*c)-1])
				m[name] = providerList
			}
			providerList.Configs = append(providerList.Configs, Config[*yaml.Node]{
				Name: configMap.Name,
				Data: node,
			})
		}
	}
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (c *Configs) MarshalYAML() (interface{}, error) {
	// Must marshal as any because yaml.v3.Node doesn't work in yaml.v2.
	data := []Config[map[string]any]{}
	for _, providerConfig := range *c {
		for _, secretConfig := range providerConfig.Configs {
			if secretConfig.Data.IsZero() {
				continue
			}
			var secretData any
			if err := secretConfig.Data.Decode(&secretData); err != nil {
				return nil, err
			}
			data = append(data, Config[map[string]any]{
				Name: secretConfig.Name,
				Data: map[string]any{
					providerConfig.Name: secretData,
				},
			})
		}
	}
	return data, nil
}

// nodeMappingSort ensures mapping nodes have sorted key/value pair
// ordering for deterministic YAML output.
// TODO(TheSpiritXIII): This is no longer necessary with yaml.v3.
func nodeMappingSort(node *yaml.Node) {
	if node.Kind == yaml.MappingNode {
		var keys []string
		valueNodes := map[string]*yaml.Node{}
		keyNodes := map[string]*yaml.Node{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			keyNodes[key.Value] = key
			valueNodes[key.Value] = node.Content[i+1]
			keys = append(keys, key.Value)
		}

		var content []*yaml.Node
		slices.Sort(keys)
		for _, key := range keys {
			content = append(content, keyNodes[key], valueNodes[key])
		}
		node.Content = content
	}
}

// Provider represents a secret provider.
type Provider[T any] interface {
	// Apply decodes the given configurations and return the associated
	// secrets. The returned secret list must match index-by-index of the
	// original configuration list. An error will prevent the
	// configuration from being loaded and must be avoided unless strictly
	// necessary, such as a decoding issue. Errors are best reported
	// during secret fetch-time.
	Apply(configs []Config[T]) ([]Secret, error)

	// Close closes this provider, unregistering any provider-specific
	// metrics if the metrics registry is provided.
	Close(reg prometheus.Registerer)
}

// ProviderFuncs allows creating a Provider from a set of functions.
type ProviderFuncs[T any] struct {
	// ApplyFunc implements Provider.Apply.
	ApplyFunc func(configs []Config[T]) ([]Secret, error)
	// CloseFunc implements Provider.Close.
	CloseFunc func(reg prometheus.Registerer)
}

// Apply implements Provider.Apply.
func (f *ProviderFuncs[T]) Apply(configs []Config[T]) ([]Secret, error) {
	return f.ApplyFunc(configs)
}

// Close implements Provider.Close.
func (f *ProviderFuncs[T]) Close(reg prometheus.Registerer) {
	if f.CloseFunc != nil {
		f.CloseFunc(reg)
	}
}

// NewProviderFunc creates a new Provider using the given options. The
// context is expected to live the entire duration of the provider. As
// such, providers may store and use it for fetching secrets in the
// background where they need a longer timeout than the one provided by
// the scrape context.
type NewProviderFunc func(ctx context.Context, opts ProviderOptions, reg prometheus.Registerer) Provider[*yaml.Node]

// ProviderOptions provides options for a Provider.
type ProviderOptions struct {
	// Logger is a dedicated logger for the provider.
	Logger log.Logger
}
