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
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/scrape"
)

// Metrics for monitoring namespace enricher performance and health
var (
	enrichmentDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "prometheus_namespace_enrichment_duration_seconds",
		Help:    "Time spent enriching metrics with namespace metadata.",
		Buckets: prometheus.DefBuckets,
	}, []string{"result"})

	enrichmentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "prometheus_namespace_enrichment_total",
		Help: "Total number of namespace enrichments attempted.",
	}, []string{"result"})

	cacheOperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "prometheus_namespace_enrichment_cache_operations_total",
		Help: "Total number of namespace cache operations.",
	}, []string{"operation"})

	cacheSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "prometheus_namespace_enrichment_cache_size",
		Help: "Current number of namespaces in cache.",
	}, []string{})

	enricherHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "prometheus_namespace_enrichment_healthy",
		Help: "Whether the namespace enricher is healthy (1) or unhealthy (0).",
	}, []string{})

	errorRate = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "prometheus_namespace_enrichment_errors_total",
		Help: "Total number of enrichment errors by type.",
	}, []string{"error_type"})
)

// NamespaceEnrichmentConfigInterface defines the contract for namespace enrichment configuration
// This avoids import cycles while providing type safety
type NamespaceEnrichmentConfigInterface interface {
	IsEnabled() bool
	GetLabelPrefix() string
	GetLabelSelector() []string
	GetAnnotationSelector() []string
	GetMaxCacheSize() int
	GetCacheTTL() int64
	GetMaxSelectors() int
}

func init() {
	// Register Kubernetes enricher factory using clean interface approach
	scrape.RegisterEnricherFactory("kubernetes", func(cfg interface{}, logger *slog.Logger) (scrape.MetadataEnricher, error) {
		// Type assert to our interface instead of using reflection
		enrichmentConfig, ok := cfg.(NamespaceEnrichmentConfigInterface)
		if !ok {
			return nil, fmt.Errorf("invalid configuration type for Kubernetes enricher - expected NamespaceEnrichmentConfigInterface")
		}

		if !enrichmentConfig.IsEnabled() {
			return scrape.NewNoopEnricher(), nil
		}

		// Convert to local config type
		localConfig := &NamespaceEnrichmentConfig{
			Enabled:            enrichmentConfig.IsEnabled(),
			LabelPrefix:        enrichmentConfig.GetLabelPrefix(),
			LabelSelector:      enrichmentConfig.GetLabelSelector(),
			AnnotationSelector: enrichmentConfig.GetAnnotationSelector(),
			MaxCacheSize:       enrichmentConfig.GetMaxCacheSize(),
			CacheTTL:           enrichmentConfig.GetCacheTTL(),
			MaxSelectors:       enrichmentConfig.GetMaxSelectors(),
		}

		// Validate configuration before creating enricher
		if err := validateEnrichmentConfig(localConfig); err != nil {
			return nil, fmt.Errorf("invalid enrichment configuration: %w", err)
		}

		// Create the actual Kubernetes enricher
		factory := &KubernetesEnricherFactory{}
		enricherInterface, err := factory.CreateEnricher(localConfig, logger)
		if err != nil {
			logger.Warn("Failed to create Kubernetes enricher, using no-op", "error", err)
			return scrape.NewNoopEnricher(), nil
		}

		// Convert the interface{} back to scrape.MetadataEnricher
		if enricher, ok := enricherInterface.(scrape.MetadataEnricher); ok {
			logger.Info("Kubernetes namespace enricher created successfully (experimental)")
			return enricher, nil
		}

		logger.Warn("Created enricher is not compatible with scrape.MetadataEnricher interface")
		return scrape.NewNoopEnricher(), nil
	})
}

