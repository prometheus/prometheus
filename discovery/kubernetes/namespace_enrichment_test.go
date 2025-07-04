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
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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
	cfg := &NamespaceEnrichmentConfig{
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
	cfg := &NamespaceEnrichmentConfig{
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

	cfg := &NamespaceEnrichmentConfig{
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

	cfg := &NamespaceEnrichmentConfig{
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

func TestNamespaceEnricherExcludesNodeMetrics(t *testing.T) {
	client := fake.NewSimpleClientset()

	// Create test namespace with metadata
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
			Labels: map[string]string{
				"environment": "prod",
			},
			Annotations: map[string]string{
				"team": "backend",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	cfg := &NamespaceEnrichmentConfig{
		Enabled:            true,
		LabelSelector:      []string{"environment"},
		AnnotationSelector: []string{"team"},
		LabelPrefix:        "ns_",
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Test node-level metrics that should NOT be enriched
	testCases := []struct {
		name   string
		labels labels.Labels
	}{
		{
			name: "node_memory metric",
			labels: labels.FromStrings(
				"__name__", "node_memory_MemTotal_bytes",
				"instance", "node-1",
				"namespace", "production", // Even with namespace label, should not enrich
			),
		},
		{
			name: "node_cpu metric",
			labels: labels.FromStrings(
				"__name__", "node_cpu_seconds_total",
				"job", "kubernetes-nodes",
				"namespace", "production",
			),
		},
		{
			name: "kube_node metric",
			labels: labels.FromStrings(
				"__name__", "kube_node_status_ready",
				"instance", "node-1",
				"namespace", "production",
			),
		},
		{
			name: "kubernetes-node-exporter job",
			labels: labels.FromStrings(
				"__name__", "some_metric",
				"job", "kubernetes-node-exporter",
				"namespace", "production",
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			enrichedLabels := enricher.EnrichWithMetadata(tc.labels)

			// Verify original labels are preserved
			require.Equal(t, tc.labels.Get("__name__"), enrichedLabels.Get("__name__"))
			require.Equal(t, tc.labels.Get("namespace"), enrichedLabels.Get("namespace"))

			// Verify NO namespace enrichment labels are added
			require.Empty(t, enrichedLabels.Get("ns_environment"))
			require.Empty(t, enrichedLabels.Get("ns_team"))

			// Verify label count hasn't increased (no enrichment happened)
			require.Equal(t, tc.labels.Len(), enrichedLabels.Len())
		})
	}

	// Test that namespace-scoped metrics ARE still enriched
	t.Run("container metrics still enriched", func(t *testing.T) {
		containerLabels := labels.FromStrings(
			"__name__", "container_memory_usage_bytes",
			"namespace", "production",
			"pod", "test-pod",
		)

		enrichedLabels := enricher.EnrichWithMetadata(containerLabels)

		// Verify enrichment DOES happen for container metrics
		require.Equal(t, "prod", enrichedLabels.Get("ns_environment"))
		require.Equal(t, "backend", enrichedLabels.Get("ns_team"))
		require.Greater(t, enrichedLabels.Len(), containerLabels.Len())
	})

	enricher.Stop()
}

func TestNamespaceEnricherKubeStateMetrics(t *testing.T) {
	client := fake.NewSimpleClientset()

	// Create test namespace with metadata
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "production",
			Labels: map[string]string{
				"environment": "prod",
				"team":        "backend",
			},
			Annotations: map[string]string{
				"owner":       "backend-team@company.com",
				"cost-center": "backend-production",
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	require.NoError(t, err)

	cfg := &NamespaceEnrichmentConfig{
		Enabled:            true,
		LabelSelector:      []string{"environment", "team"},
		AnnotationSelector: []string{"owner", "cost-center"},
		LabelPrefix:        "ns_",
	}

	enricher, err := NewNamespaceEnricher(cfg, client, promslog.NewNopLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	enricher.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Test namespace-scoped kube-state-metrics that SHOULD be enriched
	enrichableMetrics := []struct {
		name   string
		labels labels.Labels
	}{
		{
			name: "kube_pod_status_ready",
			labels: labels.FromStrings(
				"__name__", "kube_pod_status_ready",
				"namespace", "production",
				"pod", "web-app-123",
			),
		},
		{
			name: "kube_deployment_status_replicas",
			labels: labels.FromStrings(
				"__name__", "kube_deployment_status_replicas",
				"namespace", "production",
				"deployment", "web-deployment",
			),
		},
		{
			name: "kube_service_info",
			labels: labels.FromStrings(
				"__name__", "kube_service_info",
				"namespace", "production",
				"service", "web-service",
			),
		},
		{
			name: "kube_configmap_info",
			labels: labels.FromStrings(
				"__name__", "kube_configmap_info",
				"namespace", "production",
				"configmap", "app-config",
			),
		},
		{
			name: "kube_secret_info",
			labels: labels.FromStrings(
				"__name__", "kube_secret_info",
				"namespace", "production",
				"secret", "app-secrets",
			),
		},
		{
			name: "kube_limitrange_constraint",
			labels: labels.FromStrings(
				"__name__", "kube_limitrange_constraint",
				"namespace", "production",
				"limitrange", "mem-limit-range",
			),
		},
		{
			name: "kube_resourcequota_status_hard",
			labels: labels.FromStrings(
				"__name__", "kube_resourcequota_status_hard",
				"namespace", "production",
				"resourcequota", "compute-quota",
			),
		},
	}

	for _, tc := range enrichableMetrics {
		t.Run("enrichable_"+tc.name, func(t *testing.T) {
			enrichedLabels := enricher.EnrichWithMetadata(tc.labels)

			// Verify original labels are preserved
			require.Equal(t, tc.labels.Get("__name__"), enrichedLabels.Get("__name__"))
			require.Equal(t, tc.labels.Get("namespace"), enrichedLabels.Get("namespace"))

			// Verify namespace enrichment labels ARE added
			require.Equal(t, "prod", enrichedLabels.Get("ns_environment"))
			require.Equal(t, "backend", enrichedLabels.Get("ns_team"))
			require.Equal(t, "backend-team@company.com", enrichedLabels.Get("ns_owner"))
			require.Equal(t, "backend-production", enrichedLabels.Get("ns_cost_center"))

			// Verify label count increased (enrichment happened)
			require.Greater(t, enrichedLabels.Len(), tc.labels.Len())
		})
	}

	// Test cluster-level kube-state-metrics that should NOT be enriched
	clusterLevelMetrics := []struct {
		name   string
		labels labels.Labels
	}{
		{
			name: "kube_node_status_ready",
			labels: labels.FromStrings(
				"__name__", "kube_node_status_ready",
				"node", "worker-1",
				"namespace", "production", // Even with namespace label, should not enrich
			),
		},
		{
			name: "kube_persistentvolume_info",
			labels: labels.FromStrings(
				"__name__", "kube_persistentvolume_info",
				"persistentvolume", "pv-123",
				"namespace", "production",
			),
		},
		{
			name: "kube_storageclass_info",
			labels: labels.FromStrings(
				"__name__", "kube_storageclass_info",
				"storageclass", "fast-ssd",
				"namespace", "production",
			),
		},
		{
			name: "kube_clusterrole_info",
			labels: labels.FromStrings(
				"__name__", "kube_clusterrole_info",
				"clusterrole", "cluster-admin",
				"namespace", "production",
			),
		},
		{
			name: "kube_namespace_status_phase",
			labels: labels.FromStrings(
				"__name__", "kube_namespace_status_phase",
				"namespace", "production", // Metrics ABOUT namespaces, not scoped TO them
			),
		},
	}

	for _, tc := range clusterLevelMetrics {
		t.Run("cluster_level_"+tc.name, func(t *testing.T) {
			enrichedLabels := enricher.EnrichWithMetadata(tc.labels)

			// Verify original labels are preserved
			require.Equal(t, tc.labels.Get("__name__"), enrichedLabels.Get("__name__"))
			require.Equal(t, tc.labels.Get("namespace"), enrichedLabels.Get("namespace"))

			// Verify NO namespace enrichment labels are added
			require.Empty(t, enrichedLabels.Get("ns_environment"))
			require.Empty(t, enrichedLabels.Get("ns_team"))
			require.Empty(t, enrichedLabels.Get("ns_owner"))
			require.Empty(t, enrichedLabels.Get("ns_cost_center"))

			// Verify label count hasn't increased (no enrichment happened)
			require.Equal(t, tc.labels.Len(), enrichedLabels.Len())
		})
	}

	enricher.Stop()
}

func TestValidateEnrichmentConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *NamespaceEnrichmentConfig
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_basic_config",
			config: &NamespaceEnrichmentConfig{
				Enabled:            true,
				LabelPrefix:        "ns_",
				LabelSelector:      []string{"environment", "team"},
				AnnotationSelector: []string{"owner", "slack-channel"},
				MaxCacheSize:       1000,
				CacheTTL:           300,
				MaxSelectors:       20,
			},
			expectErr: false,
		},
		{
			name: "invalid_cache_size_too_large",
			config: &NamespaceEnrichmentConfig{
				Enabled:       true,
				MaxCacheSize:  200000, // Exceeds limit
				LabelSelector: []string{"environment"},
			},
			expectErr: true,
			errMsg:    "max_cache_size too large",
		},
		{
			name: "invalid_ttl_too_low",
			config: &NamespaceEnrichmentConfig{
				Enabled:       true,
				CacheTTL:      10, // Below 30 second minimum
				LabelSelector: []string{"environment"},
			},
			expectErr: true,
			errMsg:    "below minimum",
		},
		{
			name: "invalid_ttl_too_high",
			config: &NamespaceEnrichmentConfig{
				Enabled:       true,
				CacheTTL:      7200, // Above 1 hour maximum
				LabelSelector: []string{"environment"},
			},
			expectErr: true,
			errMsg:    "exceeds maximum",
		},
		{
			name: "invalid_prefix_no_underscore",
			config: &NamespaceEnrichmentConfig{
				Enabled:       true,
				LabelPrefix:   "ns", // Missing trailing underscore
				LabelSelector: []string{"environment"},
			},
			expectErr: true,
			errMsg:    "must end with underscore",
		},
		{
			name: "security_dangerous_label_selector",
			config: &NamespaceEnrichmentConfig{
				Enabled:       true,
				LabelSelector: []string{"password"}, // Dangerous
			},
			expectErr: true,
			errMsg:    "potentially dangerous",
		},
		{
			name: "no_selectors_when_enabled",
			config: &NamespaceEnrichmentConfig{
				Enabled: true,
				// No selectors specified
			},
			expectErr: true,
			errMsg:    "at least one",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnrichmentConfig(tt.config)
			if tt.expectErr {
				require.Error(t, err, "Expected validation error for %s", tt.name)
				assert.Contains(t, err.Error(), tt.errMsg, "Error message should contain expected text")
			} else {
				require.NoError(t, err, "Expected no validation error for %s", tt.name)
			}
		})
	}
}

func TestIsSecuritySensitive(t *testing.T) {
	tests := []struct {
		selector string
		expected bool
	}{
		{"environment", false},
		{"team", false},
		{"tier", false},
		{"owner", false},
		{"cost-center", false},

		// Security sensitive patterns
		{"password", true},
		{"secret", true},
		{"token", true},
		{"key", true},
		{"budget", true},
		{"salary", true},
		{"private", true},

		// Case insensitive
		{"PASSWORD", true},
		{"Secret", true},
		{"BUDGET", true},

		// Partial matches
		{"db-password", true},
		{"api-secret", true},
		{"user-token", true},
	}

	for _, tt := range tests {
		t.Run(tt.selector, func(t *testing.T) {
			result := isSecuritySensitive(tt.selector)
			assert.Equal(t, tt.expected, result, "Security sensitivity check for %s", tt.selector)
		})
	}
}

func TestObjectPooling(t *testing.T) {
	// Test that object pooling works correctly
	originalLabels := labels.FromStrings("__name__", "test_metric", "namespace", "production", "job", "test")

	// Get builder from pool
	builder1 := getLabelBuilder(originalLabels)
	assert.NotNil(t, builder1)

	// Add a label
	builder1.Set("ns_environment", "production")
	result1 := builder1.Labels()

	// Return to pool
	putLabelBuilder(builder1)

	// Get another builder (should be the same instance from pool)
	builder2 := getLabelBuilder(originalLabels)
	assert.NotNil(t, builder2)

	// Verify it was reset properly (shouldn't have the previous ns_environment label)
	result2 := builder2.Labels()
	assert.False(t, result2.Has("ns_environment"), "Builder should be reset when retrieved from pool")

	// Clean up
	putLabelBuilder(builder2)

	// Verify the first result still has the enriched label
	assert.True(t, result1.Has("ns_environment"), "Original result should retain enriched labels")
	assert.Equal(t, "production", result1.Get("ns_environment"))
}

func TestEnrichmentInterfaceCompliance(t *testing.T) {
	// Create a mock config that implements the interface
	config := &NamespaceEnrichmentConfig{
		Enabled:            true,
		LabelPrefix:        "ns_",
		LabelSelector:      []string{"environment"},
		AnnotationSelector: []string{"owner"},
		MaxCacheSize:       100,
		CacheTTL:           300,
		MaxSelectors:       10,
	}

	// Test that our config implements the interface correctly
	var iface NamespaceEnrichmentConfigInterface = config

	assert.Equal(t, config.Enabled, iface.IsEnabled())
	assert.Equal(t, config.LabelPrefix, iface.GetLabelPrefix())
	assert.Equal(t, config.LabelSelector, iface.GetLabelSelector())
	assert.Equal(t, config.AnnotationSelector, iface.GetAnnotationSelector())
	assert.Equal(t, config.MaxCacheSize, iface.GetMaxCacheSize())
	assert.Equal(t, config.CacheTTL, iface.GetCacheTTL())
	assert.Equal(t, config.MaxSelectors, iface.GetMaxSelectors())
}

func TestEnricherMetrics(t *testing.T) {
	// This test has been removed since performance metrics were removed from the implementation
	// The namespace enrichment functionality is tested in other test functions
	t.Skip("Performance metrics have been removed from namespace enrichment")
}

func TestCacheEviction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := fake.NewSimpleClientset()

	config := &NamespaceEnrichmentConfig{
		Enabled:       true,
		LabelPrefix:   "ns_",
		LabelSelector: []string{"environment"},
		MaxCacheSize:  2, // Small cache size for testing eviction
		CacheTTL:      300,
		MaxSelectors:  10,
	}

	cache, err := NewNamespaceMetadataCache(client, config, logger)
	require.NoError(t, err)

	// Create test namespaces
	for i := 0; i < 5; i++ {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("test-ns-%d", i),
				Labels: map[string]string{
					"environment": fmt.Sprintf("env-%d", i),
				},
			},
		}
		cache.updateNamespace(ns, config, logger)
	}

	// Should have evicted old entries due to size limit
	assert.LessOrEqual(t, len(cache.namespaces), config.MaxCacheSize)
	assert.Greater(t, cache.evictions, int64(0), "Should have recorded evictions")
}

func TestCacheExpiration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := fake.NewSimpleClientset()

	config := &NamespaceEnrichmentConfig{
		Enabled:       true,
		LabelPrefix:   "ns_",
		LabelSelector: []string{"environment"},
		MaxCacheSize:  100,
		CacheTTL:      1, // 1 second TTL for testing
		MaxSelectors:  10,
	}

	cache, err := NewNamespaceMetadataCache(client, config, logger)
	require.NoError(t, err)

	// Add namespace to cache
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
			Labels: map[string]string{
				"environment": "test",
			},
		},
	}
	cache.updateNamespace(ns, config, logger)

	// Should have metadata immediately
	metadata := cache.GetMetadata("test-namespace")
	require.NotNil(t, metadata, "Metadata should not be nil")
	assert.Equal(t, "test", metadata.Labels["environment"])

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Should return nil metadata after expiration
	expiredMetadata := cache.GetMetadata("test-namespace")
	assert.Nil(t, expiredMetadata, "Metadata should be nil after TTL expiration")
}

