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

package provider

import (
	"bytes"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"

	"github.com/prometheus/prometheus/secrets"
)

// DynamicProvider is a secret provider which tracks secret changes.
type DynamicProvider[T any] interface {
	// Add returns the secret fetcher for the given configuration.
	Add(config T) (secrets.Secret, error)

	// Update returns the secret fetcher for the new configuration.
	Update(configBefore, configAfter T) (secrets.Secret, error)

	// Remove ensures that the secret fetcher for the configuration is invalid.
	Remove(config T) error

	Close(reg prometheus.Registerer)
}

type dynamicManager[T any] struct {
	provider  DynamicProvider[T]
	secrets   map[string]*secretEntry[T]
	equalFunc func(x, y T) (bool, error)
}

type DynamicManagerOpt[T any] interface {
	apply(m *dynamicManager[T])
}

type dynamicManagerOptFunc[T any] func(*dynamicManager[T])

func (f *dynamicManagerOptFunc[T]) apply(m *dynamicManager[T]) {
	(*f)(m)
}

func yamlSerialize(obj any) ([]byte, error) {
	if obj == nil {
		return []byte{}, nil
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	if err := encoder.Encode(obj); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func yamlEqual(x, y any) (bool, error) {
	yamlX, err := yamlSerialize(x)
	if err != nil {
		return false, err
	}
	yamlY, err := yamlSerialize(y)
	if err != nil {
		return false, err
	}
	return bytes.Equal(yamlX, yamlY), nil
}

func NewDynamicProvider[T any](provider DynamicProvider[T], opts ...DynamicManagerOpt[T]) secrets.Provider[T] {
	m := &dynamicManager[T]{
		provider: provider,
		secrets:  map[string]*secretEntry[T]{},
	}
	for _, opt := range opts {
		opt.apply(m)
	}
	if m.equalFunc == nil {
		m.equalFunc = func(x, y T) (bool, error) {
			return yamlEqual(x, y)
		}
	}
	return m
}

func (m *dynamicManager[T]) Apply(configs []secrets.Config[T]) ([]secrets.Secret, error) {
	secretsFinal := map[string]*secretEntry[T]{}
	out := make([]secrets.Secret, 0, len(configs))
	for _, config := range configs {
		entry, err := m.updateSecret(config.Name, config.Data)
		if err != nil {
			return nil, err
		}
		secretsFinal[config.Name] = entry
		out = append(out, entry.secret)
	}
	for _, secretUnused := range m.secrets {
		if err := m.provider.Remove(secretUnused.data); err != nil {
			return nil, err
		}
	}

	m.secrets = secretsFinal
	return out, nil
}

func (m *dynamicManager[T]) updateSecret(name string, dataIncoming T) (*secretEntry[T], error) {
	// First check if we've registered this secret before.
	if secretPrevious, ok := m.secrets[name]; ok {
		// Track all the secrets we saw. The leftover are later removed.
		delete(m.secrets, name)

		// If the config didn't change, we skip this one.
		eq, err := m.equalFunc(secretPrevious.data, dataIncoming)
		if err != nil {
			return nil, err
		}
		if eq {
			return secretPrevious, err
		}

		// The config changed, so update it.
		s, err := m.provider.Update(secretPrevious.data, dataIncoming)
		if err != nil {
			return nil, err
		}
		secretPrevious.secret = s
		return secretPrevious, err
	}

	// We've never seen this secret before, so add it.
	s, err := m.provider.Add(dataIncoming)
	if err != nil {
		return nil, err
	}
	return &secretEntry[T]{
		data:   dataIncoming,
		secret: s,
	}, nil
}

func (m *dynamicManager[T]) Close(reg prometheus.Registerer) {
	m.provider.Close(reg)
}

func WithEqualFunc[T any](f func(x, y T) (bool, error)) DynamicManagerOpt[T] {
	opt := dynamicManagerOptFunc[T](func(m *dynamicManager[T]) {
		m.equalFunc = f
	})
	return &opt
}

type secretEntry[T any] struct {
	data   T
	secret secrets.Secret
}