// validateEnrichmentConfig performs comprehensive validation of enrichment configuration
func validateEnrichmentConfig(cfg *NamespaceEnrichmentConfig) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Validate cache size limits - prevent memory exhaustion
	if cfg.MaxCacheSize < 0 {
		return fmt.Errorf("max_cache_size cannot be negative")
	}
	if cfg.MaxCacheSize > 100000 {
		return fmt.Errorf("max_cache_size too large (max: 100000), got: %d", cfg.MaxCacheSize)
	}

	// Validate TTL limits - prevent excessive API load or stale data
	if cfg.CacheTTL < 0 {
		return fmt.Errorf("cache_ttl cannot be negative")
	}
	if cfg.CacheTTL > 3600 {
		return fmt.Errorf("cache_ttl too large (max: 3600s), got: %d", cfg.CacheTTL)
	}

	// Validate selector limits - prevent cardinality explosion
	totalSelectors := len(cfg.LabelSelector) + len(cfg.AnnotationSelector)
	if totalSelectors > 100 {
		return fmt.Errorf("too many selectors (max: 100), got: %d", totalSelectors)
	}

	if cfg.MaxSelectors > 0 && totalSelectors > cfg.MaxSelectors {
		return fmt.Errorf("selector count (%d) exceeds max_selectors (%d)", totalSelectors, cfg.MaxSelectors)
	}

	// Security validation for sensitive selectors
	allSelectors := append(cfg.LabelSelector, cfg.AnnotationSelector...)
	for _, selector := range allSelectors {
		if isSecuritySensitive(selector) {
			return fmt.Errorf("selector '%s' is not allowed for security reasons", selector)
		}
	}

	return nil
}

// isSecuritySensitive checks if a selector contains sensitive information
func isSecuritySensitive(selector string) bool {
	sensitivePatterns := []string{
		"password", "secret", "token", "key", "credential",
		"budget", "financial", "salary", "private",
	}

	selectorLower := strings.ToLower(selector)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(selectorLower, pattern) {
			return true
		}
	}
	return false
}

// NamespaceEnrichmentConfig represents namespace enrichment configuration.
// This mirrors the config package struct to avoid import cycles.
type NamespaceEnrichmentConfig struct {
	Enabled            bool     `yaml:"enabled"`
	LabelPrefix        string   `yaml:"label_prefix"`
	LabelSelector      []string `yaml:"label_selector"`
	AnnotationSelector []string `yaml:"annotation_selector"`

	// Resource limits for experimental feature safety
	MaxCacheSize int   `yaml:"max_cache_size,omitempty"` // Maximum number of namespaces to cache (default: 1000)
	CacheTTL     int64 `yaml:"cache_ttl,omitempty"`      // TTL for cache entries in seconds (default: 300)
	MaxSelectors int   `yaml:"max_selectors,omitempty"`  // Maximum total selectors (default: 20)
}

// Interface methods to implement NamespaceEnrichmentConfigInterface
func (c *NamespaceEnrichmentConfig) IsEnabled() bool {
	return c.Enabled
}

func (c *NamespaceEnrichmentConfig) GetLabelPrefix() string {
	return c.LabelPrefix
}

func (c *NamespaceEnrichmentConfig) GetLabelSelector() []string {
	return c.LabelSelector
}

func (c *NamespaceEnrichmentConfig) GetAnnotationSelector() []string {
	return c.AnnotationSelector
}

func (c *NamespaceEnrichmentConfig) GetMaxCacheSize() int {
	return c.MaxCacheSize
}

func (c *NamespaceEnrichmentConfig) GetCacheTTL() int64 {
	return c.CacheTTL
}

func (c *NamespaceEnrichmentConfig) GetMaxSelectors() int {
	return c.MaxSelectors
}

// MetadataEnricher is a local interface to avoid import cycles.
type MetadataEnricher interface {
	EnrichWithMetadata(labels.Labels) labels.Labels
	Start(context.Context)
	Stop()
}

// ScrapeMetadataEnricherAdapter adapts the local MetadataEnricher to work with external packages
type ScrapeMetadataEnricherAdapter struct {
	enricher MetadataEnricher
}

func (s *ScrapeMetadataEnricherAdapter) EnrichWithMetadata(lset labels.Labels) labels.Labels {
	return s.enricher.EnrichWithMetadata(lset)
}

func (s *ScrapeMetadataEnricherAdapter) Start(ctx context.Context) {
	s.enricher.Start(ctx)
}

