// Copyright 2025 The Prometheus Authors
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
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
)

func TestKubernetesNamespaceEnricherBasicFunctionality(t *testing.T) {
	// Create fake Kubernetes client
	client := fake.NewSimpleClientset()

	// Create test namespace with annotations and labels
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
			Labels: map[string]string{
				"tier":        "production",
				"environment": "prod",
			},
			Annotations: map[string]string{
				"application_owner": "team-a",
				"cost_center":       "engineering",
				"other_annotation":  "ignored",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create enricher config
	cfg := &config.NamespaceEnrichmentConfig{
		Enabled: true,
		LabelSelector: []string{
			"tier",
			"environment",
		},
		AnnotationSelector: []string{
			"application_owner",
			"cost_center",
		},
		LabelPrefix: "ns_",
	}

	// Create enricher
	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)
	require.NotNil(t, enricher)

	// Start enricher and wait for sync
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)

	// Give it time to sync
	time.Sleep(100 * time.Millisecond)

	// Test enrichment with namespace label
	originalLabels := labels.FromStrings(
		"__name__", "cpu_usage",
		"__meta_kubernetes_namespace", "production",
		"pod", "test-pod",
	)

	enrichedLabels := enricher.EnrichWithMetadata(originalLabels)

	// Verify enrichment
	require.Equal(t, "cpu_usage", enrichedLabels.Get("__name__"))
	require.Equal(t, "production", enrichedLabels.Get("__meta_kubernetes_namespace"))
	require.Equal(t, "test-pod", enrichedLabels.Get("pod"))

	// Verify namespace labels are added
	require.Equal(t, "production", enrichedLabels.Get("ns_tier"))
	require.Equal(t, "prod", enrichedLabels.Get("ns_environment"))

	// Verify namespace annotations are added
	require.Equal(t, "team-a", enrichedLabels.Get("ns_application_owner"))
	require.Equal(t, "engineering", enrichedLabels.Get("ns_cost_center"))
	require.Empty(t, enrichedLabels.Get("ns_other_annotation")) // Not in selector

	// Stop enricher
	enricher.Stop()
}

func TestKubernetesEnricherFactory(t *testing.T) {
	factory := &KubernetesEnricherFactory{}

	// Test with valid config
	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:            true,
		AnnotationSelector: []string{"team"},
		LabelPrefix:        "ns_",
	}

	// This will fail because we're not in a Kubernetes cluster
	enricher, err := factory.CreateEnricher(cfg, promslog.NewNopLogger())
	require.Error(t, err)
	require.Nil(t, enricher)

	// Test with disabled config
	cfg.Enabled = false
	enricher, err = factory.CreateEnricher(cfg, promslog.NewNopLogger())
	require.NoError(t, err)
	require.Nil(t, enricher)

	// Test with invalid config type
	enricher, err = factory.CreateEnricher("invalid", promslog.NewNopLogger())
	require.Error(t, err)
	require.Nil(t, enricher)
}

func TestNamespaceEnricherWithAlternativeNamespaceLabel(t *testing.T) {
	client := fake.NewSimpleClientset()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "development",
			Annotations: map[string]string{
				"team": "backend",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:            true,
		AnnotationSelector: []string{"team"},
		LabelPrefix:        "namespace_",
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Test with 'namespace' label instead of '__meta_kubernetes_namespace'
	originalLabels := labels.FromStrings(
		"__name__", "memory_usage",
		"namespace", "development",
		"container", "app",
	)

	enrichedLabels := enricher.EnrichWithMetadata(originalLabels)
	require.Equal(t, "backend", enrichedLabels.Get("namespace_team"))

	enricher.Stop()
}

func TestNamespaceEnricherLabelSanitization(t *testing.T) {
	client := fake.NewSimpleClientset()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			Annotations: map[string]string{
				"app.kubernetes.io/name":  "my-app",
				"deployment/version":      "v1.2.3",
				"special-char_annotation": "value",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled: true,
		AnnotationSelector: []string{
			"app.kubernetes.io/name",
			"deployment/version",
			"special-char_annotation",
		},
		LabelPrefix: "ns_",
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	originalLabels := labels.FromStrings(
		"__name__", "test_metric",
		"namespace", "test-ns",
	)

	enrichedLabels := enricher.EnrichWithMetadata(originalLabels)

	// Test that annotation keys are properly sanitized for label names
	require.Equal(t, "my-app", enrichedLabels.Get("ns_app_kubernetes_io_name"))
	require.Equal(t, "v1.2.3", enrichedLabels.Get("ns_deployment_version"))
	require.Equal(t, "value", enrichedLabels.Get("ns_special_char_annotation"))

	enricher.Stop()
}
