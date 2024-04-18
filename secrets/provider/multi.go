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

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/secrets"
)

type providerEntry[T any] struct {
	provider   secrets.Provider[T]
	cancelFunc context.CancelFunc
}

type clientProvider[TBase any, TClientConfig comparable, TSecretConfig any] struct {
	ctx              context.Context
	newProviderFunc  func(ctx context.Context, config TBase) (secrets.Provider[TBase], error)
	clientConfigFunc func(config TBase) (TClientConfig, error)
	clientMap        map[TClientConfig]providerEntry[TBase]
}

type ClientProviderOpt[T any, U comparable] interface {
	apply(*clientProvider[T, U, T])
}

type clientProviderOpt[T any, U comparable] func(m *clientProvider[T, U, T])

func (o *clientProviderOpt[T, U]) apply(m *clientProvider[T, U, T]) {
	(*o)(m)
}

func WithNewProviderFunc[T any, U comparable](f func(ctx context.Context, config T) (secrets.Provider[T], error)) ClientProviderOpt[T, U] {
	opt := clientProviderOpt[T, U](func(m *clientProvider[T, U, T]) {
		m.newProviderFunc = f
	})
	return &opt
}

func WithClientConfigFunc[T any, U comparable](f func(config T) (U, error)) ClientProviderOpt[T, U] {
	opt := clientProviderOpt[T, U](func(m *clientProvider[T, U, T]) {
		m.clientConfigFunc = f
	})
	return &opt
}

func NewMultiProvider[T any, U comparable](ctx context.Context, opts ...ClientProviderOpt[T, U]) secrets.Provider[T] {
	m := &clientProvider[T, U, T]{
		ctx: ctx,
	}
	for _, opt := range opts {
		opt.apply(m)
	}
	return m
}

func (m *clientProvider[T, U, V]) Apply(configs []secrets.Config[T]) ([]secrets.Secret, error) {
	clientsFinal := make(map[U]providerEntry[T], len(m.clientMap))
	out := make([]secrets.Secret, len(configs))

	clientsIndices := map[U][]int{}
	for i := range configs {
		secretIncoming := &configs[i]
		clientConfig, err := m.clientConfigFunc(secretIncoming.Data)
		if err != nil {
			return nil, err
		}

		provider, ok := m.clientMap[clientConfig]
		if !ok {
			ctx, cancelFunc := context.WithCancel(m.ctx)
			p, err := m.newProviderFunc(ctx, configs[i].Data)
			if err != nil {
				defer cancelFunc()
				return nil, err
			}
			provider = providerEntry[T]{
				provider:   p,
				cancelFunc: cancelFunc,
			}
		}
		clientsFinal[clientConfig] = provider

		indices := clientsIndices[clientConfig]
		indices = append(indices, i)
		clientsIndices[clientConfig] = indices
	}
	for clientConfig, indices := range clientsIndices {
		providerConfigs := make([]secrets.Config[T], 0, len(indices))
		for _, index := range indices {
			providerConfigs = append(providerConfigs, configs[index])
		}
		secrets, err := clientsFinal[clientConfig].provider.Apply(providerConfigs)
		if err != nil {
			return nil, err
		}
		for i := range secrets {
			index := clientsIndices[clientConfig][i]
			out[index] = secrets[i]
		}
	}
	for _, providerEntry := range m.clientMap {
		providerEntry.cancelFunc()
	}

	m.clientMap = clientsFinal
	return out, nil
}

func (m *clientProvider[T, U, V]) Close(reg prometheus.Registerer) {
	for _, provider := range m.clientMap {
		provider.provider.Close(reg)
	}
	m.clientMap = nil
}