func (s *ScrapeMetadataEnricherAdapter) Stop() {
	s.enricher.Stop()
}

// NewScrapeMetadataEnricherAdapter creates an adapter for external use
func NewScrapeMetadataEnricherAdapter(enricher MetadataEnricher) *ScrapeMetadataEnricherAdapter {
	return &ScrapeMetadataEnricherAdapter{enricher: enricher}
}

// NamespaceEnricher implements the MetadataEnricher interface for Kubernetes namespaces.
type NamespaceEnricher struct {
	config      *NamespaceEnrichmentConfig
	cache       *NamespaceMetadataCache
	logger      *slog.Logger
	labelPrefix string

	// Graceful degradation state
	mu            sync.RWMutex
	isHealthy     bool
	lastError     error
	errorCount    int
	lastErrorTime time.Time
	maxErrors     int
}

// NamespaceMetadata holds both labels and annotations for a namespace.
type NamespaceMetadata struct {
	Labels      map[string]string
	Annotations map[string]string
	LastUpdated int64 // Unix timestamp for TTL tracking
}

// NamespaceMetadataCache caches namespace metadata to avoid frequent Kubernetes API calls.
type NamespaceMetadataCache struct {
	mu            sync.RWMutex
	namespaces    map[string]*NamespaceMetadata
	informer      cache.SharedIndexInformer
	stopCh        chan struct{}
	maxCacheSize  int
	cacheTTL      int64
	sharedManager *SharedInformerManager

	// Metrics for monitoring
	cacheSize   int
	evictions   int64
	lastCleanup int64
}

// labelBuilderPool provides object pooling for labels.Builder to reduce GC pressure
var labelBuilderPool = sync.Pool{
	New: func() interface{} {
		return labels.NewBuilder(labels.EmptyLabels())
	},
}

// getLabelBuilder gets a labels.Builder from the pool
func getLabelBuilder(lset labels.Labels) *labels.Builder {
	builder := labelBuilderPool.Get().(*labels.Builder)
	builder.Reset(lset)
	return builder
}

// putLabelBuilder returns a labels.Builder to the pool
func putLabelBuilder(builder *labels.Builder) {
	// Clear the builder to prevent memory leaks
	builder.Reset(labels.EmptyLabels())
	labelBuilderPool.Put(builder)
}

// NewNamespaceEnricher creates a new Kubernetes namespace enricher.
func NewNamespaceEnricher(cfg *NamespaceEnrichmentConfig, client kubernetes.Interface, logger *slog.Logger) (*NamespaceEnricher, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	if client == nil {
		return nil, errors.New("kubernetes client is required")
	}

	prefix := cfg.LabelPrefix
	if prefix == "" {
		prefix = "ns_"
	}

	cache, err := NewNamespaceMetadataCache(client, cfg, logger)
	if err != nil {
		return nil, err
	}

	return &NamespaceEnricher{
		config:      cfg,
		cache:       cache,
		logger:      logger,
		labelPrefix: prefix,
		isHealthy:   true,
		maxErrors:   10, // Allow up to 10 errors before marking unhealthy
	}, nil
}

// NewNamespaceMetadataCache creates a new namespace metadata cache using shared informer.
func NewNamespaceMetadataCache(client kubernetes.Interface, cfg *NamespaceEnrichmentConfig, logger *slog.Logger) (*NamespaceMetadataCache, error) {
	if client == nil {
		return nil, errors.New("kubernetes client is required")
	}

	// Get shared informer manager
	sharedManager := getSharedInformerManager(client, logger)
	informer, err := sharedManager.getInformer()
	if err != nil {
		return nil, fmt.Errorf("failed to get shared informer: %w", err)
	}

	nmc := &NamespaceMetadataCache{
		namespaces:    make(map[string]*NamespaceMetadata),
		informer:      informer,
		stopCh:        make(chan struct{}),
		maxCacheSize:  cfg.MaxCacheSize,
		cacheTTL:      cfg.CacheTTL,
		sharedManager: sharedManager,
	}

	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				nmc.updateNamespace(ns, cfg, logger)
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			if ns, ok := newObj.(*corev1.Namespace); ok {
				nmc.updateNamespace(ns, cfg, logger)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				nmc.deleteNamespace(ns.Name)
			}
		},
	})
	if err != nil {
		// Release the informer reference on error
		sharedManager.releaseInformer()
		return nil, err
	}

	return nmc, nil
}