func TestIsNodeLevelMetric(t *testing.T) {
	tests := []struct {
		metricName string
		labels     labels.Labels
		expected   bool
	}{
		{"node_cpu_seconds_total", labels.EmptyLabels(), true},
		{"node_memory_usage", labels.EmptyLabels(), true},
		{"container_cpu_usage", labels.EmptyLabels(), false},
		{"http_requests_total", labels.EmptyLabels(), false},
		{"kube_node_info", labels.EmptyLabels(), true},
		{"kube_pod_info", labels.EmptyLabels(), false},
		{"cadvisor_version_info", labels.FromStrings("node", "worker-1", "instance", "10.0.1.2"), true},
		{"app_metrics", labels.FromStrings("job", "app-metrics"), false},
	}

	for _, tt := range tests {
		t.Run(tt.metricName, func(t *testing.T) {
			result := isNodeLevelMetric(tt.metricName, tt.labels)
			assert.Equal(t, tt.expected, result, "Node level check for %s", tt.metricName)
		})
	}
}

func TestGracefulDegradation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := fake.NewSimpleClientset()

	config := &NamespaceEnrichmentConfig{
		Enabled:       true,
		LabelPrefix:   "ns_",
		LabelSelector: []string{"environment"},
		MaxCacheSize:  100,
		CacheTTL:      300,
		MaxSelectors:  10,
	}

	enricher, err := NewNamespaceEnricher(config, client, logger)
	require.NoError(t, err)

	// Should start healthy
	assert.True(t, enricher.isHealthyToEnrich())

	// Simulate errors
	for i := 0; i < 15; i++ { // Exceed maxErrors (10)
		enricher.recordError(assert.AnError)
	}

	// Should become unhealthy after too many errors
	assert.False(t, enricher.isHealthyToEnrich())

	// Record success should restore health
	enricher.recordSuccess()
	assert.True(t, enricher.isHealthyToEnrich())
}

func TestSharedInformerManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client1 := fake.NewSimpleClientset()
	client2 := fake.NewSimpleClientset()

	// Get managers for different clients
	manager1 := getSharedInformerManager(client1, logger)
	manager2 := getSharedInformerManager(client2, logger)

	// Should return different instances for different clients
	assert.NotSame(t, manager1, manager2, "Should return different instances for different clients")

	// Get same client again - should return same manager
	manager1Again := getSharedInformerManager(client1, logger)
	assert.Same(t, manager1, manager1Again, "Should return same instance for same client")

	// Test reference counting
	informer1, err := manager1.getInformer()
	require.NoError(t, err)
	assert.Equal(t, 1, manager1.refCount)

	informer2, err := manager1.getInformer()
	require.NoError(t, err)
	assert.Equal(t, 2, manager1.refCount)
	assert.Same(t, informer1, informer2, "Should return same informer instance")

	// Test release
	manager1.releaseInformer()
	assert.Equal(t, 1, manager1.refCount)

	manager1.releaseInformer()
	assert.Equal(t, 0, manager1.refCount)
}

// Helper function to verify metric values - simplified version for compatibility
func verifyMetric(t *testing.T, metric interface{}, labelValue string, expectedValue float64) {
	t.Helper()
	// This function is kept for test compatibility but metrics have been removed
	// Tests that call this will be skipped or simplified
}

