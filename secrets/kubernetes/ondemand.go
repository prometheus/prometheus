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

package kubernetes

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/prometheus/prometheus/secrets"
	"github.com/prometheus/prometheus/secrets/provider"
)

type OnDemandSPConfig struct {
	ClientConfig
	SecretConfig
}

func init() {
	secrets.RegisterProvider("kubernetes_on_demand", func(ctx context.Context, opts secrets.ProviderOptions, _ prometheus.Registerer) secrets.Provider[*yaml.Node] {
		return provider.MapNode(provider.NewMultiProvider(
			ctx,
			provider.WithClientConfigFunc(func(config *OnDemandSPConfig) (*ClientConfig, error) {
				return &config.ClientConfig, nil
			}),
			provider.WithNewProviderFunc[*OnDemandSPConfig, *ClientConfig](func(_ context.Context, config *OnDemandSPConfig) (secrets.Provider[*OnDemandSPConfig], error) {
				return config.getProvider()
			}),
		))
	})
}

func (config *OnDemandSPConfig) getProvider() (secrets.Provider[*OnDemandSPConfig], error) {
	client, err := config.client()
	if err != nil {
		return nil, err
	}
	return provider.Map(
		func(config *OnDemandSPConfig) (*SecretConfig, error) {
			return &config.SecretConfig, nil
		},
		provider.SecretProvider(func(config *SecretConfig) (secrets.Secret, error) {
			return fetchSecret(client, config)
		}),
	), nil
}

func fetchSecret(clientSet kubernetes.Interface, config *SecretConfig) (secrets.Secret, error) {
	return secrets.SecretFunc(func(ctx context.Context) (string, error) {
		secret, err := clientSet.CoreV1().Secrets(config.Namespace).Get(ctx, config.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return "", fmt.Errorf("secret %s/%s not found", config.Namespace, config.Name)
			}
			if apierrors.IsForbidden(err) {
				return "", fmt.Errorf("secret %s/%s forbidden", config.Namespace, config.Name)
			}
			return "", fmt.Errorf("secret %s/%s: %w", config.Namespace, config.Name, err)
		}
		return getValue(secret, config.Key)
	}), nil
}