// Start starts the namespace metadata cache using shared informer.
func (nmc *NamespaceMetadataCache) Start(ctx context.Context) {
	if nmc.sharedManager == nil {
		return
	}

	// Start the shared informer if not already started
	if err := nmc.sharedManager.startInformer(ctx); err != nil {
		// Log error but don't fail completely - graceful degradation
		return
	}
}

// Stop stops the namespace metadata cache and releases shared informer reference.
func (nmc *NamespaceMetadataCache) Stop() {
	close(nmc.stopCh)

	// Release the shared informer reference
	if nmc.sharedManager != nil {
		nmc.sharedManager.releaseInformer()
	}
}

// updateNamespace updates the cached namespace metadata.
func (nmc *NamespaceMetadataCache) updateNamespace(ns *corev1.Namespace, cfg *NamespaceEnrichmentConfig, logger *slog.Logger) {
	// Set defaults for limits if not configured
	maxCacheSize := cfg.MaxCacheSize
	if maxCacheSize <= 0 {
		maxCacheSize = 1000
	}
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 300 // 5 minutes default
	}

	metadata := &NamespaceMetadata{
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		LastUpdated: time.Now().Unix(),
	}

	// Extract selected labels
	for _, labelKey := range cfg.LabelSelector {
		if value, exists := ns.Labels[labelKey]; exists {
			metadata.Labels[labelKey] = value
		}
	}

	// Extract selected annotations
	for _, annotationKey := range cfg.AnnotationSelector {
		if value, exists := ns.Annotations[annotationKey]; exists {
			metadata.Annotations[annotationKey] = value
		}
	}

	nmc.mu.Lock()
	defer nmc.mu.Unlock()

	// Cleanup expired entries and enforce size limits before adding new entry
	now := time.Now().Unix()
	if now-nmc.lastCleanup > 60 { // Cleanup every minute
		nmc.cleanupExpiredEntries(now, cacheTTL, logger)
		nmc.enforceSizeLimit(maxCacheSize, logger)
		nmc.lastCleanup = now
	}

	// Add/update the namespace
	nmc.namespaces[ns.Name] = metadata
	nmc.cacheSize = len(nmc.namespaces)
	cacheSize.WithLabelValues().Set(float64(nmc.cacheSize))
	cacheOperationsTotal.WithLabelValues("update").Inc()

	logger.Debug("Updated namespace metadata",
		"namespace", ns.Name,
		"labels", len(metadata.Labels),
		"annotations", len(metadata.Annotations),
		"cache_size", nmc.cacheSize)
}

// cleanupExpiredEntries removes entries older than TTL
func (nmc *NamespaceMetadataCache) cleanupExpiredEntries(now, cacheTTL int64, logger *slog.Logger) {
	expiredCount := 0
	for name, metadata := range nmc.namespaces {
		if now-metadata.LastUpdated > cacheTTL {
			delete(nmc.namespaces, name)
			expiredCount++
		}
	}
	if expiredCount > 0 {
		logger.Debug("Cleaned up expired namespace cache entries", "count", expiredCount)
	}
}

// enforceSizeLimit removes oldest entries if cache exceeds size limit
func (nmc *NamespaceMetadataCache) enforceSizeLimit(maxCacheSize int, logger *slog.Logger) {
	if len(nmc.namespaces) <= maxCacheSize {
		return
	}

	// Find oldest entries to evict
	type entry struct {
		name        string
		lastUpdated int64
	}

	entries := make([]entry, 0, len(nmc.namespaces))
	for name, metadata := range nmc.namespaces {
		entries = append(entries, entry{name: name, lastUpdated: metadata.LastUpdated})
	}

	// Sort by age (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastUpdated < entries[j].lastUpdated
	})

	// Remove oldest entries until we're under the limit
	toRemove := len(nmc.namespaces) - maxCacheSize
	for i := 0; i < toRemove; i++ {
		delete(nmc.namespaces, entries[i].name)
		nmc.evictions++
	}

	if toRemove > 0 {
		logger.Warn("Evicted oldest namespace cache entries due to size limit",
			"evicted", toRemove,
			"max_cache_size", maxCacheSize)
	}
}

