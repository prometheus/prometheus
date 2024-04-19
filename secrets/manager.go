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
	"sync"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
)

// Manager loads and stores secrets from a configuration.
type Manager struct {
	registry   *Registry
	registerer prometheus.Registerer
	ctx        context.Context
	opts       ProviderOptions
	mtx        sync.Mutex

	secrets   map[string]Secret
	providers map[string]*providerEntry
}

type providerEntry struct {
	cancelFn context.CancelFunc
	provider Provider[*yaml.Node]
}

// NewManager creates a new manager using the global registry.
func NewManager(ctx context.Context, opts ProviderOptions, registerer prometheus.Registerer) Manager {
	return newManager(ctx, &globalRegistry, opts, registerer)
}

func newManager(ctx context.Context, registry *Registry, opts ProviderOptions, registerer prometheus.Registerer) Manager {
	return Manager{
		registry:   registry,
		registerer: registerer,
		ctx:        ctx,
		opts:       opts,
		mtx:        sync.Mutex{},
		secrets:    map[string]Secret{},
		providers:  map[string]*providerEntry{},
	}
}

// ApplyConfig loads all secrets for the given configurations.
func (m *Manager) ApplyConfig(configs Configs) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	providersFinal := make(map[string]*providerEntry, len(configs))
	secretsFinal := map[string]Secret{}

	// failUndo undoes all staged changes and reverts to the previous state.
	failUndo := func() {
		for _, providerConfig := range providersFinal {
			providerConfig.cancelFn()
		}
	}
	for _, providerData := range configs {
		providerName := providerData.Name
		providerConfigs := providerData.Configs
		entry, ok := m.providers[providerName]
		if !ok {
			newProviderFunc := m.registry.Get(providerName)
			if newProviderFunc == nil {
				return fmt.Errorf("unknown secret provider %s", providerName)
			}

			ctx, cancel := context.WithCancel(m.ctx)
			provider := newProviderFunc(
				ctx,
				ProviderOptions{
					Logger: log.With(m.opts.Logger, "provider", providerName),
				},
				m.registerer,
			)
			if provider == nil {
				cancel()
				failUndo()
				return fmt.Errorf("secret provider %q returned nil provider", providerName)
			}

			entry = &providerEntry{
				cancelFn: cancel,
				provider: provider,
			}
		}
		providersFinal[providerName] = entry
		secrets, err := entry.provider.Apply(providerConfigs)
		if err != nil {
			failUndo()
			return fmt.Errorf("secret provider %q returned error: %w", providerName, err)
		}
		if len(secrets) != len(providerConfigs) {
			failUndo()
			return fmt.Errorf("secret provider %q returned unexpected number of secrets", providerName)
		}
		for i := range secrets {
			secretName := providerConfigs[i].Name
			if _, ok := secretsFinal[secretName]; ok {
				failUndo()
				return fmt.Errorf("secret %q already exists", secretName)
			}
			secretsFinal[secretName] = secrets[i]
		}
	}

	for _, entryUnused := range m.providers {
		entryUnused.cancelFn()
	}
	m.providers = providersFinal
	m.secrets = secretsFinal

	return nil
}

// Fetch implements github.com/prometheus/common/config.SecretManager.Fetch.
func (m *Manager) Fetch(ctx context.Context, name string) (string, error) {
	secret, ok := m.secrets[name]
	if !ok {
		return "", fmt.Errorf("secret %q not found", name)
	}
	return secret.Fetch(ctx)
}

// Close cancels the manager, stopping all secret providers.
func (m *Manager) Close() {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	for _, entry := range m.providers {
		entry.provider.Close(m.registerer)
		entry.cancelFn()
	}
	m.providers = nil
	m.secrets = nil
}
