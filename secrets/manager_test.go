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
	"errors"
	"testing"
	"time"

	"go.uber.org/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestManager(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		description string
		testFunc    func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry)
	}{
		{
			description: "empty",
			testFunc: func(ctx context.Context, t testing.TB, _ *Manager, registry *Registry) {
				manager := newManager(ctx, registry, ProviderOptions{}, nil)
				err := manager.ApplyConfig(Configs{})
				require.NoError(t, err)

				err = manager.ApplyConfig(nil)
				require.NoError(t, err)
				manager.Close()
			},
		},
		{
			description: "invalid provider",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)

				err := manager.ApplyConfig(Configs{
					ProviderConfig{
						Name: "c",
						Configs: []Config[*yaml.Node]{
							{
								Name: "i",
								Data: &yaml.Node{},
							},
							{
								Name: "j",
								Data: &yaml.Node{},
							},
						},
					},
				})
				require.ErrorContains(t, err, "unknown secret provider c")

				requireTestConfigs(ctx, t, manager)
				_, err = manager.Fetch(ctx, "i")
				require.ErrorContains(t, err, "secret \"i\" not found")
			},
		},
		{
			description: "add single secret",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				err := manager.ApplyConfig(Configs{
					ProviderConfig{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "foo",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				var secret string
				secretsBefore := manager.secrets
				require.Len(t, secretsBefore, 1)
				secret, err = manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "foo", secret)

				providersBefore := manager.providers
				require.Len(t, providersBefore, 1)
				require.Contains(t, providersBefore, "a_sp_config")

				err = manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "bar",
								},
							},
							{
								Name: "y",
								Data: &yaml.Node{
									Value: "baz",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				require.Len(t, manager.secrets, 2)
				secret, err = manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "bar", secret)
				secret, err = manager.Fetch(ctx, "y")
				require.NoError(t, err)
				require.Equal(t, "baz", secret)

				providersAfter := manager.providers
				require.Len(t, providersAfter, 1)
				require.Equal(t, providersBefore["a_sp_config"], providersAfter["a_sp_config"])
			},
		},
		{
			description: "add single provider",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				err := manager.ApplyConfig(Configs{
					ProviderConfig{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "foo",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				var secret string
				secretsBefore := manager.secrets
				require.Len(t, secretsBefore, 1)
				secret, err = manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "foo", secret)

				providersBefore := manager.providers
				require.Len(t, providersBefore, 1)
				require.Contains(t, providersBefore, "a_sp_config")

				err = manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "foo",
								},
							},
						},
					},
					{
						Name: "b",
						Configs: []Config[*yaml.Node]{
							{
								Name: "z",
								Data: &yaml.Node{
									Value: "baz",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				require.Len(t, manager.secrets, 2)
				secret, err = manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "foo", secret)
				secret, err = manager.Fetch(ctx, "z")
				require.NoError(t, err)
				require.Equal(t, "baz", secret)

				providersAfter := manager.providers
				require.Len(t, providersAfter, 2)
				require.Equal(t, providersBefore["a_sp_config"], providersAfter["a_sp_config"])
			},
		},
		{
			description: "add multiple",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				err := manager.ApplyConfig(Configs{})
				require.NoError(t, err)
				applyTestConfigs(t, manager)
				requireTestConfigs(ctx, t, manager)
			},
		},
		{
			description: "remove single secret",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				providersBefore := manager.providers

				// Remove secret y.
				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "foo",
								},
							},
						},
					},
					{
						Name: "b",
						Configs: []Config[*yaml.Node]{
							{
								Name: "z",
								Data: &yaml.Node{
									Value: "baz",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				require.Len(t, manager.secrets, 2)
				secret, err := manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "foo", secret)
				secret, err = manager.Fetch(ctx, "z")
				require.NoError(t, err)
				require.Equal(t, "baz", secret)

				providersAfter := manager.providers
				require.Len(t, providersAfter, 2)
				require.Equal(t, providersBefore["a_sp_config"], providersAfter["a_sp_config"])
				require.Equal(t, providersBefore["b"], providersAfter["b"])
			},
		},
		{
			description: "remove single provider",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				providersBefore := manager.providers

				// Remove secret z.
				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "foo",
								},
							},
							{
								Name: "y",
								Data: &yaml.Node{
									Value: "bar",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				require.Len(t, manager.secrets, 2)
				secret, err := manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "foo", secret)
				secret, err = manager.Fetch(ctx, "y")
				require.NoError(t, err)
				require.Equal(t, "bar", secret)

				providersAfter := manager.providers
				require.Len(t, providersAfter, 1)
				require.Equal(t, providersBefore["a_sp_config"], providersAfter["a_sp_config"])
			},
		},
		{
			description: "remove multiple",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				providersBefore := manager.providers

				// Remove secrets x and z.
				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "y",
								Data: &yaml.Node{
									Value: "bar",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				require.Len(t, manager.secrets, 1)
				secret, err := manager.Fetch(ctx, "y")
				require.NoError(t, err)
				require.Equal(t, "bar", secret)

				providersAfter := manager.providers
				require.Len(t, providersAfter, 1)
				require.Equal(t, providersBefore["a_sp_config"], providersAfter["a_sp_config"])
			},
		},
		{
			description: "remove all",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)

				err := manager.ApplyConfig(Configs{})
				require.NoError(t, err)
				require.Empty(t, manager.secrets)
				require.Empty(t, manager.providers)
			},
		},
		{
			description: "reorder",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				providersBefore := manager.providers

				err := manager.ApplyConfig(Configs{
					{
						Name: "b",
						Configs: []Config[*yaml.Node]{
							{
								Name: "z",
								Data: &yaml.Node{
									Value: "baz",
								},
							},
						},
					},
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "y",
								Data: &yaml.Node{
									Value: "bar",
								},
							},
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "foo",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				require.Len(t, manager.secrets, 3)
				secret, err := manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "foo", secret)
				secret, err = manager.Fetch(ctx, "y")
				require.NoError(t, err)
				require.Equal(t, "bar", secret)
				secret, err = manager.Fetch(ctx, "z")
				require.NoError(t, err)
				require.Equal(t, "baz", secret)

				providersAfter := manager.providers
				require.Len(t, providersAfter, 2)
				require.Equal(t, providersBefore["a_sp_config"], providersAfter["a_sp_config"])
				require.Equal(t, providersBefore["b"], providersAfter["b"])
			},
		},
		{
			description: "update secret name",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				providersBefore := manager.providers

				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "foo",
								},
							},
							{
								Name: "i",
								Data: &yaml.Node{
									Value: "bar",
								},
							},
						},
					},
					{
						Name: "b",
						Configs: []Config[*yaml.Node]{
							{
								Name: "z",
								Data: &yaml.Node{
									Value: "baz",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				require.Len(t, manager.secrets, 3)
				secret, err := manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "foo", secret)
				secret, err = manager.Fetch(ctx, "i")
				require.NoError(t, err)
				require.Equal(t, "bar", secret)
				secret, err = manager.Fetch(ctx, "z")
				require.NoError(t, err)
				require.Equal(t, "baz", secret)

				providersAfter := manager.providers
				require.Len(t, providersAfter, 2)
				require.Equal(t, providersBefore["a_sp_config"], providersAfter["a_sp_config"])
				require.Equal(t, providersBefore["b"], providersAfter["b"])
			},
		},
		{
			description: "update secret value",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				providersBefore := manager.providers

				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{
									Value: "foo",
								},
							},
							{
								Name: "y",
								Data: &yaml.Node{
									Value: "qux",
								},
							},
						},
					},
					{
						Name: "b",
						Configs: []Config[*yaml.Node]{
							{
								Name: "z",
								Data: &yaml.Node{
									Value: "baz",
								},
							},
						},
					},
				})
				require.NoError(t, err)

				require.Len(t, manager.secrets, 3)
				secret, err := manager.Fetch(ctx, "x")
				require.NoError(t, err)
				require.Equal(t, "foo", secret)
				secret, err = manager.Fetch(ctx, "y")
				require.NoError(t, err)
				require.Equal(t, "qux", secret)
				secret, err = manager.Fetch(ctx, "z")
				require.NoError(t, err)
				require.Equal(t, "baz", secret)

				providersAfter := manager.providers
				require.Len(t, providersAfter, 2)
				require.Equal(t, providersBefore["a_sp_config"], providersAfter["a_sp_config"])
				require.Equal(t, providersBefore["b"], providersAfter["b"])
			},
		},
		{
			description: "duplicate secret",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{},
							},
							{
								Name: "y",
								Data: &yaml.Node{},
							},
							{
								Name: "z",
								Data: &yaml.Node{},
							},
						},
					},
					{
						Name: "b",
						Configs: []Config[*yaml.Node]{
							{
								Name: "z",
								Data: &yaml.Node{},
							},
						},
					},
				})
				require.ErrorContains(t, err, "secret \"z\" already exists")
				requireTestConfigs(ctx, t, manager)
			},
		},
		{
			description: "cancel context",
			testFunc: func(ctx context.Context, t testing.TB, _ *Manager, registry *Registry) {
				reg := prometheus.NewRegistry()
				ctx, cancel := context.WithCancel(ctx)
				manager := newManager(ctx, registry, ProviderOptions{}, reg)
				applyTestConfigs(t, &manager)
				providersBefore := manager.providers

				cancel()
				// There may be a delay before we a provider knows it's canceled so wait a bit.
				timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), time.Second)
				defer timeoutCancel()
				for name, provider := range providersBefore {
					testProvider := provider.provider.(*testProvider)
					for {
						if timeoutCtx.Err() != nil {
							// On timeout, last chance to ensure the provider is closed.
							require.True(t, testProvider.closed.Load(), "provider", name)
						}
						if testProvider.closed.Load() {
							break
						}
					}
				}
			},
		},
		{
			description: "close",
			testFunc: func(ctx context.Context, t testing.TB, _ *Manager, registry *Registry) {
				reg := prometheus.NewRegistry()
				manager := newManager(ctx, registry, ProviderOptions{}, reg)
				applyTestConfigs(t, &manager)
				providersBefore := manager.providers
				metricCount, err := testutil.GatherAndCount(reg)
				require.NoError(t, err)
				require.Equal(t, 2, metricCount)

				manager.Close()

				require.Empty(t, manager.providers)
				require.Empty(t, manager.secrets)
				_, err = manager.Fetch(ctx, "x")
				require.ErrorContains(t, err, "secret \"x\" not found")
				_, err = manager.Fetch(ctx, "y")
				require.ErrorContains(t, err, "secret \"y\" not found")
				_, err = manager.Fetch(ctx, "z")
				require.ErrorContains(t, err, "secret \"z\" not found")

				for name, provider := range providersBefore {
					cast := provider.provider.(*testProvider)
					require.True(t, cast.closed.Load(), "provider", name)
				}

				metricCount, err = testutil.GatherAndCount(reg)
				require.NoError(t, err)
				require.Equal(t, 0, metricCount)
			},
		},
		{
			description: "fail-secret-provider",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{},
							},
							{
								Name: "y",
								Data: &yaml.Node{},
							},
						},
					},
					{
						Name: "fail_nil",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{},
							},
							{
								Name: "y",
								Data: &yaml.Node{},
							},
						},
					},
				})
				require.ErrorContains(t, err, "secret provider \"fail_nil\" returned nil provider")
				requireTestConfigs(ctx, t, manager)
			},
		},
		{
			description: "fail-apply-error",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{},
							},
							{
								Name: "y",
								Data: &yaml.Node{},
							},
						},
					},
					{
						Name: "fail_apply",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{},
							},
							{
								Name: "y",
								Data: &yaml.Node{},
							},
						},
					},
				})
				require.ErrorContains(t, err, "secret provider \"fail_apply\" returned error: failed")
				requireTestConfigs(ctx, t, manager)
			},
		},
		{
			description: "fail-apply-secrets",
			testFunc: func(ctx context.Context, t testing.TB, manager *Manager, registry *Registry) {
				applyTestConfigs(t, manager)
				err := manager.ApplyConfig(Configs{
					{
						Name: "a_sp_config",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{},
							},
							{
								Name: "y",
								Data: &yaml.Node{},
							},
						},
					},
					{
						Name: "fail_no_secrets",
						Configs: []Config[*yaml.Node]{
							{
								Name: "x",
								Data: &yaml.Node{},
							},
							{
								Name: "y",
								Data: &yaml.Node{},
							},
						},
					},
				})
				require.ErrorContains(t, err, "secret provider \"fail_no_secrets\" returned unexpected number of secrets")
				requireTestConfigs(ctx, t, manager)
			},
		},
	}
	for _, tc := range testCases {
		reg := prometheus.NewRegistry()
		registry := NewRegistry()
		err := registry.Register("a", func(ctx context.Context, opts ProviderOptions, reg prometheus.Registerer) Provider[*yaml.Node] {
			metric := prometheus.NewCounter(prometheus.CounterOpts{
				Name: "a_sp_metric",
			})
			reg.MustRegister(metric)
			return newTestProvider(ctx, metric)
		})
		require.NoError(t, err)
		err = registry.RegisterExact("b", func(ctx context.Context, opts ProviderOptions, reg prometheus.Registerer) Provider[*yaml.Node] {
			metric := prometheus.NewCounter(prometheus.CounterOpts{
				Name: "b_sp_metric",
			})
			reg.MustRegister(metric)
			return newTestProvider(ctx, metric)
		})
		require.NoError(t, err)

		err = registry.RegisterExact("fail_nil", func(ctx context.Context, opts ProviderOptions, reg prometheus.Registerer) Provider[*yaml.Node] {
			return nil
		})
		require.NoError(t, err)

		err = registry.RegisterExact("fail_apply", func(ctx context.Context, opts ProviderOptions, reg prometheus.Registerer) Provider[*yaml.Node] {
			return &ProviderFuncs[*yaml.Node]{
				ApplyFunc: func(configs []Config[*yaml.Node]) ([]Secret, error) {
					return nil, errors.New("failed")
				},
			}
		})
		require.NoError(t, err)

		err = registry.RegisterExact("fail_no_secrets", func(ctx context.Context, opts ProviderOptions, reg prometheus.Registerer) Provider[*yaml.Node] {
			return &ProviderFuncs[*yaml.Node]{
				ApplyFunc: func(configs []Config[*yaml.Node]) ([]Secret, error) {
					return nil, nil
				},
			}
		})
		require.NoError(t, err)

		t.Run(tc.description, func(t *testing.T) {
			manager := newManager(ctx, &registry, ProviderOptions{}, reg)
			tc.testFunc(ctx, t, &manager, &registry)
			manager.Close()
		})
	}
}