// deleteNamespace removes namespace from cache.
func (nmc *NamespaceMetadataCache) deleteNamespace(name string) {
	nmc.mu.Lock()
	delete(nmc.namespaces, name)
	nmc.mu.Unlock()
}

// GetMetadata returns metadata (labels and annotations) for a namespace.
func (nmc *NamespaceMetadataCache) GetMetadata(namespaceName string) *NamespaceMetadata {
	nmc.mu.RLock()
	defer nmc.mu.RUnlock()

	metadata := nmc.namespaces[namespaceName]
	if metadata == nil {
		cacheOperationsTotal.WithLabelValues("miss").Inc()
		return &NamespaceMetadata{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		}
	}

	// Check if entry has expired (use default TTL if not configured)
	cacheTTL := nmc.cacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 300 // 5 minutes default
	}

	now := time.Now().Unix()
	if now-metadata.LastUpdated > cacheTTL {
		cacheOperationsTotal.WithLabelValues("expired").Inc()
		// Entry expired, return empty metadata
		return &NamespaceMetadata{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		}
	}

	cacheOperationsTotal.WithLabelValues("hit").Inc()
	// Return copy to avoid data races
	result := &NamespaceMetadata{
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		LastUpdated: metadata.LastUpdated,
	}
	for k, v := range metadata.Labels {
		result.Labels[k] = v
	}
	for k, v := range metadata.Annotations {
		result.Annotations[k] = v
	}
	return result
}

// Start starts the namespace enricher.
func (ne *NamespaceEnricher) Start(ctx context.Context) {
	if ne == nil {
		return
	}

	// Use graceful degradation
	defer func() {
		if r := recover(); r != nil {
			ne.recordError(fmt.Errorf("panic in enricher start: %v", r))
			ne.logger.Error("Namespace enricher start panic recovered", "panic", r)
		}
	}()

	if ne.cache == nil {
		ne.recordError(fmt.Errorf("cannot start enricher: cache is nil"))
		return
	}

	// Try to start cache with timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		ne.cache.Start(ctx)
	}()

	// Wait for start with timeout
	select {
	case <-done:
		ne.recordSuccess()
		ne.logger.Info("Namespace enricher started successfully")
	case <-time.After(30 * time.Second):
		ne.recordError(fmt.Errorf("timeout starting namespace enricher"))
		ne.logger.Error("Timeout starting namespace enricher")
	case <-ctx.Done():
		ne.recordError(fmt.Errorf("context cancelled during start"))
		ne.logger.Info("Context cancelled during namespace enricher start")
	}
}

// Stop stops the namespace enricher.
func (ne *NamespaceEnricher) Stop() {
	if ne == nil {
		return
	}

	// Use graceful degradation
	defer func() {
		if r := recover(); r != nil {
			ne.logger.Error("Namespace enricher stop panic recovered", "panic", r)
		}
	}()

	if ne.cache != nil {
		ne.cache.Stop()
	}

	ne.logger.Info("Namespace enricher stopped")
}