// Simple validation tests for enhanced features
func TestValidateEnrichmentConfigBasic(t *testing.T) {
	tests := []struct {
		name      string
		config    *NamespaceEnrichmentConfig
		expectErr bool
	}{
		{
			name: "valid_config",
			config: &NamespaceEnrichmentConfig{
				Enabled:            true,
				LabelPrefix:        "ns_",
				LabelSelector:      []string{"environment"},
				AnnotationSelector: []string{"owner"},
				MaxCacheSize:       1000,
				CacheTTL:           300,
				MaxSelectors:       20,
			},
			expectErr: false,
		},
		{
			name: "invalid_cache_size",
			config: &NamespaceEnrichmentConfig{
				Enabled:       true,
				MaxCacheSize:  200000, // Too large
				LabelSelector: []string{"environment"},
			},
			expectErr: true,
		},
		{
			name: "security_dangerous_selector",
			config: &NamespaceEnrichmentConfig{
				Enabled:       true,
				LabelSelector: []string{"password"}, // Dangerous
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnrichmentConfig(tt.config)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestObjectPoolingBasic(t *testing.T) {
	originalLabels := labels.FromStrings("__name__", "test_metric", "namespace", "test")

	// Get builder from pool
	builder1 := getLabelBuilder(originalLabels)
	assert.NotNil(t, builder1)

	// Add a label and get result
	builder1.Set("ns_environment", "production")
	result1 := builder1.Labels()

	// Return to pool
	putLabelBuilder(builder1)

	// Get another builder - should be reset
	builder2 := getLabelBuilder(originalLabels)
	result2 := builder2.Labels()

	// Should be reset
	assert.False(t, result2.Has("ns_environment"))
	putLabelBuilder(builder2)

	// Original result should still have the label
	assert.True(t, result1.Has("ns_environment"))
}