func applyTestConfigs(t testing.TB, manager *Manager) {
	err := manager.ApplyConfig(Configs{
		{
			Name: "a_sp_config",
			Configs: []Config[*yaml.Node]{
				{
					Name: "x",
					Data: &yaml.Node{
						Value: "foo",
					},
				},
				{
					Name: "y",
					Data: &yaml.Node{
						Value: "bar",
					},
				},
			},
		},
		{
			Name: "b",
			Configs: []Config[*yaml.Node]{
				{
					Name: "z",
					Data: &yaml.Node{
						Value: "baz",
					},
				},
			},
		},
	})
	require.NoError(t, err)
}

// requireTestConfigs asserts that configs from applyTestConfigs are still
// valid. This is generally used after an error, to ensure the old
// configurations are preserved.
func requireTestConfigs(ctx context.Context, t testing.TB, manager *Manager) {
	providersBefore := manager.providers
	require.Len(t, providersBefore, 2)
	require.Contains(t, providersBefore, "a_sp_config")
	require.Contains(t, providersBefore, "b")

	require.Len(t, manager.secrets, 3)
	secret, err := manager.Fetch(ctx, "x")
	require.NoError(t, err)
	require.Equal(t, "foo", secret)
	secret, err = manager.Fetch(ctx, "y")
	require.NoError(t, err)
	require.Equal(t, "bar", secret)
	secret, err = manager.Fetch(ctx, "z")
	require.NoError(t, err)
	require.Equal(t, "baz", secret)
}

type testProvider struct {
	metric prometheus.Counter
	closed atomic.Bool
}

func newTestProvider(ctx context.Context, metric prometheus.Counter) *testProvider {
	provider := &testProvider{
		closed: atomic.Bool{},
		metric: metric,
	}
	go func() {
		<-ctx.Done()
		provider.closed.Store(true)
	}()
	return provider
}

func (p *testProvider) Apply(configs []Config[*yaml.Node]) ([]Secret, error) {
	var secrets []Secret
	for _, config := range configs {
		data := config.Data
		secrets = append(secrets, SecretFunc(func(_ context.Context) (string, error) {
			if data == nil {
				return "", errors.New("secret data is nil")
			}
			return data.Value, nil
		}))
	}
	return secrets, nil
}

func (p *testProvider) Close(reg prometheus.Registerer) {
	reg.Unregister(p.metric)
	p.closed.Store(true)
}