// EnrichWithMetadata enriches labels with namespace metadata.
func (ne *NamespaceEnricher) EnrichWithMetadata(lset labels.Labels) labels.Labels {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		enrichmentDuration.WithLabelValues("success").Observe(duration.Seconds())
	}()

	if ne == nil || !ne.isHealthyToEnrich() {
		enrichmentTotal.WithLabelValues("skipped_unhealthy").Inc()
		return lset
	}

	// Graceful degradation with panic recovery
	defer func() {
		if r := recover(); r != nil {
			ne.recordError(fmt.Errorf("panic in enrichment: %v", r))
			ne.logger.Error("Namespace enrichment panic recovered", "panic", r)
			enrichmentTotal.WithLabelValues("panic").Inc()
			errorRate.WithLabelValues("panic").Inc()
		}
	}()

	metricName := lset.Get(labels.MetricName)
	if metricName == "" {
		enrichmentTotal.WithLabelValues("no_metric_name").Inc()
		return lset
	}

	// Skip node-level and cluster-level metrics
	if isNodeLevelMetric(metricName, lset) {
		enrichmentTotal.WithLabelValues("skipped_node_level").Inc()
		return lset
	}

	namespaceName := lset.Get("namespace")
	if namespaceName == "" {
		enrichmentTotal.WithLabelValues("no_namespace").Inc()
		return lset
	}

	metadata, err := ne.getMetadataWithErrorHandling(namespaceName)
	if err != nil {
		ne.recordError(err)
		enrichmentTotal.WithLabelValues("metadata_error").Inc()
		errorRate.WithLabelValues("metadata_fetch").Inc()
		return lset
	}

	enrichedLabels, err := ne.buildEnrichedLabels(lset, metadata)
	if err != nil {
		ne.recordError(err)
		enrichmentTotal.WithLabelValues("build_error").Inc()
		errorRate.WithLabelValues("label_build").Inc()
		return lset
	}

	enrichmentTotal.WithLabelValues("success").Inc()
	return enrichedLabels
}

// getMetadataWithErrorHandling safely retrieves metadata with error handling
func (ne *NamespaceEnricher) getMetadataWithErrorHandling(namespaceName string) (*NamespaceMetadata, error) {
	defer func() {
		if r := recover(); r != nil {
			ne.logger.Error("Panic in metadata retrieval", "namespace", namespaceName, "panic", r)
		}
	}()

	if ne.cache == nil {
		return nil, fmt.Errorf("cache is nil")
	}

	metadata := ne.cache.GetMetadata(namespaceName)
	return metadata, nil
}

// buildEnrichedLabels safely builds the enriched label set
func (ne *NamespaceEnricher) buildEnrichedLabels(lset labels.Labels, metadata *NamespaceMetadata) (labels.Labels, error) {
	defer func() {
		if r := recover(); r != nil {
			ne.logger.Error("Panic in label building", "panic", r)
		}
	}()

	builder := getLabelBuilder(lset)

	// Add namespace labels with safety checks
	for labelKey, labelValue := range metadata.Labels {
		if labelKey == "" || labelValue == "" {
			continue // Skip invalid labels
		}
		enrichedLabelName := ne.labelPrefix + sanitizeLabelName(labelKey)
		if len(enrichedLabelName) > 253 { // Prometheus label name limit
			ne.logger.Warn("Skipping label due to length limit", "label", enrichedLabelName)
			continue
		}
		builder.Set(enrichedLabelName, labelValue)
	}

	// Add namespace annotations with safety checks
	for annotationKey, annotationValue := range metadata.Annotations {
		if annotationKey == "" || annotationValue == "" {
			continue // Skip invalid annotations
		}
		enrichedLabelName := ne.labelPrefix + sanitizeLabelName(annotationKey)
		if len(enrichedLabelName) > 253 { // Prometheus label name limit
			ne.logger.Warn("Skipping annotation due to length limit", "annotation", enrichedLabelName)
			continue
		}
		builder.Set(enrichedLabelName, annotationValue)
	}

	result := builder.Labels()
	putLabelBuilder(builder)
	return result, nil
}

