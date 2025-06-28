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

package scrape

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNamespaceEnricherBasicFunctionality(t *testing.T) {
	// Create fake Kubernetes client
	client := fake.NewSimpleClientset()

	// Create test namespace with annotations
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
			Annotations: map[string]string{
				"application_owner": "team-a",
				"environment":       "prod",
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
		AnnotationSelector: []string{
			"application_owner",
			"environment",
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

	enrichedLabels := enricher.EnrichWithNamespaceAnnotations(originalLabels)

	// Verify enrichment
	assert.Equal(t, "cpu_usage", enrichedLabels.Get("__name__"))
	assert.Equal(t, "production", enrichedLabels.Get("__meta_kubernetes_namespace"))
	assert.Equal(t, "test-pod", enrichedLabels.Get("pod"))
	assert.Equal(t, "team-a", enrichedLabels.Get("ns_application_owner"))
	assert.Equal(t, "prod", enrichedLabels.Get("ns_environment"))
	assert.Equal(t, "engineering", enrichedLabels.Get("ns_cost_center"))
	assert.Equal(t, "", enrichedLabels.Get("ns_other_annotation")) // Not in selector

	// Stop enricher
	enricher.Stop()
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

	enrichedLabels := enricher.EnrichWithNamespaceAnnotations(originalLabels)
	assert.Equal(t, "backend", enrichedLabels.Get("namespace_team"))

	enricher.Stop()
}

func TestNamespaceEnricherNoNamespaceLabel(t *testing.T) {
	client := fake.NewSimpleClientset()

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:            true,
		AnnotationSelector: []string{"team"},
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)

	// Test with no namespace label
	originalLabels := labels.FromStrings(
		"__name__", "cpu_usage",
		"instance", "localhost:9090",
	)

	enrichedLabels := enricher.EnrichWithNamespaceAnnotations(originalLabels)
	// Should return unchanged labels
	assert.Equal(t, originalLabels, enrichedLabels)

	enricher.Stop()
}

func TestNamespaceEnricherNonExistentNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:            true,
		AnnotationSelector: []string{"team"},
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Test with non-existent namespace
	originalLabels := labels.FromStrings(
		"__name__", "cpu_usage",
		"__meta_kubernetes_namespace", "non-existent",
	)

	enrichedLabels := enricher.EnrichWithNamespaceAnnotations(originalLabels)
	// Should return unchanged labels since namespace doesn't exist
	assert.Equal(t, originalLabels, enrichedLabels)

	enricher.Stop()
}

