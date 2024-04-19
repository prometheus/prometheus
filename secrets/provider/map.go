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
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"

	"github.com/prometheus/prometheus/secrets"
)

// Map maps one provider type to another using the given conversion function.
func Map[T, U any](f func(config T) (U, error), provider secrets.Provider[U]) secrets.Provider[T] {
	return &mapOperation[T, U]{
		f:        f,
		provider: provider,
	}
}

type mapOperation[T, U any] struct {
	f        func(config T) (U, error)
	provider secrets.Provider[U]
}

// Apply implements Provider.
func (m *mapOperation[T, U]) Apply(configs []secrets.Config[T]) ([]secrets.Secret, error) {
	out := make([]secrets.Config[U], 0, len(configs))
	for _, config := range configs {
		secret, err := m.f(config.Data)
		if err != nil {
			return nil, err
		}
		out = append(out, secrets.Config[U]{
			Name: config.Name,
			Data: secret,
		})
	}
	return m.provider.Apply(out)
}

func (m *mapOperation[T, U]) Close(reg prometheus.Registerer) {
	m.provider.Close(reg)
}

// MapNode returns a provider that maps a YAML node to a secret.
func MapNode[T any](provider secrets.Provider[T]) secrets.Provider[*yaml.Node] {
	return Map(func(config *yaml.Node) (T, error) {
		if config == nil {
			var empty T
			return empty, errors.New("yaml: empty node")
		}
		return yamlUnmarshal[T](config)
	}, provider)
}

func yamlUnmarshal[T any](node *yaml.Node) (T, error) {
	var t T
	if err := node.Decode(&t); err != nil {
		return t, err
	}
	return t, nil
}

type SecretProviderFuncs[T any] struct {
	ApplyFunc func(config []secrets.Config[T]) ([]secrets.Secret, error)
	CloseFunc func(reg prometheus.Registerer)
}

func (f SecretProviderFuncs[T]) Apply(configs []secrets.Config[T]) ([]secrets.Secret, error) {
	return f.ApplyFunc(configs)
}

func (f SecretProviderFuncs[T]) Close(reg prometheus.Registerer) {
	f.CloseFunc(reg)
}

// SecretProvider provides a single secret given a configuration object.
func SecretProvider[T any](f func(config T) (secrets.Secret, error)) secrets.Provider[T] {
	return &SecretProviderFuncs[T]{
		ApplyFunc: func(configs []secrets.Config[T]) ([]secrets.Secret, error) {
			out := make([]secrets.Secret, 0, len(configs))
			for _, config := range configs {
				secret, err := f(config.Data)
				if err != nil {
					return nil, err
				}
				out = append(out, secret)
			}
			return out, nil
		},
		CloseFunc: func(reg prometheus.Registerer) {
			// no-op
		},
	}
}

// SecretDataProvider provides a single secret value given a configuration object.
func SecretDataProvider[T any](f func(ctx context.Context, config T) (string, error)) secrets.Provider[T] {
	return SecretProvider[T](func(config T) (secrets.Secret, error) {
		return secrets.SecretFunc(func(ctx context.Context) (string, error) {
			return f(ctx, config)
		}), nil
	})
}