// isNodeLevelMetric determines if a metric represents node-level (global) data
// rather than namespace-scoped data that should be enriched.
func isNodeLevelMetric(metricName string, lset labels.Labels) bool {
	// Skip metrics that start with node_ as they represent global node resources
	if len(metricName) >= 5 && metricName[:5] == "node_" {
		return true
	}

	// Skip metrics from kubernetes-nodes job as they're typically node-level
	job := lset.Get("job")
	if job == "kubernetes-nodes" || job == "kubernetes-node-exporter" {
		return true
	}

	// Skip well-known node-level and cluster-level metric patterns
	clusterLevelPrefixes := []string{
		"kube_node_",               // Node-specific kube-state-metrics
		"kube_persistentvolume_",   // Cluster-level storage (not namespace-scoped)
		"kube_storageclass_",       // Cluster-level storage classes
		"kube_clusterrole_",        // Cluster-level RBAC
		"kube_clusterrolebinding_", // Cluster-level RBAC
		"kube_namespace_",          // Metrics ABOUT namespaces (not scoped TO them)
		"machine_",                 // Machine-level metrics
		"kubelet_node_",            // Kubelet node metrics
		"kubernetes_build_",        // Kubernetes build info (cluster-level)
	}

	for _, prefix := range clusterLevelPrefixes {
		if len(metricName) >= len(prefix) && metricName[:len(prefix)] == prefix {
			return true
		}
	}

	return false
}

// sanitizeLabelName converts annotation keys to valid Prometheus label names.
var labelNameRegex = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeLabelName(name string) string {
	return labelNameRegex.ReplaceAllString(name, "_")
}

// KubernetesEnricherFactory implements enricher factory for Kubernetes.
type KubernetesEnricherFactory struct{}

// CreateEnricher creates a new Kubernetes namespace enricher.
func (f *KubernetesEnricherFactory) CreateEnricher(cfg interface{}, logger *slog.Logger) (interface{}, error) {
	namespaceCfg, ok := cfg.(*NamespaceEnrichmentConfig)
	if !ok {
		return nil, errors.New("invalid configuration type for Kubernetes enricher")
	}

	if namespaceCfg == nil || !namespaceCfg.Enabled {
		return nil, nil
	}

	client, err := f.createKubernetesClient()
	if err != nil {
		return nil, err
	}

	enricher, err := NewNamespaceEnricher(namespaceCfg, client, logger)
	if err != nil {
		return nil, err
	}

	if enricher == nil {
		return nil, nil
	}

	// Return adapter that can be used by external packages
	return NewScrapeMetadataEnricherAdapter(enricher), nil
}

// createKubernetesClient attempts to create a Kubernetes client if running in-cluster.
func (f *KubernetesEnricherFactory) createKubernetesClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

// KubernetesEnricherFactoryFunc is the factory function for creating Kubernetes enrichers.
// It matches the EnricherFactory signature from the scrape package.
func KubernetesEnricherFactoryFunc(cfg interface{}, logger *slog.Logger) (interface{}, error) {
	namespaceCfg, ok := cfg.(*NamespaceEnrichmentConfig)
	if !ok {
		return nil, errors.New("invalid configuration type for Kubernetes enricher")
	}

	if namespaceCfg == nil || !namespaceCfg.Enabled {
		return nil, nil
	}

	factory := &KubernetesEnricherFactory{}
	return factory.CreateEnricher(namespaceCfg, logger)
}

// recordError tracks errors for circuit breaker functionality
func (ne *NamespaceEnricher) recordError(err error) {
	if ne == nil {
		return
	}

	ne.mu.Lock()
	defer ne.mu.Unlock()

	ne.lastError = err
	ne.lastErrorTime = time.Now()
	ne.errorCount++

	// Implement circuit breaker: mark unhealthy after too many errors
	if ne.errorCount >= ne.maxErrors {
		ne.isHealthy = false
		enricherHealth.WithLabelValues().Set(0)
		ne.logger.Warn("Namespace enricher marked unhealthy due to excessive errors",
			"error_count", ne.errorCount,
			"max_errors", ne.maxErrors,
			"last_error", err)
	}
}

// recordSuccess resets error state on successful operation
func (ne *NamespaceEnricher) recordSuccess() {
	if ne == nil {
		return
	}

	ne.mu.Lock()
	defer ne.mu.Unlock()

	// Reset error state on success
	if ne.errorCount > 0 {
		ne.errorCount = 0
		ne.lastError = nil
	}

	// Mark as healthy if not already
	if !ne.isHealthy {
		ne.isHealthy = true
		enricherHealth.WithLabelValues().Set(1)
		ne.logger.Info("Namespace enricher marked healthy after successful operation")
	}
}