func TestNamespaceEnricherUpdate(t *testing.T) {
	client := fake.NewSimpleClientset()

	// Create initial namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ns",
			Annotations: map[string]string{
				"team": "initial-team",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:            true,
		AnnotationSelector: []string{"team"},
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Test initial enrichment
	originalLabels := labels.FromStrings(
		"__name__", "test_metric",
		"namespace", "test-ns",
	)

	enrichedLabels := enricher.EnrichWithNamespaceAnnotations(originalLabels)
	assert.Equal(t, "initial-team", enrichedLabels.Get("ns_team"))

	// Update namespace annotations
	ns.Annotations["team"] = "updated-team"
	_, err = client.CoreV1().Namespaces().Update(context.TODO(), ns, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Give time for update to propagate
	time.Sleep(100 * time.Millisecond)

	// Test updated enrichment
	enrichedLabels = enricher.EnrichWithNamespaceAnnotations(originalLabels)
	assert.Equal(t, "updated-team", enrichedLabels.Get("ns_team"))

	enricher.Stop()
}

func TestNamespaceEnricherDelete(t *testing.T) {
	client := fake.NewSimpleClientset()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "temp-ns",
			Annotations: map[string]string{
				"team": "temp-team",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:            true,
		AnnotationSelector: []string{"team"},
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	originalLabels := labels.FromStrings(
		"__name__", "test_metric",
		"namespace", "temp-ns",
	)

	// Test enrichment before deletion
	enrichedLabels := enricher.EnrichWithNamespaceAnnotations(originalLabels)
	assert.Equal(t, "temp-team", enrichedLabels.Get("ns_team"))

	// Delete namespace
	err = client.CoreV1().Namespaces().Delete(context.TODO(), "temp-ns", metav1.DeleteOptions{})
	require.NoError(t, err)

	// Give time for deletion to propagate
	time.Sleep(100 * time.Millisecond)

	// Test enrichment after deletion
	enrichedLabels = enricher.EnrichWithNamespaceAnnotations(originalLabels)
	assert.Equal(t, originalLabels, enrichedLabels) // Should be unchanged

	enricher.Stop()
}

func TestNamespaceEnricherDisabled(t *testing.T) {
	client := fake.NewSimpleClientset()

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:            false,
		AnnotationSelector: []string{"team"},
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	assert.NoError(t, err)
	assert.Nil(t, enricher) // Should return nil when disabled

	// Test with nil enricher (should work gracefully)
	originalLabels := labels.FromStrings(
		"__name__", "test_metric",
		"namespace", "any-namespace",
	)

	var nilEnricher *NamespaceEnricher
	enrichedLabels := nilEnricher.EnrichWithNamespaceAnnotations(originalLabels)
	assert.Equal(t, originalLabels, enrichedLabels)
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

	enrichedLabels := enricher.EnrichWithNamespaceAnnotations(originalLabels)

	// Test that annotation keys are properly sanitized for label names
	assert.Equal(t, "my-app", enrichedLabels.Get("ns_app_kubernetes_io_name"))
	assert.Equal(t, "v1.2.3", enrichedLabels.Get("ns_deployment_version"))
	assert.Equal(t, "value", enrichedLabels.Get("ns_special_char_annotation"))

	enricher.Stop()
}

func TestMutateSampleLabelsWithEnricher(t *testing.T) {
	client := fake.NewSimpleClientset()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
			Annotations: map[string]string{
				"team": "backend",
				"env":  "prod",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:            true,
		AnnotationSelector: []string{"team", "env"},
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Create mock target using NewTarget constructor
	targetLabels := labels.FromStrings("job", "test-job", "instance", "localhost:9090")
	target := NewTarget(targetLabels, nil, nil, nil)

	originalLabels := labels.FromStrings(
		"__name__", "http_requests_total",
		"namespace", "production",
		"method", "GET",
	)

	// Test mutateSampleLabelsWithEnricher function
	mutatedLabels := mutateSampleLabelsWithEnricher(originalLabels, target, false, nil, enricher)

	// Verify original labels are preserved
	assert.Equal(t, "http_requests_total", mutatedLabels.Get("__name__"))
	assert.Equal(t, "production", mutatedLabels.Get("namespace"))
	assert.Equal(t, "GET", mutatedLabels.Get("method"))

	// Verify target labels are added
	assert.Equal(t, "test-job", mutatedLabels.Get("job"))
	assert.Equal(t, "localhost:9090", mutatedLabels.Get("instance"))

	// Verify namespace enrichment
	assert.Equal(t, "backend", mutatedLabels.Get("ns_team"))
	assert.Equal(t, "prod", mutatedLabels.Get("ns_env"))

	enricher.Stop()
}

func TestNamespaceEnricherWithLabels(t *testing.T) {
	// Create fake Kubernetes client
	client := fake.NewSimpleClientset()

	// Create test namespace with both labels and annotations
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
			Labels: map[string]string{
				"tier":        "production",
				"team":        "backend",
				"environment": "prod",
			},
			Annotations: map[string]string{
				"application_owner": "team-a",
				"cost_center":       "engineering",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create enricher config that selects both labels and annotations
	cfg := &config.NamespaceEnrichmentConfig{
		Enabled: true,
		LabelSelector: []string{
			"tier",
			"team",
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

	enrichedLabels := enricher.EnrichWithNamespaceMetadata(originalLabels)

	// Verify original labels are preserved
	assert.Equal(t, "cpu_usage", enrichedLabels.Get("__name__"))
	assert.Equal(t, "production", enrichedLabels.Get("__meta_kubernetes_namespace"))
	assert.Equal(t, "test-pod", enrichedLabels.Get("pod"))

	// Verify namespace labels are added
	assert.Equal(t, "production", enrichedLabels.Get("ns_tier"))
	assert.Equal(t, "backend", enrichedLabels.Get("ns_team"))
	assert.Equal(t, "prod", enrichedLabels.Get("ns_environment"))

	// Verify namespace annotations are added
	assert.Equal(t, "team-a", enrichedLabels.Get("ns_application_owner"))
	assert.Equal(t, "engineering", enrichedLabels.Get("ns_cost_center"))

	// Stop enricher
	enricher.Stop()
}

func TestNamespaceEnricherLabelsOnly(t *testing.T) {
	client := fake.NewSimpleClientset()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "development",
			Labels: map[string]string{
				"tier": "development",
				"team": "frontend",
			},
			// No annotations
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	cfg := &config.NamespaceEnrichmentConfig{
		Enabled:       true,
		LabelSelector: []string{"tier", "team"},
		// No annotation selector
		LabelPrefix: "namespace_",
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	originalLabels := labels.FromStrings(
		"__name__", "memory_usage",
		"namespace", "development",
		"container", "app",
	)

	enrichedLabels := enricher.EnrichWithNamespaceMetadata(originalLabels)

	// Verify namespace labels are added
	assert.Equal(t, "development", enrichedLabels.Get("namespace_tier"))
	assert.Equal(t, "frontend", enrichedLabels.Get("namespace_team"))

	enricher.Stop()
}