// isHealthyToEnrich checks if enricher is in a healthy state
func (ne *NamespaceEnricher) isHealthyToEnrich() bool {
	if ne == nil {
		return false
	}

	ne.mu.RLock()
	defer ne.mu.RUnlock()

	// If unhealthy, check if we should try to recover (every 5 minutes)
	if !ne.isHealthy {
		if time.Since(ne.lastErrorTime) > 5*time.Minute {
			ne.logger.Info("Attempting to recover namespace enricher after cooldown period")
			return true // Allow one attempt to recover
		}
		return false
	}

	return true
}

// SharedInformerManager manages shared namespace informers to optimize API usage
type SharedInformerManager struct {
	mu              sync.RWMutex
	informerFactory informers.SharedInformerFactory
	informer        cache.SharedIndexInformer
	stopCh          chan struct{}
	refCount        int
	isStarted       bool
	client          kubernetes.Interface
	logger          *slog.Logger
}

var sharedInformerManager *SharedInformerManager
var sharedInformerOnce sync.Once

// getSharedInformerManager returns the singleton shared informer manager
func getSharedInformerManager(client kubernetes.Interface, logger *slog.Logger) *SharedInformerManager {
	sharedInformerOnce.Do(func() {
		sharedInformerManager = &SharedInformerManager{
			client: client,
			logger: logger,
			stopCh: make(chan struct{}),
		}
	})

	// Update client if different (for testing scenarios)
	sharedInformerManager.mu.Lock()
	if sharedInformerManager.client != client {
		sharedInformerManager.client = client
		// Reset factory to use new client
		sharedInformerManager.informerFactory = nil
		sharedInformerManager.informer = nil
	}
	sharedInformerManager.mu.Unlock()

	return sharedInformerManager
}

// getInformer returns a shared namespace informer, creating it if necessary
func (sim *SharedInformerManager) getInformer() (cache.SharedIndexInformer, error) {
	sim.mu.Lock()
	defer sim.mu.Unlock()

	if sim.informer == nil {
		if sim.client == nil {
			return nil, fmt.Errorf("kubernetes client is nil")
		}

		sim.informerFactory = informers.NewSharedInformerFactory(sim.client, 0)
		sim.informer = sim.informerFactory.Core().V1().Namespaces().Informer()
	}

	sim.refCount++
	sim.logger.Debug("Informer reference acquired", "ref_count", sim.refCount)

	return sim.informer, nil
}

// startInformer starts the shared informer if not already started
func (sim *SharedInformerManager) startInformer(ctx context.Context) error {
	sim.mu.Lock()
	defer sim.mu.Unlock()

	if sim.isStarted {
		return nil
	}

	if sim.informerFactory == nil {
		return fmt.Errorf("informer factory is nil")
	}

	// Start the informer factory
	go sim.informerFactory.Start(sim.stopCh)

	// Wait for cache sync with timeout
	synced := sim.informerFactory.WaitForCacheSync(sim.stopCh)
	for informerType, hasSynced := range synced {
		if !hasSynced {
			return fmt.Errorf("failed to sync informer %v", informerType)
		}
	}

	sim.isStarted = true
	sim.logger.Info("Shared namespace informer started successfully")
	return nil
}

// releaseInformer decrements the reference count and stops the informer if no longer needed
func (sim *SharedInformerManager) releaseInformer() {
	sim.mu.Lock()
	defer sim.mu.Unlock()

	if sim.refCount > 0 {
		sim.refCount--
		sim.logger.Debug("Informer reference released", "ref_count", sim.refCount)
	}

	// Stop informer when no more references
	if sim.refCount == 0 && sim.isStarted {
		close(sim.stopCh)
		sim.isStarted = false
		sim.stopCh = make(chan struct{}) // Reset for potential restart
		sim.logger.Info("Shared namespace informer stopped - no more references")
	}
}
